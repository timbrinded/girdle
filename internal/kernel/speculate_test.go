package kernel

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// effortModel answers with the effort each call was sent with.
type effortModel struct {
	fantasy.LanguageModel
	mu    sync.Mutex
	calls []string
}

func (m *effortModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	effort := slices.Collect(maps.Keys(call.ProviderOptions))[0]
	m.mu.Lock()
	m.calls = append(m.calls, effort)
	m.mu.Unlock()
	return func(yield func(fantasy.StreamPart) bool) {
		if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: effort}) {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish})
		}
	}, nil
}

// seen waits until n calls have started, then returns their efforts, sorted:
// an abandoned guess may start after the call that replaces it.
func (m *effortModel) seen(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		calls := slices.Sorted(slices.Values(m.calls))
		m.mu.Unlock()
		if len(calls) >= n || time.Now().After(deadline) {
			return calls
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func speculate(t *testing.T, routed checkpoint.Effort, wantCalls int) (answer string, calls []string, s *Session) {
	t.Helper()
	inner := &effortModel{}
	s = &Session{cfg: Config{EffortOptions: func(e checkpoint.Effort) fantasy.ProviderOptions {
		return fantasy.ProviderOptions{string(e): nil}
	}}}
	s.routing = make(chan checkpoint.RouteDecision, 1)
	s.routing <- checkpoint.RouteDecision{Effort: routed}
	st, err := sessionModel{LanguageModel: inner, s: s}.Stream(t.Context(), fantasy.Call{})
	if err != nil {
		t.Fatal(err)
	}
	for p := range st {
		answer += p.Delta
	}
	return answer, inner.seen(t, wantCalls), s
}

func TestSpeculationKeepsAMatchingGuess(t *testing.T) {
	answer, calls, s := speculate(t, checkpoint.EffortLow, 1)
	if answer != "low" || !slices.Equal(calls, []string{"low"}) {
		t.Fatalf("answer %q from calls %v", answer, calls)
	}
	if s.routing != nil || s.callOptions == nil {
		t.Fatal("the route was not applied")
	}
}

func TestSpeculationRestartsOnADifferentRoute(t *testing.T) {
	answer, calls, _ := speculate(t, checkpoint.EffortHigh, 2)
	if answer != "high" || !slices.Equal(calls, []string{"high", "low"}) {
		t.Fatalf("answer %q from calls %v", answer, calls)
	}
}

func TestLaterCallsUseTheRoutedEffort(t *testing.T) {
	inner := &effortModel{}
	s := &Session{callOptions: fantasy.ProviderOptions{"medium": nil}}
	st, err := sessionModel{LanguageModel: inner, s: s}.Stream(t.Context(), fantasy.Call{ProviderOptions: fantasy.ProviderOptions{"low": nil}})
	if err != nil {
		t.Fatal(err)
	}
	for range st {
	}
	if calls := inner.seen(t, 1); !slices.Equal(calls, []string{"medium"}) {
		t.Fatalf("calls = %v", calls)
	}
}
