package kernel

import (
	"context"
	"fmt"
	"strings"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/tools"
)

// maxLeftoverNudges bounds how often one request is sent back for
// leftovers, so a disagreement can't loop.
const maxLeftoverNudges = 2

// leftovers returns a nudge when a name the request wants gone still
// appears where it must change, or a file it wants pruned still defines
// functions nothing uses. It returns "" when there's nothing to say.
func (s *Session) leftovers(ctx context.Context) string {
	if s.intent == nil || s.leftoverNudges >= maxLeftoverNudges {
		return ""
	}
	if s.gone == nil {
		select {
		case in := <-s.intent:
			s.gone = &in
			s.addUsage(Usage{JevTokens: in.InputTokens})
			meta := map[string]string{
				"gone": strings.Join(in.Gone, ","), "prune": strings.Join(in.Prune, ","), "latency_ms": fmt.Sprint(in.LatencyMS),
			}
			// A failed request leaves nothing to check; say why.
			if in.Error != "" {
				meta["error"] = in.Error
			}
			s.emit(Event{Type: EventLeftovers, Reason: "intent", Meta: meta, Intent: &in})
		case <-ctx.Done():
			return ""
		}
	}
	var b strings.Builder
	var found []string
	for _, n := range s.gone.Gone {
		mentions := tools.Mentions(ctx, s.cfg.Dir, n, 20)
		if len(mentions) == 0 {
			continue
		}
		d := checkpoint.MustChange(ctx, s.cfg.Jev, s.task, n, mentions)
		s.addUsage(Usage{JevTokens: d.InputTokens})
		s.emit(Event{Type: EventLeftovers, Reason: "must_change", MustChange: &d})
		if len(d.Must) > 0 {
			fmt.Fprintf(&b, "`%s` still appears where the task wants it gone:\n%s\n", n, strings.Join(d.Must, "\n"))
			found = append(found, n)
		}
	}
	for _, f := range s.gone.Prune {
		if unused := tools.UnusedDefs(ctx, s.cfg.Dir, f); len(unused) > 0 {
			fmt.Fprintf(&b, "%s still defines functions that no code uses: %s\n", f, strings.Join(unused, ", "))
			found = append(found, f)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	s.leftoverNudges++
	s.emit(Event{Type: EventLeftovers, Reason: "found", Meta: map[string]string{"found": strings.Join(found, ",")}})
	return "[Girdle] Not done yet.\n" + b.String() + "Change these, then run the check again."
}
