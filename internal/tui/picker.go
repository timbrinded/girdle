package tui

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"
	"charm.land/lipgloss/v2"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/models"
)

// Models gives the TUI a picker over OpenRouter's models.
type Models struct {
	Store models.Store
	// List is the user's models: the ones picked or added, which ctrl+p
	// cycles through.
	List    models.List
	Catalog models.Catalog
	// Note explains why the session didn't start on the default model.
	Note string
	// Open builds a model by its OpenRouter ID, and Fetch downloads the
	// catalogue of models the user's key can use.
	Open  func(context.Context, string) (fantasy.LanguageModel, error)
	Fetch func(context.Context) (models.Catalog, error)
}

// startPicker notes why the session didn't start on the default model, if
// it didn't, and records the model in use as picked, so removing another
// model never promotes one past it.
func (m *model) startPicker() {
	if m.models.Note != "" {
		m.lines = append(m.lines, decisionStyle.Render("◇ "+m.models.Note))
	}
	m.saveList(func(l *models.List) { l.Pick(m.settings.ModelName, time.Now()) })
}

// catalogMsg delivers a freshly fetched catalogue.
type catalogMsg struct {
	catalog models.Catalog
	err     error
}

// fetchCatalog refreshes the catalogue in the background.
func (m *model) fetchCatalog() tea.Cmd {
	if m.models == nil || m.models.Fetch == nil {
		return nil
	}
	m.fetching = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		c, err := m.models.Fetch(ctx)
		return catalogMsg{c, err}
	}
}

// takeCatalog keeps a fresh catalogue and acts on what it says: a model in
// use that the user's key can no longer use gives way to the most recently
// picked model it can.
func (m *model) takeCatalog(msg catalogMsg) {
	m.fetching = false
	if msg.err == nil && len(msg.catalog.Models) == 0 {
		msg.err = fmt.Errorf("the list was empty")
	}
	if msg.err != nil {
		m.fetchErr = msg.err.Error()
		if len(m.models.Catalog.Models) == 0 {
			m.notify(dimStyle.Render("◇ couldn't load OpenRouter's models, so they can't be listed or checked: " + m.fetchErr))
		}
		return
	}
	m.fetchErr = ""
	m.models.Catalog = msg.catalog
	if err := m.models.Store.SaveCatalog(msg.catalog); err != nil {
		m.appendLine(errStyle.Render("✗ caching OpenRouter's models: " + err.Error()))
	}
	if m.picker != nil {
		m.refind(false)
	}
	current := m.settings.ModelName
	if !msg.catalog.Gone(current) {
		// Efforts can change as providers come and go.
		if efforts := msg.catalog.Efforts(current); !slices.Equal(efforts, m.settings.Efforts) {
			m.settings.Efforts = efforts
			m.configure()
		}
		return
	}
	next, ok := m.models.List.Latest(func(id string) bool { return !msg.catalog.Gone(id) })
	if !ok {
		m.appendLine(errStyle.Render("✗ " + current + " isn't available to your OpenRouter key. Press ctrl+l to pick another model."))
		return
	}
	if m.useModel(next) {
		m.appendLine(decisionStyle.Render(fmt.Sprintf("◇ %s isn't available to your OpenRouter key any more, so %s, picked most recently, takes over", current, next)))
	}
}

type pickerMode int

const (
	pickModel  pickerMode = iota // every model the key can use
	pickEffort                   // the reasoning efforts the model in use accepts
)

// A row of the picker is a choice, a model ID or an effort setting, or a
// section header.
type row struct {
	header string
	id     string
}

// picker is the open model or effort picker.
type picker struct {
	mode   pickerMode
	search textinput.Model
	rows   []row
	cursor int // index into rows, always on a choice
	// fromModels is set when the effort list was opened from the model
	// list, which tab and esc go back to.
	fromModels bool
	note       string
}

// openPicker opens the picker in mode, with query already typed.
func (m *model) openPicker(mode pickerMode, query string) tea.Cmd {
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "type to search"
	in.SetValue(query)
	in.CursorEnd()
	m.picker = &picker{mode: mode, search: in}
	m.sizePicker()
	m.refind(true)
	if query == "" || mode == pickEffort {
		m.picker.moveTo(m.currentChoice())
	}
	return m.picker.search.Focus()
}

