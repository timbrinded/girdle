package kernel

import (
	"context"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
)

// testRan asks Jev whether a reproduce command that failed without the fix
// ran its test and saw it fail, rather than never running it.
func (s *Session) testRan(ctx context.Context, command, output string) bool {
	d := checkpoint.Reproduce(ctx, s.cfg.Jev, checkpoint.ReproduceState{Task: clip.Middle(s.task, 2000), Command: command, Output: output})
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.emit(Event{Type: EventReproduce, Reproduce: &d})
	return d.Ran
}
