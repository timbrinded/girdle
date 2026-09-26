// Package race sends the same LLM call several times at once and keeps the
// first complete answer.
//
// An LLM step's latency varies a lot from call to call: the wait for the
// first token, and how long the model reasons, both change between identical
// requests. Racing N copies turns one draw from that distribution into the
// fastest of N, at the price of the tokens the losing copies used before they
// were cancelled. Only the winner's answer is ever seen, so tools run once.
package race

import (
	"context"
	"errors"
	"time"

	"charm.land/fantasy"
)

// Result describes one race, for the event log.
type Result struct {
	Copies int           // calls started
	Winner int           // index of the call whose answer was used
	Failed int           // calls that ended in an error
	Took   time.Duration // time until the winning answer was complete
	Usage  fantasy.Usage // the winning call's usage
}

// Model wraps a LanguageModel so that each streamed call is raced.
type Model struct {
	fantasy.LanguageModel
	Copies int
	// OnRace, if set, is called after each race.
	OnRace func(Result)
}

// New races copies calls per request. With copies below 2 it returns m.
func New(m fantasy.LanguageModel, copies int, onRace func(Result)) fantasy.LanguageModel {
	if copies < 2 {
		return m
	}
	return &Model{LanguageModel: m, Copies: copies, OnRace: onRace}
}

var errNoAnswer = errors.New("race: every call ended without an answer")

type answer struct {
	i     int
	parts []fantasy.StreamPart
	err   error
}

// complete reports whether a stream ended normally with a finish part.
func (a answer) complete() bool {
	return a.err == nil && len(a.parts) > 0 && a.parts[len(a.parts)-1].Type == fantasy.StreamPartTypeFinish
}

// Stream starts Copies identical streams and returns the first to complete,
// replayed from a buffer. The others are cancelled. If every copy fails, the
// first failure is returned as it came, so callers handle it as usual.
func (m *Model) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	start := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	answers := make(chan answer, m.Copies)
	for i := range m.Copies {
		go func() {
			stream, err := m.LanguageModel.Stream(ctx, call)
			if err != nil {
				answers <- answer{i: i, err: err}
				return
			}
			var parts []fantasy.StreamPart
			for p := range stream {
				parts = append(parts, p)
				if p.Type == fantasy.StreamPartTypeError {
					answers <- answer{i: i, parts: parts, err: p.Error}
					return
				}
			}
			answers <- answer{i: i, parts: parts}
		}()
	}

	var firstFailure *answer
	failed := 0
	for range m.Copies {
		a := <-answers
		if a.complete() {
			cancel()
			if m.OnRace != nil {
				m.OnRace(Result{Copies: m.Copies, Winner: a.i, Failed: failed, Took: time.Since(start), Usage: a.parts[len(a.parts)-1].Usage})
			}
			return replay(a.parts), nil
		}
		failed++
		if firstFailure == nil {
			firstFailure = &a
		}
	}
	cancel()
	if len(firstFailure.parts) > 0 {
		return replay(firstFailure.parts), nil
	}
	if firstFailure.err == nil {
		return nil, errNoAnswer
	}
	return nil, firstFailure.err
}

func replay(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		for _, p := range parts {
			if !yield(p) {
				return
			}
		}
	}
}
