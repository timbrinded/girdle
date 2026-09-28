package tui

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"
	"charm.land/lipgloss/v2"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/models"
)

var catalog = models.Catalog{ForKey: true, Models: []models.Info{
	{ID: "a", Tools: true, Efforts: checkpoint.Efforts, Created: 1},
	{ID: "b", Tools: true, Efforts: []checkpoint.Effort{"low", "high"}, Created: 2},
	{ID: "c", Tools: true, Efforts: []checkpoint.Effort{"high"}, Created: 3},
	{ID: "meta/muse-spark-1.2", Name: "Meta: Muse Spark 1.2", Tools: true, Created: 10},
	{ID: "meta/muse-spark-1.3", Name: "Meta: Muse Spark 1.3", Tools: true, Created: 20, Efforts: checkpoint.Efforts},
	{ID: "meta/muse-embed", Name: "Meta: Muse Embed", Created: 30},
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
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
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
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if c, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(c[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func press(m *model, keys ...string) {
	for _, k := range keys {
		m.Update(key(k))
	}
}

// typeText types s one character at a time.
func typeText(m *model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
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

func rowIDs(p *picker) []string {
	var out []string
	for _, r := range p.rows {
		out = append(out, r.header+r.id)
	}
	return out
}

func lastLine(m *model) string { return m.lines[len(m.lines)-1] }

// plain drops a line's colours.
func plain(s string) string { return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "") }

func TestPickerListsEveryModelTheKeyCanUse(t *testing.T) {
	m := pickerModel(t, listOf("b"), "b", true)
	press(m, "ctrl+l")
	// Yours first, then every model that takes tools, by provider, newest
	// first within each; the embedding model can't call tools.
	want := []string{"Your models", "b", "All models", "a", "c", "meta/muse-spark-1.3", "meta/muse-spark-1.2"}
	if got := rowIDs(m.picker); !slices.Equal(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	if m.picker.selected() != "b" {
		t.Fatalf("cursor on %q, want the model in use", m.picker.selected())
	}
	press(m, "up")
	if m.picker.selected() != "b" {
		t.Fatalf("up from the first choice moved to %q", m.picker.selected())
	}
	press(m, "down")
	if m.picker.selected() != "a" {
		t.Fatalf("down skipped to %q, want a past the header", m.picker.selected())
	}
}

func TestSearchingAndPicking(t *testing.T) {
	m := pickerModel(t, listOf("a"), "a", true)
	press(m, "ctrl+l")
	typeText(m, "muse")
	if got := rowIDs(m.picker); !slices.Equal(got, []string{"meta/muse-spark-1.3", "meta/muse-spark-1.2"}) {
		t.Fatalf("rows %v", got)
	}
	press(m, "enter")
	if m.settings.ModelName != "meta/muse-spark-1.3" || m.picker != nil {
		t.Fatalf("in use %q, picker open %v", m.settings.ModelName, m.picker != nil)
	}
	if l := saved(t, m); !l.Has("meta/muse-spark-1.3") {
		t.Fatalf("picked model not added to yours: %+v", l)
	}
	press(m, "ctrl+l")
	typeText(m, "zzz")
	press(m, "enter")
	if m.picker == nil || !strings.Contains(m.picker.note, "no model") {
		t.Fatal("enter with no match should say so and stay open")
	}
	press(m, "esc")
	if m.picker != nil {
		t.Fatal("esc didn't close the picker")
	}
}

func TestAModelThatWontOpenIsNotUsed(t *testing.T) {
	m := pickerModel(t, listOf("a", "broken"), "a", true)
	press(m, "ctrl+l", "down", "enter")
	if m.settings.ModelName != "a" || m.picker == nil || !strings.Contains(m.picker.note, "no such model") {
		t.Fatalf("in use %q, picker %v", m.settings.ModelName, m.picker)
	}
}

func TestRemovingTheModelInUsePromotesTheLastPicked(t *testing.T) {
	l := listOf("a", "b", "c")
	l.Default.Model = "a"
	m := pickerModel(t, l, "a", true)
	// Starting on a counts as picking it, so c is the next most recent.
	press(m, "ctrl+l", "ctrl+f")
	if m.settings.ModelName != "c" {
		t.Fatalf("in use: %q, want c, the latest picked after a", m.settings.ModelName)
	}
	got := saved(t, m)
	if got.Has("a") || got.Default.Model != "c" || !strings.Contains(m.picker.note, "picked most recently") {
		t.Fatalf("saved %+v, note %q", got, m.picker.note)
	}
	// Removing a model that isn't in use leaves the model in use alone,
	// and the cursor goes back to it rather than following b down the list.
	m.picker.moveTo("b")
	press(m, "ctrl+f")
	if m.settings.ModelName != "c" || saved(t, m).Has("b") || m.picker.selected() != "c" {
		t.Fatalf("in use %q, cursor %q, saved %+v", m.settings.ModelName, m.picker.selected(), saved(t, m))
	}
	m.picker.moveTo("c")
	press(m, "ctrl+f")
	if !saved(t, m).Has("c") || !strings.Contains(m.picker.note, "need at least one") {
		t.Fatalf("removed the only model: %+v, note %q", saved(t, m), m.picker.note)
	}
	// ctrl+f on another model adds it to yours.
	m.picker.moveTo("b")
	press(m, "ctrl+f")
	if !saved(t, m).Has("b") || m.picker.rows[1].id != "c" || m.picker.rows[2].id != "b" {
		t.Fatalf("added b: %+v, rows %v", saved(t, m), rowIDs(m.picker))
	}
}

func TestCtrlPCyclesYourModels(t *testing.T) {
	m := pickerModel(t, listOf("a", "b", "c"), "b", true)
	var seen []string
	for range 3 {
		press(m, "ctrl+p")
		seen = append(seen, m.settings.ModelName)
	}
	if got := strings.Join(seen, ","); got != "c,a,b" {
		t.Fatalf("cycle %s", got)
	}
	m = pickerModel(t, listOf("a"), "a", true)
	press(m, "ctrl+p")
	if !strings.Contains(lastLine(m), "cycles through your models") {
		t.Fatalf("one model: %q", lastLine(m))
	}
}

func TestSavingTheDefault(t *testing.T) {
	m := pickerModel(t, listOf("a", "b"), "b", true)
	press(m, "shift+tab", "ctrl+l", "down", "ctrl+s")
	if d := saved(t, m).Default; d != (models.Defaults{Model: "c", Effort: "low"}) {
		t.Fatalf("default %+v", d)
	}
	if !strings.Contains(m.picker.note, "new sessions start on c") || m.settings.ModelName != "b" {
		t.Fatalf("note %q, in use %q", m.picker.note, m.settings.ModelName)
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
	m = pickerModel(t, listOf("meta/muse-spark-1.2"), "meta/muse-spark-1.2", true)
	press(m, "shift+tab")
	if m.effortLabel() != "n/a" || !strings.Contains(lastLine(m), "no reasoning effort setting") {
		t.Fatalf("label %q, last line %q", m.effortLabel(), lastLine(m))
	}
}

func TestEffortPicker(t *testing.T) {
	m := pickerModel(t, listOf("b"), "b", true)
	press(m, "ctrl+l", "tab")
	if got := rowIDs(m.picker); !slices.Equal(got, []string{"auto", "low", "high"}) || m.picker.selected() != "auto" {
		t.Fatalf("efforts %v on %q", got, m.picker.selected())
	}
	// esc goes back to the models, and tab returns.
	press(m, "esc")
	if m.picker.mode != pickModel || m.picker.selected() != "b" {
		t.Fatal("esc didn't go back to the models")
	}
	press(m, "tab", "down", "down", "enter")
	if m.picker != nil || m.effortLabel() != "high" {
		t.Fatalf("effort %q, picker open %v", m.effortLabel(), m.picker != nil)
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

func TestSlashCommands(t *testing.T) {
	m := pickerModel(t, listOf("a"), "a", true)
	run := func(text string) bool {
		_, ok := m.command(text)
		return ok
	}
	if !run("/model b high") || m.settings.ModelName != "b" || m.effortLabel() != "high" {
		t.Fatalf("/model b high: on %q at %q", m.settings.ModelName, m.effortLabel())
	}
	if !run("/model muse") || m.picker == nil || m.picker.search.Value() != "muse" || m.picker.selected() != "meta/muse-spark-1.3" {
		t.Fatal("/model muse didn't open a search for muse")
	}
	press(m, "esc")
	if !run("/effort") || m.picker == nil || m.picker.mode != pickEffort {
		t.Fatal("/effort didn't open the effort list")
	}
	press(m, "esc")
	if m.picker != nil {
		t.Fatal("esc on /effort's list should close it")
	}
	if !run("/effort turbo") || !strings.Contains(lastLine(m), "unknown effort") {
		t.Fatalf("/effort turbo: %q", lastLine(m))
	}
	if !run("/effort auto") || m.effortLabel() != "auto" {
		t.Fatalf("/effort auto: %q", m.effortLabel())
	}
	// Anything else is a prompt for the agent.
	if run("/usr/bin is missing") || run("fix /model handling") {
		t.Fatal("a prompt was taken for a command")
	}
	// Typing a command swaps the key hint for the commands' help.
	m.input.SetValue("/mo")
	if !strings.Contains(m.keyHint(), "/model [search") {
		t.Fatalf("hint %q", m.keyHint())
	}
}

func TestAWithdrawnModelGivesWay(t *testing.T) {
	m := pickerModel(t, listOf("a", "b", "c"), "c", true)
	// The key can no longer use c. b was picked after a.
	fresh := models.Catalog{ForKey: true, Models: catalog.Models[:2]}
	m.takeCatalog(catalogMsg{catalog: fresh})
	if m.settings.ModelName != "b" || !strings.Contains(lastLine(m), "isn't available to your OpenRouter key") {
		t.Fatalf("in use %q, last line %q", m.settings.ModelName, lastLine(m))
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

func TestWithoutACatalogueAnIDIsTakenAsTyped(t *testing.T) {
	m := pickerModel(t, listOf("a"), "a", true)
	m.models.Catalog = models.Catalog{}
	press(m, "ctrl+l")
	if v := m.View().Content; !strings.Contains(v, "type an exact model ID") {
		t.Fatalf("view doesn't explain:\n%s", v)
	}
	typeText(m, "x/y")
	press(m, "enter")
	if m.settings.ModelName != "x/y" || !saved(t, m).Has("x/y") {
		t.Fatalf("in use %q", m.settings.ModelName)
	}
}

func TestViewsFitTheTerminal(t *testing.T) {
	m := pickerModel(t, listOf("a", "meta/muse-spark-1.3"), "a", true)
	for _, size := range [][2]int{{140, 30}, {80, 24}, {60, 16}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, open := range []func(){func() { m.picker = nil }, func() { m.openPicker(pickModel, "") }, func() { m.openPicker(pickEffort, "") }} {
			open()
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) > size[1] {
				t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
			}
			for _, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("%dx%d: line %d wide: %q", size[0], size[1], w, l)
				}
			}
		}
	}
}

func TestContextSize(t *testing.T) {
	for tokens, want := range map[int]string{1048576: "1M", 2_000_000: "2M", 1_500_000: "1.5M", 262144: "262k"} {
		if got := contextSize(tokens); got != want {
			t.Errorf("contextSize(%d) = %q, want %q", tokens, got, want)
		}
	}
}

func TestEffortRange(t *testing.T) {
	for _, c := range []struct {
		efforts []checkpoint.Effort
		want    string
	}{
		{[]checkpoint.Effort{"low", "medium", "high", "xhigh", "max"}, "low–max"},
		{[]checkpoint.Effort{"none", "low", "medium", "high"}, "off, low–high"},
		{[]checkpoint.Effort{"low", "high", "max"}, "low/high/max"},
		{[]checkpoint.Effort{"high"}, "high"},
		{nil, "no effort setting"},
	} {
		if got := effortRange(c.efforts); got != c.want {
			t.Errorf("effortRange(%v) = %q, want %q", c.efforts, got, c.want)
		}
	}
}

func TestHintsDropTheLeastImportantFirst(t *testing.T) {
	hints := []hint{{"enter send", 4}, {"ctrl+l model", 0}, {"shift+tab effort", 1}, {"ctrl+c quit", 3}}
	if got := fitHints(100, hints, " · "); got != "enter send · ctrl+l model · shift+tab effort · ctrl+c quit" {
		t.Errorf("wide: %q", got)
	}
	if got := fitHints(45, hints, " · "); got != "ctrl+l model · shift+tab effort · ctrl+c quit" {
		t.Errorf("narrow: %q", got)
	}
	if got := fitHints(5, hints, " · "); got != "ctrl+l model" {
		t.Errorf("tiny: %q", got)
	}
}

func TestStatusShowsTheRunningModelAndWhatComesNext(t *testing.T) {
	m := pickerModel(t, listOf("a", "b"), "a", true)
	m.start("some task") // the command isn't run
	m.handleEvent(kernel.Event{Type: kernel.EventRoute, Route: &checkpoint.RouteDecision{}, Effort: "low"})
	if s := m.statusLine(); !strings.Contains(s, "effort auto → low") || strings.Contains(s, "next") {
		t.Fatalf("status %q", s)
	}
	press(m, "ctrl+p")
	s := m.statusLine()
	if !strings.Contains(s, "a") || !strings.Contains(s, "auto → low") || !strings.Contains(s, "→ next") || !strings.Contains(s, "b") {
		t.Fatalf("status %q", s)
	}
	m.Update(runDoneMsg{outcome: kernel.OutcomeDone, reason: "done"})
	if s := plain(m.statusLine()); strings.Contains(s, "next") || !strings.HasSuffix(strings.TrimSpace(s), "b • effort auto") {
		t.Fatalf("after the run: %q", s)
	}
}
