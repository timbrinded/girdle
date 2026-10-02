package tui

import (
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The TUI animates only while something is moving: a run, or the intro.
// Each tick redraws, then asks for the next one only if still needed, so an
// idle TUI costs nothing.
const frameEvery = 80 * time.Millisecond

type tickMsg struct{}

func tick() tea.Cmd { return tea.Tick(frameEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

func spinnerFrame(frame int) string {
	frames := spinner.MiniDot.Frames
	return frames[frame%len(frames)]
}

// shimmer draws s with a band of light moving across it, from base to
// highlight and back, one step per frame.
func shimmer(s string, frame int, base, highlight color.Color) string {
	runes := []rune(s)
	const band = 4
	ramp := lipgloss.Blend1D(band+1, highlight, base)
	period := len(runes) + band*2 + 6
	centre := frame%period - band
	var b strings.Builder
	for i, r := range runes {
		d := min(band, max(i-centre, centre-i))
		b.WriteString(lipgloss.NewStyle().Foreground(ramp[d]).Bold(true).Render(string(r)))
	}
	return b.String()
}

// pulse is a colour that breathes between from and to over period frames.
func pulse(frame, period int, from, to color.Color) color.Color {
	const steps = 16
	ramp := lipgloss.Blend1D(steps, from, to)
	phase := (1 - math.Cos(2*math.Pi*float64(frame%period)/float64(period))) / 2
	return ramp[int(math.Round(phase*float64(steps-1)))]
}
