// Package tui is Girdle's terminal interface: a scrolling transcript, an input
// box, and a status line. The kernel's events drive everything shown.
package tui

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/tools"
)

// Run starts the TUI and blocks until the user quits.
func Run(ctx context.Context, cfg kernel.Config, logPath string) error {
	events := make(chan kernel.Event, 4096)
	cfg.Emit = func(e kernel.Event) { events <- e }
	sess := kernel.NewSession(cfg)
	m := newModel(ctx, sess, events, cfg.ModelName, logPath)
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

var (
	userStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4CC2B5"))
	toolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#C39AD6"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#86938D"))
	decisionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E3AE5B"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#E59478"))
	doneStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7BC486"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#B5C0BB"))
)

type eventMsg kernel.Event

type runDoneMsg struct {
	outcome kernel.Outcome
	reason  string
}

type model struct {
	ctx       context.Context
	sess      *kernel.Session
	events    <-chan kernel.Event
	modelName string
	logPath   string

	vp        viewport.Model
	input     textarea.Model
	lines     []string
	streaming strings.Builder
	running   bool
	cancel    context.CancelFunc
	status    string
	width     int
}

func newModel(ctx context.Context, sess *kernel.Session, events <-chan kernel.Event, modelName, logPath string) *model {
	in := textarea.New()
	in.Placeholder = "Ask Girdle to do something. Enter sends, Ctrl+C stops or quits."
	in.ShowLineNumbers = false
	in.SetHeight(3)
	in.Prompt = "› "
	in.KeyMap.InsertNewline.SetEnabled(false)
	in.Focus()
	vp := viewport.New()
	return &model{
		ctx: ctx, sess: sess, events: events, modelName: modelName, logPath: logPath,
		vp: vp, input: in,
		status: "ready",
		lines:  []string{dimStyle.Render(fmt.Sprintf("Girdle · %s · Jev checkpoints on · log %s", modelName, logPath))},
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.waitForEvent())
}

func (m *model) waitForEvent() tea.Cmd {
	return func() tea.Msg { return eventMsg(<-m.events) }
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.input.SetWidth(msg.Width)
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(max(3, msg.Height-m.input.Height()-2))
		m.refresh()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.running {
				m.cancel()
				m.status = "stopping…"
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" || m.running {
				return m, nil
			}
			m.input.Reset()
			m.appendLine(userStyle.Render("› ") + text)
			return m, m.start(text)
		case "pgup", "pgdown":
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
	case eventMsg:
		m.handleEvent(kernel.Event(msg))
		cmds = append(cmds, m.waitForEvent())
	case runDoneMsg:
		m.running = false
		m.cancel = nil
		m.status = fmt.Sprintf("%s (%s) · %s", msg.outcome, msg.reason, usageLine(m.sess.Usage()))
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *model) start(prompt string) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.running, m.cancel, m.status = true, cancel, "working…"
	return func() tea.Msg {
		defer cancel()
		o, r := m.sess.Run(ctx, prompt)
		return runDoneMsg{outcome: o, reason: r}
	}
}

