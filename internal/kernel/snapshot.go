package kernel

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/timbrinded/girdle/internal/checkpoint"
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
	// Prefetched lists the files added because Jev judged the request
	// needs them, and Prefetch holds Jev's answers.
	Prefetched []string
	Prefetch   checkpoint.Prefetch
	Candidates int
}

// SnapshotOptions are the extras a snapshot of a large repository can add.
type SnapshotOptions struct {
	// Pick judges which other files the request needs.
	Pick Picker
}

// Picker judges how likely a request is to need each file.
type Picker func(ctx context.Context, request string, files []checkpoint.FileOutline) checkpoint.Prefetch

const (
	// DefaultSnapshotBudget is the most file text a snapshot carries, about
	// 16k tokens.
	DefaultSnapshotBudget = 64 << 10
	maxListed             = 400
	// wholeFileMax is the largest file holding named code that a snapshot
	// shows whole rather than as just the definition.
	wholeFileMax = 16 << 10
)

// skipDirs are never worth sending when the directory isn't a git repo:
// version control data, dependencies and build output.
var skipDirs = []string{".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build", "target", ".next", ".cache"}

// TakeSnapshot describes dir for a request. When every text file fits in
// budget bytes, it includes them all. A larger repository gets its file list
// plus the definitions and uses of the code the request names in backticks,
// found by exact lookup: filling the budget in path order mostly sent docs.
func TakeSnapshot(ctx context.Context, dir string, budget int, request string) Snapshot {
	return TakeSnapshotWith(ctx, dir, budget, request, SnapshotOptions{})
}

