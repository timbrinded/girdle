// Package kernel owns Girdle's loop. The LLM decides and does the work inside
// a turn; at every turn end the kernel asks Jev whether to stop, nudge the
// LLM to continue, or hand control back to the user.
package kernel

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/tools"
)

// Outcome is how a run ended.
type Outcome string

const (
	OutcomeDone      Outcome = "done"
	OutcomeNeedsUser Outcome = "needs_user"
	OutcomeError     Outcome = "error"
	OutcomeCancelled Outcome = "cancelled"
)

// Config wires a session together.
type Config struct {
	Model           fantasy.LanguageModel
	ModelName       string
	ProviderOptions fantasy.ProviderOptions
	Jev             *jev.Client
	Dir             string
	Policy          checkpoint.Policy
	// Checkpoints turns the Jev turn-end checkpoint on. When false, every
	// turn end stops the run, as in a plain agent loop.
	Checkpoints bool
	// Route asks Jev how hard each request is and sets the reasoning effort.
	Route       bool
	RoutePolicy checkpoint.RoutePolicy
	// EffortOptions builds provider options for a reasoning effort. Required
	// when Route is on.
	EffortOptions   func(checkpoint.Effort) fantasy.ProviderOptions
	MaxStepsPerTurn int
	Emit            func(Event)
	Log             *Log
}

// Session is one conversation in one working directory.
type Session struct {
	ID      string
	cfg     Config
	agent   fantasy.Agent
	history []fantasy.Message
	steps   []string
	usage   Usage
	// callOptions override the agent's provider options for this run.
	callOptions fantasy.ProviderOptions
}

// NewSession builds a session with the four built-in tools.
func NewSession(cfg Config) *Session {
	if cfg.MaxStepsPerTurn == 0 {
		cfg.MaxStepsPerTurn = 60
	}
	agent := fantasy.NewAgent(
		cfg.Model,
		fantasy.WithSystemPrompt(systemPrompt(cfg.Dir)),
		fantasy.WithTools(tools.All(cfg.Dir)...),
		fantasy.WithStopConditions(fantasy.StepCountIs(cfg.MaxStepsPerTurn)),
		fantasy.WithProviderOptions(cfg.ProviderOptions),
	)
	s := &Session{ID: uuid.New().String(), cfg: cfg, agent: agent}
	s.emit(Event{Type: EventSessionStart, Meta: map[string]string{
		"model":       cfg.ModelName,
		"jev_model":   jevModel(cfg.Jev),
		"dir":         cfg.Dir,
		"checkpoints": fmt.Sprint(cfg.Checkpoints),
		"route":       fmt.Sprint(cfg.Route),
	}})
	return s
}

// Usage returns the tokens used so far.
func (s *Session) Usage() Usage { return s.usage }

// Seed prepends an existing conversation, for scenarios that start midway.
func (s *Session) Seed(msgs []fantasy.Message) {
	s.history = append(s.history, msgs...)
}

// Run sends prompt and works until the task is done or the user is needed.
func (s *Session) Run(ctx context.Context, prompt string) (Outcome, string) {
	s.emit(Event{Type: EventUserMessage, Text: prompt})
	s.history = append(s.history, fantasy.NewUserMessage(prompt))
	s.route(ctx, prompt)
	return s.loop(ctx, prompt, true)
}

// Resume evaluates the seeded conversation's last turn first, then carries on.
func (s *Session) Resume(ctx context.Context, task string) (Outcome, string) {
	s.route(ctx, task)
	return s.loop(ctx, task, false)
}

// route asks Jev how hard the request is and sets this run's reasoning effort.
func (s *Session) route(ctx context.Context, request string) {
	s.callOptions = nil
	if !s.cfg.Route || s.cfg.Jev == nil || s.cfg.EffortOptions == nil {
		return
	}
	d := checkpoint.Route(ctx, s.cfg.Jev, checkpoint.RouteState{Request: request}, s.cfg.RoutePolicy, checkpoint.EffortMedium)
	s.usage.JevTokens += d.InputTokens
	s.callOptions = s.cfg.EffortOptions(d.Effort)
	s.emit(Event{Type: EventRoute, Route: &d})
}

func (s *Session) loop(ctx context.Context, task string, callLLM bool) (Outcome, string) {
	nudges := checkpoint.History{}
	requirements := checkpoint.SplitRequirements(task)
	lastText := lastAssistantText(s.history)
	for {
		if callLLM {
			text, err := s.turn(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return s.end(OutcomeCancelled, "cancelled")
				}
				s.emit(Event{Type: EventError, Text: err.Error()})
				return s.end(OutcomeError, err.Error())
			}
			lastText = text
		}
		callLLM = true

		if !s.cfg.Checkpoints {
			return s.end(OutcomeDone, "turn_end")
		}
		d := checkpoint.TurnEnd(ctx, s.cfg.Jev, checkpoint.TurnState{
			Task:                 task,
			Requirements:         requirements,
			LastAssistantMessage: clipMiddle(lastText, 2000),
			RecentSteps:          lastN(s.steps, 8),
		}, s.cfg.Policy, nudges)
		s.usage.JevTokens += d.InputTokens
		s.emit(Event{Type: EventDecision, Decision: &d})

		switch d.Action {
		case checkpoint.Stop:
			return s.end(OutcomeDone, d.Rule)
		case checkpoint.Nudge:
			nudges[d.Rule]++
			s.emit(Event{Type: EventNudge, Text: d.Nudge, Reason: d.Rule})
			s.history = append(s.history, fantasy.NewUserMessage(d.Nudge))
		default:
			return s.end(OutcomeNeedsUser, d.Rule)
		}
	}
}

