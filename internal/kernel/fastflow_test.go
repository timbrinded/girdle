package kernel

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// crossModel is scriptedModel plus a cross-check writer: asked for a
// cross-check, it writes a test file whose check passes or fails.
type crossModel struct {
	fantasy.LanguageModel
	crossPasses bool
}

func (m crossModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	last := call.Prompt[len(call.Prompt)-1]
	isCross := false
	for _, p := range last.Content {
		if t, ok := fantasy.AsMessagePart[fantasy.TextPart](p); ok && strings.Contains(t.Text, "Check its work independently") {
			isCross = true
		}
	}
	if !isCross {
		return scriptedModel{}.Stream(ctx, call)
	}
	check := "grep -q new a.txt && test -f girdle_crosscheck_test.txt"
	if !m.crossPasses {
		check = "grep -q brand-new a.txt"
	}
	input, _ := json.Marshal(map[string]any{
		"changes": []map[string]string{{"path": "girdle_crosscheck_test.txt", "content": "cross"}},
		"check":   check,
	})
	return func(yield func(fantasy.StreamPart) bool) {
		if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "x-1", ToolCallName: "apply", ToolCallInput: string(input)}) {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
		}
	}, nil
}

func TestCrossCheck(t *testing.T) {
	for _, passes := range []bool{true, false} {
		t.Run(map[bool]string{true: "passes", false: "fails"}[passes], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var events []Event
			s := NewSession(Config{
				Model: crossModel{crossPasses: passes}, ModelName: "scripted", Jev: fakeJev(t), Dir: dir,
				Policy: checkpoint.DefaultPolicy, Checkpoints: true,
				Route: true, RoutePolicy: checkpoint.DefaultRoutePolicy,
				EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} },
				Snapshot:      true, Batch: true, EarlyStop: true, StepPolicy: checkpoint.DefaultStepPolicy,
				Race: 1, Speculate: true, CrossCheck: true,
				Emit: func(e Event) { mu.Lock(); events = append(events, e); mu.Unlock() },
			})
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			outcome, reason := s.Run(ctx, "Replace the word old with new in a.txt.")

			var cross, nudge []Event
			for _, e := range events {
				switch {
				case e.Type == EventCrossCheck:
					cross = append(cross, e)
				case e.Type == EventNudge && e.Reason == "crosscheck":
					nudge = append(nudge, e)
				}
			}
			if len(cross) != 1 {
				t.Fatalf("%d cross-check events", len(cross))
			}
			if _, err := os.Stat(filepath.Join(dir, "girdle_crosscheck_test.txt")); !os.IsNotExist(err) {
				t.Fatal("the cross-check file was left behind")
			}
			if passes {
				if cross[0].Reason != "passed" || len(nudge) != 0 || outcome != OutcomeDone || reason != "done_early" {
					t.Fatalf("cross %q, %d nudges, Run = %s %s", cross[0].Reason, len(nudge), outcome, reason)
				}
				return
			}
			if cross[0].Reason != "failed" || len(nudge) != 1 || !strings.Contains(nudge[0].Text, "brand-new") {
				t.Fatalf("cross %q, nudges %v", cross[0].Reason, nudge)
			}
			// The scripted model answers the feedback with a reply; the
			// turn-end checkpoint then finishes the run.
			if outcome != OutcomeDone {
				t.Fatalf("Run = %s %s", outcome, reason)
			}
		})
	}
}
