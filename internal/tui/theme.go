package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// palette is the TUI's colours. The brand hues come from the Girdle mark: a
// green ring, a rust belt and a gold buckle.
type palette struct {
	fg, muted, faint, border color.Color
	brand, belt, buckle      color.Color
	user, userBg             color.Color
	tool, jev, ok, err       color.Color
}

var (
	darkPalette = palette{
		fg: lipgloss.Color("#E4E8E3"), muted: lipgloss.Color("#8E9A94"), faint: lipgloss.Color("#56625C"), border: lipgloss.Color("#3B4641"),
		brand: lipgloss.Color("#5FBF8F"), belt: lipgloss.Color("#E8875F"), buckle: lipgloss.Color("#E3AE5B"),
		user: lipgloss.Color("#4CC2B5"), userBg: lipgloss.Color("#1C2A28"),
		tool: lipgloss.Color("#C39AD6"), jev: lipgloss.Color("#E3AE5B"), ok: lipgloss.Color("#7BC486"), err: lipgloss.Color("#E5786D"),
	}
	lightPalette = palette{
		fg: lipgloss.Color("#1E2925"), muted: lipgloss.Color("#5B6863"), faint: lipgloss.Color("#9AA59F"), border: lipgloss.Color("#C6CFCA"),
		brand: lipgloss.Color("#2F7D57"), belt: lipgloss.Color("#B9532F"), buckle: lipgloss.Color("#9A6A12"),
		user: lipgloss.Color("#0F766C"), userBg: lipgloss.Color("#E4F1EE"),
		tool: lipgloss.Color("#7C4A9C"), jev: lipgloss.Color("#93650F"), ok: lipgloss.Color("#2E7D45"), err: lipgloss.Color("#B4402E"),
	}
)

// The styles below are rebuilt by setTheme when the terminal reports its
// background, so they suit light and dark terminals alike. Until then they
// assume a dark one.
var (
	pal            palette
	isDark         bool
	textStyle      lipgloss.Style
	dimStyle       lipgloss.Style
	faintStyle     lipgloss.Style
	userStyle      lipgloss.Style
	userBlockStyle lipgloss.Style
	brandStyle     lipgloss.Style
	toolStyle      lipgloss.Style
	decisionStyle  lipgloss.Style
	errStyle       lipgloss.Style
	doneStyle      lipgloss.Style
	thinkingStyle  lipgloss.Style
	titleStyle     lipgloss.Style
	panelStyle     lipgloss.Style
)

func init() { setTheme(true) }

func setTheme(dark bool) {
	isDark = dark
	pal = lightPalette
	if dark {
		pal = darkPalette
	}
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	textStyle = fg(pal.fg)
	dimStyle = fg(pal.muted)
	faintStyle = fg(pal.faint)
	userStyle = fg(pal.user).Bold(true)
	userBlockStyle = lipgloss.NewStyle().Foreground(pal.fg).Background(pal.userBg).
		Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(pal.user).BorderBackground(pal.userBg).
		Padding(0, 1)
	brandStyle = fg(pal.brand).Bold(true)
	toolStyle = fg(pal.tool)
	decisionStyle = fg(pal.jev)
	errStyle = fg(pal.err)
	doneStyle = fg(pal.ok).Bold(true)
	thinkingStyle = fg(pal.muted).Italic(true)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(pal.fg)
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(pal.border).Padding(0, 1)
}

// tone is how a note reads: plain information, something to notice, a
// failure or a success.
type tone int

const (
	toneInfo tone = iota
	toneNotice
	toneError
	toneOK
)

func (t tone) style() lipgloss.Style {
	switch t {
	case toneNotice:
		return decisionStyle
	case toneError:
		return errStyle
	case toneOK:
		return doneStyle
	}
	return dimStyle
}

func (t tone) icon() string {
	switch t {
	case toneNotice:
		return "◇"
	case toneError:
		return "✗"
	case toneOK:
		return "✓"
	}
	return "·"
}

// render draws a note on one line, as the picker shows it.
func (t tone) render(s string) string { return t.style().Render(t.icon() + " " + s) }
