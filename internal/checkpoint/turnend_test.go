package checkpoint

import (
	"slices"
	"strings"
	"testing"

	"github.com/timbrinded/girdle/internal/jev"
)

func answers(status string, conf, evidence, needless float64) map[string]jev.Answer {
	return map[string]jev.Answer{
		"status":       {Type: "choice", Choice: status, Confidence: conf},
		"evidence":     {Type: "noul", Noul: evidence},
		"needless_ask": {Type: "noul", Noul: needless},
	}
}

func TestDecide(t *testing.T) {
	p := DefaultPolicy
	cases := []struct {
		name    string
		a       map[string]jev.Answer
		h       History
		action  Action
		rule    string
		hasText bool
	}{
		{"done with evidence", answers("done", 0.9, 0.9, 0), History{}, Stop, "done", false},
		{"done without evidence", answers("done", 0.9, 0.1, 0), History{}, Nudge, "verify", true},
		{"done without evidence after a verify nudge", answers("done", 0.9, 0.1, 0), History{"verify": 1}, Stop, "done", false},
		{"announced but stopped", answers("in_progress", 0.95, 0, 0), History{}, Nudge, "continue", true},
		{"needless permission", answers("needs_user", 0.9, 0, 0.8), History{}, Nudge, "go_ahead", true},
		{"real question", answers("needs_user", 0.9, 0, 0.1), History{}, Ask, "needs_user", false},
		{"stuck", answers("stuck", 0.9, 0, 0), History{}, Ask, "stuck", false},
		{"unsure", answers("in_progress", 0.2, 0, 0), History{}, Ask, "unsure", false},
		{"budget spent", answers("in_progress", 0.95, 0, 0), History{"continue": 4}, Ask, "nudge_budget_spent", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, rule, text := p.Decide(c.a, c.h, nil)
			if action != c.action || rule != c.rule || (text != "") != c.hasText {
				t.Fatalf("Decide = (%s, %s, %q), want (%s, %s, text=%v)", action, rule, text, c.action, c.rule, c.hasText)
			}
		})
	}
}

func TestDecideCoverage(t *testing.T) {
	reqs := []string{"add the flag", "add a test for the flag"}
	a := answers("done", 0.95, 0.9, 0)
	a["req_0"] = jev.Answer{Type: "noul", Noul: 0.9}
	a["req_1"] = jev.Answer{Type: "noul", Noul: 0.2}

	action, rule, text := DefaultPolicy.Decide(a, History{}, reqs)
	if action != Nudge || rule != "coverage" || !strings.Contains(text, "add a test for the flag") || strings.Contains(text, "- add the flag\n") {
		t.Fatalf("got (%s, %s, %q)", action, rule, text)
	}
	if action, rule, _ := DefaultPolicy.Decide(a, History{"coverage": 1}, reqs); action != Stop || rule != "done" {
		t.Fatalf("coverage should nudge only once, got (%s, %s)", action, rule)
	}
	a["req_1_is_instruction"] = jev.Answer{Type: "noul", Noul: 0.1}
	if action, rule, _ := DefaultPolicy.Decide(a, History{}, reqs); action != Stop || rule != "done" {
		t.Fatalf("a description is not a requirement to cover, got (%s, %s)", action, rule)
	}
}

func TestSplitRequirements(t *testing.T) {
	list := "Make these changes:\n\n1. Add `validate(cfg)`.\n2. Make `load` call `validate` before returning.\n- Document it in README.md\n"
	got := SplitRequirements(list)
	want := []string{"Make these changes", "Add `validate(cfg)`.", "Make `load` call `validate` before returning.", "Document it in README.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("list: got %q", got)
	}
	prose := "Rename the function everywhere. Make sure the tests pass. Ok."
	got = SplitRequirements(prose)
	want = []string{"Rename the function everywhere.", "Make sure the tests pass."}
	if !slices.Equal(got, want) {
		t.Fatalf("prose: got %q", got)
	}
}
