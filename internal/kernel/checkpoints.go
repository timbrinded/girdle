package kernel

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
	"github.com/timbrinded/girdle/internal/tools"
)

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
		s.stopTurn(halt{kind: haltNudge, text: d.Nudge, reason: d.Rule})
		return true
	case checkpoint.Ask:
		s.stopTurn(halt{kind: haltUser, reason: "blocked"})
		return true
	}
	return false
}

// stepEnd asks Jev whether the tool results so far show the task done. Facts
// come first: only a request that changed files, whose latest tool result is
// a check that exited 0, is worth asking about.
func (s *Session) stepEnd(ctx context.Context, task string, requirements []string, text string) bool {
	if !s.lastOK || len(s.edited) == 0 {
		return false
	}
	// A pinned effort's route says whether the request asks for tests.
	// Waiting for it here costs no more than the Jev request that follows.
	s.takeRoute(ctx, true)
	// A fact the threshold can't see: the request asks for tests and none
	// have been written yet. Jev's coverage answers miss this.
	if s.testsAsked >= 0.5 && !s.testsEdited {
		return false
	}
	state := checkpoint.TurnState{
		Task:                 task,
		Requirements:         requirements,
		LastAssistantMessage: clip.Middle(text, 2000),
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
		state.Check, state.CheckOutput = s.lastCmd, clip.Middle(s.lastOut, 3500)
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
		fmt.Fprintf(&b, "\nChecked with: %s (exit code 0)", clip.Middle(oneLine(s.lastCmd), 200))
	}
	return b.String()
}
