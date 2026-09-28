package checkpoint

import (
	"context"

	"github.com/timbrinded/girdle/internal/jev"
)

// heartbeatQuestions ask, every few steps of a turn, whether the agent is
// getting anywhere. A turn that loops or drifts can burn a minute of steps
// before it ends, and the turn-end checkpoint only sees it then.
var heartbeatQuestions = map[string]jev.Question{
	"trajectory": jev.Choice(
		"How is the coding agent's work on `task` going, judging by `recent_steps`?",
		map[string]string{
			"progressing": "Each step gathers new information or moves the code towards the task",
			"looping":     "The recent steps repeat similar lookups, searches or edits without learning anything new",
			"drifting":    "The recent steps work on something the task does not ask for",
			"blocked":     "The agent cannot go on without something only the user can give",
		},
	),
}

// HeartbeatPolicy holds the confidence each trajectory needs before the
// kernel acts on it.
type HeartbeatPolicy struct {
	Every     int     // steps between heartbeats
	Looping   float64 // confidence to nudge a loop
	Drifting  float64 // confidence to re-pin the task
	Blocked   float64 // confidence to hand back to the user
	MaxNudges int     // heartbeat nudges per request
}

// DefaultHeartbeatPolicy acts only when Jev is sure: a wrong nudge costs a
// step, a missed loop costs several.
var DefaultHeartbeatPolicy = HeartbeatPolicy{Every: 6, Looping: 0.6, Drifting: 0.6, Blocked: 0.8, MaxNudges: 2}

// Heartbeat nudges.
const (
	NudgeLooping = "[Girdle] Your last few steps repeat similar lookups without finding anything new. Stop searching: decide from what you already have, or try a different approach, and make the change."
	NudgeDrift   = "[Girdle] Your recent steps have drifted from the task. Come back to what it asks, and leave anything else alone:\n\n"
)

// HeartbeatDecision records one heartbeat.
type HeartbeatDecision struct {
	Checkpoint string `json:"checkpoint"`
	Action     Action `json:"action"`
	Rule       string `json:"rule"`
	Nudge      string `json:"nudge,omitempty"`
	Call
}

// Heartbeat asks Jev how the turn is going. If Jev is unreachable the agent
// simply carries on.
func Heartbeat(ctx context.Context, c *jev.Client, s TurnState, p HeartbeatPolicy) HeartbeatDecision {
	d := HeartbeatDecision{Checkpoint: "heartbeat", Action: Continue, Rule: "progressing"}
	var ok bool
	if d.Call, ok = ask(ctx, c, s, heartbeatQuestions); !ok {
		d.Rule = "jev_unavailable"
		return d
	}
	d.Action, d.Rule, d.Nudge = p.Decide(d.Answers["trajectory"], s.Task)
	return d
}

// Decide maps the trajectory to an action.
func (p HeartbeatPolicy) Decide(a jev.Answer, task string) (Action, string, string) {
	conf := a.Probabilities[a.Choice]
	if conf == 0 {
		conf = a.Confidence
	}
	switch {
	case a.Choice == "looping" && conf >= p.Looping:
		return Nudge, "looping", NudgeLooping
	case a.Choice == "drifting" && conf >= p.Drifting:
		return Nudge, "drifting", NudgeDrift + task
	case a.Choice == "blocked" && conf >= p.Blocked:
		return Ask, "blocked", ""
	}
	return Continue, "progressing", ""
}
