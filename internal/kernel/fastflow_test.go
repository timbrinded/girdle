package kernel

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
)

// scriptedModel answers a request with no tool results yet with one apply
// call, and any later request with a short reply.
type scriptedModel struct{ fantasy.LanguageModel }

func (scriptedModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	afterTools := false
	for _, m := range call.Prompt {
		afterTools = afterTools || m.Role == fantasy.MessageRoleTool
	}
	return func(yield func(fantasy.StreamPart) bool) {
		if !afterTools {
			if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "call-1", ToolCallName: "apply",
				ToolCallInput: `{"changes":[{"path":"a.txt","old_text":"old","new_text":"new"}],"check":"grep -q new a.txt"}`}) {
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
			}
			return
		}
		for _, p := range []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "Done."},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		} {
			if !yield(p) {
				return
			}
		}
	}, nil
}

// fakeJev answers every question favourably. Routing is slow, so the first
// LLM call is always under way before the route arrives.
func fakeJev(t *testing.T) *jev.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Questions map[string]any `json:"questions"`
		}
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		answers := map[string]jev.Answer{}
		for k := range req.Questions {
			switch k {
			case "complexity":
				time.Sleep(100 * time.Millisecond)
				answers[k] = jev.Answer{Type: "score", Score: 1.0}
			case "status":
				answers[k] = jev.Answer{Type: "choice", Choice: "done", Confidence: 0.95}
			case "needless_ask":
				answers[k] = jev.Answer{Type: "noul", Noul: 0}
			default:
				answers[k] = jev.Answer{Type: "noul", Noul: 0.95}
			}
		}
		_ = json.MarshalWrite(w, jev.Response{Model: "jev-test", Answers: answers})
	}))
	t.Cleanup(srv.Close)
	return &jev.Client{BaseURL: srv.URL, Model: "jev-test", APIKey: "test", HTTP: srv.Client()}
}

func TestFastFlowEndToEnd(t *testing.T) {
	for _, race := range []int{1, 3} {
		t.Run(map[int]string{1: "unraced", 3: "raced"}[race], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var events []Event
			s := NewSession(Config{
				Model: scriptedModel{}, ModelName: "scripted", Jev: fakeJev(t), Dir: dir,
				Policy: checkpoint.DefaultPolicy, Checkpoints: true,
				Route: true, RoutePolicy: checkpoint.DefaultRoutePolicy,
				EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} },
				Snapshot:      true, Batch: true, EarlyStop: true, StepPolicy: checkpoint.DefaultStepPolicy,
				Race: race, Speculate: true,
				Emit: func(e Event) { events = append(events, e) },
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			outcome, reason := s.Run(ctx, "Replace the word old with new in a.txt.")
			if outcome != OutcomeDone || reason != "done_early" {
				t.Fatalf("Run = %s, %s", outcome, reason)
			}
			data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
			if strings.TrimSpace(string(data)) != "new" {
				t.Fatalf("a.txt = %q", data)
			}
			var routed bool
			for _, e := range events {
				routed = routed || e.Type == EventRoute
			}
			if !routed {
				t.Fatal("no route event")
			}
		})
	}
}
