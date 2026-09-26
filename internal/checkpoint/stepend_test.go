package checkpoint

import (
	"testing"

	"github.com/timbrinded/girdle/internal/jev"
)

func TestStepDecide(t *testing.T) {
	p := DefaultStepPolicy
	reqs := []string{"add the flag", "the tool is used by the ops team"}
	with := func(complete, req0, req1, instr1 float64) map[string]jev.Answer {
		return map[string]jev.Answer{
			"complete":             {Type: "noul", Noul: complete},
			"req_0":                {Type: "noul", Noul: req0},
			"req_0_is_instruction": {Type: "noul", Noul: 0.95},
			"req_1":                {Type: "noul", Noul: req1},
			"req_1_is_instruction": {Type: "noul", Noul: instr1},
		}
	}
	cases := []struct {
		name   string
		a      map[string]jev.Answer
		action Action
		rule   string
	}{
		{"done and verified", with(0.95, 0.9, 0.9, 0.9), Stop, "done_early"},
		{"not sure it is complete", with(0.6, 0.9, 0.9, 0.9), Continue, "not_complete"},
		{"an instruction is open", with(0.95, 0.2, 0.9, 0.9), Continue, "requirement_open"},
		{"an open description does not count", with(0.95, 0.9, 0.1, 0.1), Stop, "done_early"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, rule := p.Decide(c.a, reqs)
			if action != c.action || rule != c.rule {
				t.Fatalf("Decide = (%s, %s), want (%s, %s)", action, rule, c.action, c.rule)
			}
		})
	}
}

func TestStepEndQuestions(t *testing.T) {
	qs := StepEndQuestions([]string{"a", "b"})
	for _, k := range []string{"complete", "req_0", "req_0_is_instruction", "req_1", "req_1_is_instruction"} {
		if _, ok := qs[k]; !ok {
			t.Fatalf("missing question %q", k)
		}
	}
	if _, ok := stepEndBase["req_0"]; ok {
		t.Fatal("StepEndQuestions changed the shared base set")
	}
}
