package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// A regression test proves a fix only if it fails without the fix. apply
// can check that: with a reproduce command it undoes the request's code
// changes for a moment, runs the command, and puts the changes back. A test
// that passes on the old code doesn't reproduce the bug, however green the
// check is.

// originals remembers each file's content from before the current request
// first changed it.
type originals struct {
	mu    sync.Mutex
	files map[string]*[]byte // absolute path → content; nil if the file didn't exist
}

// fileState is a file's content, or its absence.
type fileState struct {
	data   []byte
	exists bool
}

func (st fileState) restore(p string) {
	if st.exists {
		_ = os.WriteFile(p, st.data, 0o644)
	} else {
		_ = os.Remove(p)
	}
}

func (o *originals) remember(path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.files[path]; ok {
		return
	}
	if data, err := os.ReadFile(path); err == nil {
		o.files[path] = &data
	} else {
		o.files[path] = nil
	}
}

func (o *originals) reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	clear(o.files)
}

// codeFiles returns the remembered files that aren't tests, sorted.
func (o *originals) codeFiles(dir string) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []string
	for p := range o.files {
		if rel, err := filepath.Rel(dir, p); err == nil && !IsTestFile(filepath.ToSlash(rel)) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// withOriginals puts back the original content of paths, runs fn, and then
// restores the files as they were.
func (t toolset) withOriginals(paths []string, fn func()) {
	now := map[string]fileState{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		now[p] = fileState{data, err == nil}
	}
	defer func() {
		for p, st := range now {
			st.restore(p)
		}
	}()
	t.orig.mu.Lock()
	for _, p := range paths {
		if old := t.orig.files[p]; old != nil {
			fileState{*old, true}.restore(p)
		} else {
			fileState{}.restore(p)
		}
	}
	t.orig.mu.Unlock()
	fn()
}

// reproduce runs cmd with the request's code changes undone, then restores
// them. It returns a note for the apply result, and whether the test failed
// to reproduce the bug.
func (t toolset) reproduce(ctx context.Context, cmd string) (note string, weak bool) {
	if t.orig == nil {
		return "", false
	}
	paths := t.orig.codeFiles(t.dir)
	if len(paths) == 0 {
		return "[Girdle] reproduce not run: this request changed no code, only tests.", false
	}
	var (
		out  string
		code int
		ok   bool
	)
	t.withOriginals(paths, func() { out, code, ok = t.runCheck(ctx, cmd) })
	switch {
	case !ok:
		return "[Girdle] reproduce could not be run: " + clipTail(out, 400), false
	case code == 0:
		return "[Girdle] Your reproduce command also passes WITHOUT your fix, so it doesn't reproduce the bug. Change the test so it fails on the old code and passes on the new one, then apply again.\n" + clipTail(out, 800), true
	default:
		return fmt.Sprintf("[Girdle] Without your fix, the reproduce command fails (exit code %d); with it, the check passes. The test reproduces the bug.", code), false
	}
}

func clipTail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return "…" + s[cut:]
}
