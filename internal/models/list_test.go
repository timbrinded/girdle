package models

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

func picked(ids ...string) List {
	var l List
	start := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for i, id := range ids {
		l.Pick(id, start.Add(time.Duration(i)*time.Minute))
	}
	return l
}

func TestRemovingTheDefaultPromotesTheLastPicked(t *testing.T) {
	l := picked("a", "b", "c")
	l.Default.Model = "c"
	// b was picked after a, so it takes over.
	l.Remove("c")
	if l.Default.Model != "b" || l.Has("c") {
		t.Fatalf("after removing the default: %+v", l)
	}
	l.Pick("a", time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC))
	l.Remove("b")
	if l.Default.Model != "a" {
		t.Fatalf("default = %q, want a", l.Default.Model)
	}
	l.Remove("a")
	if l.Default.Model != "" || len(l.Models) != 0 {
		t.Fatalf("empty list kept %+v", l)
	}
}

func TestRemovingAnotherModelKeepsTheDefault(t *testing.T) {
	l := picked("a", "b")
	l.Default.Model = "a"
	l.Remove("b")
	if l.Default.Model != "a" {
		t.Fatalf("default = %q", l.Default.Model)
	}
}

func TestLatestPrefersPickedModels(t *testing.T) {
	var l List
	l.Add("never")
	l.Pick("once", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	l.Add("also-never")
	if id, _ := l.Latest(func(string) bool { return true }); id != "once" {
		t.Fatalf("latest = %q, want once", id)
	}
	if id, _ := l.Latest(func(id string) bool { return id != "once" }); id != "never" {
		t.Fatalf("with once unusable, latest = %q, want the first unpicked", id)
	}
	if _, ok := l.Latest(func(string) bool { return false }); ok {
		t.Fatal("nothing usable, but found a model")
	}
}

func TestStartReplacesAWithdrawnModel(t *testing.T) {
	cat := Catalog{Models: []Info{{ID: "b"}, {ID: "builtin"}}}
	l := picked("gone", "b")
	l.Default.Model = "gone"
	if id, note := l.Start(cat, "builtin"); id != "b" || note == "" {
		t.Fatalf("start = %q (%q), want b with a note", id, note)
	}
	// Without a default, the most recently picked model the catalogue
	// lists is used, and with none, the built-in model.
	l.Default.Model = ""
	if id, note := l.Start(cat, "builtin"); id != "b" || note != "" {
		t.Fatalf("start = %q (%q), want b, picked last", id, note)
	}
	if id, note := (List{}).Start(cat, "builtin"); id != "builtin" || note != "" {
		t.Fatalf("start = %q (%q), want builtin", id, note)
	}
	// An unknown catalogue withdraws nothing.
	l.Default.Model = "gone"
	if id, _ := l.Start(Catalog{}, "builtin"); id != "gone" {
		t.Fatalf("with no catalogue, start = %q", id)
	}
	// When nothing on the list is listed, the built-in model stands in.
	if id, _ := picked("gone").Start(cat, "builtin"); id != "builtin" {
		t.Fatalf("start = %q, want builtin", id)
	}
}

func TestStoreUpdateKeepsOtherChanges(t *testing.T) {
	dir := t.TempDir()
	s := Store{ListPath: filepath.Join(dir, "config", "models.json"), CatalogPath: filepath.Join(dir, "cache", "catalog.json")}
	if l, err := s.Load(); err != nil || len(l.Models) != 0 {
		t.Fatalf("missing file: %+v, %v", l, err)
	}
	if _, err := s.Update(func(l *List) { l.Add("a") }); err != nil {
		t.Fatal(err)
	}
	l, err := s.Update(func(l *List) { l.Add("b"); l.Default = Defaults{Model: "b", Effort: EffortAuto} })
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Load(); len(again.Models) != 2 || again.Default != l.Default {
		t.Fatalf("reloaded %+v, want %+v", again, l)
	}
	cat := Catalog{Fetched: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), Models: []Info{{ID: "a", Tools: true, Efforts: []checkpoint.Effort{"low"}}}}
	if err := s.SaveCatalog(cat); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Catalog(); err != nil || !got.Fetched.Equal(cat.Fetched) || !slices.Equal(got.Efforts("a"), []checkpoint.Effort{"low"}) {
		t.Fatalf("cached catalogue %+v, %v", got, err)
	}
}

func TestParseEffort(t *testing.T) {
	if _, auto, err := ParseEffort("auto"); !auto || err != nil {
		t.Fatalf("auto: %v %v", auto, err)
	}
	if e, auto, err := ParseEffort("xhigh"); e != "xhigh" || auto || err != nil {
		t.Fatalf("xhigh: %q %v %v", e, auto, err)
	}
	if _, _, err := ParseEffort("turbo"); err == nil {
		t.Fatal("turbo accepted")
	}
}
