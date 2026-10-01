package checkpoint

import (
	"context"

	"github.com/timbrinded/girdle/internal/jev"
)

// RouteState is what Jev sees when a request arrives.
type RouteState struct {
	Request string `json:"request"`
}

// Subjects are the options of the route's subject question: what a request
// asks the agent for.
const (
	SubjectChange       = "change"
	SubjectCode         = "code_question"
	SubjectConversation = "conversation"
	SubjectGirdle       = "girdle"
)

// RouteQuestions score how much reasoning a request needs, and say what it
// is about.
var RouteQuestions = map[string]jev.Question{
	"complexity": jev.Score(
		"How much reasoning will a coding agent need to complete `request`?",
		"Trivial: one obvious step, like a lookup, a rename or a single command",
		"Moderate: a few coordinated edits, or an investigation in one area",
		"Hard: multi-file design, subtle debugging, precise edge cases or many separate requirements",
	),
	"tests": jev.Noul("Does `request` ask the agent to write new tests or change existing ones?"),
	"subject": jev.Choice(
		"What does `request` ask Girdle, the coding agent it is sent to, for?",
		map[string]string{
			SubjectChange:       "Work on the code or the machine: change, fix, add, build, test or run something",
			SubjectCode:         "Only an answer about the code, the repository or the project in the working directory",
			SubjectConversation: "Only an answer about this conversation: what the agent did, changed, found or decided so far, or why",
			SubjectGirdle:       "Only an answer about Girdle itself: how to use or configure it, its models, reasoning effort, keys, flags, commands, features or how it decides things",
		},
	),
}

// RoutePolicy maps the complexity score (0 to 2) to a reasoning effort, and
// the subject's probabilities to how the request is handled.
type RoutePolicy struct {
	// LowBelow and HighFrom are cut points on the expected score.
	LowBelow float64
	HighFrom float64
	// GirdleFrom is the P(girdle) from which the request is answered from
	// the girdle tool.
	GirdleFrom float64
	// AnswerBelow is the P(change) below which the request only wants an
	// answer, so its turn end needs no test evidence.
	AnswerBelow float64
}

// DefaultRoutePolicy routes most requests to low effort. In the 2026-09-26
// benchmark, low effort with the turn-end checkpoints passed every
// spec-heavy task that medium did, in about half the time. The low cut moved
// from 1.7 to 1.85 the same day: js-csv, scored 1.76 to 1.8, passed every run
// at low in both flows, and in the fast flow low took 37 s against 48 s at
// medium. Only requests Jev scores near "hard" get more effort. Retune from
// the decision log.
//
// The subject cuts are untuned starting points. AnswerBelow is the cautious
// one: a change misread as a question would stop without its evidence.
var DefaultRoutePolicy = RoutePolicy{LowBelow: 1.85, HighFrom: 1.9, GirdleFrom: 0.5, AnswerBelow: 0.3}

// RouteDecision records the routing checkpoint.
type RouteDecision struct {
	Checkpoint string  `json:"checkpoint"`
	Effort     Effort  `json:"effort"`
	Score      float64 `json:"score"`
	// Tests is Jev's noul for "the request asks for tests".
	Tests float64 `json:"tests,omitzero"`
	// Subject is Jev's likeliest subject. Girdle and Answer are the
	// policy's reading of all of them.
	Subject string     `json:"subject,omitempty"`
	Girdle  bool       `json:"girdle,omitzero"`
	Answer  bool       `json:"answer,omitzero"`
	State   RouteState `json:"state"`
	Call
}

// Route picks the reasoning effort for a request and reads its subject. If
// Jev is unreachable it falls back to medium effort, and treats the request
// as a change, so every check stays on.
func Route(ctx context.Context, c *jev.Client, s RouteState, p RoutePolicy) RouteDecision {
	d := RouteDecision{Checkpoint: "route", State: s, Effort: EffortMedium}
	var ok bool
	if d.Call, ok = ask(ctx, c, s, RouteQuestions); !ok {
		return d
	}
	d.Score = d.Answers["complexity"].Score
	d.Tests = d.Answers["tests"].Noul
	subject := d.Answers["subject"]
	d.Subject = subject.Choice
	d.Girdle = subject.Probabilities[SubjectGirdle] >= p.GirdleFrom
	// Without probabilities, P(change) would read as zero.
	d.Answer = len(subject.Probabilities) > 0 && subject.Probabilities[SubjectChange] < p.AnswerBelow
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

// Handling says how the request is handled beyond its effort, for display:
// "about Girdle", "answer only", or nothing for a change.
func (d RouteDecision) Handling() string {
	switch {
	case d.Girdle:
		return "about Girdle"
	case d.Answer:
		return "answer only"
	}
	return ""
}
