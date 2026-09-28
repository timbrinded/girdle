package tools

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxSearchMatches = 80
	maxLineWidth     = 200
	maxDefLines      = 160
)

// Search runs ripgrep (or grep if ripgrep isn't installed) and formats up to
// maxSearchMatches matching lines.
func Search(ctx context.Context, dir, pattern, glob string, ignoreCase bool) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("pattern is empty")
	}
	var cmd *exec.Cmd
	if _, err := exec.LookPath("rg"); err == nil {
		args := []string{"--line-number", "--no-heading", "--color=never", "--max-columns", fmt.Sprint(maxLineWidth), "--max-columns-preview"}
		if ignoreCase {
			args = append(args, "--ignore-case")
		}
		if glob != "" {
			args = append(args, "--glob", glob)
		}
		cmd = exec.CommandContext(ctx, "rg", append(args, "--", pattern, ".")...)
	} else {
		args := []string{"-rnIE", "--exclude-dir=.git"}
		if ignoreCase {
			args = append(args, "-i")
		}
		if glob != "" {
			args = append(args, "--include="+glob)
		}
		cmd = exec.CommandContext(ctx, "grep", append(args, "--", pattern, ".")...)
	}
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 && stdout.Len() == 0 {
		return "no matches for " + pattern, nil
	} else if err != nil && stdout.Len() == 0 {
		return "", fmt.Errorf("search failed: %s", strings.TrimSpace(stderr.String()+" "+err.Error()))
	}

	var b strings.Builder
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	// ripgrep searches files in parallel, so its order varies from run to
	// run. Sorting by file and line keeps results, and any prompt that
	// carries them, the same each time.
	slices.SortStableFunc(lines, func(a, b string) int {
		fa, ra, _ := strings.Cut(a, ":")
		fb, rb, _ := strings.Cut(b, ":")
		return cmp.Or(cmp.Compare(fa, fb), cmp.Compare(leadingInt(ra), leadingInt(rb)))
	})
	last := ""
	for i, line := range lines {
		if i == maxSearchMatches {
			fmt.Fprintf(&b, "… %d more matches; narrow the pattern, path or glob\n", len(lines)-maxSearchMatches)
			break
		}
		line = strings.TrimPrefix(line, "./")
		file, rest, _ := strings.Cut(line, ":")
		if file != last && last != "" {
			b.WriteString("\n")
		}
		last = file
		b.WriteString(file + ":" + clipLine(rest) + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// Definition is where a named function, method, class or type is defined,
// with its source.
type Definition struct {
	Path      string
	StartLine int // 1-based
	Source    string
	Truncated bool
}

func (d Definition) String() string {
	lines := strings.Count(d.Source, "\n") + 1
	s := fmt.Sprintf("%s:%d-%d\n%s", d.Path, d.StartLine, d.StartLine+lines-1, d.Source)
	if d.Truncated {
		s += fmt.Sprintf("\n[… cut at %d lines; read from line %d for more]", maxDefLines, d.StartLine+lines)
	}
	return s
}

// definitionPatterns find a name's definition line, per language. The name
// is quoted into the %s.
var definitionPatterns = map[string][]string{
	".go":  {`^func (\([^)]*\) )?%s(\[|\()`, `^type %s\b`, `^(var|const) %s\b`},
	".py":  {`^\s*(async\s+)?def %s\(`, `^\s*class %s\b`, `^%s\s*=`},
	".js":  {`^\s*(export\s+)?(default\s+)?(async\s+)?function\*?\s+%s\b`, `^\s*(export\s+)?(default\s+)?class %s\b`, `^\s*(export\s+)?(const|let|var) %s\s*=`},
	".ts":  {`^\s*(export\s+)?(default\s+)?(async\s+)?function\*?\s+%s\b`, `^\s*(export\s+)?(default\s+)?(abstract\s+)?class %s\b`, `^\s*(export\s+)?(const|let|var) %s\s*[=:]`, `^\s*(export\s+)?(interface|type) %s\b`},
	".mjs": {`^\s*(export\s+)?(default\s+)?(async\s+)?function\*?\s+%s\b`, `^\s*(export\s+)?(default\s+)?class %s\b`, `^\s*(export\s+)?(const|let|var) %s\s*=`},
}

var identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// FindDefinitions returns up to five definitions of name in dir's Go,
// Python, JavaScript and TypeScript files, skipping tests when a non-test
// definition exists.
func FindDefinitions(ctx context.Context, dir, name string) []Definition {
	if !identifier.MatchString(name) {
		return nil
	}
	var defs, testDefs []Definition
	for _, path := range SourceFiles(ctx, dir) {
		pats, ok := definitionPatterns[filepath.Ext(path)]
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || !bytes.Contains(data, []byte(name)) {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if !strings.Contains(line, name) || !matchesAny(pats, name, line) {
				continue
			}
			src, first, truncated := extractBody(lines, i, filepath.Ext(path))
			d := Definition{Path: path, StartLine: first + 1, Source: src, Truncated: truncated}
			if IsTestFile(path) {
				testDefs = append(testDefs, d)
			} else {
				defs = append(defs, d)
			}
		}
	}
	if len(defs) == 0 {
		defs = testDefs
	}
	return defs[:min(len(defs), 5)]
}

func matchesAny(pats []string, name, line string) bool {
	for _, p := range pats {
		if regexp.MustCompile(fmt.Sprintf(p, regexp.QuoteMeta(name))).MatchString(line) {
			return true
		}
	}
	return false
}

// extractBody returns the definition starting at lines[start]: for Python
// the indented block, for brace languages up to the matching closing brace,
// with any comments or decorators directly above. first is the index of its
// first line.
func extractBody(lines []string, start int, ext string) (src string, first int, truncated bool) {
	end := blockEnd(lines, start, ext)
	// Include comments and decorators directly above, up to a point.
	for top := start; start > 0 && top-start < 30; {
		prev := strings.TrimSpace(lines[start-1])
		if strings.HasPrefix(prev, "//") || strings.HasPrefix(prev, "@") || strings.HasPrefix(prev, "#") {
			start--
			continue
		}
		break
	}
	truncated = end-start+1 > maxDefLines
	if truncated {
		end = start + maxDefLines - 1
	}
	return strings.Join(lines[start:end+1], "\n"), start, truncated
}

func leadingWidth(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

// IsTestFile reports whether path is a test file by the usual naming
// conventions of Go, Python and JavaScript or TypeScript.
func IsTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
		strings.Contains(base, ".test.") || strings.Contains(path, "/tests/") || strings.HasPrefix(path, "tests/")
}

// skipDirs are never worth walking when dir isn't a git repository:
// version control data, dependencies and build output.
var skipDirs = []string{".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build", "target", ".next", ".cache"}

// SourceFiles returns dir's files relative to dir, sorted. It asks git first,
// since git knows what is ignored, and walks the tree otherwise.
func SourceFiles(ctx context.Context, dir string) []string {
	// An empty answer means dir is ignored by an enclosing repository:
	// walk it instead.
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z").Output()
	if err == nil && len(out) > 0 {
		var paths []string
		for p := range strings.SplitSeq(string(out), "\x00") {
			if p != "" {
				paths = append(paths, p)
			}
		}
		slices.Sort(paths)
		return slices.Compact(paths)
	}
	var paths []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && slices.Contains(skipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if rel, err := filepath.Rel(dir, path); err == nil {
				paths = append(paths, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	slices.Sort(paths)
	return paths
}

func clipLine(s string) string {
	if len(s) <= maxLineWidth {
		return s
	}
	cut := maxLineWidth
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + " …"
}

// leadingInt reads the number at the start of s, or 0.
func leadingInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
