package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/kernel"
)

// transcriptModel is a TUI on a session in a temporary project, without a
// model picker.
func transcriptModel(t *testing.T, checkpoints bool) *model {
	t.Helper()
	cfg := kernel.Config{
		Settings:    kernel.Settings{ModelName: "some/model", Efforts: checkpoint.Efforts, Effort: checkpoint.EffortMedium},
		Dir:         t.TempDir(),
		Checkpoints: checkpoints,
	}
	if checkpoints {
		cfg.Jev = jev.New("http://jev.invalid", jev.DefaultModel, "key")
	}
	m := newModel(t.Context(), kernel.NewSession(cfg), make(chan kernel.Event, 16), cfg.Settings, "log.jsonl", nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func TestParallelResultsAttachToTheirOwnCalls(t *testing.T) {
	m := transcriptModel(t, true)
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "read", CallID: "1", Input: `{"path":"a.js"}`})
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "bash", CallID: "2", Input: `{"command":"npm test"}`})
	// Results arrive in the other order.
	m.handleEvent(kernel.Event{Type: kernel.EventToolResult, Tool: "bash", CallID: "2", Text: "ok 3 tests"})
	m.handleEvent(kernel.Event{Type: kernel.EventToolResult, Tool: "read", CallID: "1", Text: "one\ntwo"})
	if len(m.blocks) != 2 {
		t.Fatalf("%d blocks, want one per call", len(m.blocks))
	}
	read, bash := m.blocks[0], m.blocks[1]
	if read.result != "one\ntwo" || bash.result != "ok 3 tests" || !read.finished || !bash.finished {
		t.Fatalf("read %+v, bash %+v", read, bash)
	}
	out := plain(m.transcript(100))
	if !strings.Contains(out, "read a.js\n  ⎿ 2 lines") || !strings.Contains(out, "bash npm test\n  ⎿ ok 3 tests") {
		t.Fatalf("transcript:\n%s", out)
	}
}

func TestPathsInTheProjectAreShownRelativeToIt(t *testing.T) {
	m := transcriptModel(t, true)
	file := filepath.Join(m.dir, "src", "slug.js")
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "write", CallID: "1", Input: fmt.Sprintf(`{"path":%q}`, file)})
	m.handleEvent(kernel.Event{Type: kernel.EventToolResult, Tool: "write", CallID: "1", Text: "wrote 12 bytes to " + file})
	if b := m.blocks[0]; b.arg != "src/slug.js" || b.result != "wrote 12 bytes to src/slug.js" {
		t.Fatalf("arg %q, result %q", b.arg, b.result)
	}
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "read", CallID: "2", Input: fmt.Sprintf(`{"path":%q}`, m.dir)})
	if b := m.blocks[1]; b.arg != "." {
		t.Fatalf("the project itself shows as %q", b.arg)
	}
}

func TestReasoningGoesBeforeTheReplyItsStepStreamed(t *testing.T) {
	m := transcriptModel(t, true)
	m.handleEvent(kernel.Event{Type: kernel.EventTextDelta, Text: "Hel"})
	m.handleEvent(kernel.Event{Type: kernel.EventTextDelta, Text: "lo"})
	m.handleEvent(kernel.Event{Type: kernel.EventReasoning, Text: "greet them"})
	m.handleEvent(kernel.Event{Type: kernel.EventAssistantText, Text: "Hello"})
	if len(m.blocks) != 2 || m.blocks[0].kind != blockThinking || m.blocks[1].kind != blockAssistant {
		t.Fatalf("blocks %+v", m.blocks)
	}
	if r := m.blocks[1]; r.text != "Hello" || r.streaming {
		t.Fatalf("reply %+v", r)
	}
}

func TestOutcomesSayHowTheRunEnded(t *testing.T) {
	for _, c := range []struct {
		o      kernel.Outcome
		reason string
		want   string
		tone   tone
	}{
		{kernel.OutcomeDone, "done", "done", toneOK},
		{kernel.OutcomeDone, "verified", "done · verified", toneOK},
		{kernel.OutcomeNeedsUser, "needs_user", "over to you", toneNotice},
		{kernel.OutcomeNeedsUser, "asked a question", "over to you · asked a question", toneNotice},
		{kernel.OutcomeCancelled, "cancelled", "stopped", toneInfo},
		{kernel.OutcomeError, "provider timed out", "failed · provider timed out", toneError},
	} {
		if got, tn := outcome(c.o, c.reason); got != c.want || tn != c.tone {
			t.Errorf("%s/%s: %q tone %d, want %q tone %d", c.o, c.reason, got, tn, c.want, c.tone)
		}
	}
}

