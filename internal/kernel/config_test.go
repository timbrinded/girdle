package kernel

import (
	"slices"
	"testing"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
)

func TestResolvedTurnsOffUnmetFeatures(t *testing.T) {
	all := Config{
		Checkpoints: true, Route: true, Speculate: true, Snapshot: true, Prefetch: true, Batch: true,
		EarlyStop: true, CrossCheck: true, Heartbeat: true, Compact: true, Reproduce: true, Leftovers: true,
		Jev:           jev.New("http://jev.invalid", jev.DefaultModel, "key"),
		EffortOptions: func(checkpoint.Effort) fantasy.ProviderOptions { return nil },
	}
	on := func(c Config) map[string]bool {
		return map[string]bool{
			"route": c.Route, "speculate": c.Speculate, "prefetch": c.Prefetch, "early_stop": c.EarlyStop,
			"crosscheck": c.CrossCheck, "heartbeat": c.Heartbeat, "compact": c.Compact,
			"reproduce": c.Reproduce, "leftovers": c.Leftovers,
		}
	}
	for name, got := range on(all.resolved()) {
		if !got {
			t.Errorf("everything on: %s turned off", name)
		}
	}

	cases := []struct {
		name string
		edit func(*Config)
		off  []string
	}{
		{"no Jev", func(c *Config) { c.Jev = nil },
			[]string{"route", "speculate", "prefetch", "early_stop", "crosscheck", "heartbeat", "compact", "leftovers"}},
		{"no checkpoints", func(c *Config) { c.Checkpoints = false },
			[]string{"early_stop", "crosscheck", "heartbeat", "compact"}},
		{"no route", func(c *Config) { c.Route = false }, []string{"route", "speculate"}},
		{"no efforts", func(c *Config) { c.EffortOptions = nil }, []string{"route", "speculate"}},
		{"no batch", func(c *Config) { c.Batch = false }, []string{"crosscheck", "reproduce"}},
		{"no snapshot", func(c *Config) { c.Snapshot = false }, []string{"prefetch"}},
		{"no early stop", func(c *Config) { c.EarlyStop = false }, []string{"early_stop", "crosscheck"}},
	}
	for _, tc := range cases {
		c := all
		tc.edit(&c)
		got := on(c.resolved())
		for name, v := range got {
			if want := !slices.Contains(tc.off, name); v != want {
				t.Errorf("%s: %s = %v, want %v", tc.name, name, v, want)
			}
		}
	}
}