// sizePicker fits the search box to the panel: its border, padding and
// prompt take six columns.
func (m *model) sizePicker() {
	m.picker.search.SetWidth(max(10, m.width-6))
}

// currentChoice is the row that stands for what is in use.
func (m *model) currentChoice() string {
	if m.picker.mode == pickEffort {
		return m.effortSetting()
	}
	return m.settings.ModelName
}

// refind rebuilds the rows. With top, the cursor goes to the first choice,
// as after typing; otherwise it stays on the same choice if it can.
func (m *model) refind(top bool) {
	p := m.picker
	was := p.selected()
	p.rows = p.rows[:0]
	switch p.mode {
	case pickEffort:
		for _, c := range m.effortChoices() {
			p.rows = append(p.rows, row{id: c})
		}
	default:
		p.rows = m.modelRows(strings.TrimSpace(p.search.Value()))
	}
	if top || !p.moveTo(was) {
		p.cursor = 0
		p.step(0)
	}
}

// modelRows lists the user's models and then every other model the key can
// use that takes tool calls, since Girdle can't work without them. A query
// ranks them all together instead.
func (m *model) modelRows(query string) []row {
	list, cat := m.models.List, m.models.Catalog
	var others []models.Info
	for _, info := range cat.Models {
		if info.Tools && !list.Has(info.ID) {
			others = append(others, info)
		}
	}
	// By provider, newest first within each.
	slices.SortFunc(others, func(a, b models.Info) int {
		return cmp.Or(strings.Compare(provider(a.ID), provider(b.ID)), cmp.Compare(b.Created, a.Created), strings.Compare(a.ID, b.ID))
	})
	if query != "" {
		var cands []candidate
		for _, e := range list.Models {
			info, _ := cat.Lookup(e.ID)
			cands = append(cands, candidate{id: e.ID, name: info.Name, yours: true, created: info.Created})
		}
		for _, info := range others {
			cands = append(cands, candidate{id: info.ID, name: info.Name, created: info.Created})
		}
		var rows []row
		for _, c := range search(query, cands) {
			rows = append(rows, row{id: c.id})
		}
		return rows
	}
	rows := []row{{header: "Your models"}}
	for _, e := range list.Models {
		rows = append(rows, row{id: e.ID})
	}
	if len(others) > 0 {
		rows = append(rows, row{header: "All models"})
		for _, info := range others {
			rows = append(rows, row{id: info.ID})
		}
	}
	return rows
}

// provider is the part of a model ID before the slash, without the tilde
// OpenRouter puts on aliases such as ~anthropic/claude-opus-latest.
func provider(id string) string {
	p, _, _ := strings.Cut(strings.TrimPrefix(id, "~"), "/")
	return p
}

// moveTo puts the cursor on choice id, if there is one.
func (p *picker) moveTo(id string) bool {
	i := slices.IndexFunc(p.rows, func(r row) bool { return r.header == "" && r.id == id })
	if i >= 0 {
		p.cursor = i
	}
	return i >= 0
}

// step moves the cursor by n choices, skipping headers and stopping at the
// ends.
func (p *picker) step(n int) {
	choices := p.choices()
	if len(choices) == 0 {
		p.cursor = 0
		return
	}
	i := max(0, slices.Index(choices, p.cursor))
	p.cursor = choices[min(len(choices)-1, max(0, i+n))]
}

// choices are the indexes of the rows that aren't headers.
func (p *picker) choices() []int {
	var out []int
	for i, r := range p.rows {
		if r.header == "" {
			out = append(out, i)
		}
	}
	return out
}

// selected is the choice under the cursor, or "" if there is none.
func (p *picker) selected() string {
	if p.cursor < len(p.rows) && p.rows[p.cursor].header == "" {
		return p.rows[p.cursor].id
	}
	return ""
}

// pageSize is how far pgup and pgdown move.
const pageSize = 10