func TestANudgeDecisionIsNotRepeated(t *testing.T) {
	m := transcriptModel(t, true)
	d := &checkpoint.Decision{Checkpoint: "turn_end", Action: checkpoint.Nudge, Rule: "verify"}
	m.handleEvent(kernel.Event{Type: kernel.EventDecision, Decision: d})
	m.handleEvent(kernel.Event{Type: kernel.EventNudge, Reason: "verify"})
	if len(m.blocks) != 1 {
		t.Fatalf("%d blocks: the nudge was shown twice", len(m.blocks))
	}
	// A nudge without a decision before it still shows.
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "bash", CallID: "1"})
	m.handleEvent(kernel.Event{Type: kernel.EventNudge, Reason: "heartbeat"})
	if last := m.blocks[len(m.blocks)-1]; last.kind != blockNote || !strings.Contains(last.text, "heartbeat") {
		t.Fatalf("last %+v", last)
	}
}

func TestNewOutputDoesNotPullAScrolledViewDown(t *testing.T) {
	m := transcriptModel(t, true)
	for i := range 60 {
		m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "bash", CallID: fmt.Sprint(i), Input: `{"command":"true"}`})
	}
	if !m.vp.AtBottom() {
		t.Fatal("the transcript should follow new output")
	}
	m.Update(key("pgup"))
	offset := m.vp.YOffset()
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "bash", CallID: "x", Input: `{"command":"true"}`})
	if m.vp.YOffset() != offset {
		t.Fatalf("scrolled from %d to %d", offset, m.vp.YOffset())
	}
	if !strings.Contains(plain(m.statusLine()), "more below") {
		t.Fatalf("status %q", plain(m.statusLine()))
	}
}

func TestANarrowStatusLineKeepsTheModel(t *testing.T) {
	m := transcriptModel(t, true)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m.start("task") // the command isn't run
	m.handleEvent(kernel.Event{Type: kernel.EventRunEnd, Outcome: kernel.OutcomeDone, Reason: "done", Usage: &kernel.Usage{InputTokens: 1000}})
	s := plain(m.statusLine())
	if !strings.Contains(s, "some/model • effort medium") || strings.Contains(s, "jev") {
		t.Fatalf("status %q", s)
	}
}

func TestTheWelcomeSaysWhetherJevJudges(t *testing.T) {
	on := plain(transcriptModel(t, true).vp.View())
	off := plain(transcriptModel(t, false).vp.View())
	if !strings.Contains(on, "checkpoints on") || !strings.Contains(off, "off: every turn end hands back to you") {
		t.Fatalf("with Jev:\n%s\nwithout:\n%s", on, off)
	}
}

func TestADecisionShowsOnlyTheScoresJevGave(t *testing.T) {
	// The fast flow's step-end checkpoint asks no "complete" question.
	d := &checkpoint.Decision{Checkpoint: "step_end", Action: checkpoint.Stop, Rule: "done_verified",
		Call: checkpoint.Call{LatencyMS: 288, Answers: map[string]jev.Answer{"status": {Choice: "done_verified", Confidence: 0.82}}}}
	label, meta, tn := describeDecision(d)
	if label != "stop · done_verified" || meta != "status 0.82 · 288ms" || tn != toneOK {
		t.Fatalf("%q %q %d", label, meta, tn)
	}
}

func TestTheStatusSaysWhichCallIsBeingWritten(t *testing.T) {
	m := transcriptModel(t, true)
	m.start("task") // the command isn't run
	m.handleEvent(kernel.Event{Type: kernel.EventToolStart, Tool: "apply", CallID: "1"})
	if p := m.phase(); p != "Writing apply" {
		t.Fatalf("phase %q", p)
	}
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "apply", CallID: "1", Input: `{"command":"go test"}`})
	if p := m.phase(); p != "Running apply" {
		t.Fatalf("phase %q", p)
	}
}

