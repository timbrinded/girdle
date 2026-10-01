package kernel

import (
	"context"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// guessEffort is the effort a request's first LLM call starts on while Jev
// is still choosing one: the effort routing picks for most requests.
const guessEffort = checkpoint.EffortLow

// route asks Jev how hard the request is, and whether it asks for tests.
// The request waits for the answer only when Jev chooses its effort and it
// isn't speculating. Otherwise its first call starts at once, on a guess or
// on the pinned effort, and the route is taken when it is needed: before a
// speculative call's answer shows, or before a pinned effort's step end
// reads whether tests were asked for (takeRoute). So a pinned effort never
// waits on Jev for its LLM calls.
func (s *Session) route(ctx context.Context, request string) {
	switch {
	case !s.cfg.Route:
	case s.cfg.AutoEffort && !s.cfg.Speculate:
		s.applyRoute(s.routeDecision(ctx, request))
	default:
		// The goroutine gets its own copy of the channel: takeRoute clears
		// s.pendingRoute, possibly before the goroutine runs.
		pending := make(chan checkpoint.RouteDecision, 1)
		s.pendingRoute = pending
		go func() { pending <- s.routeDecision(ctx, request) }()
	}
}

// takeRoute applies the route the request didn't wait for, if it has come.
// With wait it waits for it, until ctx ends. It returns the effort the
// request's calls use, and whether the route was taken.
func (s *Session) takeRoute(ctx context.Context, wait bool) (checkpoint.Effort, bool) {
	if s.pendingRoute == nil {
		return "", false
	}
	var d checkpoint.RouteDecision
	select {
	case d = <-s.pendingRoute:
	default:
		if !wait {
			return "", false
		}
		select {
		case d = <-s.pendingRoute:
		case <-ctx.Done():
			return "", false
		}
	}
	s.pendingRoute = nil
	return s.applyRoute(d), true
}

func (s *Session) routeDecision(ctx context.Context, request string) checkpoint.RouteDecision {
	return checkpoint.Route(ctx, s.cfg.Jev, checkpoint.RouteState{Request: request}, s.cfg.RoutePolicy)
}

// applyRoute takes Jev's route for the request and sets the reasoning
// effort its LLM calls use: Jev's choice with AutoEffort, unless the route
// failed, and otherwise cfg.Effort, fitted to the model. It returns that
// effort.
func (s *Session) applyRoute(d checkpoint.RouteDecision) checkpoint.Effort {
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.testsAsked = d.Tests
	s.girdle, s.answer = d.Girdle, d.Answer
	want := s.cfg.Effort
	if s.cfg.AutoEffort && d.Error == "" {
		want = d.Effort
	}
	used, opts := s.cfg.effort(want)
	s.callOptions = opts
	s.emit(Event{Type: EventRoute, Route: &d, Effort: used})
	return used
}

// sessionModel is the model a session's agent calls. It sends each call with
// the session's routed options, and lets the first call of a request start
// before Jev has routed it.
type sessionModel struct {
	fantasy.LanguageModel
	s *Session
}

// Stream starts a call. When Jev chooses the effort and hasn't yet, which
// happens only for a request's first call, the call starts on guessEffort
// straight away, but its answer is held back until the route is known. If
// the route leads to the same effort the answer is used; otherwise the call
// is cancelled and started again on the routed effort. The caller sees
// nothing before the route is known, so no tool runs on a guess.
func (m sessionModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	if !m.s.cfg.AutoEffort || m.s.pendingRoute == nil {
		if m.s.callOptions != nil {
			call.ProviderOptions = m.s.callOptions
		}
		return m.LanguageModel.Stream(ctx, call)
	}

	guessed, opts := m.s.cfg.effort(guessEffort)
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

	used, ok := m.s.takeRoute(ctx, true)
	if !ok {
		cancel()
		return nil, ctx.Err()
	}
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
