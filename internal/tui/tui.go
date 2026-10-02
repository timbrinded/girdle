// Package tui is Girdle's terminal interface: a scrolling transcript, a
// status line, an input box and a row of key hints. The kernel's events drive
// everything shown.
package tui

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/tools"
)

// Run starts the TUI and blocks until the user quits. With models, the user
// can pick OpenRouter models; without, the model is fixed. flow names the
// flow the session runs, for the welcome screen. notice, if set, runs in the
// background at start and returns a note to show, such as a newer release.
func Run(ctx context.Context, cfg kernel.Config, logPath string, models *Models, flow string, notice func(context.Context) string) error {
	events := make(chan kernel.Event, 4096)
	cfg.Emit = func(e kernel.Event) { events <- e }
	sess := kernel.NewSession(cfg)
	m := newModel(ctx, sess, events, cfg.Settings, logPath, models)
	m.flow, m.notice = flow, notice
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

type eventMsg kernel.Event

// noticeMsg delivers the start-up notice, if there is one.
type noticeMsg string

// checkNotice runs the start-up notice in the background.
func (m *model) checkNotice() tea.Cmd {
	if m.notice == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Second)
		defer cancel()
		return noticeMsg(m.notice(ctx))
	}
}

type model struct {
	ctx     context.Context
	sess    *kernel.Session
	events  <-chan kernel.Event
	logPath string
	dir     string
	flow    string
	notice  func(context.Context) string
	// dirPath matches the project's path where it appears whole.
	dirPath *regexp.Regexp

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

	vp     viewport.Model
	input  textarea.Model
	blocks []*block
	md     markdown
	// expanded shows thinking and tool output in full.
	expanded bool

	running  bool
	stopping bool
	cancel   context.CancelFunc
	runStart time.Time
	steps    int
	// writing is the tool whose call the LLM is writing, if any.
	writing string
	// reply is the current step's reply, from its first streamed text until
	// the step ends, and stepFrom where the step's entries begin, which is
	// where its reasoning goes when that arrives at the step's end.
	reply    *block
	stepFrom int
	// status is the last run's outcome, shown while idle, and usage the
	// session's token use, shown beside it when there is room.
	status     string
	statusTone tone
	usage      string

	// frame counts animation ticks; intro counts the opening ones. ticking
	// is set while a tick is on its way.
	frame, intro int
	ticking      bool

	width  int
	height int
}

func newModel(ctx context.Context, sess *kernel.Session, events <-chan kernel.Event, settings kernel.Settings, logPath string, models *Models) *model {
	in := textarea.New()
	in.ShowLineNumbers = false
	in.DynamicHeight = true
	in.MaxHeight = 8 // rows shown; the text itself is not capped
	in.MaxContentHeight = 10_000
	in.SetHeight(1)
	in.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return userStyle.Render("❯ ")
		}
		return "  "
	})
	// Enter sends; ctrl+j, shift+enter or alt+enter starts a new line.
	in.KeyMap.InsertNewline.SetKeys("ctrl+j", "shift+enter", "alt+enter")
	// ctrl+p cycles models, as in Pi; up still moves between lines.
	in.KeyMap.LinePrevious.SetKeys("up")
	in.Focus()
	vp := viewport.New()
	// Auto is only ever set when Jev can choose, as the session applies it.
	settings.AutoEffort = settings.AutoEffort && sess.CanAutoEffort()
	m := &model{
		ctx: ctx, sess: sess, events: events, logPath: logPath, dir: sess.Dir(),
		settings: settings, canRoute: sess.CanAutoEffort(), models: models,
		vp: vp, input: in,
	}
	if m.dir != "" && m.dir != string(filepath.Separator) {
		m.dirPath = regexp.MustCompile(`(?m)(^|[\s"'(=:,\[])` + regexp.QuoteMeta(m.dir) + `(/|$|[\s"':,)\]])`)
	}
	m.styleInput()
	if models != nil && models.Note != "" {
		m.say(toneNotice, models.Note)
	}
	return m
}