// TakeSnapshotWith is TakeSnapshot with extras for a large repository: the
// code around the named code, and the files a picker judges the request
// most likely to need.
func TakeSnapshotWith(ctx context.Context, dir string, budget int, request string, opts SnapshotOptions) Snapshot {
	paths := listFiles(ctx, dir)
	if total := textBytes(dir, paths); total > budget {
		return namedCodeSnapshot(ctx, dir, paths, total, budget/2, request, opts)
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

func namedCodeSnapshot(ctx context.Context, dir string, paths []string, total, budget int, request string, opts SnapshotOptions) Snapshot {
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
	addFile := func(f string) {
		data, err := os.ReadFile(filepath.Join(dir, f))
		switch {
		case err != nil || isBinary(data):
		case len(data) <= budget/4 && add(fmt.Sprintf("<file path=%q>\n%s\n</file>\n", f, strings.TrimRight(string(data), "\n"))):
			snap.Included++
		default:
			add(fmt.Sprintf("<file path=%q>%s, %d lines: too large to include; look up the parts you need</file>\n", f, size(len(data)), bytes.Count(data, []byte{'\n'})+1))
		}
	}

	// In priority order, since the budget may run out: the repository's
	// instructions for agents, the files the request names, the named
	// code's definitions and uses, and last the tests beside that code.
	files, names := NamedCode(request, func(s string) bool { return slices.Contains(paths, s) })
	files = slices.Concat(agentFiles(paths), files)
	shown := slices.Clone(files)
	for _, f := range files {
		addFile(f)
	}
	var tests []string
	for _, name := range names {
		defs := tools.FindDefinitions(ctx, dir, name)
		for _, d := range defs {
			// A small file holding the named code is worth showing whole:
			// the model reads it first anyway.
			if !slices.Contains(shown, d.Path) {
				data, err := os.ReadFile(filepath.Join(dir, d.Path))
				if err == nil && len(data) <= wholeFileMax && add(fmt.Sprintf("<file path=%q>\n%s\n</file>\n", d.Path, strings.TrimRight(string(data), "\n"))) {
					snap.Included++
					shown = append(shown, d.Path)
				} else {
					add(fmt.Sprintf("<definition name=%q>\n%s\n</definition>\n", name, d.String()))
				}
			}
			for _, t := range siblingTests(d.Path, paths) {
				if !slices.Contains(tests, t) && !slices.Contains(shown, t) {
					tests = append(tests, t)
				}
			}
		}
		// Uses of a plain lowercase word the repository doesn't define
		// ("del", "util") are noise; a code-shaped name that doesn't
		// exist yet is worth reporting as new.
		if len(defs) == 0 && strings.ToLower(name) == name {
			continue
		}
		if uses, err := tools.Search(ctx, dir, `\b`+name+`\b`, "", "", false); err == nil {
			add(fmt.Sprintf("<uses name=%q>\n%s\n</uses>\n", name, clipLines(uses, 40)))
		}
	}
	for _, t := range tests {
		shown = append(shown, t)
		addFile(t)
	}
	if opts.Pick != nil {
		prefetch(ctx, dir, paths, shown, request, opts, add, &snap)
	}

	var b strings.Builder
	also := ""
	if len(snap.Prefetched) > 0 {
		also = ", and the other files it most likely needs"
	}
	fmt.Fprintf(&b, "<repository_snapshot>\nTaken just before this message. The repository is too large to include in full (%d files, %s), so this lists every file and shows the code the request names%s. Look up anything else you need.\n\n", len(paths), size(total), also)
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

// agentFiles are the instruction files a repository writes for coding
// agents, by their conventional names.
func agentFiles(paths []string) []string {
	var out []string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GIRDLE.md"} {
		if slices.Contains(paths, name) {
			out = append(out, name)
		}
	}
	return out
}

// siblingTests are the test files that sit next to path by the usual naming
// conventions: x_test.go for Go, test_x.py or tests/test_x.py for Python,
// and x.test.js or x.spec.ts for JavaScript and TypeScript.
func siblingTests(path string, paths []string) []string {
	dir, base := filepath.Split(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	var candidates []string
	switch ext {
	case ".go":
		candidates = []string{dir + stem + "_test.go"}
	case ".py":
		candidates = []string{dir + "test_" + base, "tests/test_" + base, filepath.Dir(filepath.Clean(dir)) + "/tests/test_" + base}
	case ".js", ".ts", ".mjs", ".jsx", ".tsx":
		candidates = []string{dir + stem + ".test" + ext, dir + stem + ".spec" + ext}
	}
	var out []string
	for _, c := range candidates {
		c = strings.TrimPrefix(filepath.ToSlash(c), "./")
		if slices.Contains(paths, c) && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

const (
	// prefetchMax is the most files a prefetch adds, and prefetchMin the
	// least need Jev must report for one. Files scoring from 0.5 to 0.7
	// were mostly noise, such as a changelog (decision 0013).
	prefetchMax = 6
	prefetchMin = 0.7
	// outlineMax clips what Jev sees of each file: a generated file's
	// outline can otherwise exceed its context.
	outlineMax = 6000
)

// prefetchJudgeable is about how many files Jev can judge well within
// prefetchWait.
const prefetchJudgeable = 128

// definesCode reports whether a file has a top-level definition an outline
// would list.
func definesCode(data []byte) bool {
	for line := range strings.Lines(string(data)) {
		if defLine.MatchString(line) {
			return true
		}
	}
	return false
}

// defLine matches the top-level definitions an outline lists.
var defLine = regexp.MustCompile(`^(func |type |def |class |    def |export (default )?(async )?(function|class|const) |function |async function )`)

// outline is a file's first lines and its definitions: what Jev judges a
// file's relevance from.
func outline(data []byte) string {
	lines := strings.Split(string(data), "\n")
	var b strings.Builder
	b.WriteString(strings.Join(lines[:min(len(lines), 25)], "\n"))
	b.WriteString("\n…\n")
	n := 0
	for _, l := range lines {
		if n == 60 {
			break
		}
		if defLine.MatchString(l) {
			b.WriteString(strings.TrimSpace(l) + "\n")
			n++
		}
	}
	return clipMiddle(b.String(), outlineMax)
}

// prefetch asks pick which of the files not yet shown the request needs,
// and adds the likeliest that are small enough to show whole. An outline of
// a large file saved no lookups, since the LLM still had to look up the
// range it needed, and it slowed runs down (decision 0013).
func prefetch(ctx context.Context, dir string, paths, shown []string, request string, opts SnapshotOptions, add func(string) bool, snap *Snapshot) {
	data := map[string][]byte{}
	var cands, code []checkpoint.FileOutline
	for _, p := range paths {
		if slices.Contains(shown, p) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil || len(b) > wholeFileMax || isBinary(b) {
			continue
		}
		data[p] = b
		c := checkpoint.FileOutline{Path: p, Outline: outline(b)}
		cands = append(cands, c)
		if definesCode(b) {
			code = append(code, c)
		}
	}
	// Jev judges about 32 files every 0.3 s, and prefetch waits at most
	// prefetchWait. A repository with hundreds of test-data files used that
	// time up on them, in random order, before reaching its code, and the
	// first LLM call waited 4 s for nothing (decision 0019).
	if len(cands) > prefetchJudgeable {
		cands = code
	}
	snap.Candidates = len(cands)
	if len(cands) == 0 {
		return
	}
	snap.Prefetch = opts.Pick(ctx, request, cands)
	ranked := slices.SortedFunc(maps.Keys(snap.Prefetch.Scores), func(a, b string) int {
		return cmp.Or(cmp.Compare(snap.Prefetch.Scores[b], snap.Prefetch.Scores[a]), cmp.Compare(a, b))
	})
	for _, p := range ranked {
		if len(snap.Prefetched) == prefetchMax || snap.Prefetch.Scores[p] < prefetchMin {
			break
		}
		b, ok := data[p]
		if !ok {
			continue
		}
		if add(fmt.Sprintf("<file path=%q>\n%s\n</file>\n", p, strings.TrimRight(string(b), "\n"))) {
			snap.Included++
			snap.Prefetched = append(snap.Prefetched, p)
		}
	}
}
