package kernel

import (
	"context"
	"os"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
	"github.com/timbrinded/girdle/internal/tools"
)

// toolOptions are how the session's tools run shell commands: through the
// tripwire when it's on, and offline when the config says so.
func (s *Session) toolOptions() tools.Options {
	o := tools.Options{Offline: s.cfg.OfflineTools, DenyRead: s.cfg.DenyRead, GrepContext: s.cfg.GrepContext}
	if s.cfg.Tripwire {
		o.Guard = s.guard
	}
	return o
}

// guard is the tripwire: it reads a command's facts, blocks what the hard
// floor forbids, runs at once what shows no deleting, pushing or sending,
// and asks Jev about the rest. A block ends the turn and hands the run to
// the user (decision 0014).
func (s *Session) guard(ctx context.Context, command string) string {
	home, _ := os.UserHomeDir()
	state := checkpoint.TripwireState{Command: clip.Middle(command, 4000), ProjectDir: s.cfg.Dir, HomeDir: home,
		TempDirs: tools.TempDirs(), Task: clip.Middle(s.task, 2000)}
	f, err := tools.ReadShell(ctx, command, s.cfg.Dir, home)
	if err == nil {
		state.Facts = f
		state.DeletesResolved = len(f.DeletesOutside)+len(f.DeletesUnknown)+len(f.InlineEffects)+len(f.Destroys) == 0
		if why := f.Floor(); why != "" {
			d := checkpoint.TripwireDecision{Action: "block", Rule: "floor", Why: why, State: state}
			s.emit(Event{Type: EventTripwire, Tripwire: &d})
			s.stopTurn(halt{kind: haltUser, reason: "tripwire: " + why})
			return why
		}
		if !f.NeedsJudgement() {
			return ""
		}
	}
	// Without the parse (no ast-grep), every command is judged.
	d := checkpoint.Tripwire(ctx, s.cfg.Jev, state, s.cfg.TripwirePolicy)
	s.addUsage(Usage{JevTokens: d.InputTokens})
	s.emit(Event{Type: EventTripwire, Tripwire: &d})
	if d.Action == "block" {
		s.stopTurn(halt{kind: haltUser, reason: "tripwire: " + d.Why})
		return d.Why
	}
	return ""
}
