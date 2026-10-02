package kernel

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"charm.land/fantasy"
)

// promptModel answers like scriptedModel and keeps the last prompt it was
// sent.
type promptModel struct {
	scriptedModel
	last *[]fantasy.Message
}

func (m promptModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	*m.last = call.Prompt
	return m.scriptedModel.Stream(ctx, call)
}

func TestAResumedConversationCarriesOnFromItsLog(t *testing.T) {
	dir, logDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "first.jsonl")
	open := func() *Log {
		log, err := OpenLog(logPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { log.Close() })
		return log
	}
	var prompt []fantasy.Message
	model := promptModel{last: &prompt}
	cfg := Config{Settings: Settings{Model: model, ModelName: "m"}, Dir: dir, Batch: true, MaxStepsPerTurn: 10}

	cfg.Log = open()
	first := NewSession(cfg)
	run(t, first, "Replace the word old with new in a.txt.")
	// Another directory's conversation isn't listed.
	other := NewSession(Config{Settings: cfg.Settings, Dir: t.TempDir(), Log: cfg.Log, MaxStepsPerTurn: 10})
	run(t, other, "Say hello.")

	convs, err := Conversations(logDir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 || convs[0].ID != first.ID || convs[0].Title != "Replace the word old with new in a.txt." {
		t.Fatalf("conversations %+v", convs)
	}
	saved, err := LoadConversation(convs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Messages) != len(first.history) {
		t.Fatalf("loaded %d messages, the session had %d", len(saved.Messages), len(first.history))
	}

	cfg.Log = open()
	resumed := ResumeSession(cfg, saved)
	run(t, resumed, "Now say what you changed.")
	// The resumed request was sent the first one's call, its result and the
	// reply, then the new request.
	var calls, results int
	for _, m := range prompt {
		for _, p := range m.Content {
			if c, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](p); ok && c.ToolName == "apply" {
				calls++
			}
			if _, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](p); ok {
				results++
			}
		}
	}
	if calls != 1 || results != 1 || lastAssistantText(prompt) != "Done." {
		t.Fatalf("resumed prompt had %d apply calls, %d results, last reply %q", calls, results, lastAssistantText(prompt))
	}

	// The conversation goes on in the same log, under the same ID.
	convs, _ = Conversations(logDir, dir)
	if len(convs) != 1 || convs[0].ID != first.ID {
		t.Fatalf("conversations after resuming %+v", convs)
	}
	again, err := LoadConversation(convs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Messages) != len(resumed.history) {
		t.Fatalf("loaded %d messages, the resumed session had %d", len(again.Messages), len(resumed.history))
	}
	requests := slices.DeleteFunc(slices.Clone(again.Events), func(e Event) bool { return e.Type != EventUserMessage })
	if len(requests) != 2 {
		t.Fatalf("%d requests to show, want 2", len(requests))
	}
}

func TestFindConversationTakesAnUnambiguousPrefix(t *testing.T) {
	convs := []Conversation{{ID: "abc1"}, {ID: "abd2"}}
	if c, err := FindConversation(convs, "abc"); err != nil || c.ID != "abc1" {
		t.Fatalf("abc: %v, %v", c, err)
	}
	if _, err := FindConversation(convs, "ab"); err == nil {
		t.Fatal("ab matched two conversations without an error")
	}
	if _, err := FindConversation(convs, "x"); err == nil {
		t.Fatal("x matched nothing without an error")
	}
}
