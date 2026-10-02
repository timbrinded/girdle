package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheLogLeavesOutEventsStreamedOnlyToTheUI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []Event{
		{Type: EventTextDelta, Text: "Hel"},
		{Type: EventToolStart, Tool: "apply"},
		{Type: EventToolCall, Tool: "apply"},
	} {
		if err := l.Write(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"tool_call"`) {
		t.Fatalf("log:\n%s", b)
	}
}
