package checkpoint

import (
	"context"

	"github.com/timbrinded/girdle/internal/jev"
)

// ReproduceState is what Jev sees of a reproduce command that failed with
// the agent's fix undone.
type ReproduceState struct {
	Task    string `json:"task"`
	Command string `json:"command"`
	Output  string `json:"output"`
}

// reproduceQuestions ask whether the failure came from the test itself. Only
// a test that ran and failed shows the bug: one that was never found,
// collected or built fails too, and proves nothing.
var reproduceQuestions = map[string]jev.Question{
	"test_ran": jev.Noul(
		"Does `output` show the test that `command` runs actually running and failing, on an assertion or an error raised by the code under test, rather than never running, for example because it wasn't found or collected, the command was wrong, or the test code didn't build or import?",
	),
}

// ReproduceDecision records the reproduce checkpoint.
type ReproduceDecision struct {
	Checkpoint string         `json:"checkpoint"`
	Ran        bool           `json:"ran"`
	State      ReproduceState `json:"state"`
	Call
}

// Reproduce judges a reproduce command's failure without the fix. Ran is
// false when Jev reads it as the test never running, or when Jev can't be
// asked: a reproduction is claimed only on evidence.
func Reproduce(ctx context.Context, c *jev.Client, s ReproduceState) ReproduceDecision {
	d := ReproduceDecision{Checkpoint: "reproduce", State: s}
	var ok bool
	if d.Call, ok = ask(ctx, c, s, reproduceQuestions); ok {
		d.Ran = d.Answers["test_ran"].Noul >= 0.5
	}
	return d
}
