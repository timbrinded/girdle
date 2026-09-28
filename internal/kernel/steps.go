package kernel

import (
	"encoding/json/v2"
	"fmt"
	"strings"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/clip"
	"github.com/timbrinded/girdle/internal/tools"
)

// inputField reads one string field from a tool call's JSON input.
func inputField(input, name string) string {
	var m map[string]any
	if json.Unmarshal([]byte(input), &m) != nil {
		return ""
	}
	v, _ := m[name].(string)
	return v
}

// outputText is a tool result's text, and whether the tool reported an
// error.
func outputText(o fantasy.ToolResultOutputContent) (string, bool) {
	if t, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](o); ok {
		return t.Text, false
	}
	if e, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](o); ok && e.Error != nil {
		return e.Error.Error(), true
	}
	return "", false
}

// summarizeStep is the one-line view of a tool call that Jev sees. It keeps
// mostly the end of the output, where test and build results appear: an
// earlier version kept too little and Jev missed passing tests.
func summarizeStep(call fantasy.ToolCallContent, result string) string {
	input := call.Input
	if call.ToolName == "apply" {
		// The input holds whole files; Jev needs only what changed and how
		// it was checked.
		if in, ok := tools.ParseApplyInput(input); ok {
			input = "changes " + strings.Join(in.Paths(), ", ") + "; check: " + in.Check
		}
	}
	return fmt.Sprintf("%s %s -> %s", call.ToolName, clip.Middle(oneLine(input), 160), clip.End(oneLine(result), 700))
}

func lastAssistantText(msgs []fantasy.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != fantasy.MessageRoleAssistant {
			continue
		}
		var b strings.Builder
		for _, p := range msgs[i].Content {
			if t, ok := fantasy.AsMessagePart[fantasy.TextPart](p); ok {
				b.WriteString(t.Text)
			}
		}
		return b.String()
	}
	return ""
}

func lastN(s []string, n int) []string {
	return s[max(0, len(s)-n):]
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// renderChange shows one change the way a diff would, clipped.
func renderChange(c tools.Change) string {
	if c.OldText != "" {
		return "edit " + c.Path + "\n- " + strings.ReplaceAll(clip.Middle(c.OldText, 600), "\n", "\n- ") +
			"\n+ " + strings.ReplaceAll(clip.Middle(c.NewText, 1500), "\n", "\n+ ")
	}
	return "write " + c.Path + "\n" + clip.Middle(c.Content, 2500)
}
