// Package checkpoint holds Girdle's Jev checkpoints. Each checkpoint is a
// question set plus a policy that maps Jev's probabilities to an action.
package checkpoint

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// Action is what the kernel does after a checkpoint.
type Action string

const (
	// Stop ends the run: the task is done.
	Stop Action = "stop"
	// Nudge sends Nudge as a message and lets the LLM continue.
	Nudge Action = "nudge"
	// Ask ends the run and hands control to the user.
	Ask Action = "ask"
)

// TurnState is what Jev sees at the end of an LLM turn. Keep it small:
// Jev gets less accurate as unrelated state grows.
type TurnState struct {
	Task                 string   `json:"task"`
	Requirements         []string `json:"requirements,omitempty"`
	LastAssistantMessage string   `json:"last_assistant_message"`
	RecentSteps          []string `json:"recent_steps"`
}

// Decision records one checkpoint evaluation for the event log.
type Decision struct {
	Checkpoint  string                `json:"checkpoint"`
	Action      Action                `json:"action"`
	Rule        string                `json:"rule"`
	Nudge       string                `json:"nudge,omitempty"`
	State       TurnState             `json:"state"`
	Answers     map[string]jev.Answer `json:"answers,omitempty"`
	JevModel    string                `json:"jev_model,omitempty"`
	LatencyMS   int64                 `json:"latency_ms"`
	InputTokens int64                 `json:"input_tokens,omitzero"`
	Error       string                `json:"error,omitempty"`
}

// turnEndBase is asked whenever the LLM ends a turn without calling a tool.
var turnEndBase = map[string]jev.Question{
	"status": jev.Choice(
		"What is the state of the coding agent's work on `task`, judging by `last_assistant_message` and `recent_steps`?",
		map[string]string{
			"done":        "The agent says the whole task is finished",
			"in_progress": "The agent stopped partway: it describes a next step it has not taken yet, or parts of the task remain",
			"needs_user":  "The agent is waiting for an answer, decision or information that only the user can give",
			"stuck":       "The agent keeps failing at the same thing without a new approach, or has given up",
		},
	),
	"evidence": jev.Noul(
		"Do the command outputs in `recent_steps` show that the work described in `last_assistant_message` succeeded, for example tests or a build passing?",
	),
	"needless_ask": jev.Noul(
		"Is `last_assistant_message` asking the user for permission to do something that `task` already asked for?",
	),
}

// TurnEndQuestions returns the turn-end question set: the base questions
// plus one coverage question per requirement, all in one request.
func TurnEndQuestions(requirements []string) map[string]jev.Question {
	qs := maps.Clone(turnEndBase)
	for i := range requirements {
		qs[reqKey(i)] = jev.Noul(fmt.Sprintf(
			"Do `last_assistant_message` and `recent_steps` show that `requirements[%d]` has been done?", i))
		qs[instructionKey(i)] = jev.Noul(fmt.Sprintf(
			"Is `requirements[%d]` an instruction to do something, rather than a description of the situation?", i))
	}
	return qs
}

func reqKey(i int) string         { return fmt.Sprintf("req_%d", i) }
func instructionKey(i int) string { return fmt.Sprintf("req_%d_is_instruction", i) }

var listItem = regexp.MustCompile(`^\s*(?:\d+[.)]|[-*•])\s+(.+)$`)
var sentenceEnd = regexp.MustCompile(`([.!?])\s+`)

// SplitRequirements breaks a task into the separate things it asks for:
// list items plus any lead-in sentences, or otherwise its sentences.
// Sentences shorter than three words are dropped; list items are kept.
func SplitRequirements(task string) []string {
	const maxReqs, maxLen = 10, 300
	var items, other []string
	for line := range strings.Lines(task) {
		line = strings.TrimRight(line, "\r\n")
		if m := listItem.FindStringSubmatch(line); m != nil {
			items = append(items, strings.TrimSpace(m[1]))
		} else if t := strings.TrimSpace(line); t != "" {
			other = append(other, t)
		}
	}
	var reqs []string
	if len(items) >= 2 {
		reqs = append(longSentences(strings.Join(other, " ")), items...)
	} else {
		reqs = longSentences(task)
	}
	reqs = reqs[:min(len(reqs), maxReqs)]
	for i, r := range reqs {
		r = strings.TrimSuffix(strings.TrimSpace(r), ":")
		if len(r) > maxLen {
			r = r[:maxLen]
		}
		reqs[i] = r
	}
	return reqs
}

