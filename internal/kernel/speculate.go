package kernel

import (
	"context"
	"errors"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// guessEffort is the effort a request's first LLM call starts on while Jev
// is still routing it: the effort routing picks for most requests.
const guessEffort = checkpoint.EffortLow

// sessionModel is the model a session's agent calls. It picks the model for
// each call (the fast model, if there is one, serves every call after a
// request's first), sends the call with the session's routed options, and
// lets the first call of a request start before Jev has routed it.
type sessionModel struct {
	fantasy.LanguageModel
	fast fantasy.LanguageModel
	s    *Session
}

// pick returns the model for the next call and how to build its options.
func (m sessionModel) pick() (fantasy.LanguageModel, func(checkpoint.Effort) fantasy.ProviderOptions, bool) {
	n := m.s.calls.Add(1)
	if m.fast != nil && (m.s.cfg.FastAll || n > 1) {
		return m.fast, m.s.cfg.FastOptions, true
	}
	return m.LanguageModel, m.s.cfg.EffortOptions, false
}

// Stream starts a call. For the first call of a request whose route is still
// pending, the call starts on guessEffort straight away, but its answer is
// held back until the route is known. If Jev chose the same effort the
// answer is used; otherwise the call is cancelled and started again on Jev's
// effort. The caller sees nothing before the route is known, so no tool runs
// on a guess.
func (m sessionModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	model, options, fast := m.pick()
	m.s.lastFast.Store(fast)
	pending := m.s.routing
	m.s.routing = nil
	if pending == nil {
		switch {
		case fast:
			call.ProviderOptions = options(m.s.fastEffort())
			return m.fastStream(ctx, call, m.s.mainOptions())
		case m.s.callOptions != nil:
			call.ProviderOptions = m.s.callOptions
		}
		return model.Stream(ctx, call)
	}

	guess := call
	guessAt := guessEffort
	if fast && m.s.cfg.FastEffort != "" {
		guessAt = m.s.cfg.FastEffort
	}
	guess.ProviderOptions = options(guessAt)
	gctx, cancel := context.WithCancel(ctx)
	type started struct {
		stream fantasy.StreamResponse
		err    error
	}
	first := make(chan started, 1)
	// Whatever the goroutine needs from the session is read here: the route
	// may change the session's options while it runs.
	fallback := m.s.cfg.EffortOptions(guessEffort)
	go func() {
		if fast {
			st, err := m.fastStream(gctx, guess, fallback)
			first <- started{st, err}
			return
		}
		st, err := model.Stream(gctx, guess)
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
	if d.Effort == guessEffort || fast && m.s.cfg.FastEffort != "" {
		r := <-first
		if r.err != nil {
			cancel()
			return nil, r.err
		}
		return afterStream(r.stream, cancel), nil
	}
	cancel()
	m.s.emit(Event{Type: EventError, Text: "route chose " + string(d.Effort) + " effort: restarting the first call"})
	call.ProviderOptions = options(d.Effort)
	if fast {
		call.ProviderOptions = options(m.s.fastEffort())
		return m.fastStream(ctx, call, m.s.mainOptions())
	}
	return model.Stream(ctx, call)
}

// fastStream sends a call to the fast model and, if it fails, sends it again
// to the main model. Fast providers sometimes can't parse the model's own
// tool calls ("Parsing failed" from Groq), and a failed call would otherwise
// end the run. The answer is buffered to see whether it failed, as racing
// buffers it anyway.
func (m sessionModel) fastStream(ctx context.Context, call fantasy.Call, fallback fantasy.ProviderOptions) (fantasy.StreamResponse, error) {
	st, err := m.fast.Stream(ctx, call)
	var parts []fantasy.StreamPart
	failure := err
	if err == nil {
		for p := range st {
			parts = append(parts, p)
			if p.Type == fantasy.StreamPartTypeError {
				failure = p.Error
				break
			}
		}
		if failure == nil && (len(parts) == 0 || parts[len(parts)-1].Type != fantasy.StreamPartTypeFinish) {
			failure = errors.New("the stream ended without finishing")
		}
	}
	if failure == nil {
		return replayParts(parts), nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	m.s.emit(Event{Type: EventError, Text: "fast model failed, using the main model for this call: " + failure.Error()})
	m.s.lastFast.Store(false)
	call.ProviderOptions = fallback
	return m.LanguageModel.Stream(ctx, call)
}

func replayParts(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		for _, p := range parts {
			if !yield(p) {
				return
			}
		}
	}
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
