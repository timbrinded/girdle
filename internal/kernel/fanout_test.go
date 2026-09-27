package kernel

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/tools"
)

func TestStepFanout(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verified float64
		early    bool
	}{{"verified", 0.9, true}, {"not verified", 0.1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var stepEnds []Event
			s := NewSession(Config{
				Model: scriptedModel{}, ModelName: "scripted", Jev: fakeJev(t, map[string]float64{"done_verified": tc.verified}), Dir: dir,
				Policy: checkpoint.DefaultPolicy, Checkpoints: true,
				Batch: true, EarlyStop: true, StepPolicy: checkpoint.FanoutStepPolicy,
				Emit: func(e Event) {
					if e.Type == EventDecision && e.Decision.Checkpoint == "step_end" {
						stepEnds = append(stepEnds, e)
					}
				},
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			outcome, reason := s.Run(ctx, "Replace the word old with new in a.txt.")
			if outcome != OutcomeDone || (reason == "done_early") != tc.early {
				t.Fatalf("Run = %s, %s", outcome, reason)
			}
			if len(stepEnds) == 0 {
				t.Fatal("no step-end decision")
			}
			d := stepEnds[0].Decision
			st := d.State
			if len(st.Changes) == 0 || !strings.Contains(st.Changes[0], "a.txt") || st.CheckOutput == "" || st.Check == "" {
				t.Fatalf("state lacks the changes or the check: %+v", st)
			}
			if _, ok := d.Answers["r0_impl"]; !ok {
				t.Fatalf("the broad question set wasn't asked: %v", slices.Collect(maps.Keys(d.Answers)))
			}
		})
	}
}

func TestPrefetchAddsPickedFiles(t *testing.T) {
	dir := t.TempDir()
	big := "package p\n\n" + strings.Repeat("// padding line for a large file\n", 600) + "func Big() {}\n\nfunc Bigger() {}\n"
	writeFiles(t, dir, map[string]string{
		"a.go":      "package p\n\nfunc Foo() {}\n",
		"helper.go": "package p\n\nfunc Helper() {}\n",
		"big.go":    big,
		"other.go":  "package p\n\nfunc Other() {}\n",
	})
	var asked []string
	pick := func(_ context.Context, request string, files []checkpoint.FileOutline) checkpoint.Prefetch {
		for _, f := range files {
			asked = append(asked, f.Path)
		}
		return checkpoint.Prefetch{Scores: map[string]float64{"helper.go": 0.9, "big.go": 0.8, "other.go": 0.2}}
	}
	snap := TakeSnapshotWith(t.Context(), dir, 16<<10, "Change `Foo`.", SnapshotOptions{Pick: pick})
	slices.Sort(asked)
	// big.go is too large to show whole, and a.go is already shown.
	if !slices.Equal(asked, []string{"helper.go", "other.go"}) {
		t.Fatalf("asked about %v", asked)
	}
	if !slices.Equal(snap.Prefetched, []string{"helper.go"}) {
		t.Fatalf("Prefetched = %v", snap.Prefetched)
	}
	for _, want := range []string{`<file path="helper.go">`, "most likely needs"} {
		if !strings.Contains(snap.Text, want) {
			t.Fatalf("snapshot lacks %q:\n%s", want, snap.Text)
		}
	}
	if strings.Contains(snap.Text, "func Other()") || strings.Contains(snap.Text, "func Big()") {
		t.Fatal("a file Jev scored low, or one too large, was added")
	}
}

