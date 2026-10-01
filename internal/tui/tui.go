// Package tui is Girdle's terminal interface: a scrolling transcript, an input
// box, and a status line. The kernel's events drive everything shown.
package tui

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/tools"
)

// Run starts the TUI and blocks until the user quits. With models, the user
// can pick OpenRouter models; without, the model is fixed.
func Run(ctx context.Context, cfg kernel.Config, logPath string, models *Models) error {
	events := make(chan kernel.Event, 4096)
	cfg.Emit = func(e kernel.Event) { events <- e }
	sess := kernel.NewSession(cfg)
	m := newModel(ctx, sess, events, cfg.Settings, logPath, models)
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
	ctx     context.Context
	sess    *kernel.Session
	events  <-chan kernel.Event
	logPath string

	// settings are what the next request uses, and active what the running
	// one started with. canRoute is set when Jev can choose efforts, and
	// routed is the effort it chose for the latest request.
	settings kernel.Settings
	active   kernel.Settings
	canRoute bool
	routed   checkpoint.Effort
	// models is nil when there is no model picker, and picker is set while
	// it is open.
	models *Models
	picker *picker
	// fetching is set while the catalogue loads, and fetchErr says why it
	// last failed.
	fetching bool
	fetchErr string

	vp        viewport.Model
	input     textarea.Model
	lines     []string
	streaming strings.Builder
	running   bool
	cancel    context.CancelFunc
	status    string
	width     int
	height    int
}

func newModel(ctx context.Context, sess *kernel.Session, events <-chan kernel.Event, settings kernel.Settings, logPath string, models *Models) *model {
	in := textarea.New()
	in.Placeholder = "Ask Girdle to do something"
	in.ShowLineNumbers = false
	in.SetHeight(3)
	in.Prompt = "› "
	in.KeyMap.InsertNewline.SetEnabled(false)
	// ctrl+p cycles models, as in Pi; up still moves between lines.
	in.KeyMap.LinePrevious.SetKeys("up")
	in.Focus()
	vp := viewport.New()
	// Auto is only ever set when Jev can choose, as the session applies it.
	settings.AutoEffort = settings.AutoEffort && sess.CanAutoEffort()
	m := &model{
		ctx: ctx, sess: sess, events: events, logPath: logPath,
		settings: settings, canRoute: sess.CanAutoEffort(), models: models,
		vp: vp, input: in,
		status: "ready",
		lines:  []string{dimStyle.Render(fmt.Sprintf("Girdle · %s · Jev checkpoints on · log %s", settings.ModelName, logPath))},
	}
	if models != nil && models.Note != "" {
		m.lines = append(m.lines, decisionStyle.Render("◇ "+models.Note))
	}
	return m
}

// keyHint is the row under the input that reminds the user of the keys,
// or of the commands while one is being typed.
func (m *model) keyHint() string {
	width := m.width - 1
	if strings.HasPrefix(m.input.Value(), "/") {
		hints := []hint{{"/model [search | id [effort]]  pick a model", 0}, {"/effort [level]  set the reasoning effort", 1}}
		if m.models == nil {
			hints = hints[1:]
		}
		return fitHints(width, hints, "   ·   ")
	}
	quit := "ctrl+c quit"
	if m.running {
		quit = "ctrl+c stop"
	}
	hints := []hint{{"enter send", 4}, {"ctrl+l model", 0}, {"ctrl+p next model", 2}, {"shift+tab effort", 1}, {"/model /effort", 5}, {quit, 3}}
	if m.models == nil {
		hints = []hint{{"enter send", 2}, {"shift+tab effort", 0}, {"/effort", 3}, {quit, 1}}
	}
	return fitHints(width, hints, " · ")
}

// A hint is one key's reminder; rank 0 is the last to be dropped for room.
type hint struct {
	text string
	rank int
}

// fitHints joins hints in order, dropping the least important until they
// fit in width.
func fitHints(width int, hints []hint, sep string) string {
	hints = slices.Clone(hints)
	for {
		texts := make([]string, len(hints))
		for i, h := range hints {
			texts[i] = h.text
		}
		line := strings.Join(texts, sep)
		if len(hints) == 1 || lipgloss.Width(line) <= width {
			return line
		}
		worst := slices.MaxFunc(hints, func(a, b hint) int { return a.rank - b.rank })
		hints = slices.DeleteFunc(hints, func(h hint) bool { return h == worst })
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.waitForEvent(), m.fetchCatalog())
}

