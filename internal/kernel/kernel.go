// Package kernel owns Girdle's loop. The LLM decides and does the work inside
// a turn; at every turn end the kernel asks Jev whether to stop, nudge the
// LLM to continue, or hand control back to the user.
package kernel

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
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

// Session is one conversation in one working directory.
type Session struct {
	ID  string
	cfg Config
	// next holds settings Configure left for the next request.
	next atomic.Pointer[Settings]
	// canAuto is set when Jev can choose request efforts.
	canAuto bool
	model   sessionModel // cfg.Model, raced if cfg.Race asks for it
	tools   []fantasy.AgentTool
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
	// pendingRoute delivers Jev's route when the request didn't wait for
	// it (route).
	pendingRoute chan checkpoint.RouteDecision
	// called is set once a request's first LLM call has started.
	called atomic.Bool
	// cross delivers this request's cross-check, until it has run.
	cross chan *crossCheck
	// halt is why the kernel stopped the LLM's turn, if it did.
	halt halt
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
	// intent delivers what the request wants gone; leftoverNudges counts
	// the times it was found still there.
	intent         chan checkpoint.Intent
	gone           *checkpoint.Intent
	leftoverNudges int
}

// A halt is the kernel stopping the LLM's turn from outside, to say
// something to it or to hand the run to the user.
type halt struct {
	kind   haltKind
	text   string // haltNudge: the message for the LLM
	reason string // for the log, and the run's end reason with haltUser
}

// haltKind orders halts by how serious they are: when two arrive in one
// step, such as a blocked command during a failing cross-check, the more
// serious one wins.
type haltKind int

const (
	noHalt    haltKind = iota
	haltNudge          // send text to the LLM and carry on
	haltUser           // hand the run to the user
)

// stopTurn asks the kernel to end the LLM's turn for h.
func (s *Session) stopTurn(h halt) {
	if h.kind > s.halt.kind {
		s.halt = h
	}
}

// NewSession builds a session with the four built-in tools.
func NewSession(cfg Config) *Session {
	cfg = cfg.resolved()
	s := &Session{ID: uuid.New().String(), cfg: cfg, canAuto: cfg.canAutoEffort(), edited: map[string]bool{}}
	if cfg.Batch {
		s.tools, s.resetTools = tools.BatchedWithReset(cfg.Dir, cfg.Reproduce, s.toolOptions())
	} else {
		s.tools = tools.All(cfg.Dir, s.toolOptions())
	}
	s.useModel()
	meta := map[string]string{
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
		"stepfan":     fmt.Sprint(cfg.StepPolicy.Fanout),
		"reproduce":   fmt.Sprint(cfg.Reproduce),
		"tripwire":    fmt.Sprint(cfg.Tripwire),
		"leftovers":   fmt.Sprint(cfg.Leftovers),
		"grepctx":     fmt.Sprint(cfg.GrepContext),
	}
	maps.Copy(meta, s.settingsMeta())
	s.emit(Event{Type: EventSessionStart, Meta: meta})
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
	s.applySettings()
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
		if s.cfg.Prefetch {
			pick = func(ctx context.Context, request string, files []checkpoint.FileOutline) checkpoint.Prefetch {
				ctx, cancel := context.WithTimeout(ctx, prefetchWait)
				defer cancel()
				return checkpoint.NeedToRead(ctx, s.cfg.Jev, request, files, prefetchWorkers)
			}
		}
		wg.Go(func() {
			snap = TakeSnapshot(ctx, s.cfg.Dir, cmp.Or(s.cfg.SnapshotBudget, DefaultSnapshotBudget), prompt, pick)
		})
	}
	s.route(ctx, prompt)
	wg.Wait()
	msg := prompt
	if s.cfg.Snapshot {
		meta := map[string]string{
			"files": fmt.Sprint(snap.Files), "included": fmt.Sprint(snap.Included), "bytes": fmt.Sprint(snap.Bytes),
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
	if s.cfg.Leftovers {
		intent := make(chan checkpoint.Intent, 1)
		s.intent = intent
		files, names := NamedCode(prompt, func(p string) bool {
			info, err := os.Stat(filepath.Join(s.cfg.Dir, p))
			return err == nil && !info.IsDir()
		})
		go func() { intent <- checkpoint.AskIntent(ctx, s.cfg.Jev, prompt, names, files) }()
	}
	s.cross = nil
	if s.cfg.CrossCheck {
		s.startCrossCheck(ctx)
	}
	return s.loop(ctx, prompt, true)
}

// Resume evaluates the seeded conversation's last turn first, then carries on.
func (s *Session) Resume(ctx context.Context, task string) (Outcome, string) {
	s.applySettings()
	s.resetFacts()
	s.route(ctx, task)
	return s.loop(ctx, task, false)
}

func (s *Session) resetFacts() {
	s.edited, s.lastOK, s.lastCmd = map[string]bool{}, false, ""
	s.testsEdited, s.testsAsked = false, 0
	s.changeLog, s.lastOut = nil, ""
	s.halt = halt{}
	s.intent, s.gone, s.leftoverNudges = nil, nil, 0
	s.requestSteps, s.beats = 0, 0
	s.pruned = map[string]bool{}
	_, s.callOptions = s.cfg.effort(s.cfg.Effort)
	// A route still on its way belongs to the last request.
	s.pendingRoute = nil
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
			h := s.halt
			s.halt = halt{}
			switch {
			case h.kind == haltUser:
				return s.end(OutcomeNeedsUser, h.reason)
			case early:
				return s.finishEarly()
			case h.kind == haltNudge:
				s.nudge(h.text, h.reason)
				continue
			}
			lastText = text
			// A turn that ends by itself is checked for leftovers too.
			if len(s.edited) > 0 {
				if fb := s.leftovers(ctx); fb != "" {
					s.nudge(fb, "leftovers")
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
			LastAssistantMessage: clip.Middle(lastText, 2000),
			RecentSteps:          lastN(s.steps, 8),
		}, s.cfg.Policy, nudges)
		s.addUsage(Usage{JevTokens: d.InputTokens})
		s.emit(Event{Type: EventDecision, Decision: &d})

		switch d.Action {
		case checkpoint.Stop:
			return s.end(OutcomeDone, d.Rule)
		case checkpoint.Nudge:
			nudges[d.Rule]++
			s.nudge(d.Nudge, d.Rule)
		default:
			return s.end(OutcomeNeedsUser, d.Rule)
		}
	}
}

