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
	"github.com/timbrinded/girdle/internal/models"
)

// Models gives the TUI a picker over the user's OpenRouter models.
type Models struct {
	Store   models.Store
	List    models.List
	Catalog models.Catalog
	// Note explains why the session didn't start on the default model.
	Note string
	// Open builds a model by its OpenRouter ID, and Fetch downloads
	// OpenRouter's catalogue.
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		c, err := m.models.Fetch(ctx)
		return catalogMsg{c, err}
	}
}

// takeCatalog keeps a fresh catalogue and acts on what it says: a model in
// use that OpenRouter no longer lists gives way to the most recently picked
// model it still does.
func (m *model) takeCatalog(msg catalogMsg) {
	if msg.err != nil || len(msg.catalog.Models) == 0 {
		if len(m.models.Catalog.Models) == 0 {
			m.appendLine(dimStyle.Render("◇ couldn't load OpenRouter's model list, so models can't be checked or searched: " + cmp.Or(fmt.Sprint(msg.err), "it was empty")))
		}
		return
	}
	m.models.Catalog = msg.catalog
	if err := m.models.Store.SaveCatalog(msg.catalog); err != nil {
		m.appendLine(errStyle.Render("✗ caching OpenRouter's model list: " + err.Error()))
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
		m.appendLine(errStyle.Render("✗ " + current + " is no longer listed on OpenRouter. Press ctrl+l to add another model."))
		return
	}
	if m.useModel(next) {
		m.appendLine(decisionStyle.Render(fmt.Sprintf("◇ %s is no longer listed on OpenRouter, so %s, picked most recently, takes over", current, next)))
	}
}

type pickerMode int

const (
	choosing pickerMode = iota // choosing from the user's list
	adding                     // searching OpenRouter's catalogue to add to it
)

// picker is the open model picker.
type picker struct {
	mode   pickerMode
	cursor int
	search textinput.Model
	found  []models.Info // catalogue matches while adding
	note   string        // the result of the last action
}

func (m *model) openPicker() {
	in := textinput.New()
	in.Prompt = "search › "
	in.Placeholder = "part of a model's ID or name"
	m.picker = &picker{search: in}
	m.picker.cursor = max(0, m.models.List.Index(m.settings.ModelName))
}

// updatePicker handles a key while the picker is open.
func (m *model) updatePicker(msg tea.KeyPressMsg) tea.Cmd {
	p := m.picker
	if p.mode == adding {
		return m.updateAdding(msg)
	}
	list := m.models.List.Models
	switch msg.String() {
	case "esc", "ctrl+l":
		m.picker = nil
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = min(len(list)-1, p.cursor+1)
	case "enter":
		if len(list) > 0 && m.useModel(list[p.cursor].ID) {
			m.picker = nil
		}
	case "a":
		p.mode, p.cursor, p.note = adding, 0, ""
		p.search.Reset()
		m.refind()
		return p.search.Focus()
	case "x", "delete":
		if len(list) > 0 {
			m.removeModel(list[p.cursor].ID)
			p.cursor = min(p.cursor, len(m.models.List.Models)-1)
		}
	case "d":
		m.saveDefault()
	case "shift+tab":
		m.cycleEffort()
	}
	return nil
}

func (m *model) updateAdding(msg tea.KeyPressMsg) tea.Cmd {
	p := m.picker
	switch msg.String() {
	case "esc":
		p.mode, p.note = choosing, ""
		p.cursor = max(0, m.models.List.Index(m.settings.ModelName))
		return nil
	case "up":
		p.cursor = max(0, p.cursor-1)
		return nil
	case "down":
		p.cursor = min(len(p.found)-1, p.cursor+1)
		return nil
	case "enter":
		id := strings.TrimSpace(p.search.Value())
		switch {
		case len(p.found) > 0:
			id = p.found[p.cursor].ID
		case len(m.models.Catalog.Models) > 0 || id == "":
			p.note = "no tool-capable model on OpenRouter matches that"
			return nil
		}
		// Without a catalogue, the ID is taken as typed.
		m.saveList(func(l *models.List) { l.Add(id) })
		p.mode, p.cursor = choosing, m.models.List.Index(id)
		p.note = "added " + id + ": press enter to use it"
		return nil
	}
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	m.refind()
	return cmd
}

