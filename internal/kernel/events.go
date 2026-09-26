package kernel

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// EventType names what happened. The event log is one JSON object per line.
type EventType string

const (
	EventSessionStart  EventType = "session_start"
	EventUserMessage   EventType = "user_message"
	EventTextDelta     EventType = "text_delta" // streamed to the UI, never logged
	EventAssistantText EventType = "assistant_text"
	EventToolCall      EventType = "tool_call"
	EventToolResult    EventType = "tool_result"
	EventTurnEnd       EventType = "turn_end"
	EventDecision      EventType = "decision"
	EventRoute         EventType = "route"
	EventSnapshot      EventType = "snapshot"   // files sent with a request
	EventStep          EventType = "step"       // one LLM call: timing and usage
	EventRace          EventType = "race"       // a raced LLM call: estimated usage of the losing copies
	EventCrossCheck    EventType = "crosscheck" // an independent test of the request: Reason is passed, failed, invalid, not ready or none written
	EventHeartbeat     EventType = "heartbeat"  // Jev's view of whether the turn is progressing
	EventCompact       EventType = "compact"    // older tool output pruned from the context
	EventReasoning     EventType = "reasoning"  // the LLM's reasoning summary for a step, logged only
	EventNudge         EventType = "nudge"
	EventRunEnd        EventType = "run_end"
	EventError         EventType = "error"
)

// Event is one thing that happened in a session. Only the fields relevant to
// its type are set.
type Event struct {
	Time     time.Time                 `json:"time"`
	Session  string                    `json:"session"`
	Type     EventType                 `json:"type"`
	Text     string                    `json:"text,omitempty"`
	Tool     string                    `json:"tool,omitempty"`
	CallID   string                    `json:"call_id,omitempty"`
	Input    string                    `json:"input,omitempty"`
	IsError  bool                      `json:"is_error,omitzero"`
	Steps    int                       `json:"steps,omitzero"`
	Usage    *Usage                    `json:"usage,omitempty"`
	Decision *checkpoint.Decision      `json:"decision,omitempty"`
	Route    *checkpoint.RouteDecision `json:"route,omitempty"`
	// Heartbeat is set on heartbeat events.
	Heartbeat *checkpoint.HeartbeatDecision `json:"heartbeat,omitempty"`
	Outcome   Outcome                       `json:"outcome,omitempty"`
	Reason    string                        `json:"reason,omitempty"`
	Meta      map[string]string             `json:"meta,omitempty"`
	// TTFTMS is how long a step waited for its first streamed token, and
	// DurationMS how long the LLM call took in all.
	TTFTMS     int64 `json:"ttft_ms,omitzero"`
	DurationMS int64 `json:"duration_ms,omitzero"`
}

// Usage is LLM token use for a turn or a run.
type Usage struct {
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens,omitzero"`
	CacheReadTokens int64   `json:"cache_read_tokens,omitzero"`
	CostUSD         float64 `json:"cost_usd,omitzero"`
	JevTokens       int64   `json:"jev_tokens,omitzero"`
	// The share of the tokens above that the fast model used.
	FastInputTokens     int64 `json:"fast_input_tokens,omitzero"`
	FastOutputTokens    int64 `json:"fast_output_tokens,omitzero"`
	FastCacheReadTokens int64 `json:"fast_cache_read_tokens,omitzero"`
}

// Log appends events to a JSONL file. It is safe for concurrent use.
type Log struct {
	mu sync.Mutex
	f  *os.File
}

// OpenLog creates the log file and its directory.
func OpenLog(path string) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Log{f: f}, nil
}

// Write appends one event. Streaming deltas are skipped.
func (l *Log) Write(e Event) error {
	if l == nil || e.Type == EventTextDelta {
		return nil
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.f.Write(append(b, '\n'))
	return err
}

// Close closes the file.
func (l *Log) Close() error {
	if l == nil {
		return nil
	}
	return l.f.Close()
}
