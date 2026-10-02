package tui

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/timbrinded/girdle/internal/tools"
)

// blockKind is what a transcript entry is, which decides how it looks.
type blockKind int

const (
	blockNote      blockKind = iota // a message from Girdle itself
	blockUser                       // what the user asked
	blockAssistant                  // the agent's reply, in Markdown
	blockThinking                   // the agent's reasoning summary for a step
	blockTool                       // a tool call and, once it arrives, its result
	blockJev                        // a Jev checkpoint decision
	blockError                      // something that failed
	blockEnd                        // the end of a run
)

// block is one transcript entry. Finished blocks are drawn once per width
// and cached; a running tool or a reply still streaming is redrawn on each
// frame.
type block struct {
	kind blockKind
	tone tone
	text string
	// label and meta frame a decision or a run's end: what happened, and
	// the detail behind it. source is who decided, when not Jev.
	label, meta, source string

	tool, arg, callID string
	result            string
	resultErr         bool
	finished          bool // a tool's result arrived, or its run ended

	streaming bool // a reply still arriving
	// settled is a streaming reply's finished paragraphs, drawn as Markdown,
	// and settledLen how much of text they cover.
	settled      string
	settledLen   int
	settledWidth int
	settledDark  bool

	cacheWidth    int
	cacheDark     bool
	cacheExpanded bool
	cache         string
}

func (b *block) live() bool { return b.streaming || (b.kind == blockTool && !b.finished) }

func (b *block) invalidate() { b.cache = "" }

// Blocks within a burst of work sit together; a blank line separates the
// conversation's turns and the work from the words around it.
func tight(k blockKind) bool { return k == blockTool || k == blockJev || k == blockNote }

// transcript draws every block for the given width.
func (m *model) transcript(width int) string {
	var b strings.Builder
	for i, bl := range m.blocks {
		if i > 0 {
			b.WriteString("\n")
			if !tight(m.blocks[i-1].kind) || !tight(bl.kind) {
				b.WriteString("\n")
			}
		}
		b.WriteString(m.drawBlock(bl, width))
	}
	return b.String()
}

func (m *model) drawBlock(b *block, width int) string {
	if !b.live() && b.cache != "" && b.cacheWidth == width && b.cacheDark == isDark && b.cacheExpanded == m.expanded {
		return b.cache
	}
	out := m.renderBlock(b, width)
	if !b.live() {
		b.cache, b.cacheWidth, b.cacheDark, b.cacheExpanded = out, width, isDark, m.expanded
	}
	return out
}

func (m *model) renderBlock(b *block, width int) string {
	inner := max(10, width-2)
	switch b.kind {
	case blockUser:
		return userBlockStyle.Width(width).Render(b.text)
	case blockAssistant:
		var body string
		if b.streaming {
			body = m.renderStreaming(b, inner)
			if m.frame/5%2 == 0 {
				body += brandStyle.Render("▍")
			}
		} else {
			body = m.md.render(b.text, inner)
		}
		return brandStyle.Render("◆ girdle") + "\n" + indent(body, "  ")
	case blockThinking:
		lines := wrapLines(b.text, inner-2)
		lines, more := clipLines(lines, m.expanded, 4)
		var out strings.Builder
		out.WriteString(thinkingStyle.Render("✻ thinking"))
		for _, l := range lines {
			out.WriteString("\n" + faintStyle.Render("│ ") + thinkingStyle.Render(l))
		}
		if more > 0 {
			out.WriteString("\n" + faintStyle.Render(fmt.Sprintf("│ … %d more lines · ctrl+o to expand", more)))
		}
		return out.String()
	case blockTool:
		return m.renderTool(b, width)
	case blockJev:
		head := decisionStyle.Render("◇ "+cmp.Or(b.source, "jev")) + " " + b.tone.style().Bold(true).Render(b.label)
		return ansi.Truncate(head+dimStyle.Render("  "+b.meta), width, "…")
	case blockError:
		box := lipgloss.NewStyle().Foreground(pal.err).
			Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(pal.err).PaddingLeft(1).Width(width)
		return box.Render("✗ " + b.text)
	case blockEnd:
		icon := b.tone.icon()
		if b.tone == toneInfo {
			icon = "◼" // stopped
		}
		label := " " + icon + " " + b.label + " "
		if b.meta != "" {
			label += dimStyle.Render("· " + b.meta + " ")
		}
		style := b.tone.style()
		lead := style.Render("──")
		rest := max(0, width-2-lipgloss.Width(label))
		return lead + style.Bold(true).Render(label) + faintStyle.Render(strings.Repeat("─", rest))
	}
	// A note.
	lines := wrapLines(b.text, width-2)
	for i, l := range lines {
		prefix := "  "
		if i == 0 {
			prefix = b.tone.icon() + " "
		}
		lines[i] = b.tone.style().Render(prefix + l)
	}
	return strings.Join(lines, "\n")
}

