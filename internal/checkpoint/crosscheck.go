package checkpoint

import (
	"context"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// CrossCheckState is what Jev sees when an independent cross-check of a
// request fails against the agent's change.
type CrossCheckState struct {
	Task   string `json:"task"`
	Test   string `json:"test"`
	Output string `json:"output"`
}

// crossCheckQuestions ask whether a failing cross-check is the test's own
// fault. A test that doesn't even build or load, or that expects something
// the task never asked for, would only cost the agent a step to dismiss.
var crossCheckQuestions = map[string]jev.Question{
	"test_at_fault": jev.Noul(
		"Does `output` show `test` failing because of a mistake in the test itself, such as a wrong package name, a wrong import path, a compile or syntax error in the test code, or an expected value that `task` does not ask for, rather than because the code under test gets something in `task` wrong?",
	),
}

// CrossCheckDecision records the cross-check checkpoint.
type CrossCheckDecision struct {
	Checkpoint  string                `json:"checkpoint"`
	Valid       bool                  `json:"valid"`
	Answers     map[string]jev.Answer `json:"answers,omitempty"`
	JevModel    string                `json:"jev_model,omitempty"`
	LatencyMS   int64                 `json:"latency_ms"`
	InputTokens int64                 `json:"input_tokens,omitzero"`
	Error       string                `json:"error,omitempty"`
}

// CrossCheck judges a failing cross-check. It is valid, and goes to the
// agent, unless Jev reads the failure as the test's own fault. If Jev is
// unreachable the failure goes to the agent, which can still dismiss it.
func CrossCheck(ctx context.Context, c *jev.Client, s CrossCheckState) CrossCheckDecision {
	d := CrossCheckDecision{Checkpoint: "crosscheck", Valid: true}
	start := time.Now()
	res, err := c.Ask(ctx, s, crossCheckQuestions)
	d.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		d.Error = err.Error()
		return d
	}
	d.Answers, d.JevModel, d.InputTokens = res.Answers, res.Model, res.Usage.InputTokens
	d.Valid = res.Answers["test_at_fault"].Noul < 0.5
	return d
}
