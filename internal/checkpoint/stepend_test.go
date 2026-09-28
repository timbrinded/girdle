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

func TestFanoutPolicyReadsVerifiedProbability(t *testing.T) {
	p := FanoutStepPolicy
	for _, tc := range []struct {
		verified, exercises float64
		want                Action
	}{{0.5, 0.9, Stop}, {0.34, 0.7, Stop}, {0.2, 0.9, Continue}, {0.9, 0.39, Continue}} {
		a := map[string]jev.Answer{
			"status":          {Type: "choice", Choice: "done_unverified", Probabilities: map[string]float64{"done_verified": tc.verified}},
			"check_exercises": {Type: "noul", Noul: tc.exercises},
		}
		if got, _ := p.decideFan(a); got != tc.want {
			t.Errorf("P(done_verified)=%.2f, exercises %.2f: %s, want %s", tc.verified, tc.exercises, got, tc.want)
		}
	}
}
