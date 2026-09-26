// Package kernel owns Girdle's loop. The LLM decides and does the work inside
// a turn; at every turn end the kernel asks Jev whether to stop, nudge the
// LLM to continue, or hand control back to the user.
package kernel

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"fmt"
	"maps"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/race"
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

	// Snapshot sends the repository's files with each request, so the LLM
	// doesn't spend steps listing and reading them.
	Snapshot       bool
	SnapshotBudget int
	// Batch asks the LLM to make all its edits and run the checks in one
	// step, since every extra step costs seconds of latency.
	Batch bool
	// EarlyStop asks Jev, after any step whose last command succeeded,
	// whether the task is already done, and ends the run there if so.
	// It needs Checkpoints.
	EarlyStop  bool
	StepPolicy checkpoint.StepPolicy
	// Race sends each LLM call this many times at once and keeps the first
	// complete answer. Below 2, calls are not raced.
	Race int
	// Speculate starts a request's first LLM call on the usual effort while
	// Jev routes it, instead of waiting for the route. It needs Route.
	Speculate bool
}

// Session is one conversation in one working directory.
type Session struct {
	ID      string
	cfg     Config
	model   fantasy.LanguageModel // cfg.Model, raced if cfg.Race asks for it
	tools   []fantasy.AgentTool
	agent   fantasy.Agent
	history []fantasy.Message
	steps   []string
	mu      sync.Mutex // guards usage while LLM calls run concurrently
	emitMu  sync.Mutex // serialises emit
	usage   Usage
	// callOptions override the agent's provider options for this run.
	callOptions fantasy.ProviderOptions
	// routing delivers Jev's route while the first call runs on a guess.
	routing chan checkpoint.RouteDecision

	// Facts about the current request, for the step-end checkpoint.
	edited  map[string]bool // files changed by a successful edit or apply
	lastOK  bool            // the latest tool result was a check that exited 0
	lastCmd string          // that check
}

// NewSession builds a session with the four built-in tools.
func NewSession(cfg Config) *Session {
	if cfg.MaxStepsPerTurn == 0 {
		cfg.MaxStepsPerTurn = 60
	}
	s := &Session{ID: uuid.New().String(), cfg: cfg, tools: tools.All(cfg.Dir), edited: map[string]bool{}}
	if cfg.Batch {
		s.tools = tools.Batched(cfg.Dir)
	}
	s.model = sessionModel{LanguageModel: race.New(cfg.Model, cfg.Race, s.noteRace), s: s}
	s.agent = fantasy.NewAgent(
		s.model,
		fantasy.WithSystemPrompt(systemPrompt(cfg)),
		fantasy.WithTools(s.tools...),
		fantasy.WithStopConditions(fantasy.StepCountIs(cfg.MaxStepsPerTurn)),
		fantasy.WithProviderOptions(cfg.ProviderOptions),
	)
	s.emit(Event{Type: EventSessionStart, Meta: map[string]string{
		"model":       cfg.ModelName,
		"jev_model":   jevModel(cfg.Jev),
		"dir":         cfg.Dir,
		"checkpoints": fmt.Sprint(cfg.Checkpoints),
		"route":       fmt.Sprint(cfg.Route),
		"snapshot":    fmt.Sprint(cfg.Snapshot),
		"batch":       fmt.Sprint(cfg.Batch),
		"early_stop":  fmt.Sprint(cfg.EarlyStop),
		"race":        fmt.Sprint(max(cfg.Race, 1)),
		"speculate":   fmt.Sprint(cfg.Speculate),
	}})
	return s
}

// Usage returns the tokens used so far.
func (s *Session) Usage() Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

// addUsage adds an LLM or Jev call's tokens to the session's total.
func (s *Session) addUsage(u Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage.InputTokens += u.InputTokens
	s.usage.OutputTokens += u.OutputTokens
	s.usage.ReasoningTokens += u.ReasoningTokens
	s.usage.CacheReadTokens += u.CacheReadTokens
	s.usage.JevTokens += u.JevTokens
}

func usageOf(u fantasy.Usage) Usage {
	return Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, ReasoningTokens: u.ReasoningTokens, CacheReadTokens: u.CacheReadTokens}
}