// styleInput colours the input for the current theme, without the default
// highlight on the cursor's line.
func (m *model) styleInput() {
	st := textarea.DefaultStyles(isDark)
	for _, s := range []*textarea.StyleState{&st.Focused, &st.Blurred} {
		s.CursorLine = lipgloss.NewStyle()
		s.Text = textStyle
		s.Placeholder = faintStyle
		s.Prompt = userStyle
		s.EndOfBuffer = lipgloss.NewStyle()
	}
	st.Cursor.Color = pal.user
	m.input.SetStyles(st)
	m.setPlaceholder()
}

func (m *model) setPlaceholder() {
	m.input.Placeholder = "Ask Girdle to do something"
	if m.running {
		m.input.Placeholder = "Girdle is working · ctrl+c stops it"
	}
}

// keyHint is the row under the input that reminds the user of the keys,
// or of the commands while one is being typed.
func (m *model) keyHint() string {
	width := m.width - 2
	if strings.HasPrefix(m.input.Value(), "/") {
		hints := []hint{{"/model [search | id [effort]]  pick a model", 0}, {"/effort [level]  set the reasoning effort", 1}}
		if m.models == nil {
			hints = hints[1:]
		}
		return fitHints(width, hints, "   ·   ")
	}
	quit := "ctrl+c quit"
	if m.running && !m.stopping {
		quit = "ctrl+c stop"
	}
	expand := "ctrl+o expand"
	if m.expanded {
		expand = "ctrl+o collapse"
	}
	hints := []hint{{"enter send", 4}, {"ctrl+j newline", 6}, {"ctrl+l model", 0}, {"ctrl+p next model", 2}, {"shift+tab effort", 1}, {expand, 7}, {"/model /effort", 5}, {quit, 3}}
	if m.models == nil {
		hints = []hint{{"enter send", 2}, {"ctrl+j newline", 4}, {"shift+tab effort", 0}, {expand, 5}, {"/effort", 3}, {quit, 1}}
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
	return tea.Batch(textarea.Blink, m.waitForEvent(), m.fetchCatalog(), m.checkNotice(), tea.RequestBackgroundColor, m.animate())
}

func (m *model) waitForEvent() tea.Cmd {
	return func() tea.Msg { return eventMsg(<-m.events) }
}

// animate starts the animation ticks if something is moving and they
// aren't already running.
func (m *model) animate() tea.Cmd {
	if m.ticking || !(m.running || m.intro < introFrames) {
		return nil
	}
	m.ticking = true
	return tick()
}

// layout sizes the transcript to what the status line, the input box and
// the key hints leave.
func (m *model) layout() {
	if m.width == 0 {
		return
	}
	// Resizing the view moves its end; keep following it if it was.
	follow := m.vp.AtBottom()
	m.input.SetWidth(max(1, m.width-4))
	box := m.input.Height() + 2
	m.vp.SetWidth(m.width)
	m.vp.SetHeight(max(3, m.height-box-2))
	if follow {
		m.vp.GotoBottom()
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		if m.picker != nil {
			m.sizePicker()
		}
		m.refresh()
	case tea.BackgroundColorMsg:
		setTheme(msg.IsDark())
		m.styleInput()
		m.refresh()
	case tickMsg:
		m.ticking = false
		m.frame++
		if m.intro < introFrames {
			m.intro++
		}
		m.refresh()
		return m, m.animate()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			// The first ctrl+c stops a running request; a second quits
			// without waiting for it to stop.
			if m.running && !m.stopping {
				m.cancel()
				m.stopping = true
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
		case "ctrl+o":
			m.expanded = !m.expanded
			m.refresh()
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if cmd, ok := m.command(text); ok {
				m.input.Reset()
				m.layout()
				return m, cmd
			}
			if text == "" || m.running {
				return m, nil
			}
			m.input.Reset()
			m.layout()
			m.blocks = append(m.blocks, &block{kind: blockUser, text: text})
			m.refresh()
			m.vp.GotoBottom()
			return m, tea.Batch(m.start(text), m.animate())
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
	case noticeMsg:
		if msg != "" {
			m.add(&block{kind: blockNote, tone: toneNotice, text: string(msg)})
		}
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
	before := m.input.Height()
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	if m.input.Height() != before {
		m.layout()
		m.refresh()
	}
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
	m.running, m.cancel, m.status = true, cancel, ""
	m.runStart, m.steps, m.writing = time.Now(), 0, ""
	m.active, m.routed = m.settings, ""
	m.setPlaceholder()
	m.reply, m.stepFrom = nil, len(m.blocks)
	// The run's end arrives as a run_end event, in order after everything the
	// run showed, so this command has nothing to report.
	return func() tea.Msg {
		defer cancel()
		m.sess.Run(ctx, prompt)
		return nil
	}
}

func (m *model) handleEvent(e kernel.Event) {
	switch e.Type {
	case kernel.EventTextDelta:
		if m.reply == nil {
			m.reply = &block{kind: blockAssistant, streaming: true}
			m.add(m.reply)
		}
		m.reply.text += e.Text
		// Deltas come fast; the next frame draws them.
		return
	case kernel.EventAssistantText:
		// A step's text arrives whole as it ends, after any tool calls; it
		// replaces what streamed, if anything did.
		if b := m.reply; b != nil {
			b.text, b.streaming = e.Text, false
			b.invalidate()
			m.reply = nil
		} else {
			m.add(&block{kind: blockAssistant, text: e.Text})
		}
	case kernel.EventReasoning:
		// A step's reasoning arrives as it ends, but it came first.
		m.blocks = slices.Insert(m.blocks, min(m.stepFrom, len(m.blocks)), &block{kind: blockThinking, text: e.Text})
	case kernel.EventStep:
		// The step's stream is over: its reply, if any, has streamed, and its
		// tool calls come next. Background work reports steps of its own.
		if e.Meta["role"] != "" {
			break
		}
		m.steps++
		m.stepFrom = len(m.blocks)
		if m.reply != nil {
			m.stepFrom = slices.Index(m.blocks, m.reply)
		}
	case kernel.EventToolStart:
		m.endReply()
		m.writing = e.Tool
	case kernel.EventToolCall:
		m.endReply()
		m.writing = ""
		m.add(&block{kind: blockTool, tool: e.Tool, arg: m.rel(toolArg(e.Input)), callID: e.CallID})
	case kernel.EventToolResult:
		b := m.toolBlock(e.CallID)
		if b == nil {
			b = &block{kind: blockTool, tool: e.Tool}
			m.add(b)
		}
		b.result, b.resultErr, b.finished = m.rel(e.Text), e.IsError, true
		b.invalidate()
	case kernel.EventDecision:
		label, meta, t := describeDecision(e.Decision)
		m.add(&block{kind: blockJev, label: label, meta: meta, tone: t})
	case kernel.EventRoute:
		if r := e.Route; r != nil {
			m.routed = e.Effort
			meta := fmt.Sprintf("complexity %.2f", r.Score)
			if h := r.Handling(); h != "" {
				meta += " · " + h
			}
			m.add(&block{kind: blockJev, tone: toneInfo, label: cmp.Or(string(e.Effort), "no") + " effort",
				meta: fmt.Sprintf("%s · %dms", meta, r.LatencyMS)})
		}
	case kernel.EventHeartbeat:
		if h := e.Heartbeat; h != nil && h.Action != checkpoint.Continue {
			m.add(&block{kind: blockJev, tone: toneNotice, label: "heartbeat", meta: h.Rule})
		}
	case kernel.EventTripwire:
		if d := e.Tripwire; d != nil {
			m.add(tripwireBlock(d))
		}
	case kernel.EventCrossCheck:
		m.add(&block{kind: blockJev, tone: toneInfo, label: "cross-check", meta: e.Reason})
	case kernel.EventNudge:
		// The decision that nudged is already shown; only a nudge from
		// elsewhere needs a line of its own.
		if n := len(m.blocks); n == 0 || m.blocks[n-1].kind != blockJev || !strings.HasPrefix(m.blocks[n-1].label, string(checkpoint.Nudge)) {
			m.add(&block{kind: blockNote, tone: toneNotice, text: "nudged to continue · " + e.Reason})
		}
	case kernel.EventConfigure:
		// The session changed its own settings: keep them, so the next
		// change made here starts from them.
		if e.Settings != nil {
			m.settings = *e.Settings
		}
		m.add(&block{kind: blockNote, tone: toneNotice, text: e.Text})
	case kernel.EventRunEnd:
		m.endReply()
		m.reply, m.writing = nil, ""
		for _, b := range m.blocks {
			if b.kind == blockTool && !b.finished {
				b.finished = true
				b.invalidate()
			}
		}
		label, t := outcome(e.Outcome, e.Reason)
		m.add(&block{kind: blockEnd, tone: t, label: label, meta: m.runTime()})
		if m.changed() {
			m.routed = ""
		}
		m.running, m.stopping = false, false
		m.setPlaceholder()
		m.status, m.statusTone = label+" · "+m.runTime(), t
		if e.Usage != nil {
			m.usage = usageLine(*e.Usage)
		}
	case kernel.EventError:
		m.add(&block{kind: blockError, text: e.Text})
	}
	m.refresh()
}

// outcome says how a run ended, without repeating a reason that only
// restates it, and how that reads.
func outcome(o kernel.Outcome, reason string) (string, tone) {
	label, t := "over to you", toneNotice
	switch o {
	case kernel.OutcomeDone:
		label, t = "done", toneOK
	case kernel.OutcomeCancelled:
		label, t = "stopped", toneInfo
	case kernel.OutcomeError:
		label, t = "failed", toneError
	}
	if reason == "" || reason == string(o) || reason == label {
		return label, t
	}
	return label + " · " + reason, t
}

func (m *model) add(b *block) { m.blocks = append(m.blocks, b) }

// runTime is how long the current or last request has taken.
func (m *model) runTime() string { return elapsed(time.Since(m.runStart)) }

// endReply marks the current step's reply, if any, as no longer streaming.
func (m *model) endReply() {
	if b := m.reply; b != nil && b.streaming {
		b.streaming = false
		b.invalidate()
	}
}

// toolBlock is the call a result belongs to.
func (m *model) toolBlock(callID string) *block {
	if callID == "" {
		return nil
	}
	for _, b := range slices.Backward(m.blocks) {
		if b.kind == blockTool && b.callID == callID {
			return b
		}
	}
	return nil
}

// rel shows paths in the project relative to it. Only the project's own
// path counts: a sibling such as /a/project2 next to /a/proj is left alone.
func (m *model) rel(s string) string {
	if m.dirPath == nil {
		return s
	}
	// A match takes the character after the path with it, so two paths a
	// single space apart need a second pass.
	for range 2 {
		s = m.dirPath.ReplaceAllStringFunc(s, func(match string) string {
			sub := m.dirPath.FindStringSubmatch(match)
			if sub[2] == string(filepath.Separator) {
				return sub[1]
			}
			return sub[1] + "." + sub[2]
		})
	}
	return s
}

// say adds a note from Girdle to the transcript.
func (m *model) say(t tone, s string) {
	m.add(&block{kind: blockNote, tone: t, text: s})
	m.refresh()
}

// notify shows the result of an action: in the picker while it is open,
// otherwise in the transcript.
func (m *model) notify(t tone, s string) {
	if m.picker != nil {
		m.picker.note = t.render(s)
		return
	}
	m.say(t, s)
}

// conversation reports whether anything beyond Girdle's own notes has
// happened yet.
func (m *model) conversation() bool {
	return slices.ContainsFunc(m.blocks, func(b *block) bool { return b.kind != blockNote })
}

func (m *model) refresh() {
	if m.width == 0 {
		return
	}
	follow := m.vp.AtBottom()
	width := max(10, m.width-2)
	var content string
	if m.conversation() {
		content = m.transcript(width)
	} else {
		notes := m.transcript(width)
		room := m.vp.Height()
		if notes != "" {
			room -= lipgloss.Height(notes) + 1
		}
		content = m.splash(width, max(1, room))
		if notes != "" {
			content += "\n" + notes
		}
	}
	m.vp.SetContent(indent(content, " "))
	if follow {
		m.vp.GotoBottom()
	}
}

func (m *model) View() tea.View {
	var v tea.View
	if m.picker != nil {
		v = tea.NewView(m.pickerView(m.width, max(8, m.height-1)) + "\n" + m.statusLine())
	} else {
		hint := lipgloss.NewStyle().MaxWidth(max(1, m.width)).Render(faintStyle.Render("  " + m.keyHint()))
		v = tea.NewView(m.vp.View() + "\n" + m.statusLine() + "\n" + m.inputBox() + "\n" + hint)
	}
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// inputBox frames the prompt. Its border lights up once there is something
// to send, and breathes while a request runs.
func (m *model) inputBox() string {
	border := pal.border
	switch {
	case m.running:
		border = pulse(m.frame, 26, pal.border, pal.tool)
	case strings.TrimSpace(m.input.Value()) != "":
		border = pal.user
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(m.width).Render(m.input.View())
}

// activity is the left of the status line: what the running request is
// doing, or how the last one ended. Brief leaves out the detail.
func (m *model) activity(brief bool) string {
	switch {
	case m.stopping:
		return errStyle.Render("◼ stopping…")
	case m.running:
		meta := " · " + m.runTime()
		if m.steps > 0 && !brief {
			meta += fmt.Sprintf(" · step %d", m.steps+1)
		}
		return brandStyle.Render(spinnerFrame(m.frame)) + " " + shimmer(m.phase(), m.frame, pal.muted, pal.fg) + dimStyle.Render(meta)
	case m.status != "":
		s := m.status
		if m.usage != "" && !brief {
			s += " · " + m.usage
		}
		return m.statusTone.style().Render(m.statusTone.icon()) + " " + dimStyle.Render(s)
	}
	return brandStyle.Render("●") + dimStyle.Render(" ready")
}

// phase names what a running request is waiting on.
func (m *model) phase() string {
	for _, b := range slices.Backward(m.blocks) {
		if b.kind == blockTool && !b.finished {
			return "Running " + b.tool
		}
	}
	if m.writing != "" {
		return "Writing " + m.writing
	}
	if m.reply != nil && m.reply.streaming {
		return "Writing"
	}
	return "Thinking"
}

// statusLine shows how the run stands on the left and, as in Pi's footer,
// the model and effort on the right: those of the running request, and any
// change waiting for the next. When they don't fit, the detail goes first.
func (m *model) statusLine() string {
	settings := func(s kernel.Settings, effort string) string {
		return dimStyle.Render(s.ModelName) + faintStyle.Render(" • ") + dimStyle.Render("effort "+effort)
	}
	right := settings(m.settings, m.effortLabel()) + " "
	if m.running && m.changed() {
		right = settings(m.active, effortLabel(m.active, m.routed)) + dimStyle.Render(" → next ") + settings(m.settings, m.effortLabel()) + " "
	}
	if m.picker == nil && !m.vp.AtBottom() {
		right = decisionStyle.Render("↓ more below") + faintStyle.Render(" · ") + right
	}
	for _, brief := range []bool{false, true} {
		left := " " + m.activity(brief)
		if gap := m.width - lipgloss.Width(left) - lipgloss.Width(right); gap >= 1 {
			return left + strings.Repeat(" ", gap) + right
		}
	}
	// Still too wide: cut the activity short, keeping the settings whole
	// while they leave it some room, else cut them too.
	left := " " + m.activity(true)
	if room := m.width - lipgloss.Width(right) - 1; room >= 16 {
		return ansi.Truncate(left, room, "…") + " " + right
	}
	left += "  "
	return left + lipgloss.NewStyle().MaxWidth(max(1, m.width-lipgloss.Width(left))).Render(right)
}

// tripwireBlock shows the tripwire's verdict on a shell command: the code
// floor's, or Jev's when the floor left it to judgement.
func tripwireBlock(d *checkpoint.TripwireDecision) *block {
	b := &block{kind: blockJev, source: "jev", tone: toneInfo, label: "tripwire · allowed", meta: fmt.Sprintf("judged safe · %dms", d.LatencyMS)}
	if d.Rule == "floor" {
		b.source = "tripwire"
	}
	if d.Action == "block" {
		b.tone, b.label, b.meta = toneError, "tripwire · blocked", fmt.Sprintf("%s · %dms", d.Why, d.LatencyMS)
		if d.Rule == "floor" {
			b.label, b.meta = "blocked", d.Why
		}
	}
	return b
}

// describeDecision puts a Jev decision as what it decided, the scores
// behind that, and how it reads. Only the answers Jev gave are shown: each
// checkpoint asks its own questions.
func describeDecision(d *checkpoint.Decision) (string, string, tone) {
	if d == nil {
		return "decision", "", toneInfo
	}
	if d.Error != "" {
		return "unavailable → " + string(d.Action), d.Error, toneError
	}
	t := toneInfo
	switch d.Action {
	case checkpoint.Stop:
		t = toneOK
	case checkpoint.Ask, checkpoint.Nudge:
		t = toneNotice
	}
	var parts []string
	if a, ok := d.Answers["status"]; ok && a.Choice != "" {
		if a.Choice == d.Rule {
			parts = append(parts, fmt.Sprintf("status %.2f", a.Confidence))
		} else {
			parts = append(parts, fmt.Sprintf("status %s %.2f", a.Choice, a.Confidence))
		}
	}
	for _, name := range []string{"complete", "evidence"} {
		if a, ok := d.Answers[name]; ok {
			parts = append(parts, fmt.Sprintf("%s %.2f", name, a.Noul))
		}
	}
	parts = append(parts, fmt.Sprintf("%dms", d.LatencyMS))
	if what := nudgeSummary(d); what != "" {
		parts = append([]string{what}, parts...)
	}
	return fmt.Sprintf("%s · %s", d.Action, d.Rule), strings.Join(parts, " · "), t
}

// nudgeSummary says in a few words what a nudge told the agent: for
// unfinished parts of the task, the parts it listed.
func nudgeSummary(d *checkpoint.Decision) string {
	if d.Action != checkpoint.Nudge {
		return ""
	}
	switch d.Rule {
	case "verify":
		return "asked it to check its work"
	case "continue":
		return "asked it to carry on"
	case "go_ahead":
		return "told it to go ahead"
	case "coverage":
		var missing []string
		for line := range strings.Lines(d.Nudge) {
			if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
				missing = append(missing, "“"+item+"”")
			}
		}
		if len(missing) > 0 {
			return "not done yet: " + strings.Join(missing, ", ")
		}
	}
	return ""
}

// toolArg shows the argument people care about: the command or the path.
func toolArg(input string) string {
	if in, ok := tools.ParseApplyInput(input); ok && len(in.Changes) > 0 {
		return firstLine(strings.Join(in.Paths(), ", ") + " · check: " + in.Check)
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
		return firstLine(strings.Join(parts, " · "))
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return firstLine(input)
	}
	for _, k := range []string{"command", "path"} {
		if v, ok := args[k].(string); ok {
			return firstLine(v)
		}
	}
	return firstLine(input)
}

func firstLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 1 {
		return lines[0] + fmt.Sprintf(" … (%d lines)", len(lines))
	}
	return lines[0]
}

func usageLine(u kernel.Usage) string {
	return fmt.Sprintf("%dk in / %dk out · jev %d tok", u.InputTokens/1000, u.OutputTokens/1000, u.JevTokens)
}
