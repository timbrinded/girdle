package tools

import (
	"strings"
	"testing"

	"charm.land/fantasy"
)

const goSource = `package p

// Parse reads s.
func Parse(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

type parser struct {
	pos int
}

func (p *parser) next() int {
	p.pos++
	return p.pos
}
`

const pySource = `import os


class Window:
    """A sliding window."""

    def __init__(self, n):
        self.n = n

    def push(self, x):
        if x is None:
            raise ValueError("x")
        return x


def helper():
    return 1
`

func TestUnitAround(t *testing.T) {
	lines := strings.Split(goSource, "\n")
	for line, want := range map[int][2]int{
		6:  {3, 10},  // inside Parse; the comment above counts
		4:  {3, 10},  // the definition line itself
		13: {12, 14}, // a struct field
		18: {16, 19}, // a method
	} {
		start, end, ok := unitAround(lines, line-1, ".go")
		if !ok || start+1 != want[0] || end+1 != want[1] {
			t.Errorf("Go line %d: got %d-%d %v, want %d-%d", line, start+1, end+1, ok, want[0], want[1])
		}
	}
	if _, _, ok := unitAround(lines, 0, ".go"); ok {
		t.Error("the package clause is in no definition")
	}
	lines = strings.Split(pySource, "\n")
	for line, want := range map[int][2]int{
		12: {10, 13}, // inside a method: the method, not the class
		8:  {7, 8},
		17: {16, 17},
	} {
		start, end, ok := unitAround(lines, line-1, ".py")
		if !ok || start+1 != want[0] || end+1 != want[1] {
			t.Errorf("Python line %d: got %d-%d %v, want %d-%d", line, start+1, end+1, ok, want[0], want[1])
		}
	}
}

func TestSearchResultsShowTheirDefinitions(t *testing.T) {
	dir := writeTree(t, map[string]string{"p.go": goSource, "w.py": pySource})
	ts := newToolset(dir, Options{GrepContext: true})
	res, _ := ts.bash(t.Context(), bashInput{Command: "grep -rn 'p.pos' ."}, fantasy.ToolCall{})
	if !strings.Contains(res.Content, "[Girdle] The matches are in these definitions:") || !strings.Contains(res.Content, "func (p *parser) next() int {") {
		t.Fatalf("no definitions shown:\n%s", res.Content)
	}
	if code, _ := ExitCode(res.Content); code != 0 {
		t.Fatalf("the exit code line must stay last:\n%s", res.Content)
	}
	// A command that doesn't search gets nothing added.
	res, _ = ts.bash(t.Context(), bashInput{Command: "cat p.go"}, fantasy.ToolCall{})
	if strings.Contains(res.Content, "[Girdle]") {
		t.Fatalf("a read was annotated:\n%s", res.Content)
	}
	// Without the option, nothing changes.
	res, _ = newToolset(dir, Options{}).bash(t.Context(), bashInput{Command: "grep -rn 'p.pos' ."}, fantasy.ToolCall{})
	if strings.Contains(res.Content, "[Girdle]") {
		t.Fatalf("annotated without the option:\n%s", res.Content)
	}
}

func TestManyDefinitionsAreListedNotShown(t *testing.T) {
	var src strings.Builder
	src.WriteString("package p\n\n")
	for i := range 5 {
		src.WriteString("func F" + string(rune('a'+i)) + "() int {\n\treturn target\n}\n\n")
	}
	dir := writeTree(t, map[string]string{"p.go": src.String()})
	note := newToolset(dir, Options{GrepContext: true}).aroundMatches("p.go:4:\treturn target\np.go:8:\treturn target\np.go:12:\treturn target\np.go:16:\treturn target\n")
	if !strings.Contains(note, "p.go:3-5 func Fa() int {") || strings.Contains(note, "===") {
		t.Fatalf("four definitions should be listed, not shown:\n%s", note)
	}
}

func TestLookupSearchRetriesIgnoringCase(t *testing.T) {
	dir := writeTree(t, map[string]string{"p.go": goSource})
	ts := newToolset(dir, Options{GrepContext: true})
	res, _ := ts.lookup(t.Context(), LookupInput{Searches: []string{"func parse\\("}}, fantasy.ToolCall{})
	if !strings.Contains(res.Content, "ignoring case") || !strings.Contains(res.Content, "func Parse(s string) int {") {
		t.Fatalf("no case-insensitive retry:\n%s", res.Content)
	}
}