// Seed prepends an existing conversation, for scenarios that start midway.
func (s *Session) Seed(msgs []fantasy.Message) {
	s.history = append(s.history, msgs...)
}

// Run sends prompt and works until the task is done or the user is needed.
func (s *Session) Run(ctx context.Context, prompt string) (Outcome, string) {
	s.emit(Event{Type: EventUserMessage, Text: prompt})
	s.resetFacts()
	// Routing waits on Jev and the snapshot on the disk: do both at once.
	var snap Snapshot
	var wg sync.WaitGroup
	if s.cfg.Snapshot {
		wg.Go(func() { snap = TakeSnapshot(ctx, s.cfg.Dir, cmp.Or(s.cfg.SnapshotBudget, DefaultSnapshotBudget)) })
	}
	if s.routingOn() && s.cfg.Speculate {
		// The goroutine gets its own copy of the channel: the first LLM call
		// takes s.routing and clears it, possibly before the goroutine runs.
		routing := make(chan checkpoint.RouteDecision, 1)
		s.routing = routing
		go func() { routing <- s.routeDecision(ctx, prompt) }()
	} else {
		s.route(ctx, prompt)
	}
	wg.Wait()
	msg := prompt
	if s.cfg.Snapshot {
		s.emit(Event{Type: EventSnapshot, Meta: map[string]string{
			"files": fmt.Sprint(snap.Files), "included": fmt.Sprint(snap.Included), "bytes": fmt.Sprint(snap.Bytes),
		}})
		msg = snap.Text + "\n\n" + prompt
	}
	s.history = append(s.history, fantasy.NewUserMessage(msg))
	return s.loop(ctx, prompt, true)
}

// Resume evaluates the seeded conversation's last turn first, then carries on.
func (s *Session) Resume(ctx context.Context, task string) (Outcome, string) {
	s.resetFacts()
	s.route(ctx, task)
	return s.loop(ctx, task, false)
}

func (s *Session) resetFacts() {
	s.edited, s.lastOK, s.lastCmd = map[string]bool{}, false, ""
}

// route asks Jev how hard the request is and sets this run's reasoning effort.
func (s *Session) route(ctx context.Context, request string) {
	s.callOptions = nil
	if s.routingOn() {
		s.applyRoute(s.routeDecision(ctx, request))
	}
}

func (s *Session) routingOn() bool {
	return s.cfg.Route && s.cfg.Jev != nil && s.cfg.EffortOptions != nil
}

func (s *Session) routeDecision(ctx context.Context, request string) checkpoint.RouteDecision {
	return checkpoint.Route(ctx, s.cfg.Jev, checkpoint.RouteState{Request: request}, s.cfg.RoutePolicy, checkpoint.EffortMedium)
}

func (s *Session) applyRoute(d checkpoint.RouteDecision) {
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.callOptions = s.cfg.EffortOptions(d.Effort)
	s.emit(Event{Type: EventRoute, Route: &d})
}

