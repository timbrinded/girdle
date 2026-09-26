package checkpoint

import (
	"context"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// Effort is the reasoning effort the LLM is asked to use.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// RouteState is what Jev sees when a request arrives.
type RouteState struct {
	Request string `json:"request"`
}

// RouteQuestions score how much reasoning a request needs.
var RouteQuestions = map[string]jev.Question{
	"complexity": jev.Score(
		"How much reasoning will a coding agent need to complete `request`?",
		"Trivial: one obvious step, like a lookup, a rename or a single command",
		"Moderate: a few coordinated edits, or an investigation in one area",
		"Hard: multi-file design, subtle debugging, precise edge cases or many separate requirements",
	),
}

// RoutePolicy maps the complexity score (0 to 2) to a reasoning effort.
type RoutePolicy struct {
	// LowBelow and HighFrom are cut points on the expected score.
	LowBelow float64
	HighFrom float64
}

// DefaultRoutePolicy routes most requests to low effort. In the 2026-09-26
// benchmark, low effort with the turn-end checkpoints passed every
// spec-heavy task that medium did, in about half the time. Only requests Jev
// scores near "hard" get more effort. Retune from the decision log.
var DefaultRoutePolicy = RoutePolicy{LowBelow: 1.7, HighFrom: 1.9}

// RouteDecision records the routing checkpoint.
type RouteDecision struct {
	Checkpoint  string                `json:"checkpoint"`
	Effort      Effort                `json:"effort"`
	Score       float64               `json:"score"`
	State       RouteState            `json:"state"`
	Answers     map[string]jev.Answer `json:"answers,omitempty"`
	JevModel    string                `json:"jev_model,omitempty"`
	LatencyMS   int64                 `json:"latency_ms"`
	InputTokens int64                 `json:"input_tokens,omitzero"`
	Error       string                `json:"error,omitempty"`
}

// Route picks the reasoning effort for a request. If Jev is unreachable it
// falls back to fallback.
func Route(ctx context.Context, c *jev.Client, s RouteState, p RoutePolicy, fallback Effort) RouteDecision {
	d := RouteDecision{Checkpoint: "route", State: s, Effort: fallback}
	start := time.Now()
	res, err := c.Ask(ctx, s, RouteQuestions)
	d.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		d.Error = err.Error()
		return d
	}
	d.Answers, d.JevModel, d.InputTokens = res.Answers, res.Model, res.Usage.InputTokens
	d.Score = res.Answers["complexity"].Score
	switch {
	case d.Score < p.LowBelow:
		d.Effort = EffortLow
	case d.Score >= p.HighFrom:
		d.Effort = EffortHigh
	default:
		d.Effort = EffortMedium
	}
	return d
}
