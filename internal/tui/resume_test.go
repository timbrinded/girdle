package tui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/kernel"
)

// replyModel answers every call with the same short reply.
type replyModel struct{ fantasy.LanguageModel }

func (replyModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	return func(yield func(fantasy.StreamPart) bool) {
		for _, p := range []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "Hello back."},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		} {
			if !yield(p) {
				return
			}
		}
	}, nil
}

func TestResumeCarriesOnAnEarlierConversation(t *testing.T) {
	dir, logDir := t.TempDir(), t.TempDir()
	cfg := kernel.Config{Settings: kernel.Settings{Model: replyModel{}, ModelName: "a"}, Dir: dir, MaxStepsPerTurn: 5}

	log, err := kernel.OpenLog(filepath.Join(logDir, "earlier.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	earlier := cfg
	earlier.Log = log
	old := kernel.NewSession(earlier)
	if outcome, reason := old.Run(t.Context(), "Say hello."); outcome != kernel.OutcomeDone {
		t.Fatalf("Run = %s, %s", outcome, reason)
	}
	log.Close()

	m := newModel(t.Context(), kernel.NewSession(cfg), make(chan kernel.Event, 64), cfg.Settings, "now.jsonl", nil)
	m.cfg, m.logDir = cfg, logDir
	t.Cleanup(m.closeLogs)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m.input.SetValue("/res")
	if !strings.Contains(m.keyHint(), "/resume [search]") {
		t.Fatalf("hint %q", m.keyHint())
	}
	m.input.Reset()
	if _, ok := m.command("/resume"); !ok || m.picker == nil || m.picker.mode != pickConversation {
		t.Fatal("/resume didn't open the conversations")
	}
	if v := plain(m.View().Content); !strings.Contains(v, "Say hello.") {
		t.Fatalf("the picker doesn't show the earlier request:\n%s", v)
	}
	press(m, "enter")
	if m.picker != nil || m.sess.ID != old.ID || m.logPath != filepath.Join(logDir, "earlier.jsonl") {
		t.Fatalf("picker open %v, session %s, log %s: not carrying on %s", m.picker != nil, m.sess.ID, m.logPath, old.ID)
	}
	kinds := map[blockKind]string{}
	for _, b := range m.blocks {
		kinds[b.kind] += b.text
	}
	if kinds[blockUser] != "Say hello." || kinds[blockAssistant] != "Hello back." || !slices.ContainsFunc(m.blocks, func(b *block) bool { return b.kind == blockEnd }) {
		t.Fatalf("the earlier conversation isn't shown: %v", kinds)
	}
	// The session it carries on is no longer listed.
	m.command("/resume")
	if m.picker != nil || !strings.Contains(lastLine(m), "no earlier conversation") {
		t.Fatalf("picker open %v, last line %q", m.picker != nil, lastLine(m))
	}
}
