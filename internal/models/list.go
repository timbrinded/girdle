package models

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// EffortAuto, as an effort setting, lets Jev choose each request's reasoning
// effort.
const EffortAuto = "auto"

// ParseEffort reads an effort setting: EffortAuto, or a reasoning effort.
func ParseEffort(s string) (effort checkpoint.Effort, auto bool, err error) {
	if s == EffortAuto {
		return "", true, nil
	}
	e := checkpoint.Effort(s)
	if !slices.Contains(checkpoint.Efforts, e) {
		names := []string{EffortAuto}
		for _, e := range checkpoint.Efforts {
			names = append(names, string(e))
		}
		return "", false, fmt.Errorf("unknown effort %q: use %s", s, strings.Join(names, ", "))
	}
	return e, false, nil
}

// Entry is a model on the user's list.
type Entry struct {
	ID string `json:"id"`
	// Picked is when the model was last chosen, zero if never.
	Picked time.Time `json:"picked,omitzero"`
}

// Defaults are what new sessions start with.
type Defaults struct {
	Model string `json:"model,omitempty"`
	// Effort is EffortAuto or a reasoning effort.
	Effort string `json:"effort,omitempty"`
}

// List is the user's models, the ones the picker offers, in the order they
// were added.
type List struct {
	Models  []Entry  `json:"models"`
	Default Defaults `json:"default,omitzero"`
}

// Has reports whether id is on the list.
func (l List) Has(id string) bool { return l.Index(id) >= 0 }

// Index is id's position on the list, or -1 if it isn't there.
func (l List) Index(id string) int {
	return slices.IndexFunc(l.Models, func(e Entry) bool { return e.ID == id })
}

// Add puts id at the end of the list, unless it is already there.
func (l *List) Add(id string) {
	if !l.Has(id) {
		l.Models = append(l.Models, Entry{ID: id})
	}
}

// Pick records that id was chosen at now, adding it if needed.
func (l *List) Pick(id string, now time.Time) {
	l.Add(id)
	l.Models[l.Index(id)].Picked = now
}

// Remove takes id off the list. If id was the default model, the most
// recently picked of the rest becomes the default.
func (l *List) Remove(id string) {
	l.Models = slices.DeleteFunc(l.Models, func(e Entry) bool { return e.ID == id })
	if l.Default.Model == id {
		l.Default.Model, _ = l.Latest(func(string) bool { return true })
	}
}

// Latest returns the most recently picked model on the list for which
// usable is true. Models never picked come after every picked one, in list
// order.
func (l List) Latest(usable func(id string) bool) (string, bool) {
	var best *Entry
	for i, e := range l.Models {
		if usable(e.ID) && (best == nil || e.Picked.After(best.Picked)) {
			best = &l.Models[i]
		}
	}
	if best == nil {
		return "", false
	}
	return best.ID, true
}

// Start chooses the model a session starts on: the default model, or
// without one the most recently picked model, as OpenCode does, or
// fallback. If the catalogue says that model is gone, it is the most
// recently picked model the catalogue still lists, or fallback. note
// explains a replacement.
func (l List) Start(c Catalog, fallback string) (id, note string) {
	listed := func(id string) bool { return !c.Gone(id) }
	want := l.Default.Model
	if want == "" {
		want, _ = l.Latest(listed)
	}
	want = cmp.Or(want, fallback)
	if !c.Gone(want) {
		return want, ""
	}
	id, ok := l.Latest(listed)
	if !ok && listed(fallback) {
		id, ok = fallback, true
	}
	if !ok {
		return want, want + " is no longer listed on OpenRouter"
	}
	return id, fmt.Sprintf("%s is no longer listed on OpenRouter, so %s is in use", want, id)
}

// Store is where the list and the cached catalogue live.
type Store struct {
	ListPath    string
	CatalogPath string
}

// DefaultStore keeps the list under $XDG_CONFIG_HOME/girdle and the
// catalogue under $XDG_CACHE_HOME/girdle, the way event logs live under
// $XDG_STATE_HOME.
func DefaultStore() Store {
	home, _ := os.UserHomeDir()
	dir := func(env string, fallback ...string) string {
		return filepath.Join(cmp.Or(os.Getenv(env), filepath.Join(append([]string{home}, fallback...)...)), "girdle")
	}
	return Store{
		ListPath:    filepath.Join(dir("XDG_CONFIG_HOME", ".config"), "models.json"),
		CatalogPath: filepath.Join(dir("XDG_CACHE_HOME", ".cache"), "openrouter-models.json"),
	}
}

// Load reads the list. A missing file is an empty list.
func (s Store) Load() (List, error) {
	var l List
	return l, readJSON(s.ListPath, &l)
}

// Update reads the list, applies change and writes it back. Reading first
// keeps another session's changes.
func (s Store) Update(change func(*List)) (List, error) {
	l, err := s.Load()
	if err != nil {
		return l, err
	}
	change(&l)
	return l, writeJSON(s.ListPath, l)
}

// Catalog reads the cached catalogue. A missing file is an empty catalogue.
func (s Store) Catalog() (Catalog, error) {
	var c Catalog
	return c, readJSON(s.CatalogPath, &c)
}

// SaveCatalog caches c.
func (s Store) SaveCatalog(c Catalog) error { return writeJSON(s.CatalogPath, c) }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// writeJSON replaces path in one step, so a reader never sees half a file.
func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
