package checkpoint

import (
	"strings"
	"testing"

	"github.com/timbrinded/girdle/internal/jev"
)

func TestHeartbeatDecide(t *testing.T) {
	p := DefaultHeartbeatPolicy
	ans := func(choice string, conf float64) jev.Answer {
		return jev.Answer{Type: "choice", Choice: choice, Probabilities: map[string]float64{choice: conf}}
	}
	cases := []struct {
		a      jev.Answer
		action Action
		rule   string
	}{
		{ans("progressing", 0.9), Continue, "progressing"},
		{ans("looping", 0.9), Nudge, "looping"},
		{ans("looping", 0.4), Continue, "progressing"},
		{ans("drifting", 0.8), Nudge, "drifting"},
		{ans("blocked", 0.9), Ask, "blocked"},
		{ans("blocked", 0.6), Continue, "progressing"},
	}
	for _, c := range cases {
		action, rule, nudge := p.Decide(c.a, "fix the parser")
		if action != c.action || rule != c.rule {
			t.Errorf("%s %.1f: got %s %s", c.a.Choice, c.a.Probabilities[c.a.Choice], action, rule)
		}
		if rule == "drifting" && !strings.HasSuffix(nudge, "fix the parser") {
			t.Errorf("drift nudge doesn't restate the task: %q", nudge)
		}
	}
}
