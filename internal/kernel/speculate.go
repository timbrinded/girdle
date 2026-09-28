package kernel

import (
	"context"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// guessEffort is the effort a request's first LLM call starts on while Jev
// is still choosing one: the effort routing picks for most requests.
const guessEffort = checkpoint.EffortLow

// sessionModel is the model a session's agent calls. It sends each call with
// the session's routed options, and lets the first call of a request start
// before Jev has routed it.
type sessionModel struct {
	fantasy.LanguageModel
	s *Session
}

// Stream starts a call. For the first call of a request whose route is still
// pending, the call starts straight away, on the pinned effort or on
// guessEffort, but its answer is held back until the route is known. If the
// route leads to the same effort the answer is used; otherwise the call is
// cancelled and started again on the routed effort. The caller sees nothing
// before the route is known, so no tool runs on a guess.
func (m sessionModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	pending := m.s.routing
	m.s.routing = nil
	if pending == nil {
		if m.s.callOptions != nil {
			call.ProviderOptions = m.s.callOptions
		}
		return m.LanguageModel.Stream(ctx, call)
	}

	want := guessEffort
	if !m.s.cfg.AutoEffort {
		want = m.s.cfg.Effort
	}
	guessed, opts := m.s.cfg.effort(want)
	guess := call
	guess.ProviderOptions = opts
	gctx, cancel := context.WithCancel(ctx)
	type started struct {
		stream fantasy.StreamResponse
		err    error
	}
	first := make(chan started, 1)
	go func() {
		st, err := m.LanguageModel.Stream(gctx, guess)
		first <- started{st, err}
	}()

	var d checkpoint.RouteDecision
	select {
	case d = <-pending:
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
	used := m.s.applyRoute(d)
	if used == guessed {
		r := <-first
		if r.err != nil {
			cancel()
			return nil, r.err
		}
		return afterStream(r.stream, cancel), nil
	}
	cancel()
	m.s.emit(Event{Type: EventError, Text: "route chose " + string(used) + " effort: restarting the first call"})
	call.ProviderOptions = m.s.callOptions
	return m.LanguageModel.Stream(ctx, call)
}

// afterStream calls done once the stream has been read to the end or
// abandoned.
func afterStream(st fantasy.StreamResponse, done func()) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		defer done()
		for p := range st {
			if !yield(p) {
				return
			}
		}
	}
}
