package kernel

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
)

func TestLastAssistantText(t *testing.T) {
	msgs := []fantasy.Message{
		fantasy.NewUserMessage("do it"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "first"}}},
		fantasy.NewUserMessage("more"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "second"}}},
		{Role: fantasy.MessageRoleTool},
	}
	if got := lastAssistantText(msgs); got != "second" {
		t.Fatalf("got %q", got)
	}
	if got := lastAssistantText(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestSummarizeStep(t *testing.T) {
	call := fantasy.ToolCallContent{ToolName: "bash", Input: `{"command":"go test ./..."}`}
	got := summarizeStep(call, "ok  \texample.com/x\t0.1s\n[exit code 0]")
	if !strings.HasPrefix(got, "bash ") || !strings.Contains(got, "go test") || !strings.Contains(got, "[exit code 0]") || strings.Contains(got, "\n") {
		t.Fatalf("got %q", got)
	}
}

func TestLastN(t *testing.T) {
	if got := lastN([]string{"a", "b", "c"}, 2); len(got) != 2 || got[0] != "b" {
		t.Fatalf("got %v", got)
	}
	if got := lastN([]string{"a"}, 5); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestSummarizeStepKeepsTheEnd(t *testing.T) {
	out := strings.Repeat("grep match line\n", 200) + "ok  \texample.com/shop\t0.2s\n[exit code 0]"
	got := summarizeStep(fantasy.ToolCallContent{ToolName: "bash", Input: `{"command":"grep -rn x .; go test ./..."}`}, out)
	if !strings.Contains(got, "ok example.com/shop") || !strings.Contains(got, "[exit code 0]") || !utf8.ValidString(got) {
		t.Fatalf("test result lost: %q", got)
	}
	if len(got) > 1000 {
		t.Fatalf("summary too long: %d bytes", len(got))
	}
}