// renderStreaming draws a reply still arriving: as Markdown up to its last
// finished paragraph, kept until another finishes, and as plain text after
// that. Rendering the whole reply on every frame would slow as it grows.
func (m *model) renderStreaming(b *block, width int) string {
	cut := settledLen(b.text)
	if cut != b.settledLen || width != b.settledWidth || isDark != b.settledDark {
		b.settled = ""
		if cut > 0 {
			b.settled = m.md.render(b.text[:cut], width)
		}
		b.settledLen, b.settledWidth, b.settledDark = cut, width, isDark
	}
	tail := strings.TrimSpace(b.text[cut:])
	switch {
	case tail == "":
		return b.settled
	case b.settled == "":
		return textStyle.Render(strings.Join(wrapLines(tail, width), "\n"))
	}
	return b.settled + "\n\n" + textStyle.Render(strings.Join(wrapLines(tail, width), "\n"))
}

// settledLen is how much of a streaming reply ends at a paragraph break
// outside a code block, so that Markdown can draw it.
func settledLen(text string) int {
	cut := 0
	for i := 0; ; {
		j := strings.Index(text[i:], "\n\n")
		if j < 0 {
			return cut
		}
		i += j + 2
		if strings.Count(text[:i], "```")%2 == 0 {
			cut = i
		}
	}
}

// renderTool draws a call on one line, with a spinner while it runs, and
// its result underneath once it arrives.
func (m *model) renderTool(b *block, width int) string {
	icon := toolStyle.Render(spinnerFrame(m.frame))
	// A shell command's exit code is the last line of its result.
	code, hasCode := tools.ExitCode(b.result)
	switch {
	case b.finished && (b.resultErr || hasCode && code != 0):
		icon = errStyle.Render("●")
	case b.finished:
		icon = toolStyle.Render("●")
	}
	head := icon + " " + toolStyle.Bold(true).Render(b.tool) + " " + textStyle.Render(b.arg)
	head = ansi.Truncate(head, width, "…")
	if !b.finished || b.result == "" && !b.resultErr {
		return head
	}
	style := dimStyle
	if b.resultErr {
		style = errStyle
	}
	all := resultLines(b.tool, b.result)
	exit := ""
	if hasCode && !m.expanded && len(all) > 4 {
		// Keep the exit code in sight when the output is cut short.
		exit, all = all[len(all)-1], all[:len(all)-1]
	}
	lines, more := clipLines(all, m.expanded, 3)
	var out strings.Builder
	out.WriteString(head)
	for i, l := range lines {
		prefix := "    "
		if i == 0 {
			prefix = faintStyle.Render("  ⎿ ")
		}
		out.WriteString("\n" + prefix + style.Render(ansi.Truncate(l, max(1, width-4), "…")))
	}
	if more > 0 {
		out.WriteString("\n    " + faintStyle.Render(fmt.Sprintf("… %d more lines", more)))
	}
	if exit != "" {
		style := dimStyle
		if code != 0 {
			style = errStyle
		}
		out.WriteString("\n    " + style.Render(exit))
	}
	return out.String()
}

// resultLines is what is worth showing of a tool's result: how much a read
// returned, otherwise its first lines.
func resultLines(tool, result string) []string {
	text := strings.Trim(result, "\n")
	if text == "" {
		return []string{"(no output)"}
	}
	lines := strings.Split(text, "\n")
	if tool == "read" && len(lines) > 1 {
		return []string{fmt.Sprintf("%d lines", len(lines))}
	}
	return lines
}

// clipLines keeps the first n lines unless expanded, and says how many it
// left out.
func clipLines(lines []string, expanded bool, n int) ([]string, int) {
	if expanded {
		n = max(n, 40)
	}
	if len(lines) <= n {
		return lines, 0
	}
	return lines[:n], len(lines) - n
}

func wrapLines(s string, width int) []string {
	return strings.Split(lipgloss.NewStyle().Width(max(1, width)).Render(strings.TrimSpace(s)), "\n")
}

func indent(s, prefix string) string { return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix) }

// elapsed is a duration as a person reads it: 850ms, 12s, 2m05s.
func elapsed(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}
