package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/timbrinded/girdle/internal/clip"
)

// A search result is a list of lines, and the agent's next step is often to
// read the code around one of them: in the fast flow's logs, a grep was
// followed straight away by a read of a file it matched 0.7 times a run, and
// every step costs about 2 s before it writes anything. With grep context,
// a search result names the definition each match sits in, and when the
// matches fall in a few definitions, shows them. The definition's bounds come
// from the code itself, as lookup's definitions do, not from a guess at a
// line range.

const (
	// aroundMaxUnits is the most definitions whose source a search result
	// shows, and aroundMaxLines the most lines they may take together.
	aroundMaxUnits = 3
	aroundMaxLines = 150
	// aroundMaxListed is the most definitions a result names.
	aroundMaxListed = 8
)

// grepCommand matches a shell command that runs a search tool.
var grepCommand = regexp.MustCompile(`(^|[;&|(]\s*|\bcd\s+\S+\s*&&\s*)(grep|rg|git grep|ag)\s`)

// matchLine is a search result line: path, line number, then the text.
var matchLine = regexp.MustCompile(`(?m)^(?:\./)?([^\s:]+\.(?:go|py|js|mjs|ts|tsx|jsx)):(\d+):`)

// unitStart matches the first line of a top-level definition in a brace
// language: it starts at the left margin.
var unitStart = regexp.MustCompile(`^(func |type |var |const |class |function |async function |export |interface )`)

// pyUnit matches a Python function or class line.
var pyUnit = regexp.MustCompile(`^\s*(async\s+)?(def|class)\s`)

// location is one search match.
type location struct {
	path string // relative to the working directory
	line int    // 1-based
}

// unitAround returns the bounds, as 0-based indices, of the definition that
// holds lines[i]: the top-level one in a brace language, the innermost
// function or class in Python.
func unitAround(lines []string, i int, ext string) (start, end int, ok bool) {
	if ext == ".py" {
		indent := leadingWidth(lines[i])
		for j := i; j >= 0; j-- {
			if !pyUnit.MatchString(lines[j]) || (j != i && leadingWidth(lines[j]) >= indent) {
				continue
			}
			_, first, _ := extractBody(lines, j, ext)
			end := blockEnd(lines, j, ext)
			if i <= end {
				return first, end, true
			}
			indent = leadingWidth(lines[j])
		}
		return 0, 0, false
	}
	for j := i; j >= 0; j-- {
		if unitStart.MatchString(lines[j]) {
			_, first, _ := extractBody(lines, j, ext)
			end := blockEnd(lines, j, ext)
			return first, end, i <= end
		}
	}
	return 0, 0, false
}

// blockEnd is where the definition starting at lines[start] ends: for Python
// the indented block, for brace languages the matching closing brace. A
// declaration with no body before a blank line ends where it starts.
func blockEnd(lines []string, start int, ext string) int {
	end := start
	if ext == ".py" {
		indent := leadingWidth(lines[start])
		for end+1 < len(lines) {
			next := lines[end+1]
			if strings.TrimSpace(next) != "" && leadingWidth(next) <= indent && !strings.HasPrefix(strings.TrimSpace(next), ")") {
				break
			}
			end++
		}
		for end > start && strings.TrimSpace(lines[end]) == "" {
			end--
		}
		return end
	}
	depth, opened := 0, false
	for end = start; end < len(lines); end++ {
		depth += strings.Count(lines[end], "{") - strings.Count(lines[end], "}")
		if strings.Contains(lines[end], "{") {
			opened = true
		}
		if opened && depth <= 0 {
			return end
		}
		if !opened && end > start && (strings.TrimSpace(lines[end]) == "" || end >= start+6) {
			return start
		}
	}
	return len(lines) - 1
}

// matchLocations reads the search matches in output, in order, once each.
func (t toolset) matchLocations(output string) []location {
	var locs []location
	seen := map[location]bool{}
	for _, m := range matchLine.FindAllStringSubmatch(output, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		p := m[1]
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(t.dir, p)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			p = rel
		}
		l := location{filepath.ToSlash(p), n}
		if !seen[l] {
			seen[l] = true
			locs = append(locs, l)
		}
	}
	return locs
}

// aroundMatches describes the definitions that hold the matches in a search
// result, and shows them when there are few. It returns "" when there is
// nothing to add.
func (t toolset) aroundMatches(output string) string {
	locs := t.matchLocations(output)
	if len(locs) == 0 {
		return ""
	}
	type unit struct {
		path       string
		start, end int // 0-based, inclusive
		head       string
		lines      []string
		hits       []int
	}
	var units []*unit
	files := map[string][]string{}
	for _, l := range locs {
		lines, ok := files[l.path]
		if !ok {
			data, err := os.ReadFile(filepath.Join(t.dir, l.path))
			if err == nil && !t.denied(filepath.Join(t.dir, l.path)) {
				lines = strings.Split(string(data), "\n")
			}
			files[l.path] = lines
		}
		if l.line < 1 || l.line > len(lines) {
			continue
		}
		start, end, ok := unitAround(lines, l.line-1, filepath.Ext(l.path))
		if !ok {
			continue
		}
		found := false
		for _, u := range units {
			if u.path == l.path && u.start == start {
				u.hits = append(u.hits, l.line)
				found = true
				break
			}
		}
		if !found {
			head := start
			// The header is the definition line itself, not a comment above it.
			for head < end && !unitStart.MatchString(lines[head]) && !pyUnit.MatchString(lines[head]) {
				head++
			}
			units = append(units, &unit{path: l.path, start: start, end: end, head: clip.Head(strings.TrimSpace(lines[head]), maxLineWidth), lines: lines, hits: []int{l.line}})
		}
	}
	if len(units) == 0 {
		return ""
	}
	var b strings.Builder
	total := 0
	for _, u := range units {
		total += u.end - u.start + 1
	}
	if len(units) <= aroundMaxUnits && total <= aroundMaxLines {
		b.WriteString("[Girdle] The matches are in these definitions:\n")
		for _, u := range units {
			fmt.Fprintf(&b, "=== %s:%d-%d\n%s\n", u.path, u.start+1, u.end+1, strings.Join(u.lines[u.start:u.end+1], "\n"))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString("[Girdle] The matches are in these definitions:\n")
	for i, u := range units {
		if i == aroundMaxListed {
			fmt.Fprintf(&b, "… and %d more\n", len(units)-aroundMaxListed)
			break
		}
		fmt.Fprintf(&b, "%s:%d-%d %s\n", u.path, u.start+1, u.end+1, u.head)
	}
	return strings.TrimRight(b.String(), "\n")
}
