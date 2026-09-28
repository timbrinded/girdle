package tui

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/timbrinded/girdle/internal/models"
)

// The picker lists the models the user's key can use, or the reasoning
// efforts the model in use accepts, for the user to choose from. Choosing
// acts through models.go and effort.go; pickerview.go draws it.

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
	// available counts the models the key can use that take tools.
	available int
	note      string
}

// openPicker opens the picker in mode, with query already typed. The
// prompt is blurred meanwhile, so nothing typed or pasted reaches it.
func (m *model) openPicker(mode pickerMode, query string) tea.Cmd {
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "type to search"
	in.SetValue(query)
	in.CursorEnd()
	m.picker = &picker{mode: mode, search: in}
	m.input.Blur()
	m.sizePicker()
	m.refind(true)
	if query == "" || mode == pickEffort {
		m.picker.moveTo(m.currentChoice())
	}
	return m.picker.search.Focus()
}

// closePicker closes the picker and gives the prompt back its focus.
func (m *model) closePicker() tea.Cmd {
	m.picker = nil
	return m.input.Focus()
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
	switch p.mode {
	case pickEffort:
		p.rows = nil
		for _, c := range m.effortChoices() {
			p.rows = append(p.rows, row{id: c})
		}
	default:
		p.rows, p.available = m.modelRows(strings.TrimSpace(p.search.Value()))
	}
	if top || !p.moveTo(was) {
		p.cursor = 0
		p.step(0)
	}
}

// modelRows lists the user's models and then every other model the key can
// use that takes tool calls, since Girdle can't work without them. A query
// ranks them all together instead. available counts the models the key can
// use that take tools.
func (m *model) modelRows(query string) (rows []row, available int) {
	cat := m.models.Catalog
	// Yours are the saved ones, and the model in use until it is saved.
	var yours []string
	for _, e := range m.models.List.Models {
		yours = append(yours, e.ID)
	}
	if !slices.Contains(yours, m.settings.ModelName) {
		yours = append([]string{m.settings.ModelName}, yours...)
	}
	var others []models.Info
	for _, info := range cat.Models {
		if !info.Tools {
			continue
		}
		available++
		if !slices.Contains(yours, info.ID) {
			others = append(others, info)
		}
	}
	// By provider, newest first within each.
	slices.SortFunc(others, func(a, b models.Info) int {
		return cmp.Or(strings.Compare(provider(a.ID), provider(b.ID)), cmp.Compare(b.Created, a.Created), strings.Compare(a.ID, b.ID))
	})
	if query != "" {
		var cands []candidate
		for _, id := range yours {
			info, _ := cat.Lookup(id)
			cands = append(cands, candidate{id: id, name: info.Name, yours: true, created: info.Created})
		}
		for _, info := range others {
			cands = append(cands, candidate{id: info.ID, name: info.Name, created: info.Created})
		}
		for _, c := range search(query, cands) {
			rows = append(rows, row{id: c.id})
		}
		return rows, available
	}
	rows = []row{{header: "Your models"}}
	for _, id := range yours {
		rows = append(rows, row{id: id})
	}
	if len(others) > 0 {
		rows = append(rows, row{header: "All models"})
		for _, info := range others {
			rows = append(rows, row{id: info.ID})
		}
	}
	return rows, available
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
		return m.closePicker()
	case "enter":
		id := p.selected()
		if id == "" {
			// With no catalogue to search, an ID is taken as typed.
			id = strings.TrimSpace(p.search.Value())
			switch {
			case len(m.models.Catalog.Models) > 0 || id == "":
				p.note = "no model your key can use matches that"
				return nil
			case strings.ContainsFunc(id, unicode.IsSpace):
				p.note = "a model ID has no spaces, such as meta/muse-spark-1.3"
				return nil
			}
		}
		if m.useModel(id) {
			return m.closePicker()
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
			return m.closePicker()
		}
		p.mode, p.note = pickModel, ""
		m.refind(true)
		p.moveTo(m.settings.ModelName)
	case "ctrl+l":
		return m.closePicker()
	case "enter":
		// Closed first, so the result shows in the transcript.
		if s := p.selected(); s != "" {
			cmd := m.closePicker()
			m.setEffort(s)
			return cmd
		}
	}
	return nil
}
