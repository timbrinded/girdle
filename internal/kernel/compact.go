package kernel

import (
	"context"
	"fmt"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
)

// Long exploration fills the context with tool output the agent has already
// used: file reads, search results, test logs. Every later step pays for it
// in prompt tokens and time to first token. Every few steps, Jev judges which
// older, larger outputs are still needed, and the rest are replaced by a
// one-line stub for every later step. Stubs are only ever added, so the
// prompt prefix changes rarely and stays cached between compactions.

const prunedStub = "[Girdle removed this output to keep the context small. Look it up again if you need it.]"

// compactPrepare returns a PrepareStep function that stubs pruned tool
// results, and at every Every-th step asks Jev which to prune next.
func (s *Session) compactPrepare(task string) fantasy.PrepareStepFunction {
	return func(ctx context.Context, opts fantasy.PrepareStepFunctionOptions) (context.Context, fantasy.PrepareStepResult, error) {
		p := s.cfg.CompactPolicy
		if opts.StepNumber > 0 && opts.StepNumber%p.Every == 0 {
			s.compact(ctx, task, opts.Messages)
		}
		if len(s.pruned) == 0 {
			return ctx, fantasy.PrepareStepResult{}, nil
		}
		return ctx, fantasy.PrepareStepResult{Messages: stubPruned(opts.Messages, s.pruned)}, nil
	}
}

// compact asks Jev about each tool result that is older than the most recent
// KeepRecent and larger than MinBytes, and prunes those it judges unneeded.
func (s *Session) compact(ctx context.Context, task string, msgs []fantasy.Message) {
	p := s.cfg.CompactPolicy
	type candidate struct {
		id, summary string
	}
	var results []candidate
	calls := map[string]string{}
	for _, m := range msgs {
		for _, part := range m.Content {
			if tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
				calls[tc.ToolCallID] = tc.ToolName + " " + clip.Middle(oneLine(tc.Input), 200)
			}
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
				text, _ := outputText(tr.Output)
				if len(text) >= p.MinBytes && !s.pruned[tr.ToolCallID] {
					results = append(results, candidate{tr.ToolCallID, calls[tr.ToolCallID] + " -> " + clip.Middle(oneLine(text), 600)})
				}
			}
		}
	}
	if len(results) <= p.KeepRecent {
		return
	}
	older := results[:len(results)-p.KeepRecent]
	summaries := make([]string, len(older))
	for i, c := range older {
		summaries[i] = c.summary
	}
	d := checkpoint.Compact(ctx, s.cfg.Jev, checkpoint.CompactState{Task: task, RecentSteps: lastN(s.steps, 6), Results: summaries})
	s.addUsage(Usage{JevTokens: d.InputTokens})
	pruned := 0
	for i, keep := range d.Keep {
		if !keep {
			s.pruned[older[i].id] = true
			pruned++
		}
	}
	s.emit(Event{Type: EventCompact, Compact: &d, Meta: map[string]string{"candidates": fmt.Sprint(len(older)), "pruned": fmt.Sprint(pruned)}})
}

func stubPruned(msgs []fantasy.Message, pruned map[string]bool) []fantasy.Message {
	out := make([]fantasy.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.Role != fantasy.MessageRoleTool {
			continue
		}
		var parts []fantasy.MessagePart
		changed := false
		for _, part := range m.Content {
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && pruned[tr.ToolCallID] {
				tr.Output = fantasy.ToolResultOutputContentText{Text: prunedStub}
				parts = append(parts, tr)
				changed = true
				continue
			}
			parts = append(parts, part)
		}
		if changed {
			out[i].Content = parts
		}
	}
	return out
}
