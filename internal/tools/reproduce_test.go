package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/fantasy"
)

// testRan stands in for Jev's judgement of whether a reproduce command ran
// its test.
func testRan(ran bool) Options {
	return Options{TestRan: func(context.Context, string, string) bool { return ran }}
}

func applyTool(t *testing.T, dir string) (fantasy.AgentTool, func()) {
	t.Helper()
	return applyToolWith(t, dir, testRan(true))
}

func applyToolWith(t *testing.T, dir string, opts Options) (fantasy.AgentTool, func()) {
	t.Helper()
	ts, reset := BatchedWithReset(dir, true, opts)
	for _, tool := range ts {
		if tool.Info().Name == "apply" {
			return tool, reset
		}
	}
	t.Fatal("no apply tool")
	return nil, nil
}

func runApply(t *testing.T, tool fantasy.AgentTool, input string) fantasy.ToolResponse {
	t.Helper()
	res, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "1", Name: "apply", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestReproduceConfirmsARealFix(t *testing.T) {
	dir := writeTree(t, map[string]string{"calc.txt": "2\n"})
	apply, reset := applyTool(t, dir)
	reset()
	res := runApply(t, apply, `{"changes":[{"path":"calc.txt","old_text":"2","new_text":"3"},{"path":"test_calc.sh","content":"grep -q 3 calc.txt\n"},{"path":"helper.txt","content":"new\n"}],"check":"sh test_calc.sh","reproduce":"sh test_calc.sh && test ! -f helper.txt"}`)
	if !strings.Contains(res.Content, "The test reproduces the bug") {
		t.Fatalf("no confirmation:\n%s", res.Content)
	}
	if code, ok := ExitCode(res.Content); !ok || code != 0 {
		t.Fatalf("exit code %d %v", code, ok)
	}
	// The fix, and the file it created, are back.
	if data, _ := os.ReadFile(filepath.Join(dir, "calc.txt")); string(data) != "3\n" {
		t.Fatalf("calc.txt = %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "helper.txt")); err != nil {
		t.Fatal("helper.txt was not restored")
	}
}

func TestReproduceRejectsATestThatPassesWithoutTheFix(t *testing.T) {
	dir := writeTree(t, map[string]string{"calc.txt": "2\n"})
	apply, reset := applyTool(t, dir)
	reset()
	res := runApply(t, apply, `{"changes":[{"path":"calc.txt","old_text":"2","new_text":"3"},{"path":"test_calc.sh","content":"true\n"}],"check":"sh test_calc.sh","reproduce":"sh test_calc.sh"}`)
	if !strings.Contains(res.Content, "doesn't reproduce the bug") {
		t.Fatalf("no warning:\n%s", res.Content)
	}
	if code, _ := ExitCode(res.Content); code != 1 {
		t.Fatalf("a test that doesn't reproduce the bug must not pass as proof; exit code %d", code)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "calc.txt")); string(data) != "3\n" {
		t.Fatalf("calc.txt = %q", data)
	}
}

func TestReproduceUndoesEveryChangeOfTheRequest(t *testing.T) {
	dir := writeTree(t, map[string]string{"calc.txt": "1\n"})
	apply, reset := applyTool(t, dir)
	reset()
	// Two applies in one request: the second's reproduce must undo both.
	runApply(t, apply, `{"changes":[{"path":"calc.txt","old_text":"1","new_text":"2"}],"check":"true"}`)
	res := runApply(t, apply, `{"changes":[{"path":"calc.txt","old_text":"2","new_text":"3"}],"check":"true","reproduce":"grep -q 1 calc.txt && exit 1; exit 0"}`)
	if !strings.Contains(res.Content, "The test reproduces the bug") {
		t.Fatalf("the original content was not restored for reproduce:\n%s", res.Content)
	}
	// A new request forgets the old originals.
	reset()
	res = runApply(t, apply, `{"changes":[{"path":"calc.txt","old_text":"3","new_text":"4"}],"check":"true","reproduce":"grep -q 3 calc.txt && exit 1; exit 0"}`)
	if !strings.Contains(res.Content, "The test reproduces the bug") {
		t.Fatalf("the new request's original is wrong:\n%s", res.Content)
	}
}

// A failure without the fix proves the bug only if the test itself ran and
// failed: one that was never found or run proves nothing, and no
// reproduction is claimed for it.
func TestReproduceClaimsNothingWithoutEvidence(t *testing.T) {
	for name, tc := range map[string]struct {
		opts      Options
		reproduce string
		want      string
	}{
		"test never ran":  {testRan(false), "exit 4", "isn't shown to reproduce the bug"},
		"no judge":        {Options{}, "exit 1", "isn't shown to reproduce the bug"},
		"command missing": {testRan(true), "no-such-command-girdle", "could not be run"},
		"not executable":  {testRan(true), "exit 126", "could not be run"},
	} {
		dir := writeTree(t, map[string]string{"calc.txt": "2\n"})
		apply, reset := applyToolWith(t, dir, tc.opts)
		reset()
		res := runApply(t, apply, `{"changes":[{"path":"calc.txt","old_text":"2","new_text":"3"}],"check":"true","reproduce":"`+tc.reproduce+`"}`)
		if !strings.Contains(res.Content, tc.want) || strings.Contains(res.Content, "The test reproduces the bug") {
			t.Errorf("%s:\n%s", name, res.Content)
		}
		// The check itself passed: only the claim is withheld.
		if code, _ := ExitCode(res.Content); code != 0 {
			t.Errorf("%s: exit code %d", name, code)
		}
	}
}