// refind searches the catalogue for tool-capable models matching every word
// typed, since Girdle can't work without tools.
func (m *model) refind() {
	p := m.picker
	words := strings.Fields(strings.ToLower(p.search.Value()))
	p.found = p.found[:0]
	for _, info := range m.models.Catalog.Models {
		text := strings.ToLower(info.ID + " " + info.Name)
		if info.Tools && !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(text, w) }) {
			p.found = append(p.found, info)
		}
	}
	p.cursor = min(p.cursor, max(0, len(p.found)-1))
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

// removeModel takes id off the list. If it was in use, the most recently
// picked of the rest takes over.
func (m *model) removeModel(id string) {
	if len(m.models.List.Models) == 1 {
		m.notify(dimStyle.Render("the list needs one model: add another before removing this one"))
		return
	}
	m.saveList(func(l *models.List) { l.Remove(id) })
	m.notify(dimStyle.Render("removed " + id))
	if id != m.settings.ModelName {
		return
	}
	cat := m.models.Catalog
	next, ok := m.models.List.Latest(func(id string) bool { return !cat.Gone(id) })
	if !ok {
		next, _ = m.models.List.Latest(func(string) bool { return true })
	}
	m.useModel(next)
}

// saveDefault makes the current model and effort setting what new sessions
// start with.
func (m *model) saveDefault() {
	d := models.Defaults{Model: m.settings.ModelName, Effort: m.effortSetting()}
	m.saveList(func(l *models.List) {
		l.Add(d.Model)
		l.Default = d
	})
	m.notify(dimStyle.Render(fmt.Sprintf("new sessions start on %s at effort %s", d.Model, d.Effort)))
}

