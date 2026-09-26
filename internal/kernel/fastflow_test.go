package kernel

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

// fakeJev answers every question favourably, except as overrides say.
// Routing is slow, so the first LLM call is always under way before the
// route arrives.
func fakeJev(t *testing.T, overrides ...map[string]float64) *jev.Client {
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
			if len(overrides) > 0 {
				if v, ok := overrides[0][k]; ok {
					answers[k] = jev.Answer{Type: "noul", Noul: v}
					continue
				}
			}
			switch k {
			case "trajectory":
				choice := "progressing"
				if len(overrides) > 0 && overrides[0]["looping"] > 0 {
					choice = "looping"
				}
				answers[k] = jev.Answer{Type: "choice", Choice: choice, Probabilities: map[string]float64{choice: 0.9}}
			case "complexity":
				time.Sleep(100 * time.Millisecond)
				answers[k] = jev.Answer{Type: "score", Score: 1.0}
			case "status":
				answers[k] = jev.Answer{Type: "choice", Choice: "done", Confidence: 0.95}
			case "needless_ask", "tests", "test_at_fault":
				answers[k] = jev.Answer{Type: "noul", Noul: 0}
			default:
				if strings.HasPrefix(k, "needed_") {
					answers[k] = jev.Answer{Type: "noul", Noul: 0.1}
					break
				}
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
	cases := []struct {
		name        string
		passes      bool
		testAtFault float64
	}{
		{"passes", true, 0.1},
		{"fails", false, 0.1},
		{"fails through its own fault", false, 0.9},
	}
	for _, c := range cases {
		passes := c.passes
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var events []Event
			s := NewSession(Config{
				Model: crossModel{crossPasses: passes}, ModelName: "scripted", Jev: fakeJev(t, map[string]float64{"test_at_fault": c.testAtFault}), Dir: dir,
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
			if c.testAtFault > 0.5 {
				// Jev dismissed it: the agent never hears of it.
				if cross[0].Reason != "invalid" || len(nudge) != 0 || outcome != OutcomeDone || reason != "done_early" {
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

func TestNoEarlyStopBeforeRequestedTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewSession(Config{
		Model: scriptedModel{}, ModelName: "scripted", Jev: fakeJev(t, map[string]float64{"tests": 0.9}), Dir: dir,
		Policy: checkpoint.DefaultPolicy, Checkpoints: true,
		Route: true, RoutePolicy: checkpoint.DefaultRoutePolicy,
		EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} },
		Snapshot:      true, Batch: true, EarlyStop: true, StepPolicy: checkpoint.DefaultStepPolicy,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// The request asks for tests and the change touches none: the run must
	// not stop early, and ends at the turn-end checkpoint instead.
	if outcome, reason := s.Run(ctx, "Replace old with new in a.txt and add a test."); outcome != OutcomeDone || reason == "done_early" {
		t.Fatalf("Run = %s, %s", outcome, reason)
	}
}

// loopModel looks something up on every step until a Girdle nudge arrives,
// then replies.
type loopModel struct{ fantasy.LanguageModel }

func (loopModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	nudged := false
	for _, m := range call.Prompt {
		for _, p := range m.Content {
			if t, ok := fantasy.AsMessagePart[fantasy.TextPart](p); ok && strings.Contains(t.Text, "repeat similar lookups") {
				nudged = true
			}
		}
	}
	return func(yield func(fantasy.StreamPart) bool) {
		if nudged {
			for _, p := range []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "Changing approach."},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			} {
				if !yield(p) {
					return
				}
			}
			return
		}
		if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: fmt.Sprint(len(call.Prompt)), ToolCallName: "lookup", ToolCallInput: `{"searches":["old"]}`}) {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
		}
	}, nil
}

func TestHeartbeatNudgesALoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var nudges []Event
	lookups := 0
	s := NewSession(Config{
		Model: loopModel{}, ModelName: "loop", Jev: fakeJev(t, map[string]float64{"looping": 1}), Dir: dir,
		Policy: checkpoint.DefaultPolicy, Checkpoints: true, Batch: true,
		Heartbeat: true, HeartbeatPolicy: checkpoint.DefaultHeartbeatPolicy,
		Emit: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			if e.Type == EventNudge {
				nudges = append(nudges, e)
			}
			if e.Type == EventToolCall {
				lookups++
			}
		},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	s.Run(ctx, "Find where old is used.")
	if len(nudges) != 1 || nudges[0].Reason != "looping" || lookups != checkpoint.DefaultHeartbeatPolicy.Every {
		t.Fatalf("%d nudges (%v) after %d lookups", len(nudges), nudges, lookups)
	}
}

// countingModel counts the calls it serves.
type countingModel struct {
	fantasy.LanguageModel
	n *atomic.Int32
}

func (m countingModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.n.Add(1)
	return m.LanguageModel.Stream(ctx, call)
}

func TestFastModelServesLaterCalls(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "later calls", true: "every call"}[all], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var mainCalls, fastCalls atomic.Int32
			opts := func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} }
			s := NewSession(Config{
				Model: countingModel{scriptedModel{}, &mainCalls}, ModelName: "main", Jev: fakeJev(t), Dir: dir,
				Policy: checkpoint.DefaultPolicy, Checkpoints: true,
				Route: true, RoutePolicy: checkpoint.DefaultRoutePolicy, EffortOptions: opts,
				Snapshot: true, Batch: true, Speculate: true,
				FastModel: countingModel{scriptedModel{}, &fastCalls}, FastModelName: "fast", FastOptions: opts, FastAll: all,
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			// Without early stop: an apply, then a reply, so two calls.
			if outcome, _ := s.Run(ctx, "Replace old with new in a.txt."); outcome != OutcomeDone {
				t.Fatalf("outcome %s", outcome)
			}
			wantMain, wantFast := int32(1), int32(1)
			if all {
				wantMain, wantFast = 0, 2
			}
			if mainCalls.Load() != wantMain || fastCalls.Load() != wantFast {
				t.Fatalf("main %d, fast %d calls; want %d and %d", mainCalls.Load(), fastCalls.Load(), wantMain, wantFast)
			}
			if u := s.Usage(); all && u.FastOutputTokens != u.OutputTokens {
				t.Fatalf("usage not all fast: %+v", u)
			}
		})
	}
}

