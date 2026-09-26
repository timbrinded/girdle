package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/fantasy"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, body := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFindDefinitions(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/heading.go":      "package a\n\n// Parse reads a heading.\nfunc (p *parser) Parse(b []byte) int {\n\tif len(b) > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n\ntype Level int\n\nfunc other() {}\n",
		"a/heading_test.go": "package a\n\nfunc Parse() {}\n",
		"more.py":           "import x\n\n\n@decorate\ndef window(seq, n,\n           step=1):\n    \"\"\"Doc.\"\"\"\n    if n:\n\n        return seq\n    return []\n\n\ndef after():\n    pass\n",
		"lib.js":            "export function slugify(s) {\n  return s.toLowerCase();\n}\nconst other = 1;\n",
	})
	check := func(name, wantPath, wantStart string, wantIn []string, wantOut []string) {
		t.Helper()
		defs := FindDefinitions(t.Context(), dir, name)
		if len(defs) != 1 {
			t.Fatalf("%s: %d definitions: %+v", name, len(defs), defs)
		}
		got := defs[0].String()
		if !strings.HasPrefix(got, wantPath+":"+wantStart) {
			t.Errorf("%s: header %q", name, strings.SplitN(got, "\n", 2)[0])
		}
		for _, w := range wantIn {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %q in\n%s", name, w, got)
			}
		}
		for _, w := range wantOut {
			if strings.Contains(got, w) {
				t.Errorf("%s: should not contain %q in\n%s", name, w, got)
			}
		}
	}
	check("Parse", "a/heading.go", "3-9", []string{"// Parse reads", "return 0\n}"}, []string{"type Level"})
	check("Level", "a/heading.go", "11-11", []string{"type Level int"}, []string{"other"})
	check("window", "more.py", "4-11", []string{"@decorate", "step=1):", "return []"}, []string{"def after"})
	check("slugify", "lib.js", "1-3", []string{"toLowerCase"}, []string{"const other"})
	if defs := FindDefinitions(t.Context(), dir, "missing"); len(defs) != 0 {
		t.Fatalf("found %v", defs)
	}
}

func TestSearch(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\nfunc IndentWidth() {}\n",
		"b.go": "package b\nvar x = a.IndentWidth()\n// indentwidth in lower case\n",
	})
	out, err := Search(t.Context(), dir, `\bIndentWidth\b`, "", "", false)
	if err != nil || !strings.Contains(out, "a.go:2:func IndentWidth") || !strings.Contains(out, "b.go:2:") || strings.Contains(out, "lower case") {
		t.Fatalf("search = %q, %v", out, err)
	}
	out, _ = Search(t.Context(), dir, `indentwidth`, "", "*.go", true)
	if !strings.Contains(out, "b.go:3:// indentwidth") || !strings.Contains(out, "a.go:2:") {
		t.Fatalf("ignore case: %q", out)
	}
	out, _ = Search(t.Context(), dir, `nothing here`, "", "", false)
	if !strings.HasPrefix(out, "no matches") {
		t.Fatalf("no match: %q", out)
	}
}

func TestEditIgnoresWhitespace(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"f.go": "func f() {\n\tif x {\n\t\ty()\n\t}\n}\n",
	})
	var edit fantasy.AgentTool
	for _, tool := range All(dir) {
		if tool.Info().Name == "edit" {
			edit = tool
		}
	}
	// The model wrote spaces where the file has tabs.
	res, err := edit.Run(t.Context(), fantasy.ToolCall{ID: "1", Name: "edit",
		Input: `{"path":"f.go","old_text":"    if x {\n        y()\n    }","new_text":"    if x {\n        y()\n        z()\n    }"}`})
	if err != nil || res.IsError {
		t.Fatalf("edit: %+v %v", res, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "f.go"))
	if want := "func f() {\n\tif x {\n\t\ty()\n\t\tz()\n\t}\n}\n"; string(data) != want {
		t.Fatalf("file = %q, want %q", data, want)
	}

	// An ambiguous whitespace-insensitive match is refused.
	dir = writeTree(t, map[string]string{"g.go": "a\n\tb\nc\n  b\n"})
	for _, tool := range All(dir) {
		if tool.Info().Name == "edit" {
			edit = tool
		}
	}
	res, _ = edit.Run(t.Context(), fantasy.ToolCall{ID: "2", Name: "edit", Input: `{"path":"g.go","old_text":"b","new_text":"x"}`})
	if !res.IsError {
		t.Fatalf("ambiguous match was applied: %+v", res)
	}
}

func TestApplyWithOnlyACheck(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.txt": "hi\n"})
	var apply fantasy.AgentTool
	for _, tool := range Batched(dir) {
		if tool.Info().Name == "apply" {
			apply = tool
		}
	}
	res, err := apply.Run(t.Context(), fantasy.ToolCall{ID: "1", Name: "apply", Input: `{"changes":[],"check":"cat a.txt"}`})
	if err != nil || res.IsError || !strings.Contains(res.Content, "hi") {
		t.Fatalf("apply = %+v %v", res, err)
	}
	if code, ok := ExitCode(res.Content); !ok || code != 0 {
		t.Fatalf("exit code %d %v", code, ok)
	}
}