func TestPrefetchSkipsDataInABigRepository(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.go":     "package p\n\nfunc Foo() {}\n",
		"lex.go":   "package p\n\nfunc lex() {}\n",
		"parse.go": "package p\n\ntype parser struct{}\n",
	}
	for i := range prefetchJudgeable + 10 {
		files[fmt.Sprintf("internal/testdata/case%03d.toml", i)] = strings.Repeat("key = 1\n", 30)
	}
	writeFiles(t, dir, files)
	var asked []string
	pick := func(_ context.Context, _ string, files []checkpoint.FileOutline) checkpoint.Prefetch {
		for _, f := range files {
			asked = append(asked, f.Path)
		}
		return checkpoint.Prefetch{}
	}
	TakeSnapshotWith(t.Context(), dir, 16<<10, "Change `Foo`.", SnapshotOptions{Pick: pick})
	slices.Sort(asked)
	if !slices.Equal(asked, []string{"lex.go", "parse.go"}) {
		t.Fatalf("asked about %v, not just the code", asked)
	}
}

// commandModel runs one shell command, then replies.
type commandModel struct {
	fantasy.LanguageModel
	command string
}

func (m commandModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	ran := false
	for _, msg := range call.Prompt {
		ran = ran || msg.Role == fantasy.MessageRoleTool
	}
	input, _ := json.Marshal(map[string]string{"command": m.command})
	return func(yield func(fantasy.StreamPart) bool) {
		if ran {
			for _, p := range []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "Done."},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			} {
				if !yield(p) {
					return
				}
			}
			return
		}
		if yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "c1", ToolCallName: "bash", ToolCallInput: string(input)}) {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
		}
	}, nil
}

func TestTripwireHandsBackToTheUser(t *testing.T) {
	if !tools.HaveAstGrep() {
		t.Skip("ast-grep is not installed")
	}
	marker := filepath.Join(t.TempDir(), "ran")
	for _, tc := range []struct {
		name, command string
		blocked       bool
	}{
		{"catastrophic", "rm -rf /nonexistent-girdle-tripwire-test/* && touch " + marker, true},
		{"ordinary", "touch " + marker, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(marker)
			var trips []Event
			s := NewSession(Config{
				Model: commandModel{command: tc.command}, ModelName: "cmd", Dir: t.TempDir(), Tripwire: true,
				Emit: func(e Event) {
					if e.Type == EventTripwire {
						trips = append(trips, e)
					}
				},
			})
			outcome, reason := s.Run(t.Context(), "Tidy up.")
			_, err := os.Stat(marker)
			ran := err == nil
			if tc.blocked {
				if outcome != OutcomeNeedsUser || !strings.HasPrefix(reason, "tripwire") || ran || len(trips) != 1 || trips[0].Tripwire.Rule != "floor" {
					t.Fatalf("Run = %s %q, ran %v, trips %d", outcome, reason, ran, len(trips))
				}
				return
			}
			if outcome != OutcomeDone || !ran || len(trips) != 0 {
				t.Fatalf("Run = %s %q, ran %v, trips %d", outcome, reason, ran, len(trips))
			}
		})
	}
}

func TestLeftoversNudge(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "old\n", "b.txt": "still old here\n"})
	var leftovers, nudges []Event
	s := NewSession(Config{
		Model: scriptedModel{}, ModelName: "scripted", Jev: fakeJev(t), Dir: dir,
		Policy: checkpoint.DefaultPolicy, Checkpoints: true, Batch: true, Leftovers: true,
		Emit: func(e Event) {
			switch {
			case e.Type == EventLeftovers && e.Reason == "found":
				leftovers = append(leftovers, e)
			case e.Type == EventNudge && e.Reason == "leftovers":
				nudges = append(nudges, e)
			}
		},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	// The scripted model only changes a.txt, so b.txt keeps the old name.
	s.Run(ctx, "Rename `old` to `new` everywhere.")
	if len(leftovers) == 0 || len(nudges) == 0 || !strings.Contains(nudges[0].Text, "b.txt:1") {
		t.Fatalf("leftovers %v, nudges %v", leftovers, nudges)
	}
	if len(leftovers) > maxLeftoverNudges {
		t.Fatalf("%d leftover nudges, want at most %d", len(leftovers), maxLeftoverNudges)
	}
}