// failingModel fails every call the way a provider does when it can't parse
// the model's output.
type failingModel struct {
	fantasy.LanguageModel
	n *atomic.Int32
}

func (m failingModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	m.n.Add(1)
	return func(yield func(fantasy.StreamPart) bool) {
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: errors.New("Parsing failed")})
	}, nil
}

func TestFastModelFailureFallsBackToTheMainModel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mainCalls, fastCalls atomic.Int32
	opts := func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} }
	s := NewSession(Config{
		Model: countingModel{scriptedModel{}, &mainCalls}, ModelName: "main", Jev: fakeJev(t), Dir: dir,
		Policy: checkpoint.DefaultPolicy, Checkpoints: true,
		Route: true, RoutePolicy: checkpoint.DefaultRoutePolicy, EffortOptions: opts,
		Snapshot: true, Batch: true, Speculate: true,
		FastModel: failingModel{n: &fastCalls}, FastModelName: "fast", FastOptions: opts, FastAll: true,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if outcome, reason := s.Run(ctx, "Replace old with new in a.txt."); outcome != OutcomeDone {
		t.Fatalf("Run = %s %s", outcome, reason)
	}
	if fastCalls.Load() == 0 || mainCalls.Load() != fastCalls.Load() {
		t.Fatalf("fast %d, main %d calls: every failed fast call should fall back once", fastCalls.Load(), mainCalls.Load())
	}
}

// bigLookupModel looks up a large file on each of its first 12 steps, then
// replies. It records whether any prompt carried a pruned stub.
type bigLookupModel struct {
	fantasy.LanguageModel
	sawStub *atomic.Bool
}

func (m bigLookupModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	results := 0
	for _, msg := range call.Prompt {
		for _, part := range msg.Content {
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
				results++
				if t, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](tr.Output); ok && t.Text == prunedStub {
					m.sawStub.Store(true)
				}
			}
		}
	}
	return func(yield func(fantasy.StreamPart) bool) {
		if results >= 12 {
			for _, p := range []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "Done looking."},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			} {
				if !yield(p) {
					return
				}
			}
			return
		}
		if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: fmt.Sprint("c", results), ToolCallName: "lookup", ToolCallInput: `{"files":["big.txt"]}`}) {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
		}
	}, nil
}

func TestCompactionPrunesOldOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("line of text\n", 400)), 0o644); err != nil {
		t.Fatal(err)
	}
	var sawStub atomic.Bool
	var mu sync.Mutex
	var compactions []Event
	s := NewSession(Config{
		Model: bigLookupModel{sawStub: &sawStub}, ModelName: "big", Jev: fakeJev(t), Dir: dir,
		Policy: checkpoint.DefaultPolicy, Checkpoints: true, Batch: true,
		Compact: true, CompactPolicy: checkpoint.DefaultCompactPolicy,
		Emit: func(e Event) {
			if e.Type == EventCompact {
				mu.Lock()
				compactions = append(compactions, e)
				mu.Unlock()
			}
		},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	s.Run(ctx, "Read big.txt a lot.")
	if len(compactions) == 0 || compactions[0].Meta["pruned"] == "0" {
		t.Fatalf("compactions %v", compactions)
	}
	if !sawStub.Load() {
		t.Fatal("no later prompt carried the pruned stub")
	}
}
