package kernel

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/timbrinded/girdle/internal/tools"
)

// Snapshot is a picture of the working directory's text files. It goes out
// with a request so the LLM starts with the code in front of it, instead of
// spending whole LLM steps listing directories and reading files.
type Snapshot struct {
	Text     string
	Files    int // files found
	Included int // files whose full text is included
	Bytes    int // bytes of file text included
}

const (
	// DefaultSnapshotBudget is the most file text a snapshot carries, about
	// 16k tokens.
	DefaultSnapshotBudget = 64 << 10
	maxListed             = 400
)

// skipDirs are never worth sending when the directory isn't a git repo:
// version control data, dependencies and build output.
var skipDirs = []string{".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build", "target", ".next", ".cache"}

// TakeSnapshot describes dir for a request. When every text file fits in
// budget bytes, it includes them all. A larger repository gets its file list
// plus the definitions and uses of the code the request names in backticks,
// found by exact lookup: filling the budget in path order mostly sent docs.
func TakeSnapshot(ctx context.Context, dir string, budget int, request string) Snapshot {
	paths := listFiles(ctx, dir)
	if total := textBytes(dir, paths); total > budget {
		return namedCodeSnapshot(ctx, dir, paths, total, budget/2, request)
	}
	var snap Snapshot
	snap.Files = len(paths)

	var list, files strings.Builder
	for i, p := range paths {
		data, err := os.ReadFile(filepath.Join(dir, p))
		note := ""
		switch {
		case err != nil:
			note = "unreadable"
		case isBinary(data):
			note = "binary, not included"
		case snap.Bytes+len(data) > budget:
			note = fmt.Sprintf("%s, not included: read it if you need it", size(len(data)))
		default:
			note = fmt.Sprintf("%d lines", bytes.Count(data, []byte{'\n'})+1)
			snap.Included++
			snap.Bytes += len(data)
			fmt.Fprintf(&files, "<file path=%q>\n%s\n</file>\n", p, strings.TrimRight(string(data), "\n"))
		}
		if i < maxListed {
			fmt.Fprintf(&list, "- %s (%s)\n", p, note)
		}
	}
	if len(paths) > maxListed {
		fmt.Fprintf(&list, "- … and %d more files\n", len(paths)-maxListed)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<repository_snapshot>\nTaken just before this message. %d files; the full text of %d is below.\n\n", snap.Files, snap.Included)
	b.WriteString(list.String())
	b.WriteString("\n")
	b.WriteString(files.String())
	b.WriteString("</repository_snapshot>")
	snap.Text = b.String()
	return snap
}

// listFiles returns dir's files relative to dir, sorted. It asks git first,
// since git knows what is ignored, and walks the tree otherwise.
func listFiles(ctx context.Context, dir string) []string {
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
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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

func isBinary(data []byte) bool {
	head := data[:min(len(data), 8000)]
	return bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(data)
}

func size(n int) string {
	if n < 1<<10 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%d KB", n>>10)
}

// textBytes adds up the sizes of paths, stopping early once it is clear
// they are large.
func textBytes(dir string, paths []string) int {
	total := 0
	for _, p := range paths {
		if info, err := os.Stat(filepath.Join(dir, p)); err == nil {
			total += int(info.Size())
		}
		if total > 64<<20 {
			break
		}
	}
	return total
}

var codeSpan = regexp.MustCompile("`([^`\n]+)`")

// NamedCode returns the names the request puts in code spans: a path that
// exists in the repository, or the last part of a dotted name, without any
// call arguments. `extension.NewThing("x")` names NewThing.
func NamedCode(request string, isFile func(string) bool) (files, names []string) {
	for _, m := range codeSpan.FindAllStringSubmatch(request, -1) {
		span := strings.TrimSpace(m[1])
		if isFile(span) {
			if !slices.Contains(files, span) {
				files = append(files, span)
			}
			continue
		}
		span, _, _ = strings.Cut(span, "(")
		if _, after, ok := strings.CutLast(span, "."); ok {
			span = after
		}
		if identifierName.MatchString(span) && !slices.Contains(names, span) {
			names = append(names, span)
		}
	}
	return files, names
}

var identifierName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{2,}$`)

func namedCodeSnapshot(ctx context.Context, dir string, paths []string, total, budget int, request string) Snapshot {
	snap := Snapshot{Files: len(paths)}
	var list strings.Builder
	for i, p := range paths {
		if i == maxListed {
			fmt.Fprintf(&list, "- … and %d more files\n", len(paths)-maxListed)
			break
		}
		list.WriteString("- " + p + "\n")
	}

	var named strings.Builder
	add := func(text string) bool {
		if named.Len()+len(text) > budget {
			return false
		}
		named.WriteString(text)
		snap.Bytes += len(text)
		return true
	}
	files, names := NamedCode(request, func(s string) bool { return slices.Contains(paths, s) })
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		switch {
		case err != nil || isBinary(data):
		case len(data) <= budget/4 && add(fmt.Sprintf("<file path=%q>\n%s\n</file>\n", f, strings.TrimRight(string(data), "\n"))):
			snap.Included++
		default:
			add(fmt.Sprintf("<file path=%q>%s, %d lines: too large to include; use search, definition or read with an offset</file>\n", f, size(len(data)), bytes.Count(data, []byte{'\n'})+1))
		}
	}
	for _, name := range names {
		for _, d := range tools.FindDefinitions(ctx, dir, name) {
			add(fmt.Sprintf("<definition name=%q>\n%s\n</definition>\n", name, d.String()))
		}
		if uses, err := tools.Search(ctx, dir, `\b`+name+`\b`, "", "", false); err == nil {
			add(fmt.Sprintf("<uses name=%q>\n%s\n</uses>\n", name, clipLines(uses, 40)))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<repository_snapshot>\nTaken just before this message. The repository is too large to include in full (%d files, %s), so this lists every file and shows the code the request names. Find anything else with search and definition.\n\n", len(paths), size(total))
	b.WriteString(list.String())
	if named.Len() > 0 {
		b.WriteString("\n" + named.String())
	}
	b.WriteString("</repository_snapshot>")
	snap.Text = b.String()
	return snap
}

func clipLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… %d more lines", len(lines)-n)
}