// nudge sends the LLM a message from the kernel before its next turn.
func (s *Session) nudge(text, reason string) {
	s.emit(Event{Type: EventNudge, Text: text, Reason: reason})
	s.history = append(s.history, fantasy.NewUserMessage(text))
}

// turn runs the LLM until it stops calling tools, and returns its final text.
// early reports that the step-end checkpoint ended the turn because the tool
// results already showed the task done.
func (s *Session) turn(ctx context.Context, task string, requirements []string) (text string, early bool, err error) {
	calls := map[string]fantasy.ToolCallContent{}

	stopWhen := []fantasy.StopCondition{
		fantasy.StepCountIs(s.cfg.MaxStepsPerTurn),
		// A halt, such as a blocked command, ends the turn at once.
		func([]fantasy.StepResult) bool { return s.halt.kind != noHalt },
	}
	if s.cfg.Heartbeat {
		stopWhen = append(stopWhen, func(steps []fantasy.StepResult) bool {
			if len(steps[len(steps)-1].Content.ToolCalls()) == 0 {
				return false
			}
			return s.heartbeat(ctx, task, requirements)
		})
	}
	if s.cfg.EarlyStop {
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
					s.stopTurn(halt{kind: haltNudge, text: fb, reason: "crosscheck"})
					return true
				}
			}
			if s.lastOK && len(s.edited) > 0 {
				if fb := s.leftovers(ctx); fb != "" {
					s.stopTurn(halt{kind: haltNudge, text: fb, reason: "leftovers"})
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
	if s.cfg.Compact {
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
			text, isErr := outputText(tr.Result)
			s.emit(Event{Type: EventToolResult, Tool: tr.ToolName, CallID: tr.ToolCallID, Text: clip.Middle(text, 4000), IsError: isErr})
			s.steps = append(s.steps, summarizeStep(calls[tr.ToolCallID], text))
			s.noteResult(calls[tr.ToolCallID], text, isErr)
			return nil
		},
		OnStepFinish: func(sr fantasy.StepResult) error {
			// Muse Spark's reasoning is encrypted; what comes back is a short
			// summary. It is logged for reading traces. Sending it to Jev
			// changed no decision (decision 0011).
			if r := strings.TrimSpace(sr.Content.ReasoningText()); r != "" {
				s.emit(Event{Type: EventReasoning, Text: clip.Middle(r, 2000)})
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

func (s *Session) end(o Outcome, reason string) (Outcome, string) {
	// Log a route the request didn't wait for, if it came.
	s.takeRoute(context.Background(), false)
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

func jevModel(c *jev.Client) string {
	if c == nil {
		return ""
	}
	return c.Model
}

const (
	// prefetchWait bounds the prefetch, which holds up the first LLM call,
	// and prefetchWorkers is how many of its Jev requests run at once.
	prefetchWait    = 4 * time.Second
	prefetchWorkers = 32
)
