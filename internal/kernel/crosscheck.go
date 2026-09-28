package kernel

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/race"
	"github.com/timbrinded/girdle/internal/tools"
)

// The cross-check is an independent test of a request, written from the
// task's words alone while the main LLM call does the work. A one-shot
// answer gets fewer chances than a step-by-step one to notice a case its
// own tests miss, and the same model tends to test what it thought of when
// it wrote the code. A test written apart from the code doesn't share its
// blind spots.

// crossCheckMarker must appear in every file the cross-check writes, so its
// files can never be mistaken for the agent's, and are always removed.
const crossCheckMarker = "girdle_crosscheck"

// crossCheckWait is how long a passing check waits for a cross-check that
// isn't written yet before going on without it. In 110 benchmark runs the
// writer finished a median 10.5 s before it was needed; the two times it
// wasn't ready, waiting 15 s bought nothing.
const crossCheckWait = 5 * time.Second

const crossCheckRole = `[Girdle] Another agent is doing the task above right now, and you won't see its code. Check its work independently. Write one small test file for the behaviour the task asks for, straight from the task's words: the cases it states, and the edge cases those words imply, such as empty input, one-pass iterators where it says iterable, equal items, boundaries and rounding. A wrong test costs the other agent time, so test only what the task clearly specifies, and compare each result with a literal expected value you worked out by hand, never with a value computed in the test. Use only the names the task and the existing code give, in the repository's own test framework and style, with the same package name and import paths as the existing tests beside the code. Make it a new file with girdle_crosscheck in its name, for example girdle_crosscheck_test.go beside the code, tests/test_girdle_crosscheck.py, or girdle_crosscheck.test.js. Send it in one apply call whose check runs only that file and never hides its exit code. If the snapshot doesn't show the code, write the test anyway from the names and behaviour the task gives. Only if the task asks for nothing a test could check, send an apply with no changes.`

// crossCheck is a written cross-check, ready to run, or why there is none.
type crossCheck struct {
	files map[string]string // path relative to the working directory → content
	check string
	none  string // set when there is nothing to run
}

func (s *Session) crossCheckOn() bool {
	return s.cfg.CrossCheck && s.cfg.Batch && s.earlyStopOn()
}

// startCrossCheck asks for a cross-check in the background. It uses the
// plain model at low effort: it must not race the main call or take its
// speculative route.
func (s *Session) startCrossCheck(ctx context.Context) {
	ch := make(chan *crossCheck, 1)
	s.cross = ch
	history := slices.Clone(s.history)
	go func() { ch <- s.writeCrossCheck(ctx, history) }()
}

func (s *Session) writeCrossCheck(ctx context.Context, history []fantasy.Message) *crossCheck {
	var in tools.ApplyInput
	recorded := false
	record := fantasy.NewAgentTool("apply", tools.ApplyDescription,
		func(_ context.Context, a tools.ApplyInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			in, recorded = a, true
			return fantasy.NewTextResponse("Recorded."), nil
		})
	// It is mechanical work, so it runs at low effort: a slow writer holds
	// up a quick agent.
	opts := s.cfg.ProviderOptions
	if s.cfg.EffortOptions != nil {
		opts = s.cfg.EffortOptions(checkpoint.EffortLow)
	}
	// The writer is hedged: a copy that hasn't answered in 2 s gets company.
	agent := fantasy.NewAgent(race.New(s.cfg.Model, 3, func() time.Duration { return 2 * time.Second }, s.noteRace),
		fantasy.WithSystemPrompt(systemPrompt(s.cfg)),
		fantasy.WithTools(record),
		fantasy.WithProviderOptions(opts),
	)
	start := time.Now()
	res, err := agent.Stream(ctx, fantasy.AgentStreamCall{
		Messages: append(history, fantasy.NewUserMessage(crossCheckRole)),
		StopWhen: []fantasy.StopCondition{fantasy.StepCountIs(1)},
	})
	if err != nil {
		if ctx.Err() == nil {
			s.emit(Event{Type: EventError, Text: "cross-check: " + err.Error()})
		}
		return &crossCheck{none: "the call failed"}
	}
	u := usageOf(res.TotalUsage)
	s.addUsage(u)
	s.emit(Event{Type: EventStep, DurationMS: time.Since(start).Milliseconds(), Usage: &u, Meta: map[string]string{"role": "crosscheck"}})
	switch {
	case !recorded:
		return &crossCheck{none: "no apply call: " + clipMiddle(res.Response.Content.Text(), 300)}
	case len(in.Changes) == 0:
		return &crossCheck{none: "no changes"}
	case in.Check == "":
		return &crossCheck{none: "no check"}
	}
	cc := &crossCheck{files: map[string]string{}, check: in.Check}
	for _, c := range in.Changes {
		// Only new files of its own: a cross-check never edits the code.
		switch {
		case c.OldText != "":
			return &crossCheck{none: "edits an existing file: " + c.Path}
		case !strings.Contains(filepath.Base(c.Path), crossCheckMarker) || filepath.IsAbs(c.Path) || strings.Contains(c.Path, ".."):
			return &crossCheck{none: "writes outside its own files: " + c.Path}
		}
		cc.files[c.Path] = c.Content
	}
	return cc
}

