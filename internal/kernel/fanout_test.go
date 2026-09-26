package kernel

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timbrinded/girdle/internal/checkpoint"
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
	snap := TakeSnapshotWith(t.Context(), dir, 16<<10, "Change `Foo`.", pick)
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
