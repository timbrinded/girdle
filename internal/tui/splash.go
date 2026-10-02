package tui

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/timbrinded/girdle/internal/buildinfo"
)

// The wordmark, three rows of block letters, and the belt that buckles
// under it while the TUI starts.
var wordmark = []string{
	"█▀▀▀ █ █▀▀▄ █▀▀▄ █    █▀▀▀",
	"█ ▀█ █ █▀▀▄ █  █ █    █▀▀ ",
	"▀▀▀▀ ▀ ▀  ▀ ▀▀▀  ▀▀▀▀ ▀▀▀▀",
}

// introFrames is how long the belt takes to buckle.
const introFrames = 14

// splash is what an empty session shows: the wordmark, what the session
// runs with, and how to start.
func (m *model) splash(width, height int) string {
	mark := m.drawWordmark()
	about := []string{
		thinkingStyle.Render("an agent you can leave alone"),
		"",
	}
	jev := "checkpoints on"
	if !m.sess.Judged() {
		jev = "off: every turn end hands back to you"
	}
	effort := m.effortLabel()
	if m.settings.AutoEffort {
		effort = "auto · Jev picks per request"
	}
	rows := [][2]string{
		{"model", m.settings.ModelName},
		{"effort", effort},
		{"jev", jev},
		{"flow", cmp.Or(m.flow, "default")},
		{"project", tilde(m.dir)},
		{"log", tilde(m.logPath)},
		{"version", buildinfo.Version()},
	}
	room := min(64, max(16, width-14))
	for _, r := range rows {
		about = append(about, dimStyle.Render(fmt.Sprintf("%-9s", r[0]))+textStyle.Render(shorten(r[1], room)))
	}
	about = append(about, "",
		dimStyle.Render("Type a request below and press ")+textStyle.Render("enter")+dimStyle.Render("."),
		dimStyle.Render("ctrl+l picks a model · shift+tab changes effort · ctrl+j adds a line"),
	)
	// Fit long values to the screen rather than wrapping them.
	body := lipgloss.NewStyle().MaxWidth(max(1, width-4)).Render(
		lipgloss.JoinVertical(lipgloss.Left, mark, "", strings.Join(about, "\n")))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

// drawWordmark colours the letters along a gradient and draws the belt: it
// slides in from the left during the intro, then the buckle closes.
func (m *model) drawWordmark() string {
	w := len([]rune(wordmark[0]))
	ramp := lipgloss.Blend1D(w, pal.brand, pal.user)
	var b strings.Builder
	for _, row := range wordmark {
		for i, r := range []rune(row) {
			b.WriteString(lipgloss.NewStyle().Foreground(ramp[i]).Render(string(r)))
		}
		b.WriteString("\n")
	}
	belt := lipgloss.NewStyle().Foreground(pal.belt)
	buckle := lipgloss.NewStyle().Foreground(pal.buckle).Bold(true)
	const at = 19 // the buckle's column
	shown := w
	if m.intro < introFrames {
		shown = w * (m.intro + 1) / introFrames
	}
	for i := range w {
		switch {
		case i >= shown:
			b.WriteString(" ")
		case i == at && m.intro >= introFrames-2:
			b.WriteString(buckle.Render("■"))
		case i == at:
			b.WriteString(buckle.Render("□"))
		default:
			b.WriteString(belt.Render("━"))
		}
	}
	return b.String()
}

// shorten keeps the start and the end of s, which for a path are the
// most telling parts, to fit width.
func shorten(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	head := width / 3
	return string(r[:head]) + "…" + string(r[len(r)-(width-head-1):])
}

// tilde shortens a path under the home directory.
func tilde(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || p == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return p
}