// runCrossCheck runs the request's cross-check once, against the changes
// made so far, and removes its files again. It returns feedback for the LLM
// when the cross-check fails, and "" when it passes, is missing or can't run.
func (s *Session) runCrossCheck(ctx context.Context) string {
	ch := s.cross
	s.cross = nil
	if ch == nil {
		return ""
	}
	var cc *crossCheck
	select {
	case cc = <-ch:
	case <-time.After(crossCheckWait):
		s.emit(Event{Type: EventCrossCheck, Reason: "not ready"})
		return ""
	case <-ctx.Done():
		return ""
	}
	if cc.none != "" {
		s.emit(Event{Type: EventCrossCheck, Reason: "none written", Text: cc.none})
		return ""
	}

	paths := slices.Sorted(maps.Keys(cc.files))
	var written []string
	defer func() {
		for _, p := range written {
			os.Remove(p)
			// Python leaves compiled copies beside the tests.
			caches, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "__pycache__", "*"+crossCheckMarker+"*"))
			for _, c := range caches {
				os.Remove(c)
			}
		}
	}()
	for _, p := range paths {
		full := filepath.Join(s.cfg.Dir, p)
		if _, err := os.Stat(full); err == nil {
			return "" // never overwrite a file
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return ""
		}
		if err := os.WriteFile(full, []byte(cc.files[p]), 0o644); err != nil {
			return ""
		}
		written = append(written, full)
	}
	out, code, ok := tools.RunCheck(ctx, s.cfg.Dir, cc.check, s.toolOptions())
	meta := map[string]string{"files": strings.Join(paths, ", "), "check": cc.check}
	if ok && code == 0 {
		s.emit(Event{Type: EventCrossCheck, Reason: "passed", Text: clipMiddle(out, 4000), Meta: meta})
		s.steps = append(s.steps, "independent cross-check "+strings.Join(paths, ", ")+" -> passed")
		return ""
	}

	// Jev screens the failure: a test that fails through its own fault
	// would only cost the agent a step to dismiss.
	var test strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&test, "// %s\n%s\n", p, cc.files[p])
	}
	d := checkpoint.CrossCheck(ctx, s.cfg.Jev, checkpoint.CrossCheckState{Task: s.task, Test: clipMiddle(test.String(), 6000), Output: clipEnd(out, 3000)})
	s.addUsage(Usage{JevTokens: d.InputTokens})
	meta["jev_valid"] = fmt.Sprint(d.Valid)
	if !d.Valid {
		s.emit(Event{Type: EventCrossCheck, Reason: "invalid", Text: clipMiddle(out, 4000), Meta: meta})
		return ""
	}
	s.emit(Event{Type: EventCrossCheck, Reason: "failed", Text: clipMiddle(out, 4000), Meta: meta})

	var b strings.Builder
	b.WriteString("[Girdle] An independent test, written from the task's words by another agent that couldn't see your code, fails against your change. It has been removed again. Decide whether the test or your code is wrong. If your code is wrong, fix it, add the failing case to your own tests, and apply again with your check. If the test is wrong, say why in one sentence.\n\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "<test path=%q>\n%s\n</test>\n", p, clipMiddle(cc.files[p], 6000))
	}
	fmt.Fprintf(&b, "<check>%s</check>\n<output>\n%s\n</output>", cc.check, clipEnd(out, 3000))
	return b.String()
}