func TestAStepWithTextAndToolsShowsItsReplyOnce(t *testing.T) {
	m := transcriptModel(t, true)
	m.start("task") // the command isn't run
	// fantasy's order for one step: the text streams, the stream ends, the
	// tool calls run, then the step's reasoning and whole text arrive.
	for _, e := range []kernel.Event{
		{Type: kernel.EventTextDelta, Text: "I'll read "},
		{Type: kernel.EventTextDelta, Text: "it"},
		{Type: kernel.EventStep},
		{Type: kernel.EventToolCall, Tool: "read", CallID: "1", Input: `{"path":"a.go"}`},
		{Type: kernel.EventToolResult, Tool: "read", CallID: "1", Text: "package a"},
		{Type: kernel.EventReasoning, Text: "plan"},
		{Type: kernel.EventAssistantText, Text: "I'll read it"},
	} {
		m.handleEvent(e)
	}
	var kinds []blockKind
	for _, b := range m.blocks {
		kinds = append(kinds, b.kind)
	}
	if want := []blockKind{blockThinking, blockAssistant, blockTool}; !slices.Equal(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	if r := m.blocks[1]; r.text != "I'll read it" || r.streaming {
		t.Fatalf("reply %+v", r)
	}
}

func TestReasoningGoesBeforeAStepThatOnlyCalledTools(t *testing.T) {
	m := transcriptModel(t, true)
	m.start("task")
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "bash", CallID: "1"})
	m.handleEvent(kernel.Event{Type: kernel.EventToolResult, Tool: "bash", CallID: "1", Text: "ok"})
	// The second step: no text, so nothing streams before its tool call.
	m.handleEvent(kernel.Event{Type: kernel.EventStep})
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "read", CallID: "2"})
	m.handleEvent(kernel.Event{Type: kernel.EventReasoning, Text: "now read"})
	if m.blocks[1].kind != blockThinking || m.blocks[2].tool != "read" {
		t.Fatalf("blocks %+v", m.blocks)
	}
}

func TestAnErrorMidStreamDoesNotLeaveTheReplyStreaming(t *testing.T) {
	m := transcriptModel(t, true)
	m.start("task")
	for _, e := range []kernel.Event{
		{Type: kernel.EventTextDelta, Text: "Hel"},
		{Type: kernel.EventError, Text: "cross-check: boom"},
		{Type: kernel.EventTextDelta, Text: "lo"},
		{Type: kernel.EventAssistantText, Text: "Hello"},
		{Type: kernel.EventRunEnd, Outcome: kernel.OutcomeDone, Reason: "done"},
	} {
		m.handleEvent(e)
	}
	replies := 0
	for _, b := range m.blocks {
		if b.kind == blockAssistant {
			replies++
			if b.streaming || b.text != "Hello" {
				t.Fatalf("reply %+v", b)
			}
		}
	}
	if replies != 1 || m.running {
		t.Fatalf("%d replies, running %v", replies, m.running)
	}
}

func TestSiblingPathsAreNotRewritten(t *testing.T) {
	m := transcriptModel(t, true)
	d := m.dir
	in := fmt.Sprintf("%s2/x.go and %s.bak and %s/y.go and %q and %s", d, d, d, d, d)
	want := fmt.Sprintf("%s2/x.go and %s.bak and y.go and \".\" and .", d, d)
	if got := m.rel(in); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestTheViewKeepsFollowingWhenTheInputGrows(t *testing.T) {
	m := transcriptModel(t, true)
	for i := range 60 {
		m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "bash", CallID: fmt.Sprint(i)})
	}
	typeText(m, "a")
	press(m, "ctrl+j")
	if m.input.Height() != 2 || !m.vp.AtBottom() {
		t.Fatalf("input height %d, at bottom %v", m.input.Height(), m.vp.AtBottom())
	}
}

func TestTheInputTakesMoreLinesThanItShows(t *testing.T) {
	m := transcriptModel(t, true)
	for range 12 {
		typeText(m, "a")
		press(m, "ctrl+j")
	}
	if n := m.input.LineCount(); n != 13 {
		t.Fatalf("%d lines", n)
	}
}

func TestATinyTerminalFitsTheScreen(t *testing.T) {
	m := transcriptModel(t, true)
	m.Update(tea.WindowSizeMsg{Width: 12, Height: 30})
	if n := strings.Count(m.View().Content, "\n") + 1; n != 30 {
		t.Fatalf("%d lines on a 30-line screen", n)
	}
}

