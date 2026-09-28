package kernel

import (
	"cmp"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// recorder notes which model each LLM call went to, and the effort it asked
// for ("-" for none).
type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) model(name string) fantasy.LanguageModel {
	return recordingModel{LanguageModel: scriptedModel{}, name: name, r: r}
}

func (r *recorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

type recordingModel struct {
	fantasy.LanguageModel
	name string
	r    *recorder
}

func (m recordingModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	effort := strings.Join(slices.Sorted(maps.Keys(call.ProviderOptions)), ",")
	m.r.mu.Lock()
	m.r.calls = append(m.r.calls, m.name+":"+cmp.Or(effort, "-"))
	m.r.mu.Unlock()
	return m.LanguageModel.Stream(ctx, call)
}

func settingsSession(t *testing.T, set Settings, jevOverrides ...map[string]float64) (*Session, *[]Event, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := &[]Event{}
	s := NewSession(Config{
		Settings: set, Jev: fakeJev(t, jevOverrides...), Dir: dir,
		Policy: checkpoint.DefaultPolicy, Checkpoints: true,
		Route: true, RoutePolicy: checkpoint.DefaultRoutePolicy,
		EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions { return fantasy.ProviderOptions{string(e): nil} },
		Batch:         true, EarlyStop: true, StepPolicy: checkpoint.DefaultStepPolicy,
		Emit: func(e Event) { *events = append(*events, e) },
	})
	return s, events, dir
}

func run(t *testing.T, s *Session, prompt string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	outcome, reason := s.Run(ctx, prompt)
	if outcome != OutcomeDone {
		t.Fatalf("Run = %s, %s", outcome, reason)
	}
	return reason
}

func routeEfforts(events []Event) []checkpoint.Effort {
	var got []checkpoint.Effort
	for _, e := range events {
		if e.Type == EventRoute {
			got = append(got, e.Effort)
		}
	}
	return got
}

func TestConfigureSwitchesModelAtTheNextRequest(t *testing.T) {
	var r recorder
	s, events, dir := settingsSession(t, Settings{Model: r.model("a"), ModelName: "a", Efforts: checkpoint.Efforts, AutoEffort: true})
	run(t, s, "Replace the word old with new in a.txt.")
	// Model b only reasons at high, so Jev's low is fitted up to it.
	s.Configure(Settings{Model: r.model("b"), ModelName: "b", Efforts: []checkpoint.Effort{checkpoint.EffortHigh}, AutoEffort: true})
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s, "Replace the word old with new in a.txt again.")
	if got, want := r.seen(), []string{"a:low", "b:high"}; !slices.Equal(got, want) {
		t.Fatalf("calls %v, want %v", got, want)
	}
	if got := routeEfforts(*events); !slices.Equal(got, []checkpoint.Effort{"low", "high"}) {
		t.Fatalf("route efforts %v", got)
	}
	i := slices.IndexFunc(*events, func(e Event) bool { return e.Type == EventSettings })
	if i < 0 || (*events)[i].Meta["model"] != "b" || (*events)[i].Meta["efforts"] != "high" {
		t.Fatalf("no settings event for b: %+v", *events)
	}
}

func TestPinnedEffortStillHeedsRequestedTests(t *testing.T) {
	var r recorder
	s, events, _ := settingsSession(t, Settings{Model: r.model("a"), ModelName: "a", Efforts: checkpoint.Efforts, Effort: checkpoint.EffortXHigh},
		map[string]float64{"tests": 0.9})
	// The request asks for tests and the change touches none. Jev still
	// routes a pinned effort to learn that, so the run doesn't stop early.
	if reason := run(t, s, "Replace old with new in a.txt and add a test."); reason == "done_early" {
		t.Fatal("stopped early without the requested tests")
	}
	for _, c := range r.seen() {
		if c != "a:xhigh" {
			t.Fatalf("calls %v, want every one at the pinned xhigh", r.seen())
		}
	}
	if got := routeEfforts(*events); !slices.Equal(got, []checkpoint.Effort{"xhigh"}) {
		t.Fatalf("route efforts %v", got)
	}
}