func (m *model) handleEvent(e kernel.Event) {
	switch e.Type {
	case kernel.EventTextDelta:
		m.streaming.WriteString(e.Text)
		m.refresh()
	case kernel.EventAssistantText:
		m.streaming.Reset()
		m.appendLine(e.Text)
	case kernel.EventToolCall:
		m.flushStreaming()
		m.appendLine(toolStyle.Render("● "+e.Tool) + " " + toolArg(e.Input))
	case kernel.EventToolResult:
		style := dimStyle
		if e.IsError {
			style = errStyle
		}
		m.appendLine(style.Render("  └ " + firstLines(e.Text, 3)))
	case kernel.EventDecision:
		m.appendLine(decisionStyle.Render("◆ " + describeDecision(e.Decision)))
	case kernel.EventRoute:
		if r := e.Route; r != nil {
			m.appendLine(decisionStyle.Render(fmt.Sprintf("◆ jev · complexity %.2f → %s reasoning · %dms", r.Score, r.Effort, r.LatencyMS)))
		}
	case kernel.EventCrossCheck:
		m.appendLine(decisionStyle.Render("◆ cross-check " + e.Reason))
	case kernel.EventNudge:
		m.appendLine(dimStyle.Render("↻ nudged (" + e.Reason + ")"))
	case kernel.EventRunEnd:
		m.flushStreaming()
		style := doneStyle
		label := "done"
		if e.Outcome != kernel.OutcomeDone {
			style, label = decisionStyle, "over to you"
		}
		m.appendLine(style.Render(fmt.Sprintf("■ %s · %s", label, e.Reason)))
	case kernel.EventError:
		m.appendLine(errStyle.Render("✗ " + e.Text))
	}
}

func (m *model) flushStreaming() {
	if m.streaming.Len() > 0 {
		m.lines = append(m.lines, m.streaming.String())
		m.streaming.Reset()
	}
}

func (m *model) appendLine(s string) {
	m.lines = append(m.lines, s)
	m.refresh()
}

func (m *model) refresh() {
	content := strings.Join(m.lines, "\n")
	if m.streaming.Len() > 0 {
		content += "\n" + m.streaming.String()
	}
	if m.width > 0 {
		content = lipgloss.NewStyle().Width(m.width).Render(content)
	}
	m.vp.SetContent(content)
	m.vp.GotoBottom()
}

func (m *model) View() tea.View {
	status := statusStyle.Render(fmt.Sprintf(" %s · %s", m.status, m.modelName))
	v := tea.NewView(m.vp.View() + "\n" + status + "\n" + m.input.View())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func describeDecision(d *checkpoint.Decision) string {
	if d == nil {
		return "jev"
	}
	if d.Error != "" {
		return fmt.Sprintf("jev unavailable → %s", d.Action)
	}
	if d.Checkpoint == "step_end" {
		return fmt.Sprintf("jev · complete %.2f → %s (%s) · %dms",
			d.Answers["complete"].Noul, d.Action, d.Rule, d.LatencyMS)
	}
	st := d.Answers["status"]
	return fmt.Sprintf("jev · %s %.2f · evidence %.2f → %s (%s) · %dms",
		st.Choice, st.Confidence, d.Answers["evidence"].Noul, d.Action, d.Rule, d.LatencyMS)
}

// toolArg shows the argument people care about: the command or the path.
func toolArg(input string) string {
	if paths, check, ok := tools.ParseApply(input); ok && len(paths) > 0 {
		return firstLines(strings.Join(paths, ", ")+" · check: "+check, 1)
	}
	var lk tools.LookupInput
	if json.Unmarshal([]byte(input), &lk) == nil && len(lk.Files)+len(lk.Definitions)+len(lk.Searches) > 0 {
		var parts []string
		for _, group := range []struct {
			name  string
			items []string
		}{{"files", lk.Files}, {"definitions", lk.Definitions}, {"searches", lk.Searches}} {
			if len(group.items) > 0 {
				parts = append(parts, group.name+" "+strings.Join(group.items, ", "))
			}
		}
		return firstLines(strings.Join(parts, " · "), 1)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return firstLines(input, 1)
	}
	for _, k := range []string{"command", "path"} {
		if v, ok := args[k].(string); ok {
			return firstLines(v, 1)
		}
	}
	return firstLines(input, 1)
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		return strings.Join(lines[:n], " ⏎ ") + fmt.Sprintf(" … (%d lines)", len(lines))
	}
	return strings.Join(lines, " ⏎ ")
}

func usageLine(u kernel.Usage) string {
	return fmt.Sprintf("%dk in / %dk out · jev %d tok", u.InputTokens/1000, u.OutputTokens/1000, u.JevTokens)
}