// saveList changes the saved list. If it can't be saved, the change still
// holds for this session.
func (m *model) saveList(change func(*models.List)) {
	l, err := m.models.Store.Update(change)
	if err != nil {
		change(&m.models.List)
		m.notify(errStyle.Render("✗ saving the model list: " + err.Error()))
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

// cycleEffort steps the effort setting through auto, when Jev can route,
// then each effort the model accepts, lowest first.
func (m *model) cycleEffort() {
	efforts := m.settings.Efforts
	if len(efforts) == 0 {
		m.notify(dimStyle.Render(m.settings.ModelName + " has no reasoning effort setting"))
		return
	}
	var choices []string
	if m.canRoute {
		choices = append(choices, models.EffortAuto)
	}
	for _, e := range efforts {
		choices = append(choices, string(e))
	}
	// A pinned effort the model doesn't accept steps on from the one it
	// is fitted to.
	current := models.EffortAuto
	if !m.settings.AutoEffort {
		current = string(m.settings.Effort.Fit(efforts))
	}
	next := choices[(slices.Index(choices, current)+1)%len(choices)]
	e, auto, _ := models.ParseEffort(next)
	m.settings.AutoEffort = auto
	if !auto {
		m.settings.Effort = e
	}
	m.configure()
	// The status line shows the new effort. A run under way keeps its own.
	if m.running {
		m.notify(dimStyle.Render("◇ effort " + m.effortLabel() + m.fromNext()))
	}
}

// configure hands the settings to the session for its next request.
func (m *model) configure() {
	m.routed = ""
	m.sess.Configure(m.settings)
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

// effortLabel describes the effort requests use: auto, with the effort Jev
// chose for the latest request, or the pinned effort, and what it is
// fitted to when the model doesn't accept it.
func (m *model) effortLabel() string {
	if len(m.settings.Efforts) == 0 {
		return "n/a"
	}
	if m.settings.AutoEffort && m.canRoute {
		if m.routed != "" {
			return "auto → " + string(m.routed)
		}
		return "auto"
	}
	e := m.settings.Effort
	if fit := e.Fit(m.settings.Efforts); fit != e {
		return fmt.Sprintf("%s (asked %s)", fit, e)
	}
	return string(e)
}

var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4CC2B5"))
	titleStyle    = lipgloss.NewStyle().Bold(true)
)

// pickerView draws the picker in the space the transcript uses.
func (m *model) pickerView(height int) string {
	p := m.picker
	var head []string
	var rows []string
	var keys string
	if p.mode == adding {
		n := 0
		for _, info := range m.models.Catalog.Models {
			if info.Tools {
				n++
			}
		}
		title := fmt.Sprintf("Add a model · %d on OpenRouter take tool calls", n)
		if n == 0 {
			title = "Add a model · OpenRouter's list isn't loaded, so type an exact ID"
		}
		head = []string{titleStyle.Render(title), p.search.View(), ""}
		for i, info := range p.found {
			rows = append(rows, m.row(i == p.cursor, info.ID, "", m.describe(info.ID)))
		}
		keys = "enter add · ↑/↓ choose · esc back"
	} else {
		head = []string{titleStyle.Render("Models · OpenRouter") + dimStyle.Render("   ● in use · ★ default"), ""}
		for i, e := range m.models.List.Models {
			mark := "  "
			if e.ID == m.settings.ModelName {
				mark = "● "
			}
			if e.ID == m.models.List.Default.Model {
				mark += "★ "
			} else {
				mark += "  "
			}
			rows = append(rows, m.row(i == p.cursor, e.ID, mark, m.describe(e.ID)))
		}
		keys = "enter use · a add · x remove · d make default · shift+tab effort (" + m.effortLabel() + ") · esc close"
	}
	foot := []string{"", dimStyle.Render(keys)}
	if p.note != "" {
		foot = append(foot, p.note)
	}
	// Scroll so the cursor stays in view.
	room := max(1, height-len(head)-len(foot))
	start := max(0, min(p.cursor-room+1, len(rows)-room))
	rows = rows[start:min(len(rows), start+room)]
	lines := slices.Concat(head, rows, foot)
	for len(lines) < height {
		lines = slices.Insert(lines, len(head)+len(rows), "")
	}
	return strings.Join(lines, "\n")
}

// row draws one model: its ID, then what OpenRouter says about it.
func (m *model) row(selected bool, id, mark, about string) string {
	cursor, style := "  ", lipgloss.NewStyle()
	if selected {
		cursor, style = "› ", selectedStyle
	}
	line := cursor + mark + style.Render(fmt.Sprintf("%-40s", id)) + "  " + dimStyle.Render(about)
	return lipgloss.NewStyle().MaxWidth(max(20, m.width)).Render(line)
}

// describe summarises what the catalogue says about a model: price,
// context, reasoning efforts and any withdrawal.
func (m *model) describe(id string) string {
	info, ok := m.models.Catalog.Lookup(id)
	switch {
	case m.models.Catalog.Gone(id):
		return "no longer listed on OpenRouter"
	case !ok:
		return "not checked against OpenRouter's list"
	}
	var parts []string
	switch {
	case info.Input < 0:
		parts = append(parts, "price varies")
	case info.Input == 0 && info.Output == 0:
		parts = append(parts, "free")
	default:
		parts = append(parts, fmt.Sprintf("$%.2f in · $%.2f out per M", info.Input, info.Output))
	}
	if info.Context > 0 {
		parts = append(parts, contextSize(info.Context))
	}
	switch {
	case slices.Equal(info.Efforts, checkpoint.Efforts):
		parts = append(parts, "any effort")
	case len(info.Efforts) > 0:
		efforts := make([]string, len(info.Efforts))
		for i, e := range info.Efforts {
			efforts[i] = string(e)
		}
		parts = append(parts, "effort "+strings.Join(efforts, "/"))
	default:
		parts = append(parts, "no effort setting")
	}
	if !info.Tools {
		parts = append(parts, "no tool calls")
	}
	if info.Expires != "" {
		parts = append(parts, "withdrawn on "+info.Expires)
	}
	return strings.Join(parts, " · ")
}

// contextSize reads 1048576 tokens as 1M and 262144 as 262k.
func contextSize(tokens int) string {
	if tokens >= 1_000_000 {
		return strconv.FormatFloat(math.Round(float64(tokens)/1e5)/10, 'f', -1, 64) + "M context"
	}
	return fmt.Sprintf("%dk context", tokens/1000)
}
