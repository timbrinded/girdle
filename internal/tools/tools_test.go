package tools

import (
	"encoding/json/v2"
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

func TestExitCode(t *testing.T) {
	cases := []struct {
		in   string
		code int
		ok   bool
	}{
		{"ok\n[exit code 0]", 0, true},
		{"FAIL\n[exit code 1]", 1, true},
		{"prints [exit code 0] inside\n[exit code 2]", 2, true},
		{"no trailer", 0, false},
		{"[exit code x]", 0, false},
	}
	for _, c := range cases {
		code, ok := ExitCode(c.in)
		if code != c.code || ok != c.ok {
			t.Errorf("ExitCode(%q) = (%d, %v), want (%d, %v)", c.in, code, ok, c.code, c.ok)
		}
	}
}

func TestApply(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var apply fantasy.AgentTool
	for _, tool := range Batched(dir) {
		if tool.Info().Name == "apply" {
			apply = tool
		}
	}
	run := func(in ApplyInput) fantasy.ToolResponse {
		t.Helper()
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		res, err := apply.Run(t.Context(), fantasy.ToolCall{ID: "1", Name: "apply", Input: string(raw)})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := run(ApplyInput{
		Changes: []Change{
			{Path: "a.txt", OldText: "world", NewText: "girdle"},
			{Path: "sub/b.txt", Content: "new file\n"},
		},
		Check: "cat a.txt sub/b.txt",
	})
	if res.IsError || !strings.Contains(res.Content, "hello girdle") || !strings.Contains(res.Content, "new file") {
		t.Fatalf("apply = %+v", res)
	}
	if code, ok := ExitCode(res.Content); !ok || code != 0 {
		t.Fatalf("exit code = %d, %v", code, ok)
	}

	res = run(ApplyInput{
		Changes: []Change{{Path: "a.txt", OldText: "missing", NewText: "x"}},
		Check:   "echo should-not-run",
	})
	if !res.IsError || strings.Contains(res.Content, "should-not-run") {
		t.Fatalf("failed change should skip the check: %+v", res)
	}

	res = run2(t, apply, `{"changes":"[{\"path\":\"a.txt\",\"old_text\":\"girdle\",\"new_text\":\"string\"}]","check":"cat a.txt"}`)
	if res.IsError || !strings.Contains(res.Content, "hello string") {
		t.Fatalf("changes sent as a string: %+v", res)
	}

	paths, check, ok := ParseApply(`{"changes":[{"path":"a.go","old_text":"x","new_text":"y"},{"path":"a.go","content":"z"},{"path":"b.go","content":""}],"check":"go test ./..."}`)
	if !ok || !slices.Equal(paths, []string{"a.go", "b.go"}) || check != "go test ./..." {
		t.Fatalf("ParseApply = %v, %q, %v", paths, check, ok)
	}
}

func TestBatchedRequiredParams(t *testing.T) {
	want := map[string][]string{"read": {"path"}, "apply": {"changes", "check"}, "bash": {"command"}}
	for _, tool := range Batched(t.TempDir()) {
		info := tool.Info()
		got := slices.Sorted(slices.Values(info.Required))
		if !slices.Equal(got, want[info.Name]) {
			t.Errorf("%s: required = %v, want %v", info.Name, got, want[info.Name])
		}
	}
}

func run2(t *testing.T, tool fantasy.AgentTool, input string) fantasy.ToolResponse {
	t.Helper()
	res, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "2", Name: "apply", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return res
}