// updatePicker handles a key while the picker is open.
func (m *model) updatePicker(msg tea.KeyPressMsg) tea.Cmd {
	p := m.picker
	switch msg.String() {
	case "up", "ctrl+p":
		p.step(-1)
		return nil
	case "down", "ctrl+n":
		p.step(1)
		return nil
	case "pgup":
		p.step(-pageSize)
		return nil
	case "pgdown":
		p.step(pageSize)
		return nil
	case "shift+tab":
		m.cycleEffort()
		if p.mode == pickEffort {
			m.refind(false)
			p.moveTo(m.effortSetting())
		}
		return nil
	}
	if p.mode == pickEffort {
		return m.updateEffortPicker(msg)
	}
	switch msg.String() {
	case "esc", "ctrl+l":
		m.picker = nil
	case "enter":
		id := p.selected()
		if id == "" {
			// With no catalogue to search, an ID is taken as typed.
			id = strings.TrimSpace(p.search.Value())
			if len(m.models.Catalog.Models) > 0 || id == "" {
				p.note = "no model your key can use matches that"
				return nil
			}
		}
		if m.useModel(id) {
			m.picker = nil
		}
	case "tab":
		p.mode, p.fromModels, p.note = pickEffort, true, ""
		m.refind(true)
		p.moveTo(m.effortSetting())
	case "ctrl+s":
		if id := p.selected(); id != "" {
			m.saveDefault(id)
		}
	case "ctrl+f":
		if id := p.selected(); id != "" {
			m.toggleYours(id)
		}
	default:
		before := p.search.Value()
		var cmd tea.Cmd
		p.search, cmd = p.search.Update(msg)
		if p.search.Value() != before {
			p.note = ""
			m.refind(true)
		}
		return cmd
	}
	return nil
}

func (m *model) updateEffortPicker(msg tea.KeyPressMsg) tea.Cmd {
	p := m.picker
	switch msg.String() {
	case "esc", "tab":
		if !p.fromModels {
			m.picker = nil
			return nil
		}
		p.mode, p.note = pickModel, ""
		m.refind(true)
		p.moveTo(m.settings.ModelName)
	case "ctrl+l":
		m.picker = nil
	case "enter":
		// Closed first, so the result shows in the transcript.
		if s := p.selected(); s != "" {
			m.picker = nil
			m.setEffort(s)
		}
	}
	return nil
}

// useModel switches the session to model id from its next request, and
// records that it was picked. It reports whether the switch was made.
func (m *model) useModel(id string) bool {
	lm, err := m.models.Open(m.ctx, id)
	if err != nil {
		m.notify(errStyle.Render("✗ " + id + ": " + err.Error()))
		return false
	}
	m.settings.Model, m.settings.ModelName, m.settings.Efforts = lm, id, m.models.Catalog.Efforts(id)
	m.configure()
	m.saveList(func(l *models.List) { l.Pick(id, time.Now()) })
	m.appendLine(dimStyle.Render(fmt.Sprintf("◇ model %s · effort %s%s", id, m.effortLabel(), m.fromNext())))
	return true
}

// nextModel switches to the model after the current one on the user's
// list, wrapping round.
func (m *model) nextModel() {
	list := m.models.List.Models
	if len(list) < 2 {
		m.notify(dimStyle.Render("◇ ctrl+p cycles through your models: add some with ctrl+l, then enter or ctrl+f"))
		return
	}
	i := m.models.List.Index(m.settings.ModelName)
	m.useModel(list[(i+1)%len(list)].ID)
}

// toggleYours adds id to the user's models, or removes it. Removing the
// model in use promotes the most recently picked of the rest.
func (m *model) toggleYours(id string) {
	if !m.models.List.Has(id) {
		m.saveList(func(l *models.List) { l.Add(id) })
		m.notify(dimStyle.Render("added " + id + " to your models"))
		m.refind(false)
		return
	}
	if len(m.models.List.Models) == 1 {
		m.notify(dimStyle.Render("your models need at least one: add another before removing this one"))
		return
	}
	m.saveList(func(l *models.List) { l.Remove(id) })
	m.notify(dimStyle.Render("removed " + id + " from your models"))
	if id == m.settings.ModelName {
		cat := m.models.Catalog
		next, ok := m.models.List.Latest(func(id string) bool { return !cat.Gone(id) })
		if !ok {
			next, _ = m.models.List.Latest(func(string) bool { return true })
		}
		if m.useModel(next) {
			m.notify(dimStyle.Render("removed " + id + " from your models, so " + next + ", picked most recently, is in use"))
		}
	}
	// The removed model drops into the long list; stay with yours.
	m.refind(false)
	m.picker.moveTo(m.settings.ModelName)
}