// turn runs the LLM until it stops calling tools, and returns its final text.
func (s *Session) turn(ctx context.Context) (string, error) {
	calls := map[string]fantasy.ToolCallContent{}
	res, err := s.agent.Stream(ctx, fantasy.AgentStreamCall{
		Messages:        s.history,
		ProviderOptions: s.callOptions,
		OnTextDelta: func(_, text string) error {
			s.emit(Event{Type: EventTextDelta, Text: text})
			return nil
		},
		OnToolCall: func(tc fantasy.ToolCallContent) error {
			calls[tc.ToolCallID] = tc
			s.emit(Event{Type: EventToolCall, Tool: tc.ToolName, CallID: tc.ToolCallID, Input: tc.Input})
			return nil
		},
		OnToolResult: func(tr fantasy.ToolResultContent) error {
			text, isErr := resultText(tr)
			s.emit(Event{Type: EventToolResult, Tool: tr.ToolName, CallID: tr.ToolCallID, Text: clipMiddle(text, 4000), IsError: isErr})
			s.steps = append(s.steps, summarizeStep(calls[tr.ToolCallID], text))
			return nil
		},
		OnStepFinish: func(sr fantasy.StepResult) error {
			if t := strings.TrimSpace(sr.Content.Text()); t != "" {
				s.emit(Event{Type: EventAssistantText, Text: t})
			}
			return nil
		},
	})
	if err != nil {
		return "", err
	}
	for _, st := range res.Steps {
		s.history = append(s.history, st.Messages...)
	}
	u := Usage{
		InputTokens:     res.TotalUsage.InputTokens,
		OutputTokens:    res.TotalUsage.OutputTokens,
		ReasoningTokens: res.TotalUsage.ReasoningTokens,
		CacheReadTokens: res.TotalUsage.CacheReadTokens,
	}
	s.usage.InputTokens += u.InputTokens
	s.usage.OutputTokens += u.OutputTokens
	s.usage.ReasoningTokens += u.ReasoningTokens
	s.usage.CacheReadTokens += u.CacheReadTokens
	s.emit(Event{Type: EventTurnEnd, Steps: len(res.Steps), Usage: &u})
	return res.Response.Content.Text(), nil
}

func (s *Session) end(o Outcome, reason string) (Outcome, string) {
	u := s.usage
	s.emit(Event{Type: EventRunEnd, Outcome: o, Reason: reason, Usage: &u})
	return o, reason
}

func (s *Session) emit(e Event) {
	e.Time = time.Now()
	e.Session = s.ID
	if err := s.cfg.Log.Write(e); err != nil && s.cfg.Emit != nil {
		s.cfg.Emit(Event{Time: e.Time, Session: s.ID, Type: EventError, Text: "event log: " + err.Error()})
	}
	if s.cfg.Emit != nil {
		s.cfg.Emit(e)
	}
}

// commonTools are the commands whose presence is worth telling the model
// about, so it doesn't waste a step guessing (for example python vs python3).
var commonTools = []string{"git", "go", "python3", "python", "node", "npm", "bun", "cargo", "java", "ruby", "make"}

func availableTools() string {
	var found []string
	for _, t := range commonTools {
		if _, err := exec.LookPath(t); err == nil {
			found = append(found, t)
		}
	}
	if len(found) == 0 {
		return "none detected"
	}
	return strings.Join(found, ", ")
}

func systemPrompt(dir string) string {
	return fmt.Sprintf(`You are Girdle, a coding agent working in the repository at %s (%s/%s). Today is %s. Commands available on PATH: %s.

Use the tools to read and change files and to run commands. Work until the task is completely done: make the change, then verify it by building and running the relevant tests, and fix anything that fails.

When you finish, reply briefly with what you changed and the evidence that it works. Only ask the user a question when you need a decision that only they can make.`,
		dir, runtime.GOOS, runtime.GOARCH, time.Now().Format("2 January 2006"), availableTools())
}

func resultText(tr fantasy.ToolResultContent) (string, bool) {
	if t, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](tr.Result); ok {
		return t.Text, false
	}
	if e, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](tr.Result); ok && e.Error != nil {
		return e.Error.Error(), true
	}
	return "", false
}

// summarizeStep is the one-line view of a tool call that Jev sees. It keeps
// mostly the end of the output, where test and build results appear: an
// earlier version kept too little and Jev missed passing tests.
func summarizeStep(call fantasy.ToolCallContent, result string) string {
	return fmt.Sprintf("%s %s -> %s", call.ToolName, clipMiddle(oneLine(call.Input), 160), clipEnd(oneLine(result), 700))
}

// clipEnd is clipMiddle weighted towards the end of s.
func clipEnd(s string, n int) string { return clipKeeping(s, n, n/5) }

func lastAssistantText(msgs []fantasy.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != fantasy.MessageRoleAssistant {
			continue
		}
		var b strings.Builder
		for _, p := range msgs[i].Content {
			if t, ok := fantasy.AsMessagePart[fantasy.TextPart](p); ok {
				b.WriteString(t.Text)
			}
		}
		return b.String()
	}
	return ""
}

func jevModel(c *jev.Client) string {
	if c == nil {
		return ""
	}
	return c.Model
}

func lastN(s []string, n int) []string {
	return s[max(0, len(s)-n):]
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// clipMiddle keeps the start and end of s, where the useful parts usually
// are.
func clipMiddle(s string, n int) string { return clipKeeping(s, n, n/3) }

// clipKeeping shortens s to about n bytes, keeping head bytes from the start
// and the rest from the end. It always returns valid UTF-8: Jev and the event
// log reject anything else, and tool output can be arbitrary bytes.
func clipKeeping(s string, n, head int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= n {
		return s
	}
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	tail := len(s) - (n - head)
	for tail < len(s) && !utf8.RuneStart(s[tail]) {
		tail++
	}
	return s[:head] + " … " + s[tail:]
}
