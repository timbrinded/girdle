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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	// Prefetch asks Jev, for a repository too large to snapshot whole,
	// which other files the request needs, and adds them. It needs Snapshot.
	Prefetch bool
	// Structure adds, for a repository too large to snapshot whole, the
	// code around the named code, found by parsing with ast-grep. It needs
	// Snapshot.
	Structure bool
	// Batch asks the LLM to make all its edits and run the checks in one
	// step, since every extra step costs seconds of latency.
	Batch bool
	// EarlyStop asks Jev, after any step whose last command succeeded,
	// whether the task is already done, and ends the run there if so.
	// It needs Checkpoints.
	EarlyStop  bool
	StepPolicy checkpoint.StepPolicy
	// Race sends each LLM call this many times and keeps the first complete
	// answer. Below 2, calls are not raced. The first call of a request
	// starts every copy at once; later calls start an extra copy only after
	// waiting Hedge for an answer, so quick steps cost one call. A zero
	// Hedge starts every copy at once for every call.
	Race  int
	Hedge time.Duration
	// Speculate starts a request's first LLM call on the usual effort while
	// Jev routes it, instead of waiting for the route. It needs Route.
	Speculate bool
	// CrossCheck writes an independent test of each request in the
	// background and runs it once the agent's own check passes. It needs
	// Batch and EarlyStop.
	CrossCheck bool
	// Heartbeat asks Jev every few steps whether the turn is looping or
	// drifting, and nudges it if so. It needs Checkpoints.
	Heartbeat       bool
	HeartbeatPolicy checkpoint.HeartbeatPolicy
	// Compact prunes older tool output that Jev judges no longer needed,
	// every CompactPolicy.Every steps. It needs Checkpoints.
	Compact       bool
	CompactPolicy checkpoint.CompactPolicy
	// Reproduce lets apply check that a bug fix's regression test fails
	// without the fix. It needs Batch.
	Reproduce bool
	// Leftovers asks Jev which names and files the request wants gone, and
	// before the run stops, checks that none of them remain.
	Leftovers bool
	// Tripwire checks every shell command before it runs and blocks the
	// catastrophic ones, handing the run back to the user.
	Tripwire       bool
	TripwirePolicy checkpoint.TripwirePolicy
}

// Session is one conversation in one working directory.
type Session struct {
	ID    string
	cfg   Config
	model sessionModel // cfg.Model, raced if cfg.Race asks for it
	tools []fantasy.AgentTool
	// resetTools tells the tools a new request has started.
	resetTools func()
	// pruned holds the tool calls whose output compaction replaced.
	pruned  map[string]bool
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
	// called is set once a request's first LLM call has started.
	called atomic.Bool
	// cross delivers this request's cross-check, until it has run.
	cross chan *crossCheck
	// pending is a message waiting to go to the LLM once the kernel has
	// stopped its turn: a failed cross-check or a heartbeat nudge.
	pending, pendingWhy string
	// steps and heartbeat nudges in this request.
	requestSteps, beats int
	// task is the current request, as the user wrote it.
	task string

	// Facts about the current request, for the step-end checkpoint.
	edited  map[string]bool // files changed by a successful edit or apply
	lastOK  bool            // the latest tool result was a check that exited 0
	lastCmd string          // that check
	// testsEdited is set once a change in this request touched a test file.
	testsEdited bool
	// testsAsked is Jev's noul for "the request asks for tests".
	testsAsked float64
	// changeLog holds this request's changes, and lastOut the latest
	// check's output, for the step-end fan-out.
	changeLog []string
	lastOut   string
	// tripped is why the tripwire blocked a command in this request.
	tripped string
	// intent delivers what the request wants gone; leftoverNudges counts
	// the times it was found still there.
	intent         chan checkpoint.Intent
	gone           *checkpoint.Intent
	leftoverNudges int
}