func TestBackgroundStepsAreNotCounted(t *testing.T) {
	m := transcriptModel(t, true)
	m.start("task")
	m.handleEvent(kernel.Event{Type: kernel.EventStep, Meta: map[string]string{"role": "crosscheck"}})
	if m.steps != 0 {
		t.Fatalf("steps %d", m.steps)
	}
}

func TestAStreamingReplyDrawsFinishedParagraphsAsMarkdown(t *testing.T) {
	text := "One **bold**.\n\n```go\nx := 1\n\ny := 2\n```\n\nstill typ"
	if cut := settledLen(text); text[:cut] != "One **bold**.\n\n```go\nx := 1\n\ny := 2\n```\n\n" {
		t.Fatalf("settled %q", text[:cut])
	}
	m := transcriptModel(t, true)
	m.handleEvent(kernel.Event{Type: kernel.EventTextDelta, Text: text})
	out := plain(m.renderStreaming(m.reply, 80))
	if strings.Contains(out, "**") || !strings.Contains(out, "still typ") {
		t.Fatalf("streaming:\n%s", out)
	}
}

func TestAReplyStartsUnderItsLabel(t *testing.T) {
	m := transcriptModel(t, true)
	m.handleEvent(kernel.Event{Type: kernel.EventAssistantText, Text: "- one\n- two"})
	lines := strings.Split(plain(m.transcript(80)), "\n")
	if len(lines) < 2 || !strings.Contains(lines[1], "one") {
		t.Fatalf("reply:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTheTripwireSaysWhoBlockedACommand(t *testing.T) {
	m := transcriptModel(t, true)
	m.handleEvent(kernel.Event{Type: kernel.EventTripwire, Tripwire: &checkpoint.TripwireDecision{Action: "block", Rule: "floor", Why: "it deletes outside the project: /home/x"}})
	m.handleEvent(kernel.Event{Type: kernel.EventTripwire, Tripwire: &checkpoint.TripwireDecision{Action: "block", Rule: "deletes_outside", Why: "Jev judged it deletes outside (0.93)", Call: checkpoint.Call{LatencyMS: 310}}})
	out := plain(m.transcript(120))
	for _, want := range []string{
		"◇ tripwire blocked  it deletes outside the project: /home/x",
		"◇ jev tripwire · blocked  Jev judged it deletes outside (0.93) · 310ms",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestALongOutcomeIsCutBeforeTheModel(t *testing.T) {
	m := transcriptModel(t, true)
	m.start("task")
	m.handleEvent(kernel.Event{Type: kernel.EventRunEnd, Outcome: kernel.OutcomeNeedsUser,
		Reason: "tripwire: Jev judged it deletes outside (0.93) and more words besides", Usage: &kernel.Usage{}})
	s := plain(m.statusLine())
	if !strings.HasSuffix(strings.TrimSpace(s), "some/model • effort medium") || !strings.Contains(s, "…") || lipgloss.Width(s) > 100 {
		t.Fatalf("status %q", s)
	}
}

func TestAFailedCheckShowsItsExitCode(t *testing.T) {
	m := transcriptModel(t, true)
	m.handleEvent(kernel.Event{Type: kernel.EventToolCall, Tool: "apply", CallID: "1"})
	m.handleEvent(kernel.Event{Type: kernel.EventToolResult, Tool: "apply", CallID: "1",
		Text: "✓ replaced 1 occurrence(s) in a.go\n✓ replaced 1 occurrence(s) in b.go\n$ go test ./...\nFAIL\n./a.go:3: old name\n[exit code 1]"})
	out := plain(m.transcript(100))
	if !strings.Contains(out, "… 2 more lines\n    [exit code 1]") {
		t.Fatalf("transcript:\n%s", out)
	}
}

func TestANudgeSaysWhatItAsked(t *testing.T) {
	d := &checkpoint.Decision{Checkpoint: "turn_end", Action: checkpoint.Nudge, Rule: "coverage",
		Nudge: "[Girdle] Before you finish: …\n- add a line to README.md\nDo or verify each of them now."}
	_, meta, _ := describeDecision(d)
	if !strings.HasPrefix(meta, "not done yet: “add a line to README.md”") {
		t.Fatalf("meta %q", meta)
	}
	d = &checkpoint.Decision{Checkpoint: "turn_end", Action: checkpoint.Nudge, Rule: "verify"}
	if _, meta, _ := describeDecision(d); !strings.HasPrefix(meta, "asked it to check its work") {
		t.Fatalf("meta %q", meta)
	}
}
