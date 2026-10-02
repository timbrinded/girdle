package tui

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"

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

// needModels reports whether models can be chosen, and says why not when
// they can't.
func (m *model) needModels() bool {
	if m.models == nil {
		m.say(toneInfo, "choosing a model works with OpenRouter only")
	}
	return m.models != nil
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
			m.notify(toneInfo, "couldn't load OpenRouter's models, so they can't be listed or checked: "+m.fetchErr)
		}
		return
	}
	m.fetchErr = ""
	m.models.Catalog = msg.catalog
	if err := m.models.Store.SaveCatalog(msg.catalog); err != nil {
		m.say(toneError, "caching OpenRouter's models: "+err.Error())
	}
	if m.picker != nil {
		m.refind(false)
	}
	current := m.settings.ModelName
	if msg.catalog.Listed(current) {
		// Efforts can change as providers come and go.
		if efforts := msg.catalog.Efforts(current); !slices.Equal(efforts, m.settings.Efforts) {
			m.settings.Efforts = efforts
			m.configure()
		}
		return
	}
	next, ok := m.models.List.Latest(msg.catalog.Listed)
	if !ok {
		m.say(toneError, current+" isn't available to your OpenRouter key. Press ctrl+l to pick another model.")
		return
	}
	if m.useModel(next) {
		m.say(toneNotice, fmt.Sprintf("%s isn't available to your OpenRouter key any more, so %s, picked most recently, takes over", current, next))
	}
}

// useModel switches the session to model id from its next request, and
// records that it was picked. It reports whether the switch was made.
func (m *model) useModel(id string) bool {
	lm, err := m.models.Open(m.ctx, id)
	if err != nil {
		m.notify(toneError, id+": "+err.Error())
		return false
	}
	m.settings.Model, m.settings.ModelName, m.settings.Efforts = lm, id, m.models.Catalog.Efforts(id)
	m.configure()
	m.saveList(func(l *models.List) { l.Pick(id, time.Now()) })
	m.say(toneInfo, fmt.Sprintf("model %s · effort %s%s", id, m.effortLabel(), m.fromNext()))
	return true
}

// nextModel switches to the next of the user's models after the one in
// use, wrapping round and passing over models the key can no longer use.
// From a model off the list, it starts at the first.
func (m *model) nextModel() {
	list := m.models.List.Models
	start := m.models.List.Index(m.settings.ModelName) + 1
	for k := range len(list) {
		if id := list[(start+k)%len(list)].ID; id != m.settings.ModelName && m.models.Catalog.Listed(id) {
			m.useModel(id)
			return
		}
	}
	m.notify(toneInfo, "ctrl+p cycles through your models: add some with ctrl+l, then enter or ctrl+f")
}

// toggleYours adds id to the user's models, or removes it. Removing the
// model in use promotes the most recently picked of the rest.
func (m *model) toggleYours(id string) {
	if !m.models.List.Has(id) {
		m.saveList(func(l *models.List) { l.Add(id) })
		m.notify(toneInfo, "added "+id+" to your models")
		m.refind(false)
		return
	}
	if len(m.models.List.Models) == 1 {
		m.notify(toneInfo, "your models need at least one: add another before removing this one")
		return
	}
	m.saveList(func(l *models.List) { l.Remove(id) })
	m.notify(toneInfo, "removed "+id+" from your models")
	if id == m.settings.ModelName {
		next, ok := m.models.List.Latest(m.models.Catalog.Listed)
		if !ok {
			next, _ = m.models.List.Latest(func(string) bool { return true })
		}
		if m.useModel(next) {
			m.notify(toneInfo, "removed "+id+" from your models, so "+next+", picked most recently, is in use")
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
	m.notify(toneInfo, fmt.Sprintf("new sessions start on %s at effort %s", d.Model, d.Effort))
	m.refind(false)
}

// saveList changes the saved list. If it can't be saved, the change still
// holds for this session.
func (m *model) saveList(change func(*models.List)) {
	l, err := m.models.Store.Update(change)
	if err != nil {
		change(&m.models.List)
		m.notify(toneError, "saving your models: "+err.Error())
		return
	}
	m.models.List = l
}