func (m *model) waitForEvent() tea.Cmd {
	return func() tea.Msg { return eventMsg(<-m.events) }
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(msg.Width)
		m.vp.SetWidth(msg.Width)
		// The status line and the key hint take a line each.
		m.vp.SetHeight(max(3, msg.Height-m.input.Height()-2))
		if m.picker != nil {
			m.sizePicker()
		}
		m.refresh()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.running {
				m.cancel()
				m.status = "stopping…"
				return m, nil
			}
			return m, tea.Quit
		}
		if m.picker != nil {
			return m, m.updatePicker(msg)
		}
		switch msg.String() {
		case "shift+tab":
			m.cycleEffort()
			return m, nil
		case "ctrl+l":
			if !m.needModels() {
				return m, nil
			}
			return m, m.openPicker(pickModel, "")
		case "ctrl+p":
			if m.needModels() {
				m.nextModel()
			}
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if cmd, ok := m.command(text); ok {
				m.input.Reset()
				return m, cmd
			}
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
	case catalogMsg:
		m.takeCatalog(msg)
	case runDoneMsg:
		if m.changed() {
			m.routed = ""
		}
		m.running = false
		m.cancel = nil
		m.status = fmt.Sprintf("%s (%s) · %s", msg.outcome, msg.reason, usageLine(m.sess.Usage()))
	case tea.MouseWheelMsg:
		if m.picker != nil {
			switch msg.Button {
			case tea.MouseWheelUp:
				m.picker.step(-1)
			case tea.MouseWheelDown:
				m.picker.step(1)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	// While the picker is open the prompt is blurred, and ignores input.
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	if p := m.picker; p != nil {
		// Its search takes pastes, and blinks on messages like any other.
		before := p.search.Value()
		p.search, cmd = p.search.Update(msg)
		cmds = append(cmds, cmd)
		if p.search.Value() != before && p.mode == pickModel {
			p.note = ""
			m.refind(true)
		}
	}
	return m, tea.Batch(cmds...)
}

// command runs a slash command typed at the prompt, and reports whether
// text was one. Only a known command's name counts, so a prompt such as
// "/usr/bin is missing" still goes to the agent.
func (m *model) command(text string) (tea.Cmd, bool) {
	name, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "/model", "/models":
		if !m.needModels() {
			return nil, true
		}
		// "/model <id> [effort]" switches at once, as in Grok Build, when the
		// catalogue or the user's list knows the ID. Without a catalogue,
		// anything shaped like an OpenRouter ID, provider/model, is taken as
		// one. Anything else is a search.
		fields := strings.Fields(arg)
		if len(fields) > 0 {
			id := fields[0]
			_, listed := m.models.Catalog.Lookup(id)
			unchecked := len(m.models.Catalog.Models) == 0 && strings.Contains(id, "/")
			if listed || m.models.List.Has(id) || unchecked {
				if m.useModel(id) && len(fields) > 1 {
					m.setEffort(fields[1])
				}
				return nil, true
			}
		}
		return m.openPicker(pickModel, arg), true
	case "/effort":
		if arg == "" {
			return m.openPicker(pickEffort, ""), true
		}
		m.setEffort(arg)
		return nil, true
	}
	return nil, false
}

func (m *model) start(prompt string) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.running, m.cancel, m.status = true, cancel, "working…"
	m.active, m.routed = m.settings, ""
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
			m.routed = e.Effort
			line := fmt.Sprintf("◆ jev · complexity %.2f · %s reasoning", r.Score, cmp.Or(string(e.Effort), "no"))
			if h := r.Handling(); h != "" {
				line += " · " + h
			}
			m.appendLine(decisionStyle.Render(fmt.Sprintf("%s · %dms", line, r.LatencyMS)))
		}
	case kernel.EventHeartbeat:
		if h := e.Heartbeat; h != nil && h.Action != checkpoint.Continue {
			m.appendLine(decisionStyle.Render("◆ jev · heartbeat: " + h.Rule))
		}
	case kernel.EventCrossCheck:
		m.appendLine(decisionStyle.Render("◆ cross-check " + e.Reason))
	case kernel.EventNudge:
		m.appendLine(dimStyle.Render("↻ nudged (" + e.Reason + ")"))
	case kernel.EventConfigure:
		// The session changed its own settings: keep them, so the next
		// change made here starts from them.
		if e.Settings != nil {
			m.settings = *e.Settings
		}
		m.appendLine(dimStyle.Render("◇ " + e.Text))
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

// notify shows the result of an action: in the picker while it is open,
// otherwise in the transcript.
func (m *model) notify(s string) {
	if m.picker != nil {
		m.picker.note = s
		return
	}
	m.appendLine(s)
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
	var v tea.View
	if m.picker != nil {
		v = tea.NewView(m.pickerView(m.width, max(8, m.height-1)) + "\n" + m.statusLine())
	} else {
		hint := lipgloss.NewStyle().MaxWidth(max(1, m.width)).Render(dimStyle.Render(" " + m.keyHint()))
		v = tea.NewView(m.vp.View() + "\n" + m.statusLine() + "\n" + m.input.View() + "\n" + hint)
	}
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// statusLine shows how the run stands on the left and, as in Pi's footer,
// the model and effort on the right: those of the running request, and any
// change waiting for the next.
func (m *model) statusLine() string {
	left := statusStyle.Render(" " + m.status)
	settings := func(s kernel.Settings, effort string) string {
		return statusStyle.Render(s.ModelName) + dimStyle.Render(" • ") + statusStyle.Render("effort "+effort)
	}
	right := settings(m.settings, m.effortLabel()) + " "
	if m.running && m.changed() {
		right = settings(m.active, effortLabel(m.active, m.routed)) + dimStyle.Render(" → next ") + settings(m.settings, m.effortLabel()) + " "
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return lipgloss.NewStyle().MaxWidth(max(1, m.width)).Render(left + " · " + right)
	}
	return left + strings.Repeat(" ", gap) + right
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
	if in, ok := tools.ParseApplyInput(input); ok && len(in.Changes) > 0 {
		return firstLines(strings.Join(in.Paths(), ", ")+" · check: "+in.Check, 1)
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
