package checkpoint

import (
	"context"
	"errors"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// Call records one Jev request for the decision log: the answers, the model
// that gave them, how long they took and what they cost, or why the request
// failed. Every checkpoint's decision carries one.
type Call struct {
	Answers     map[string]jev.Answer `json:"answers,omitempty"`
	JevModel    string                `json:"jev_model,omitempty"`
	LatencyMS   int64                 `json:"latency_ms"`
	InputTokens int64                 `json:"input_tokens,omitzero"`
	Error       string                `json:"error,omitempty"`
}

var errNoClient = errors.New("no Jev client")

// ask sends questions about state to Jev and records the call. ok is false
// when there is no client or the request failed, and the checkpoint then
// falls back to its default: every checkpoint degrades the same way.
func ask(ctx context.Context, c *jev.Client, state any, questions map[string]jev.Question) (call Call, ok bool) {
	if c == nil {
		return Call{Error: errNoClient.Error()}, false
	}
	start := time.Now()
	res, err := c.Ask(ctx, state, questions)
	call.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		call.Error = err.Error()
		return call, false
	}
	call.Answers, call.JevModel, call.InputTokens = res.Answers, res.Model, res.Usage.InputTokens
	return call, true
}
