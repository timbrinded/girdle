package kernel

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// answerModel replies to every call with a short answer and no tool calls,
// and records the user messages each call was sent.
type answerModel struct {
	fantasy.LanguageModel
	mu    sync.Mutex
	calls []string
}

func (m *answerModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	var b strings.Builder
	for _, msg := range call.Prompt {
		if msg.Role != fantasy.MessageRoleUser {
			continue
		}
		for _, p := range msg.Content {
			if t, ok := fantasy.AsMessagePart[fantasy.TextPart](p); ok {
				b.WriteString(t.Text + "\n")
			}
		}
	}
	m.mu.Lock()
	m.calls = append(m.calls, b.String())
	m.mu.Unlock()
	return func(yield func(fantasy.StreamPart) bool) {
		for _, p := range []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "Press Shift+Tab."},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		} {
			if !yield(p) {
				return
			}
		}
	}, nil
}

// A question about Girdle is answered from the girdle tool's output, sent
// with the request when the route is back in time and at the turn end
// otherwise, and its answer stops without test evidence.
func TestGirdleQuestion(t *testing.T) {
	for _, c := range []struct {
		name string
		// speculate starts the first call before the slow route arrives.
		speculate bool
		// wantCalls is how many LLM calls the request takes.
		wantCalls int
	}{
		{"routed before the request", false, 1},
		{"routed during the first call", true, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			model := &answerModel{}
			var nudges []string
			s := NewSession(Config{
				Model: model, ModelName: "answer", Dir: t.TempDir(),
				Jev:    fakeJev(t, map[string]float64{"subject_girdle": 0.9, "evidence": 0.1}),
				Policy: checkpoint.DefaultPolicy, Checkpoints: true,
				Route: true, AutoEffort: true, Efforts: checkpoint.Efforts, RoutePolicy: checkpoint.DefaultRoutePolicy,
				EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} },
				Speculate:     c.speculate,
				Emit: func(e Event) {
					if e.Type == EventNudge {
						nudges = append(nudges, e.Reason)
					}
				},
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			outcome, reason := s.Run(ctx, "How do I change the reasoning effort?")
			if outcome != OutcomeDone || reason != "answered" {
				t.Fatalf("Run = %s, %s; nudges %v", outcome, reason, nudges)
			}
			if len(model.calls) != c.wantCalls {
				t.Fatalf("%d LLM calls, want %d; nudges %v", len(model.calls), c.wantCalls, nudges)
			}
			last := model.calls[len(model.calls)-1]
			if !strings.Contains(last, "Model: answer") || !strings.Contains(last, "=== guide usage") {
				t.Fatalf("last call wasn't sent the girdle tool's answer:\n%s", last)
			}
			if strings.Count(last, "=== this session") != 1 {
				t.Fatal("the girdle tool's answer was sent more than once")
			}
		})
	}
}
