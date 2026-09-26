package tools

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"charm.land/fantasy"
)

func TestEdit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("a b a"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := toolset{dir: dir}

	res, _ := ts.edit(t.Context(), editInput{Path: "f.txt", OldText: "a", NewText: "x"}, fantasy.ToolCall{})
	if !res.IsError || !strings.Contains(res.Content, "matches 2 times") {
		t.Fatalf("ambiguous edit: %+v", res)
	}
	res, _ = ts.edit(t.Context(), editInput{Path: "f.txt", OldText: "a", NewText: "x", ReplaceAll: true}, fantasy.ToolCall{})
	if res.IsError {
		t.Fatalf("replace_all: %+v", res)
	}
	if got, _ := os.ReadFile(p); string(got) != "x b x" {
		t.Fatalf("file = %q", got)
	}
	res, _ = ts.edit(t.Context(), editInput{Path: "f.txt", OldText: "zzz", NewText: "y"}, fantasy.ToolCall{})
	if !res.IsError {
		t.Fatalf("missing text should fail: %+v", res)
	}
}

func TestBashExitCodeAndTimeout(t *testing.T) {
	ts := toolset{dir: t.TempDir()}
	res, _ := ts.bash(t.Context(), bashInput{Command: "echo hi; exit 3"}, fantasy.ToolCall{})
	if !strings.Contains(res.Content, "hi") || !strings.Contains(res.Content, "[exit code 3]") {
		t.Fatalf("got %q", res.Content)
	}
	res, _ = ts.bash(t.Context(), bashInput{Command: "sleep 5", TimeoutSeconds: 1}, fantasy.ToolCall{})
	if !strings.Contains(res.Content, "timed out") {
		t.Fatalf("got %q", res.Content)
	}
}

func TestReadOffset(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("1\n2\n3\n4"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ := toolset{dir: dir}.read(t.Context(), readInput{Path: "f.txt", Offset: 2, Limit: 2}, fantasy.ToolCall{})
	if !strings.HasPrefix(res.Content, "2\n3") || !strings.Contains(res.Content, "showing lines 2-3 of 4") {
		t.Fatalf("got %q", res.Content)
	}
}

func TestOptionalParamsAreNotRequired(t *testing.T) {
	want := map[string][]string{
		"read":  {"path"},
		"write": {"path", "content"},
		"edit":  {"path", "old_text", "new_text"},
		"bash":  {"command"},
	}
	for _, tool := range All(t.TempDir()) {
		info := tool.Info()
		got := slices.Sorted(slices.Values(info.Required))
		if !slices.Equal(got, slices.Sorted(slices.Values(want[info.Name]))) {
			t.Errorf("%s: required = %v, want %v", info.Name, got, want[info.Name])
		}
	}
}
