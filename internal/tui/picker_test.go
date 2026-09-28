package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/models"
)

var catalog = models.Catalog{Models: []models.Info{
	{ID: "a", Tools: true, Efforts: checkpoint.Efforts},
	{ID: "b", Tools: true, Efforts: []checkpoint.Effort{"low", "high"}},
	{ID: "c", Tools: true, Efforts: []checkpoint.Effort{"high"}},
	{ID: "meta/muse-spark-1.3", Name: "Meta: Muse Spark", Tools: true},
	{ID: "meta/muse-embed", Name: "Meta: Muse Embed"},
}}

// pickerModel builds the TUI's model around a session on current, with the
// list saved in a temporary store. With jevOn, Jev can choose efforts.
func pickerModel(t *testing.T, list models.List, current string, jevOn bool) *model {
	t.Helper()
	dir := t.TempDir()
	store := models.Store{ListPath: filepath.Join(dir, "models.json"), CatalogPath: filepath.Join(dir, "catalog.json")}
	if _, err := store.Update(func(l *models.List) { *l = list }); err != nil {
		t.Fatal(err)
	}
	cfg := kernel.Config{
		Settings: kernel.Settings{ModelName: current, Efforts: catalog.Efforts(current), AutoEffort: true, Effort: checkpoint.EffortMedium},
		Dir:      dir,
	}
	if jevOn {
		cfg.Jev, cfg.Route = jev.New("http://jev.invalid", jev.DefaultModel, "key"), true
		cfg.EffortOptions = func(checkpoint.Effort) fantasy.ProviderOptions { return nil }
	}
	open := func(_ context.Context, id string) (fantasy.LanguageModel, error) {
		if id == "broken" {
			return nil, errors.New("no such model")
		}
		return nil, nil
	}
	ms := &Models{Store: store, List: list, Catalog: catalog, Open: open}
	m := newModel(t.Context(), kernel.NewSession(cfg), make(chan kernel.Event, 16), cfg.Settings, "log.jsonl", ms)
	m.width, m.height = 140, 30
	return m
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "ctrl+l":
		return tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func press(m *model, keys ...string) {
	for _, k := range keys {
		m.Update(key(k))
	}
}

func saved(t *testing.T, m *model) models.List {
	t.Helper()
	l, err := m.models.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func listOf(ids ...string) models.List {
	var l models.List
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range ids {
		l.Pick(id, start.Add(time.Duration(i)*time.Hour))
	}
	return l
}

func TestRemovingTheModelInUsePromotesTheLastPicked(t *testing.T) {
	l := listOf("a", "b", "c")
	l.Default.Model = "a"
	m := pickerModel(t, l, "a", true)
	// Starting on a counts as picking it, so b is now the next most recent.
	press(m, "ctrl+l", "x")
	if m.settings.ModelName != "c" {
		t.Fatalf("in use: %q, want c, the latest picked after a", m.settings.ModelName)
	}
	got := saved(t, m)
	if got.Has("a") || got.Default.Model != "c" {
		t.Fatalf("saved %+v", got)
	}
	// The cursor stays on the first row, now b. Removing b leaves c in use.
	press(m, "x")
	if m.settings.ModelName != "c" || saved(t, m).Has("b") {
		t.Fatalf("in use %q, saved %+v", m.settings.ModelName, saved(t, m))
	}
	press(m, "x")
	if !saved(t, m).Has("c") || !strings.Contains(m.picker.note, "needs one model") {
		t.Fatalf("removed the only model: %+v, note %q", saved(t, m), m.picker.note)
	}
}

func TestPickingAModel(t *testing.T) {
	m := pickerModel(t, listOf("a", "broken", "b"), "a", true)
	press(m, "ctrl+l", "down", "enter")
	if m.settings.ModelName != "a" || m.picker == nil || !strings.Contains(m.picker.note, "no such model") {
		t.Fatalf("a model that won't open was used: %q", m.settings.ModelName)
	}
	press(m, "down", "enter")
	if m.settings.ModelName != "b" || m.picker != nil {
		t.Fatalf("in use %q, picker open %v", m.settings.ModelName, m.picker != nil)
	}
	if id, _ := saved(t, m).Latest(func(string) bool { return true }); id != "b" {
		t.Fatalf("latest picked %q, want b", id)
	}
}

func TestSavingTheDefault(t *testing.T) {
	m := pickerModel(t, listOf("a", "b"), "b", true)
	press(m, "shift+tab", "ctrl+l", "d")
	if d := saved(t, m).Default; d != (models.Defaults{Model: "b", Effort: "low"}) {
		t.Fatalf("default %+v", d)
	}
}

func TestEffortCycle(t *testing.T) {
	m := pickerModel(t, listOf("b"), "b", true)
	var seen []string
	for range 4 {
		press(m, "shift+tab")
		seen = append(seen, m.effortLabel())
	}
	if got := strings.Join(seen, ","); got != "low,high,auto,low" {
		t.Fatalf("cycle %s", got)
	}
	// Without Jev there is no auto.
	m = pickerModel(t, listOf("b"), "b", false)
	seen = nil
	for range 3 {
		press(m, "shift+tab")
		seen = append(seen, m.effortLabel())
	}
	if got := strings.Join(seen, ","); got != "low,high,low" {
		t.Fatalf("cycle without Jev %s", got)
	}
	// A model with no effort setting has nothing to cycle.
	m = pickerModel(t, listOf("meta/muse-spark-1.3"), "meta/muse-spark-1.3", true)
	press(m, "shift+tab")
	if m.effortLabel() != "n/a" || !strings.Contains(m.lines[len(m.lines)-1], "no reasoning effort setting") {
		t.Fatalf("label %q, last line %q", m.effortLabel(), m.lines[len(m.lines)-1])
	}
}

func TestPinnedEffortFitsTheNextModel(t *testing.T) {
	m := pickerModel(t, listOf("b", "c"), "b", true)
	press(m, "shift+tab") // low
	press(m, "ctrl+l", "down", "enter")
	if m.settings.ModelName != "c" || m.effortLabel() != "high (asked low)" {
		t.Fatalf("on %q effort %q", m.settings.ModelName, m.effortLabel())
	}
	// Stepping on starts from the effort the model uses.
	press(m, "shift+tab")
	if m.effortLabel() != "auto" {
		t.Fatalf("after high: %q", m.effortLabel())
	}
}

func TestAddingAModel(t *testing.T) {
	m := pickerModel(t, listOf("a"), "a", true)
	press(m, "ctrl+l", "a", "m", "u", "s", "e")
	// Only models that take tool calls are offered.
	if len(m.picker.found) != 1 || m.picker.found[0].ID != "meta/muse-spark-1.3" {
		t.Fatalf("found %+v", m.picker.found)
	}
	if v := m.View(); !strings.Contains(v.Content, "meta/muse-spark-1.3") {
		t.Fatalf("view doesn't show the match:\n%s", v.Content)
	}
	press(m, "enter")
	if !saved(t, m).Has("meta/muse-spark-1.3") || m.picker.mode != choosing || m.settings.ModelName != "a" {
		t.Fatalf("after adding: %+v, in use %q", saved(t, m), m.settings.ModelName)
	}
	// The new model is under the cursor, ready to use.
	press(m, "enter")
	if m.settings.ModelName != "meta/muse-spark-1.3" {
		t.Fatalf("in use %q", m.settings.ModelName)
	}
	// Without a catalogue, an ID is taken as typed.
	m.models.Catalog = models.Catalog{}
	press(m, "ctrl+l", "a", "x", "/", "y", "enter")
	if !saved(t, m).Has("x/y") {
		t.Fatalf("typed ID not added: %+v", saved(t, m))
	}
}

func TestAWithdrawnModelGivesWay(t *testing.T) {
	m := pickerModel(t, listOf("a", "b", "c"), "c", true)
	// OpenRouter drops c. b was picked after a.
	fresh := models.Catalog{Models: catalog.Models[:2]}
	m.takeCatalog(catalogMsg{catalog: fresh})
	if m.settings.ModelName != "b" || !strings.Contains(m.lines[len(m.lines)-1], "no longer listed") {
		t.Fatalf("in use %q, last line %q", m.settings.ModelName, m.lines[len(m.lines)-1])
	}
	if got, _ := m.models.Store.Catalog(); len(got.Models) != 2 {
		t.Fatalf("catalogue not cached: %+v", got)
	}
	// A failed fetch changes nothing.
	m.takeCatalog(catalogMsg{err: errors.New("offline")})
	if m.settings.ModelName != "b" {
		t.Fatalf("in use %q after a failed fetch", m.settings.ModelName)
	}
}

func TestContextSize(t *testing.T) {
	for tokens, want := range map[int]string{1048576: "1M context", 2_000_000: "2M context", 1_500_000: "1.5M context", 262144: "262k context"} {
		if got := contextSize(tokens); got != want {
			t.Errorf("contextSize(%d) = %q, want %q", tokens, got, want)
		}
	}
}
