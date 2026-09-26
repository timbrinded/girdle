package kernel

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
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

// TakeSnapshot lists dir's files and includes the text of as many as fit in
// budget bytes, in path order. Binary files and files that would not fit are
// listed but left out, and the snapshot says so.
func TakeSnapshot(ctx context.Context, dir string, budget int) Snapshot {
	paths := listFiles(ctx, dir)
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
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z").Output()
	if err == nil {
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
