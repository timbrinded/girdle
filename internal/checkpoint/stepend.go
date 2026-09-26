package checkpoint

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// Continue lets the LLM carry on with its turn: the checkpoint found no
// reason to step in.
const Continue Action = "continue"

// stepEndBase is asked after a step whose last command succeeded, while the
// LLM's turn is still open. When the tool results already show the task done,
// the run can end there instead of paying for one more LLM step that only
// writes a summary.
var stepEndBase = map[string]jev.Question{
	"complete": jev.Noul(
		"Do `recent_steps` show that all of `task` has been done, and does the output of the last command show the result working, for example the build and the tests passing?",
	),
}

// StepEndQuestions returns the step-end question set: the base question plus
// one coverage question per requirement, all in one request.
func StepEndQuestions(requirements []string) map[string]jev.Question {
	qs := maps.Clone(stepEndBase)
	for i := range requirements {
		qs[reqKey(i)] = jev.Noul(fmt.Sprintf(
			"Do `recent_steps` show that `requirements[%d]` has been done?", i))
		qs[instructionKey(i)] = jev.Noul(fmt.Sprintf(
			"Is `requirements[%d]` an instruction to do something, rather than a description of the situation?", i))
	}
	return qs
}

// StepPolicy holds the thresholds for ending a run before the LLM's turn ends.
type StepPolicy struct {
	// Complete is the noul threshold for "the task is done and verified".
	Complete float64
	// Coverage is the noul threshold below which a requirement counts as
	// not done.
	Coverage float64
	// Fanout asks the broad question set about the request's changes and
	// the latest check instead. It stops when P(done_verified) reaches
	// Verified and Jev reads the check as exercising the task at Exercises
	// or more. The state must carry the changes and the check's output.
	Fanout    bool
	Verified  float64
	Exercises float64
}

// FanoutStepPolicy is the step-end fan-out's policy (decision 0013).
var FanoutStepPolicy = StepPolicy{Fanout: true, Verified: 0.34, Exercises: 0.7}

// DefaultStepPolicy stops early from a "complete" noul of 0.7. In the
// 2026-09-26 decision logs every early stop from 0.80 to 0.85 passed its
// hidden tests (27 of 27), and in all 28 runs that went on from 0.70 to 0.80
// no later change touched code. The runs that went on only added tests, so
// the kernel also refuses to stop early while the request asks for tests and
// no test file has changed.
var DefaultStepPolicy = StepPolicy{Complete: 0.7, Coverage: 0.5}

// Decide maps Jev's answers to Stop or Continue.
func (p StepPolicy) Decide(a map[string]jev.Answer, requirements []string) (Action, string) {
	if a["complete"].Noul < p.Complete {
		return Continue, "not_complete"
	}
	for i := range requirements {
		done, ok := a[reqKey(i)]
		instr, known := a[instructionKey(i)]
		if ok && done.Noul < p.Coverage && (!known || instr.Noul >= 0.5) {
			return Continue, "requirement_open"
		}
	}
	return Stop, "done_early"
}

// StepEnd asks Jev whether the tool results so far show the task done. If Jev
// is unreachable the LLM simply carries on and the turn-end checkpoint
// decides as usual.
func StepEnd(ctx context.Context, c *jev.Client, s TurnState, p StepPolicy) Decision {
	d := Decision{Checkpoint: "step_end", State: s}
	start := time.Now()
	qs := StepEndQuestions(s.Requirements)
	if p.Fanout {
		qs = StepFanQuestions(s.Requirements)
	}
	res, err := c.Ask(ctx, s, qs)
	d.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		d.Action, d.Rule, d.Error = Continue, "jev_unavailable", err.Error()
		return d
	}
	d.Answers, d.JevModel, d.InputTokens = res.Answers, res.Model, res.Usage.InputTokens
	if p.Fanout {
		d.Action, d.Rule = p.decideFan(res.Answers)
	} else {
		d.Action, d.Rule = p.Decide(res.Answers, s.Requirements)
	}
	return d
}
