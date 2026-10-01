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

// configModel calls the girdle tool with input until a tool result is in
// the conversation, then answers.
type configModel struct {
	fantasy.LanguageModel
	input string
}

func (m configModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	for _, msg := range call.Prompt {
		if msg.Role == fantasy.MessageRoleTool {
			return (&answerModel{}).Stream(context.Background(), call)
		}
	}
	return func(yield func(fantasy.StreamPart) bool) {
		if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "call-1", ToolCallName: "girdle", ToolCallInput: m.input}) {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
		}
	}, nil
}

// The girdle tool changes settings, from the next request, only when Jev
// reads the user's request as about Girdle, and never the safety floor.
func TestGirdleConfigure(t *testing.T) {
	const change = `{"effort":"high","turn_on":["heartbeat"]}`
	for _, c := range []struct {
		name   string
		girdle float64 // Jev's P(girdle) for the user's request
		input  string
		want   bool // the change is made
	}{
		{"asked for by the user", 0.9, change, true},
		{"asked for by the model during other work", 0, change, false},
		{"lowering the floor", 0.9, `{"turn_off":["tripwire"]}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var refused bool
			s := NewSession(Config{
				Model: configModel{input: c.input}, ModelName: "config", Dir: t.TempDir(),
				Jev:    fakeJev(t, map[string]float64{"subject_girdle": c.girdle}),
				Policy: checkpoint.DefaultPolicy, Checkpoints: true, Tripwire: true,
				Route: true, AutoEffort: true, Efforts: checkpoint.Efforts, Effort: checkpoint.EffortLow, RoutePolicy: checkpoint.DefaultRoutePolicy,
				EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} },
				Emit: func(e Event) {
					refused = refused || e.Type == EventToolResult && e.Tool == "girdle" && e.IsError
				},
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			s.Run(ctx, "Use high reasoning effort and turn on the heartbeat.")
			if s.cfg.Effort != checkpoint.EffortLow || s.cfg.Heartbeat {
				t.Fatal("a change took effect during the request that made it")
			}
			s.Run(ctx, "Thanks.")
			made := s.cfg.Effort == checkpoint.EffortHigh && !s.cfg.AutoEffort && s.cfg.Heartbeat
			if made != c.want || refused == c.want || !s.cfg.Tripwire {
				t.Fatalf("made = %v, refused = %v, tripwire = %v; want made = %v", made, refused, s.cfg.Tripwire, c.want)
			}
		})
	}
}
