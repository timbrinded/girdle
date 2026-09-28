package checkpoint

import (
	"context"
	"fmt"

	"github.com/timbrinded/girdle/internal/jev"
)

// CompactState is what Jev sees when the kernel compacts the context: the
// task, the latest steps, and older tool results that could be dropped.
type CompactState struct {
	Task        string   `json:"task"`
	RecentSteps []string `json:"recent_steps"`
	Results     []string `json:"results"`
}

// CompactPolicy says when and what to compact.
type CompactPolicy struct {
	Every      int // steps between compactions
	KeepRecent int // the most recent large results are always kept
	MinBytes   int // smaller results are never worth pruning
}

// DefaultCompactPolicy compacts every 8 steps, keeping the 4 most recent
// large results.
var DefaultCompactPolicy = CompactPolicy{Every: 8, KeepRecent: 4, MinBytes: 2000}

// CompactDecision records one compaction.
type CompactDecision struct {
	Checkpoint string `json:"checkpoint"`
	Keep       []bool `json:"keep"`
	Call
}

// Compact asks Jev which of the results are still needed. A result is dropped
// only when Jev is fairly sure it isn't needed; if Jev is unreachable,
// everything is kept.
func Compact(ctx context.Context, c *jev.Client, s CompactState) CompactDecision {
	d := CompactDecision{Checkpoint: "compact", Keep: make([]bool, len(s.Results))}
	for i := range d.Keep {
		d.Keep[i] = true
	}
	qs := map[string]jev.Question{}
	for i := range s.Results {
		qs[fmt.Sprintf("needed_%d", i)] = jev.Noul(fmt.Sprintf(
			"Will the coding agent still need the content of `results[%d]` to finish `task`, given `recent_steps`?", i))
	}
	var ok bool
	if d.Call, ok = ask(ctx, c, s, qs); !ok {
		return d
	}
	for i := range s.Results {
		d.Keep[i] = d.Answers[fmt.Sprintf("needed_%d", i)].Noul >= 0.3
	}
	return d
}
