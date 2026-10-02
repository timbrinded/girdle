package tui

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/models"
)

// pickerView draws the picker as a panel width by height.
func (m *model) pickerView(width, height int) string {
	p := m.picker
	inner := max(20, width-4) // border and padding
	var head, foot []string
	switch p.mode {
	case pickEffort:
		head = []string{titleStyle.Render("Reasoning effort") + dimStyle.Render(" · "+m.settings.ModelName), ""}
		keys := []hint{{"enter set", 0}, {"↑/↓ choose", 2}, {"shift+tab next", 3}, {"esc close", 1}}
		if p.fromModels {
			keys = []hint{{"enter set", 0}, {"↑/↓ choose", 2}, {"tab models", 3}, {"esc back", 1}}
		}
		foot = []string{"", dimStyle.Render(fitHints(inner, keys, " · "))}
	case pickConversation:
		head = []string{titleStyle.Render("Carry on a conversation") + dimStyle.Render(" · "+tilde(m.dir)), p.search.View(), ""}
		keys := []hint{{"enter carry on", 0}, {"↑/↓ choose", 2}, {"esc close", 1}}
		foot = []string{"", dimStyle.Render(m.counter()), dimStyle.Render(fitHints(inner, keys, " · "))}
	default:
		head = []string{titleStyle.Render("Select model") + dimStyle.Render(" · "+m.catalogSummary()), p.search.View()}
		switch {
		case len(m.models.Catalog.Models) > 0:
		case m.fetching:
			head = append(head, dimStyle.Render("loading the models your OpenRouter key can use…"))
		default:
			head = append(head, errStyle.Render("OpenRouter's models aren't loaded ("+cmp.Or(m.fetchErr, "nothing cached")+"): type an exact model ID and press enter"))
		}
		head = append(head, "")
		foot = append([]string{"", dimStyle.Render(m.counter())}, m.details()...)
		keys := []hint{{"enter use", 0}, {"ctrl+s set default", 2}, {"ctrl+f add/remove yours", 3}, {"tab effort", 4}, {"esc close", 1}}
		foot = append(foot, dimStyle.Render(fitHints(inner, keys, " · ")))
	}
	if p.note != "" {
		foot = append(foot, p.note)
	}
	room := max(1, height-2-len(head)-len(foot))
	lines := slices.Concat(head, m.visibleRows(inner, room), foot)
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(inner).Render(l)
	}
	return panelStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

// visibleRows draws the rows that fit in room lines, scrolled so the cursor
// shows, padded to room.
func (m *model) visibleRows(width, room int) []string {
	p := m.picker
	var out []string
	switch {
	case len(p.choices()) == 0 && p.mode == pickEffort:
		out = append(out, dimStyle.Render(m.settings.ModelName+" has no reasoning effort setting"))
	case len(p.choices()) == 0:
		out = append(out, dimStyle.Render("no matches"))
	default:
		start := max(0, min(p.cursor-room/2, len(p.rows)-room))
		// Keep a section's header with its first choice.
		if start > 0 && p.rows[start-1].header != "" && p.cursor-start < room-1 {
			start--
		}
		shown := p.rows[start:min(len(p.rows), start+room)]
		// An effort's name fits in 8 columns; a model's column fits the
		// longest name shown, up to half the width, and a conversation's
		// first request takes what its age and ID leave.
		col := 8
		switch p.mode {
		case pickConversation:
			col = max(12, width-26)
		case pickModel:
			for _, r := range shown {
				col = max(col, lipgloss.Width(m.rowName(r)))
			}
			col = min(col, max(12, width/2))
		}
		for i, r := range shown {
			out = append(out, m.drawRow(r, start+i == p.cursor, col))
		}
	}
	for len(out) < room {
		out = append(out, "")
	}
	return out
}

// rowName is how a model's row names it.
func (m *model) rowName(r row) string {
	if r.id == m.models.List.Default.Model {
		return r.id + " · default"
	}
	return r.id
}

// drawRow draws a header, or a choice with its name in col columns and
// what it means after.
func (m *model) drawRow(r row, selected bool, col int) string {
	if r.header != "" {
		return decisionStyle.Render(r.header)
	}
	cursor, style := "  ", lipgloss.NewStyle()
	if selected {
		cursor, style = "› ", userStyle
	}
	mark := "  "
	if r.id == m.currentChoice() {
		mark = "✓ "
	}
	name, about := r.id, ""
	switch m.picker.mode {
	case pickEffort:
		about = effortAbout[r.id]
		if r.id == models.EffortAuto && m.routed != "" {
			about += ", latest " + string(m.routed)
		}
	case pickConversation:
		if c, ok := m.listed(r.id); ok {
			name, about = c.Title, conversationAbout(c)
		}
	default:
		name, about = m.rowName(r), m.facts(r.id)
	}
	name = ansi.Truncate(name, col, "…")
	name += strings.Repeat(" ", col-lipgloss.Width(name))
	return cursor + mark + style.Render(name) + "  " + dimStyle.Render(about)
}