func TestNoEffortForAModelWithoutOne(t *testing.T) {
	var r recorder
	s, _, _ := settingsSession(t, Settings{Model: r.model("plain"), ModelName: "plain", AutoEffort: true})
	run(t, s, "Replace the word old with new in a.txt.")
	if got := r.seen(); !slices.Equal(got, []string{"plain:-"}) {
		t.Fatalf("calls %v, want no effort sent", got)
	}
}

func TestSwitchingModelDropsTheOldModelsReasoning(t *testing.T) {
	var r recorder
	s, _, _ := settingsSession(t, Settings{Model: r.model("a"), ModelName: "a", AutoEffort: true})
	history := []fantasy.Message{
		fantasy.NewUserMessage("task"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			fantasy.ReasoningPart{Text: "a's private thoughts"},
			fantasy.TextPart{Text: "Done."},
		}},
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.ReasoningPart{Text: "only thoughts"}}},
		fantasy.NewUserMessage("next"),
	}
	s.Seed(history)
	reasoning := func() int {
		n := 0
		for _, m := range s.history {
			for _, p := range m.Content {
				if p.GetType() == fantasy.ContentTypeReasoning {
					n++
				}
			}
		}
		return n
	}
	// The same model keeps its own reasoning.
	s.Configure(Settings{Model: r.model("a"), ModelName: "a", Effort: checkpoint.EffortHigh})
	s.applySettings()
	if reasoning() != 2 || len(s.history) != 4 {
		t.Fatalf("same model: %d reasoning parts in %d messages", reasoning(), len(s.history))
	}
	// Another model gets the conversation without it; a message that was
	// only reasoning goes.
	s.Configure(Settings{Model: r.model("b"), ModelName: "b"})
	s.applySettings()
	if reasoning() != 0 || len(s.history) != 3 {
		t.Fatalf("new model: %d reasoning parts in %d messages", reasoning(), len(s.history))
	}
	if text := s.history[1].Content[0].(fantasy.TextPart).Text; text != "Done." {
		t.Fatalf("assistant text %q", text)
	}
	// The seeded slice is untouched.
	if len(history[1].Content) != 2 {
		t.Fatal("the caller's messages were changed")
	}
}

func TestPortableSplitsParallelToolCalls(t *testing.T) {
	call := func(id string) fantasy.ToolCallPart {
		return fantasy.ToolCallPart{ToolCallID: id, ToolName: "read", Input: `{"path":"` + id + `"}`}
	}
	result := func(id string) fantasy.ToolResultPart {
		return fantasy.ToolResultPart{ToolCallID: id, Output: fantasy.ToolResultOutputContentText{Text: id + " contents"}}
	}
	history := []fantasy.Message{
		fantasy.NewUserMessage("task"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			fantasy.ReasoningPart{Text: "thinking"}, fantasy.TextPart{Text: "Reading both."}, call("a"), &fantasy.ToolCallPart{ToolCallID: "b", ToolName: "read"},
		}},
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{result("b"), result("a")}},
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{call("c")}},
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{result("c")}},
	}
	// Each call is its own assistant message, followed by its own result,
	// in the order the calls were made; the text goes with the first.
	var got []string
	for _, m := range portable(history) {
		var parts []string
		for _, p := range m.Content {
			switch p.GetType() {
			case fantasy.ContentTypeText:
				parts = append(parts, "text")
			case fantasy.ContentTypeToolCall:
				c, _ := fantasy.AsMessagePart[fantasy.ToolCallPart](p)
				parts = append(parts, "call "+c.ToolCallID)
			case fantasy.ContentTypeToolResult:
				r, _ := fantasy.AsMessagePart[fantasy.ToolResultPart](p)
				parts = append(parts, "result "+r.ToolCallID)
			default:
				parts = append(parts, string(p.GetType()))
			}
		}
		got = append(got, string(m.Role)+": "+strings.Join(parts, ", "))
	}
	want := []string{
		"user: text",
		"assistant: text, call a", "tool: result a",
		"assistant: call b", "tool: result b",
		"assistant: call c", "tool: result c",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
