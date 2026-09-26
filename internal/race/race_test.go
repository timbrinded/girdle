package race

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
)

// fake streams "answer i" after delays[i], or fails when errs[i] is set.
type fake struct {
	fantasy.LanguageModel
	delays    []time.Duration
	errs      []error
	calls     atomic.Int32
	cancelled atomic.Int32
}

func (f *fake) Stream(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	i := int(f.calls.Add(1)) - 1
	return func(yield func(fantasy.StreamPart) bool) {
		if f.errs != nil && f.errs[i] != nil {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: f.errs[i]})
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "thinking"}) {
			return
		}
		select {
		case <-time.After(f.delays[i]):
		case <-ctx.Done():
			f.cancelled.Add(1)
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "answer"}) {
			return
		}
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, Usage: fantasy.Usage{OutputTokens: int64(i + 1)}})
	}, nil
}

func collect(t *testing.T, s fantasy.StreamResponse) []fantasy.StreamPart {
	t.Helper()
	var parts []fantasy.StreamPart
	for p := range s {
		parts = append(parts, p)
	}
	return parts
}

func TestFastestCompleteAnswerWins(t *testing.T) {
	f := &fake{delays: []time.Duration{300 * time.Millisecond, 10 * time.Millisecond, 300 * time.Millisecond}}
	var got Result
	m := New(f, 3, func(r Result) { got = r })
	s, err := m.Stream(t.Context(), fantasy.Call{})
	if err != nil {
		t.Fatal(err)
	}
	parts := collect(t, s)
	if len(parts) != 3 || parts[2].Type != fantasy.StreamPartTypeFinish {
		t.Fatalf("parts = %+v", parts)
	}
	if got.Copies != 3 || got.Took > 200*time.Millisecond || got.Usage.OutputTokens == 0 {
		t.Fatalf("result = %+v", got)
	}
	deadline := time.Now().Add(time.Second)
	for f.cancelled.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.cancelled.Load() != 2 {
		t.Fatalf("losers cancelled = %d, want 2", f.cancelled.Load())
	}
}

func TestFailureLosesToAnAnswer(t *testing.T) {
	boom := errors.New("boom")
	f := &fake{delays: []time.Duration{0, 20 * time.Millisecond}, errs: []error{boom, nil}}
	var got Result
	m := New(f, 2, func(r Result) { got = r })
	s, err := m.Stream(t.Context(), fantasy.Call{})
	if err != nil {
		t.Fatal(err)
	}
	if parts := collect(t, s); parts[len(parts)-1].Type != fantasy.StreamPartTypeFinish {
		t.Fatalf("parts = %+v", parts)
	}
	if got.Failed != 1 {
		t.Fatalf("result = %+v", got)
	}
}

func TestAllFailReturnsTheFirstFailure(t *testing.T) {
	boom := errors.New("boom")
	f := &fake{delays: []time.Duration{0, 0}, errs: []error{boom, boom}}
	s, err := New(f, 2, nil).Stream(t.Context(), fantasy.Call{})
	if err != nil {
		t.Fatal(err)
	}
	parts := collect(t, s)
	if len(parts) != 1 || !errors.Is(parts[0].Error, boom) {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestOneCopyIsTheModelItself(t *testing.T) {
	f := &fake{}
	if New(f, 1, nil) != fantasy.LanguageModel(f) {
		t.Fatal("New with one copy should return the model unchanged")
	}
}
