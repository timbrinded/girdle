package tools

import (
	"strings"
	"testing"

	"charm.land/fantasy"
)

func TestHintsGo(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":        "module example.com/mark/v2\n\ngo 1.25\n",
		"mark.go":       "package mark\n\n// New makes one.\nfunc New(opts ...Option) *Markdown { return nil }\n\ntype Markdown struct{}\n\nfunc (m *Markdown) Convert(src []byte) error { return nil }\n\nfunc helper() {}\n",
		"option.go":     "package mark\n\ntype Option func()\n\nfunc WithParser(p int) Option { return nil }\n",
		"ext/x_test.go": "package ext\n\nimport (\n\t\"testing\"\n\n\t\"example.com/mark/v2\"\n)\n",
	})
	out := "# example.com/mark/v2/ext\next/x_test.go:9:7: undefined: mark.WithRenderer\next/x_test.go:10:4: m.Render undefined (type *mark.Markdown has no field or method Render)\nFAIL\n[exit code 1]"
	hints := Hints(t.Context(), dir, out)
	for _, want := range []string{
		"package mark (example.com/mark/v2) exports", "func New(opts ...Option) *Markdown", "func WithParser(p int) Option", "type Markdown struct",
		"methods of Markdown", "func (m *Markdown) Convert",
	} {
		if !strings.Contains(hints, want) {
			t.Errorf("hints are missing %q:\n%s", want, hints)
		}
	}
	if strings.Contains(hints, "helper") {
		t.Errorf("hints list an unexported name:\n%s", hints)
	}
}

func TestHintsPythonAndJS(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"pkg/__init__.py": "",
		"pkg/invoice.py":  "RATE = 1\n\ndef line_total(q, p):\n    return q * p\n\ndef _private():\n    pass\n\nclass Invoice:\n    def __init__(self):\n        pass\n\n    def add(self, item):\n        pass\n\n    def _hidden(self):\n        pass\n",
		"slug.js":         "export function slugify(s) {\n  return s;\n}\nexport const MAX = 60;\nfunction inner() {}\n",
	})
	py := Hints(t.Context(), dir, "ImportError: cannot import name 'total' from 'pkg.invoice'\nAttributeError: 'Invoice' object has no attribute 'remove'")
	for _, want := range []string{"module pkg.invoice defines", "def line_total(q, p)", "RATE =", "class Invoice", "def add(self, item)"} {
		if !strings.Contains(py, want) {
			t.Errorf("python hints are missing %q:\n%s", want, py)
		}
	}
	if strings.Contains(py, "_private") || strings.Contains(py, "_hidden") {
		t.Errorf("python hints list private names:\n%s", py)
	}
	js := Hints(t.Context(), dir, "SyntaxError: The requested module './slug.js' does not provide an export named 'slug'")
	if !strings.Contains(js, "export function slugify(s)") || !strings.Contains(js, "export const MAX = 60") || strings.Contains(js, "inner") {
		t.Errorf("js hints:\n%s", js)
	}
	if Hints(t.Context(), dir, "FAIL: expected 3, got 4") != "" {
		t.Error("hints for an ordinary failure")
	}
}

func TestApplyAddsHintsBeforeTheExitCode(t *testing.T) {
	dir := writeTree(t, map[string]string{"pkg/invoice.py": "def line_total(q, p):\n    return q * p\n"})
	var apply fantasy.AgentTool
	for _, tool := range Batched(dir) {
		if tool.Info().Name == "apply" {
			apply = tool
		}
	}
	res, err := apply.Run(t.Context(), fantasy.ToolCall{ID: "1", Name: "apply",
		Input: `{"changes":[],"check":"echo \"ImportError: cannot import name 'total' from 'pkg.invoice'\"; exit 3"}`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "def line_total(q, p)") {
		t.Fatalf("no hints:\n%s", res.Content)
	}
	if code, ok := ExitCode(res.Content); !ok || code != 3 || !strings.HasSuffix(res.Content, "[exit code 3]") {
		t.Fatalf("exit code %d %v, output:\n%s", code, ok, res.Content)
	}
}