// saveDefault makes id and the current effort setting what new sessions
// start with.
func (m *model) saveDefault(id string) {
	d := models.Defaults{Model: id, Effort: m.effortSetting()}
	m.saveList(func(l *models.List) {
		l.Add(d.Model)
		l.Default = d
	})
	m.notify(dimStyle.Render(fmt.Sprintf("new sessions start on %s at effort %s", d.Model, d.Effort)))
	m.refind(false)
}

// saveList changes the saved list. If it can't be saved, the change still
// holds for this session.
func (m *model) saveList(change func(*models.List)) {
	l, err := m.models.Store.Update(change)
	if err != nil {
		change(&m.models.List)
		m.notify(errStyle.Render("✗ saving your models: " + err.Error()))
		return
	}
	m.models.List = l
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

// effortChoices are the effort settings for the model in use: auto, when
// Jev can route, then each effort the model accepts, lowest first.
func (m *model) effortChoices() []string {
	efforts := m.settings.Efforts
	var choices []string
	if m.canRoute && len(efforts) > 0 {
		choices = append(choices, models.EffortAuto)
	}
	for _, e := range efforts {
		choices = append(choices, string(e))
	}
	return choices
}

// cycleEffort steps the effort setting on to the next choice.
func (m *model) cycleEffort() {
	choices := m.effortChoices()
	if len(choices) == 0 {
		m.notify(dimStyle.Render("◇ " + m.settings.ModelName + " has no reasoning effort setting"))
		return
	}
	// A pinned effort the model doesn't accept steps on from the one it
	// is fitted to.
	current := models.EffortAuto
	if !m.settings.AutoEffort || !m.canRoute {
		current = string(m.settings.Effort.Fit(m.settings.Efforts))
	}
	m.applyEffort(choices[(slices.Index(choices, current)+1)%len(choices)])
	if m.running {
		m.notify(dimStyle.Render("◇ effort " + m.effortLabel() + m.fromNext()))
	}
}

// setEffort applies an effort setting typed or picked by the user, and
// reports whether it was valid.
func (m *model) setEffort(setting string) bool {
	e, auto, err := models.ParseEffort(setting)
	switch {
	case err != nil:
		m.notify(errStyle.Render("✗ " + err.Error()))
		return false
	case len(m.settings.Efforts) == 0:
		m.notify(dimStyle.Render("◇ " + m.settings.ModelName + " has no reasoning effort setting"))
		return false
	case auto && !m.canRoute:
		m.notify(dimStyle.Render("◇ auto needs Jev, which is off: pick an effort instead"))
		return false
	}
	m.applyEffort(setting)
	if !auto && e.Fit(m.settings.Efforts) != e {
		m.notify(dimStyle.Render(fmt.Sprintf("◇ %s doesn't take %s, so it gets %s", m.settings.ModelName, e, m.effortLabel())))
		return true
	}
	m.notify(dimStyle.Render("◇ effort " + m.effortLabel() + m.fromNext()))
	return true
}

func (m *model) applyEffort(setting string) {
	e, auto, _ := models.ParseEffort(setting)
	m.settings.AutoEffort = auto
	if !auto {
		m.settings.Effort = e
	}
	m.configure()
}

// configure hands the settings to the session for its next request. Jev's
// latest route belongs to the old settings, unless a request is still
// running on them.
func (m *model) configure() {
	if !m.running {
		m.routed = ""
	}
	m.sess.Configure(m.settings)
}

// changed reports that the next request's settings differ from those the
// running request started with.
func (m *model) changed() bool {
	a, s := m.active, m.settings
	return a.ModelName != s.ModelName || a.AutoEffort != s.AutoEffort || a.Effort != s.Effort
}

// fromNext notes that a change waits for the running request to end.
func (m *model) fromNext() string {
	if m.running {
		return " (from the next request)"
	}
	return ""
}

// effortSetting is the setting as saved: auto or an effort.
func (m *model) effortSetting() string {
	if m.settings.AutoEffort && m.canRoute {
		return models.EffortAuto
	}
	return string(m.settings.Effort)
}

// effortLabel describes the effort the next request uses.
func (m *model) effortLabel() string {
	if m.running && m.changed() {
		return m.labelFor(m.settings, "")
	}
	return m.labelFor(m.settings, m.routed)
}

// labelFor describes the effort requests on s use: auto, with the effort
// Jev routed the latest one to, or the pinned effort, and what it is fitted
// to when the model doesn't accept it.
func (m *model) labelFor(s kernel.Settings, routed checkpoint.Effort) string {
	if len(s.Efforts) == 0 {
		return "n/a"
	}
	if s.AutoEffort && m.canRoute {
		if routed != "" {
			return "auto → " + string(routed)
		}
		return "auto"
	}
	e := s.Effort
	if fit := e.Fit(s.Efforts); fit != e {
		return fmt.Sprintf("%s (asked %s)", fit, e)
	}
	return string(e)
}

// effortAbout says what each effort setting means.
var effortAbout = map[string]string{
	models.EffortAuto: "Jev chooses for each request",
	"none":            "no reasoning",
	"minimal":         "the least reasoning, for the fastest answers",
	"low":             "quick reasoning, for simple steps",
	"medium":          "balanced",
	"high":            "thorough reasoning",
	"xhigh":           "extra thorough",
	"max":             "as much reasoning as the model offers",
}

var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4CC2B5"))
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E3AE5B"))
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#5A6660")).Padding(0, 1)
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
		// The ID column fits the longest ID shown, up to half the width.
		col := 0
		for _, r := range shown {
			col = max(col, len(m.rowName(r)))
		}
		col = min(col, max(12, width/2))
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

// drawRow draws a header, a model with its name in col columns, or an
// effort setting.
func (m *model) drawRow(r row, selected bool, col int) string {
	if r.header != "" {
		return headerStyle.Render(r.header)
	}
	cursor, style := "  ", lipgloss.NewStyle()
	if selected {
		cursor, style = "› ", selectedStyle
	}
	mark := "  "
	if r.id == m.currentChoice() {
		mark = "✓ "
	}
	if m.picker.mode == pickEffort {
		about := effortAbout[r.id]
		if r.id == models.EffortAuto && m.routed != "" {
			about += ", latest " + string(m.routed)
		}
		return cursor + mark + style.Render(fmt.Sprintf("%-8s", r.id)) + "  " + dimStyle.Render(about)
	}
	name := m.rowName(r)
	if len(name) > col {
		name = name[:col-1] + "…"
	}
	return cursor + mark + style.Render(fmt.Sprintf("%-*s", col, name)) + "  " + dimStyle.Render(m.facts(r.id))
}

// catalogSummary says where the list comes from.
func (m *model) catalogSummary() string {
	cat := m.models.Catalog
	n := 0
	for _, info := range cat.Models {
		if info.Tools {
			n++
		}
	}
	switch {
	case n == 0 && m.fetching:
		return "loading OpenRouter's models"
	case n == 0:
		return "OpenRouter's models aren't loaded"
	case cat.ForKey:
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
	case m.models.Catalog.Gone(id):
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
	if len(info.Efforts) == 0 {
		parts = append(parts, "no effort")
	} else {
		parts = append(parts, effortRange(info.Efforts))
	}
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
	if len(info.Efforts) > 0 {
		efforts := make([]string, len(info.Efforts))
		for i, e := range info.Efforts {
			efforts[i] = string(e)
		}
		parts = append(parts, "effort "+strings.Join(efforts, ", "))
	} else {
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
	var prefix string
	if len(efforts) > 0 && efforts[0] == checkpoint.EffortNone {
		prefix, efforts = "off, ", efforts[1:]
	}
	switch {
	case len(efforts) == 0 && prefix == "":
		return "no effort setting"
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
	names := make([]string, len(efforts))
	for i, e := range efforts {
		names[i] = string(e)
	}
	return prefix + strings.Join(names, "/")
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
