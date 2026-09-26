package kernel

import (
	"context"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// guessEffort is the effort a request's first LLM call starts on while Jev
// is still routing it: the effort routing picks for most requests.
const guessEffort = checkpoint.EffortLow

// sessionModel is the model a session's agent calls. It sends every call
// with the session's current routed options, and it lets the first call of
// a request start before Jev has routed it.
type sessionModel struct {
	fantasy.LanguageModel
	s *Session
}

// Stream starts a call. For the first call of a request whose route is still
// pending, the call starts on guessEffort straight away, but its answer is
// held back until the route is known. If Jev chose the same effort the
// answer is used; otherwise the call is cancelled and started again on Jev's
// effort. The caller sees nothing before the route is known, so no tool runs
// on a guess.
func (m sessionModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	pending := m.s.routing
	m.s.routing = nil
	if pending == nil {
		if m.s.callOptions != nil {
			call.ProviderOptions = m.s.callOptions
		}
		return m.LanguageModel.Stream(ctx, call)
	}

	guess := call
	guess.ProviderOptions = m.s.cfg.EffortOptions(guessEffort)
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
	m.s.applyRoute(d)
	if d.Effort == guessEffort {
		r := <-first
		if r.err != nil {
			cancel()
			return nil, r.err
		}
		return afterStream(r.stream, cancel), nil
	}
	cancel()
	m.s.emit(Event{Type: EventError, Text: "route chose " + string(d.Effort) + " effort: restarting the first call"})
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