func longSentences(text string) []string {
	var out []string
	for _, s := range sentences(text) {
		if len(strings.Fields(s)) >= 3 {
			out = append(out, s)
		}
	}
	return out
}

func sentences(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	return strings.Split(sentenceEnd.ReplaceAllString(text, "$1\n"), "\n")
}

// Policy holds the thresholds for the turn-end checkpoint.
type Policy struct {
	// MinConfidence below which Jev's status is too unsure to act on.
	MinConfidence float64
	// Evidence is the noul threshold for accepting "done".
	Evidence float64
	// NeedlessAsk is the noul threshold for answering "go ahead" ourselves.
	NeedlessAsk float64
	// Coverage is the noul threshold below which a requirement counts as
	// not done.
	Coverage float64
	// MaxNudges is the most nudges one run may send before asking the user.
	MaxNudges int
}

// DefaultPolicy is conservative until the decision log says otherwise.
var DefaultPolicy = Policy{MinConfidence: 0.4, Evidence: 0.5, NeedlessAsk: 0.6, Coverage: 0.5, MaxNudges: 4}

// Nudge messages. They say they come from Girdle, not the user.
const (
	NudgeContinue = "[Girdle] You stopped before finishing. Carry on now with the next step you described, using the tools. Don't announce it first."
	NudgeVerify   = "[Girdle] You said the task is done, but your tool results don't show it working yet. Verify it now (for example run the build or the tests) and fix anything that fails."
	NudgeGoAhead  = "[Girdle] Yes, go ahead: the task already asks for this. Carry on without asking."
)

// History counts the nudges already sent in this run, by rule.
type History map[string]int

// Total is the number of nudges sent so far.
func (h History) Total() int {
	n := 0
	for v := range maps.Values(h) {
		n += v
	}
	return n
}

// Decide maps Jev's answers to an action. It returns the rule that fired
// and, for a nudge, the message to send.
func (p Policy) Decide(a map[string]jev.Answer, h History, requirements []string) (Action, string, string) {
	status := a["status"]
	if h.Total() >= p.MaxNudges {
		return Ask, "nudge_budget_spent", ""
	}
	if status.Confidence < p.MinConfidence {
		return Ask, "unsure", ""
	}
	switch status.Choice {
	case "done":
		if a["evidence"].Noul < p.Evidence && h["verify"] == 0 {
			return Nudge, "verify", NudgeVerify
		}
		if h["coverage"] == 0 {
			var missing []string
			for i, r := range requirements {
				done, ok := a[reqKey(i)]
				instr, known := a[instructionKey(i)]
				if ok && done.Noul < p.Coverage && (!known || instr.Noul >= 0.5) {
					missing = append(missing, r)
				}
			}
			if len(missing) > 0 {
				return Nudge, "coverage", coverageNudge(missing)
			}
		}
		return Stop, "done", ""
	case "in_progress":
		return Nudge, "continue", NudgeContinue
	case "needs_user":
		if a["needless_ask"].Noul >= p.NeedlessAsk {
			return Nudge, "go_ahead", NudgeGoAhead
		}
		return Ask, "needs_user", ""
	case "stuck":
		return Ask, "stuck", ""
	}
	return Ask, "unknown_status", ""
}

func coverageNudge(missing []string) string {
	var b strings.Builder
	b.WriteString("[Girdle] Before you finish: your tool results don't yet show these parts of the task done and checked:\n")
	for _, m := range missing {
		b.WriteString("- " + m + "\n")
	}
	b.WriteString("Do or verify each of them now, then report back with the evidence.")
	return b.String()
}

// TurnEnd asks Jev about the end of a turn and applies the policy. If Jev is
// unreachable it hands control to the user rather than guessing.
func TurnEnd(ctx context.Context, c *jev.Client, s TurnState, p Policy, h History) Decision {
	d := Decision{Checkpoint: "turn_end", State: s}
	start := time.Now()
	res, err := c.Ask(ctx, s, TurnEndQuestions(s.Requirements))
	d.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		d.Action, d.Rule, d.Error = Ask, "jev_unavailable", err.Error()
		return d
	}
	d.Answers, d.JevModel, d.InputTokens = res.Answers, res.Model, res.Usage.InputTokens
	d.Action, d.Rule, d.Nudge = p.Decide(res.Answers, h, s.Requirements)
	return d
}