// catalogSummary says where the list comes from.
func (m *model) catalogSummary() string {
	n := m.picker.available
	switch {
	case n == 0 && m.fetching:
		return "loading OpenRouter's models"
	case n == 0:
		return "OpenRouter's models aren't loaded"
	case m.models.Catalog.ForKey:
		return fmt.Sprintf("%d models your OpenRouter key can use with tools", n)
	default:
		return fmt.Sprintf("%d OpenRouter models that take tools", n)
	}
}

// counter shows where the cursor is among the choices.
func (m *model) counter() string {
	p := m.picker
	choices := p.choices()
	if len(choices) == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", slices.Index(choices, p.cursor)+1, len(choices))
}

// facts is a model's row summary: price, context and efforts.
func (m *model) facts(id string) string {
	info, ok := m.models.Catalog.Lookup(id)
	switch {
	case !ok && len(m.models.Catalog.Models) > 0:
		return "not available to your key"
	case !ok:
		return "not checked with OpenRouter"
	}
	var parts []string
	switch {
	case info.Input < 0:
		parts = append(parts, "price varies")
	case info.Input == 0 && info.Output == 0:
		parts = append(parts, "free")
	default:
		parts = append(parts, price(info.Input)+"/"+price(info.Output))
	}
	if info.Context > 0 {
		parts = append(parts, contextSize(info.Context))
	}
	parts = append(parts, effortRange(info.Efforts))
	return strings.Join(parts, " · ")
}

// details describes the model under the cursor in full, in two lines.
func (m *model) details() []string {
	info, ok := m.models.Catalog.Lookup(m.picker.selected())
	if !ok {
		return []string{"", ""}
	}
	first := info.Name
	if info.About != "" {
		first += dimStyle.Render(" — " + info.About)
	}
	var parts []string
	switch {
	case info.Input == 0 && info.Output == 0:
		parts = append(parts, "free")
	case info.Input > 0:
		parts = append(parts, fmt.Sprintf("%s in, %s out per million tokens", price(info.Input), price(info.Output)))
	}
	if info.Context > 0 {
		parts = append(parts, contextSize(info.Context)+" context")
	}
	switch {
	case slices.Equal(info.Efforts, checkpoint.Efforts):
		parts = append(parts, "takes any effort")
	case len(info.Efforts) > 0:
		parts = append(parts, "effort "+checkpoint.JoinEfforts(info.Efforts, ", "))
	default:
		parts = append(parts, "no effort setting")
	}
	second := dimStyle.Render(strings.Join(parts, " · "))
	if info.Expires != "" {
		second += errStyle.Render(" · withdrawn on " + info.Expires)
	}
	return []string{first, second}
}

// effortRange shows a model's efforts briefly: "low–max", or "off, low–high"
// when reasoning can be turned off.
func effortRange(efforts []checkpoint.Effort) string {
	if slices.Equal(efforts, checkpoint.Efforts) {
		return "any effort"
	}
	var prefix string
	if len(efforts) > 0 && efforts[0] == checkpoint.EffortNone {
		prefix, efforts = "off, ", efforts[1:]
	}
	switch {
	case len(efforts) == 0 && prefix == "":
		return "no effort"
	case len(efforts) == 0:
		return "reasoning off only"
	case len(efforts) == 1:
		return prefix + string(efforts[0])
	}
	// A run of neighbouring efforts reads as a range.
	first := slices.Index(checkpoint.Efforts, efforts[0])
	if first+len(efforts) <= len(checkpoint.Efforts) && slices.Equal(efforts, checkpoint.Efforts[first:first+len(efforts)]) {
		return prefix + string(efforts[0]) + "–" + string(efforts[len(efforts)-1])
	}
	return prefix + checkpoint.JoinEfforts(efforts, "/")
}

// price shows dollars per million tokens: $0.10, or $0.075 below ten cents.
func price(p float64) string {
	if p == 0 || p >= 0.1 {
		return fmt.Sprintf("$%.2f", p)
	}
	return "$" + strconv.FormatFloat(p, 'g', 2, 64)
}

// contextSize reads 1048576 tokens as 1M and 262144 as 262k.
func contextSize(tokens int) string {
	if tokens >= 1_000_000 {
		return strconv.FormatFloat(math.Round(float64(tokens)/1e5)/10, 'f', -1, 64) + "M"
	}
	return fmt.Sprintf("%dk", tokens/1000)
}