func (s *Session) loop(ctx context.Context, task string, callLLM bool) (Outcome, string) {
	nudges := checkpoint.History{}
	requirements := checkpoint.SplitRequirements(task)
	lastText := lastAssistantText(s.history)
	for {
		if callLLM {
			text, early, err := s.turn(ctx, task, requirements)
			if err != nil {
				if ctx.Err() != nil {
					return s.end(OutcomeCancelled, "cancelled")
				}
				s.emit(Event{Type: EventError, Text: err.Error()})
				return s.end(OutcomeError, err.Error())
			}
			if early {
				return s.finishEarly()
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
		s.addUsage(Usage{JevTokens: d.InputTokens})
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
// early reports that the step-end checkpoint ended the turn because the tool
// results already showed the task done.
func (s *Session) turn(ctx context.Context, task string, requirements []string) (text string, early bool, err error) {
	calls := map[string]fantasy.ToolCallContent{}

	stopWhen := []fantasy.StopCondition{fantasy.StepCountIs(s.cfg.MaxStepsPerTurn)}
	if s.earlyStopOn() {
		stopWhen = append(stopWhen, func(steps []fantasy.StepResult) bool {
			last := steps[len(steps)-1]
			// A step without tool calls ends the turn anyway, and the turn-end
			// checkpoint judges it.
			if len(last.Content.ToolCalls()) == 0 {
				return false
			}
			early = s.stepEnd(ctx, task, requirements, last.Content.Text())
			return early
		})
	}

	var stepStart, firstToken time.Time
	gotToken := func() {
		if firstToken.IsZero() {
			firstToken = time.Now()
		}
	}
	res, err := s.agent.Stream(ctx, fantasy.AgentStreamCall{
		Messages:        s.history,
		ProviderOptions: s.callOptions,
		StopWhen:        stopWhen,
		OnStepStart: func(int) error {
			stepStart, firstToken = time.Now(), time.Time{}
			return nil
		},
		OnReasoningDelta: func(_, _ string) error {
			gotToken()
			return nil
		},
		OnToolInputStart: func(_, _ string) error {
			gotToken()
			return nil
		},
		OnStreamFinish: func(u fantasy.Usage, _ fantasy.FinishReason, _ fantasy.ProviderMetadata) error {
			step := usageOf(u)
			e := Event{Type: EventStep, DurationMS: time.Since(stepStart).Milliseconds(), Usage: &step}
			if !firstToken.IsZero() {
				e.TTFTMS = firstToken.Sub(stepStart).Milliseconds()
			}
			s.emit(e)
			return nil
		},
		OnTextDelta: func(_, text string) error {
			gotToken()
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
			s.noteResult(calls[tr.ToolCallID], text, isErr)
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
		return "", false, err
	}
	for _, st := range res.Steps {
		s.history = append(s.history, st.Messages...)
	}
	u := usageOf(res.TotalUsage)
	s.addUsage(u)
	s.emit(Event{Type: EventTurnEnd, Steps: len(res.Steps), Usage: &u})
	return res.Response.Content.Text(), early, nil
}

// noteResult records the facts the step-end checkpoint needs from a tool
// result: which files changed, and whether the latest command succeeded.
func (s *Session) noteResult(call fantasy.ToolCallContent, text string, isErr bool) {
	switch call.ToolName {
	case "edit", "write":
		s.lastOK = false
		if p := inputField(call.Input, "path"); p != "" && !isErr {
			s.edited[p] = true
		}
	case "bash":
		code, ok := tools.ExitCode(text)
		s.lastOK = ok && code == 0 && !isErr
		s.lastCmd = inputField(call.Input, "command")
	case "apply":
		// An apply that failed may have applied some changes, but its check
		// did not run, so it is never evidence of success.
		s.lastOK = false
		paths, check, ok := tools.ParseApply(call.Input)
		if !ok || isErr {
			return
		}
		for _, p := range paths {
			s.edited[p] = true
		}
		code, ran := tools.ExitCode(text)
		s.lastOK = check != "" && ran && code == 0
		s.lastCmd = check
	default:
		s.lastOK = false
	}
}

// noteRace logs a raced LLM call. The losing copies were cancelled when the
// winner finished, so each had used at most about the winner's tokens. They
// are counted at that bound, with their input as uncached, so reported cost
// never flatters racing.
func (s *Session) noteRace(r race.Result) {
	losers := int64(r.Copies - 1)
	extra := Usage{
		InputTokens:     losers * (r.Usage.InputTokens + r.Usage.CacheReadTokens),
		OutputTokens:    losers * r.Usage.OutputTokens,
		ReasoningTokens: losers * r.Usage.ReasoningTokens,
	}
	s.addUsage(extra)
	s.emit(Event{Type: EventRace, Usage: &extra, DurationMS: r.Took.Milliseconds(), Meta: map[string]string{
		"copies": fmt.Sprint(r.Copies), "winner": fmt.Sprint(r.Winner), "failed": fmt.Sprint(r.Failed),
	}})
}

func (s *Session) earlyStopOn() bool {
	return s.cfg.EarlyStop && s.cfg.Checkpoints && s.cfg.Jev != nil
}

// stepEnd asks Jev whether the tool results so far show the task done. Facts
// come first: only a request that changed files, whose latest tool result is
// a check that exited 0, is worth asking about.
func (s *Session) stepEnd(ctx context.Context, task string, requirements []string, text string) bool {
	if !s.lastOK || len(s.edited) == 0 {
		return false
	}
	d := checkpoint.StepEnd(ctx, s.cfg.Jev, checkpoint.TurnState{
		Task:                 task,
		Requirements:         requirements,
		LastAssistantMessage: clipMiddle(text, 2000),
		RecentSteps:          lastN(s.steps, 12),
	}, s.cfg.StepPolicy)
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.emit(Event{Type: EventDecision, Decision: &d})
	return d.Action == checkpoint.Stop
}

// finishEarly ends a request that the step-end checkpoint found done.
func (s *Session) finishEarly() (Outcome, string) {
	summary := s.earlySummary()
	s.emit(Event{Type: EventAssistantText, Text: summary})
	s.history = append(s.history, fantasy.Message{Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: summary}}})
	return s.end(OutcomeDone, "done_early")
}

// earlySummary is the reply for a run the step-end checkpoint ended. It is
// built from facts, so no LLM step is spent writing it.
func (s *Session) earlySummary() string {
	var b strings.Builder
	b.WriteString("Done. The tool results show the task complete, so Girdle stopped here.\n")
	fmt.Fprintf(&b, "Changed: %s", strings.Join(slices.Sorted(maps.Keys(s.edited)), ", "))
	if s.lastCmd != "" {
		fmt.Fprintf(&b, "\nChecked with: %s (exit code 0)", clipMiddle(oneLine(s.lastCmd), 200))
	}
	return b.String()
}

// inputField reads one string field from a tool call's JSON input.
func inputField(input, name string) string {
	var m map[string]any
	if json.Unmarshal([]byte(input), &m) != nil {
		return ""
	}
	v, _ := m[name].(string)
	return v
}

func (s *Session) end(o Outcome, reason string) (Outcome, string) {
	u := s.Usage()
	s.emit(Event{Type: EventRunEnd, Outcome: o, Reason: reason, Usage: &u})
	return o, reason
}

// emit logs an event and passes it to cfg.Emit. Raced and speculative LLM
// calls report from their own goroutines, so emits are serialised: callers
// of Emit never see two events at once, and the log stays in time order.
func (s *Session) emit(e Event) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
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

func systemPrompt(cfg Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, `You are Girdle, a coding agent working in the repository at %s (%s/%s). Today is %s. Commands available on PATH: %s.

Use the tools to read and change files and to run commands. Work until the task is completely done: make the change, then verify it by building and running the relevant tests, and fix anything that fails.

When you finish, reply briefly with what you changed and the evidence that it works. Only ask the user a question when you need a decision that only they can make.`,
		cfg.Dir, runtime.GOOS, runtime.GOARCH, time.Now().Format("2 January 2006"), availableTools())
	if cfg.Snapshot {
		b.WriteString(`

Each request starts with a repository snapshot: every file, and the full text of each one that fits. It was taken just before the request, so work from it. Don't list the directory or read those files again; only read a file the snapshot left out.`)
	}
	if cfg.Batch {
		b.WriteString(`

Every response you send costs the user several seconds, so finish in as few as you can. Make changes with the apply tool: put every edit and new file the task needs into one apply call, and set its check to a command that proves the whole task is done. That means building the code and running the tests, plus a quick check for any part of the task the tests can't show, such as grep confirming a renamed name is gone everywhere, comments included. apply runs the check straight after the changes, so a single response both changes and verifies the code. If the check fails, send one more apply with the fixes. Keep any text to a sentence.

Writing takes time too, so write as little as the task allows. Change existing files with old_text and new_text edits, each old_text short but unique; use content only for new files or files you are mostly rewriting. Keep new tests compact: one focused test per behaviour.`)
	}
	return b.String()
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
	input := call.Input
	if call.ToolName == "apply" {
		// The input holds whole files; Jev needs only what changed and how
		// it was checked.
		if paths, check, ok := tools.ParseApply(input); ok {
			input = "changes " + strings.Join(paths, ", ") + "; check: " + check
		}
	}
	return fmt.Sprintf("%s %s -> %s", call.ToolName, clipMiddle(oneLine(input), 160), clipEnd(oneLine(result), 700))
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