// NewSession builds a session with the four built-in tools.
func NewSession(cfg Config) *Session {
	if cfg.MaxStepsPerTurn == 0 {
		cfg.MaxStepsPerTurn = 60
	}
	s := &Session{ID: uuid.New().String(), cfg: cfg, edited: map[string]bool{}}
	s.tools = tools.All(cfg.Dir, s.shellGuard())
	if cfg.Batch {
		s.tools, s.resetTools = tools.BatchedWithReset(cfg.Dir, cfg.Reproduce, s.shellGuard())
	}
	s.model = sessionModel{LanguageModel: race.New(cfg.Model, cfg.Race, s.stagger, s.noteRace), s: s}
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
		"crosscheck":  fmt.Sprint(cfg.CrossCheck),
		"heartbeat":   fmt.Sprint(cfg.Heartbeat),
		"compact":     fmt.Sprint(cfg.Compact),
		"prefetch":    fmt.Sprint(cfg.Prefetch),
		"structure":   fmt.Sprint(cfg.Structure),
		"stepfan":     fmt.Sprint(cfg.StepPolicy.Fanout),
		"reproduce":   fmt.Sprint(cfg.Reproduce),
		"tripwire":    fmt.Sprint(cfg.Tripwire),
		"leftovers":   fmt.Sprint(cfg.Leftovers),
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
	s.task = prompt
	s.resetFacts()
	if s.resetTools != nil {
		s.resetTools()
	}
	s.called.Store(false)
	// Routing waits on Jev and the snapshot on the disk: do both at once.
	var snap Snapshot
	var wg sync.WaitGroup
	if s.cfg.Snapshot {
		var pick Picker
		if s.cfg.Prefetch && s.cfg.Jev != nil {
			pick = func(ctx context.Context, request string, files []checkpoint.FileOutline) checkpoint.Prefetch {
				ctx, cancel := context.WithTimeout(ctx, prefetchWait)
				defer cancel()
				return checkpoint.NeedToRead(ctx, s.cfg.Jev, request, files, prefetchWorkers)
			}
		}
		wg.Go(func() {
			snap = TakeSnapshotWith(ctx, s.cfg.Dir, cmp.Or(s.cfg.SnapshotBudget, DefaultSnapshotBudget), prompt,
				SnapshotOptions{Pick: pick, Related: s.cfg.Structure})
		})
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
		meta := map[string]string{
			"files": fmt.Sprint(snap.Files), "included": fmt.Sprint(snap.Included), "bytes": fmt.Sprint(snap.Bytes),
		}
		if len(snap.Related) > 0 {
			meta["related"] = strings.Join(snap.Related, ",")
		}
		if snap.Candidates > 0 {
			s.addUsage(Usage{JevTokens: snap.Prefetch.InputTokens})
			meta["prefetched"] = strings.Join(snap.Prefetched, ",")
			meta["candidates"] = fmt.Sprint(snap.Candidates)
			meta["scored"] = fmt.Sprint(len(snap.Prefetch.Scores))
			meta["prefetch_ms"] = fmt.Sprint(snap.Prefetch.LatencyMS)
		}
		s.emit(Event{Type: EventSnapshot, Meta: meta})
		msg = snap.Text + "\n\n" + prompt
	}
	s.history = append(s.history, fantasy.NewUserMessage(msg))
	if s.cfg.Leftovers && s.cfg.Jev != nil {
		intent := make(chan checkpoint.Intent, 1)
		s.intent = intent
		files, names := NamedCode(prompt, func(p string) bool {
			info, err := os.Stat(filepath.Join(s.cfg.Dir, p))
			return err == nil && !info.IsDir()
		})
		go func() { intent <- checkpoint.AskIntent(ctx, s.cfg.Jev, prompt, names, files) }()
	}
	s.cross = nil
	if s.crossCheckOn() {
		s.startCrossCheck(ctx)
	}
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
	s.testsEdited, s.testsAsked = false, 0
	s.changeLog, s.lastOut = nil, ""
	s.tripped = ""
	s.intent, s.gone, s.leftoverNudges = nil, nil, 0
	s.requestSteps, s.beats = 0, 0
	s.pruned = map[string]bool{}
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
	s.testsAsked = d.Tests
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
			if why := s.tripped; why != "" {
				s.tripped = ""
				return s.end(OutcomeNeedsUser, "tripwire: "+why)
			}
			if early {
				return s.finishEarly()
			}
			if msg := s.pending; msg != "" {
				// The kernel stopped the turn to say something: a failed
				// cross-check, or a heartbeat nudge.
				s.pending = ""
				s.emit(Event{Type: EventNudge, Text: msg, Reason: s.pendingWhy})
				s.history = append(s.history, fantasy.NewUserMessage(msg))
				continue
			}
			if s.pendingWhy == "blocked" {
				s.pendingWhy = ""
				return s.end(OutcomeNeedsUser, "blocked")
			}
			lastText = text
			// A turn that ends by itself is checked for leftovers too.
			if len(s.edited) > 0 {
				if fb := s.leftovers(ctx); fb != "" {
					s.emit(Event{Type: EventNudge, Text: fb, Reason: "leftovers"})
					s.history = append(s.history, fantasy.NewUserMessage(fb))
					continue
				}
			}
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

	stopWhen := []fantasy.StopCondition{
		fantasy.StepCountIs(s.cfg.MaxStepsPerTurn),
		// A blocked command hands the run back to the user at once.
		func([]fantasy.StepResult) bool { return s.tripped != "" },
	}
	if s.cfg.Heartbeat && s.cfg.Checkpoints && s.cfg.Jev != nil {
		stopWhen = append(stopWhen, func(steps []fantasy.StepResult) bool {
			if len(steps[len(steps)-1].Content.ToolCalls()) == 0 {
				return false
			}
			return s.heartbeat(ctx, task, requirements)
		})
	}
	if s.earlyStopOn() {
		stopWhen = append(stopWhen, func(steps []fantasy.StepResult) bool {
			last := steps[len(steps)-1]
			// A step without tool calls ends the turn anyway, and the turn-end
			// checkpoint judges it.
			if len(last.Content.ToolCalls()) == 0 {
				return false
			}
			// Before asking Jev, run the independent cross-check once, if
			// the agent's own check just passed.
			if s.cross != nil && s.lastOK && len(s.edited) > 0 {
				if fb := s.runCrossCheck(ctx); fb != "" {
					s.pending, s.pendingWhy = fb, "crosscheck"
					return true
				}
			}
			if s.lastOK && len(s.edited) > 0 {
				if fb := s.leftovers(ctx); fb != "" {
					s.pending, s.pendingWhy = fb, "leftovers"
					return true
				}
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
	var prepare fantasy.PrepareStepFunction
	if s.cfg.Compact && s.cfg.Checkpoints && s.cfg.Jev != nil {
		prepare = s.compactPrepare(task)
	}
	res, err := s.agent.Stream(ctx, fantasy.AgentStreamCall{
		Messages:        s.history,
		ProviderOptions: s.callOptions,
		StopWhen:        stopWhen,
		PrepareStep:     prepare,
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
			s.addUsage(step)
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
			// Muse Spark's reasoning is encrypted; what comes back is a short
			// summary. It is logged for reading traces. Sending it to Jev
			// changed no decision (decision 0011).
			if r := strings.TrimSpace(sr.Content.ReasoningText()); r != "" {
				s.emit(Event{Type: EventReasoning, Text: clipMiddle(r, 2000)})
			}
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
	// Each call's tokens were counted as it finished; this is the turn's
	// total, for the log.
	u := usageOf(res.TotalUsage)
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
			s.testsEdited = s.testsEdited || tools.IsTestFile(p)
		}
	case "bash":
		// With apply, checks go through apply: a grep that exits 0 is not
		// evidence the task is done.
		code, ok := tools.ExitCode(text)
		s.lastOK = !s.cfg.Batch && ok && code == 0 && !isErr
		s.lastCmd = inputField(call.Input, "command")
		s.lastOut = text
	case "apply":
		// An apply that failed may have applied some changes, but its check
		// did not run, so it is never evidence of success.
		s.lastOK = false
		in, ok := tools.ParseApplyInput(call.Input)
		if !ok || isErr {
			return
		}
		paths, check := in.Paths(), in.Check
		for _, c := range in.Changes {
			s.changeLog = append(s.changeLog, renderChange(c))
		}
		if check != "" {
			s.lastOut = text
		}
		for _, p := range paths {
			s.edited[p] = true
			s.testsEdited = s.testsEdited || tools.IsTestFile(p)
		}
		code, ran := tools.ExitCode(text)
		s.lastOK = check != "" && ran && code == 0
		s.lastCmd = check
	default:
		s.lastOK = false
	}
}

// stagger is how long a raced call waits before starting each extra copy:
// nothing for a request's first call, which usually does the most work,
// and cfg.Hedge after that.
func (s *Session) stagger() time.Duration {
	if !s.called.Swap(true) {
		return 0
	}
	return s.cfg.Hedge
}

// noteRace logs a raced LLM call and counts the losing copies' tokens. Each
// loser is priced like the winner: the same prompt, cached the same way, and
// the whole answer, though it was cancelled partway through. That is still an
// upper bound. On the scale suite it came to $0.0072 a run against $0.0055
// actually billed, where treating the losers' prompts as uncached had come
// to $0.0142.
func (s *Session) noteRace(r race.Result) {
	losers := int64(r.Copies - 1)
	extra := Usage{
		InputTokens:     losers * r.Usage.InputTokens,
		CacheReadTokens: losers * r.Usage.CacheReadTokens,
		OutputTokens:    losers * r.Usage.OutputTokens,
		ReasoningTokens: losers * r.Usage.ReasoningTokens,
	}
	s.addUsage(extra)
	s.emit(Event{Type: EventRace, Usage: &extra, DurationMS: r.Took.Milliseconds(), Meta: map[string]string{
		"copies": fmt.Sprint(r.Copies), "winner": fmt.Sprint(r.Winner), "failed": fmt.Sprint(r.Failed),
	}})
}

// heartbeat asks Jev every few steps of a request whether the work is
// looping, drifting or blocked, and stops the turn to act on it.
func (s *Session) heartbeat(ctx context.Context, task string, requirements []string) bool {
	s.requestSteps++
	p := s.cfg.HeartbeatPolicy
	if p.Every <= 0 || s.requestSteps%p.Every != 0 || s.beats >= p.MaxNudges {
		return false
	}
	d := checkpoint.Heartbeat(ctx, s.cfg.Jev, checkpoint.TurnState{
		Task: task, Requirements: requirements, RecentSteps: lastN(s.steps, p.Every+2),
	}, p)
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.emit(Event{Type: EventHeartbeat, Heartbeat: &d})
	switch d.Action {
	case checkpoint.Nudge:
		s.beats++
		s.pending, s.pendingWhy = d.Nudge, d.Rule
		return true
	case checkpoint.Ask:
		s.pendingWhy = "blocked"
		return true
	}
	return false
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
	// A fact the threshold can't see: the request asks for tests and none
	// have been written yet. Jev's coverage answers miss this.
	if s.testsAsked >= 0.5 && !s.testsEdited {
		return false
	}
	state := checkpoint.TurnState{
		Task:                 task,
		Requirements:         requirements,
		LastAssistantMessage: clipMiddle(text, 2000),
		RecentSteps:          lastN(s.steps, 12),
	}
	if s.cfg.StepPolicy.Fanout {
		state.Changes = lastN(s.changeLog, 12)
		state.FilesChanged = slices.Sorted(maps.Keys(s.edited))
		for _, p := range state.FilesChanged {
			if tools.IsTestFile(p) {
				state.TestsChanged = append(state.TestsChanged, p)
			}
		}
		state.Check, state.CheckOutput = s.lastCmd, clipMiddle(s.lastOut, 3500)
	}
	d := checkpoint.StepEnd(ctx, s.cfg.Jev, state, s.cfg.StepPolicy)
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

Each request starts with a repository snapshot, taken just before the request. For a small repository it holds the full text of every file: work from it, and don't read those files again. For a large one it lists every file, includes the repository's instructions for agents, and already shows the code the request names: its files or definitions, its uses and the tests beside it. Don't look those up again.`)
	}
	if cfg.Batch {
		b.WriteString(`

Every response you send costs the user several seconds, so finish in as few as you can. Work in at most three moves: gather, change, and only if needed fix.
- Gather: if the snapshot isn't enough, make one lookup call with every file, line range, definition and search you will need. Ask generously rather than coming back for more.
- Change: make one apply call with every edit and new file the task needs, and set its check to a command that proves the whole task is done. That means building the code and running the tests, plus a quick check for any part of the task the tests can't show, such as grep confirming a renamed name is gone everywhere, comments included. When the task reports a bug, put a test that reproduces it in the same apply as the fix, so the check shows it fixed.`)
		if cfg.Reproduce {
			b.WriteString(` Set reproduce to a command that runs only that test: Girdle runs it once without your fix to show it fails there.`)
		}
		b.WriteString(` Before you write, work out the edge cases the task's words imply, such as empty input, a one-pass iterator wherever it says iterable, equal items, and the smallest and largest sizes, and make the code handle them and the tests cover them: one attempt has to be right. apply runs the check straight after the changes, so one response both changes and verifies the code.
- Fix: if the check fails, send one more apply with the fixes. Start its check with a quick run of just what failed, joined to the full proof with &&, so a repeat failure shows in seconds.
The check must fail when anything is wrong, so never hide its exit code with "; echo" or "|| true". Keep any text to a sentence.

Writing takes time too, so write as little as the task allows. Change existing files with old_text and new_text edits, each old_text short but unique; use content only for new files or files you are mostly rewriting. Changes apply in order, so never let two changes touch the same lines: merge them into one. Keep new tests compact: one focused test per behaviour.`)
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

const (
	// prefetchWait bounds the prefetch, which holds up the first LLM call,
	// and prefetchWorkers is how many of its Jev requests run at once.
	prefetchWait    = 4 * time.Second
	prefetchWorkers = 32
)

// renderChange shows one change the way a diff would, clipped.
func renderChange(c tools.Change) string {
	if c.OldText != "" {
		return "edit " + c.Path + "\n- " + strings.ReplaceAll(clipMiddle(c.OldText, 600), "\n", "\n- ") +
			"\n+ " + strings.ReplaceAll(clipMiddle(c.NewText, 1500), "\n", "\n+ ")
	}
	return "write " + c.Path + "\n" + clipMiddle(c.Content, 2500)
}

// shellGuard returns the tripwire as the tools' guard, or nil when it's off.
func (s *Session) shellGuard() tools.Guard {
	if !s.cfg.Tripwire {
		return nil
	}
	return s.guard
}

// guard is the tripwire: it reads a command's facts, blocks what the hard
// floor forbids, runs at once what shows no deleting, pushing or sending,
// and asks Jev about the rest. A block ends the turn and hands the run to
// the user (decision 0014).
func (s *Session) guard(ctx context.Context, command string) string {
	home, _ := os.UserHomeDir()
	state := checkpoint.TripwireState{Command: clipMiddle(command, 4000), ProjectDir: s.cfg.Dir, HomeDir: home, Task: clipMiddle(s.task, 2000)}
	f, err := tools.ReadShell(ctx, command, s.cfg.Dir, home)
	if err == nil {
		state.Facts = f
		if why := f.Floor(); why != "" {
			d := checkpoint.TripwireDecision{Action: "block", Rule: "floor", Why: why, State: state}
			s.emit(Event{Type: EventTripwire, Tripwire: &d})
			s.tripped = why
			return why
		}
		if !f.NeedsJudgement() {
			return ""
		}
	}
	// Without the parse (no ast-grep), every command is judged.
	d := checkpoint.Tripwire(ctx, s.cfg.Jev, state, s.cfg.TripwirePolicy)
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.emit(Event{Type: EventTripwire, Tripwire: &d})
	if d.Action == "block" {
		s.tripped = d.Why
		return d.Why
	}
	return ""
}

// maxLeftoverNudges bounds how often one request is sent back for
// leftovers, so a disagreement can't loop.
const maxLeftoverNudges = 2

// leftovers returns a nudge when a name the request wants gone still
// appears where it must change, or a file it wants pruned still defines
// functions nothing uses. It returns "" when there's nothing to say.
func (s *Session) leftovers(ctx context.Context) string {
	if s.intent == nil || s.leftoverNudges >= maxLeftoverNudges {
		return ""
	}
	if s.gone == nil {
		select {
		case in := <-s.intent:
			s.gone = &in
			s.addUsage(Usage{JevTokens: in.Tokens})
			s.emit(Event{Type: EventLeftovers, Reason: "intent", Meta: map[string]string{
				"gone": strings.Join(in.Gone, ","), "prune": strings.Join(in.Prune, ","), "latency_ms": fmt.Sprint(in.LatencyMS),
			}})
		case <-ctx.Done():
			return ""
		}
	}
	var b strings.Builder
	var found []string
	for _, n := range s.gone.Gone {
		mentions := tools.Mentions(ctx, s.cfg.Dir, n, 20)
		if len(mentions) == 0 {
			continue
		}
		must, tokens := checkpoint.MustChange(ctx, s.cfg.Jev, s.task, n, mentions)
		s.addUsage(Usage{JevTokens: tokens})
		if len(must) > 0 {
			fmt.Fprintf(&b, "`%s` still appears where the task wants it gone:\n%s\n", n, strings.Join(must, "\n"))
			found = append(found, n)
		}
	}
	for _, f := range s.gone.Prune {
		if unused := tools.UnusedDefs(ctx, s.cfg.Dir, f); len(unused) > 0 {
			fmt.Fprintf(&b, "%s still defines functions that no code uses: %s\n", f, strings.Join(unused, ", "))
			found = append(found, f)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	s.leftoverNudges++
	s.emit(Event{Type: EventLeftovers, Reason: "found", Meta: map[string]string{"found": strings.Join(found, ",")}})
	return "[Girdle] Not done yet.\n" + b.String() + "Change these, then run the check again."
}
