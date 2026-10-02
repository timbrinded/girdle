package main

import "testing"

func TestTheFastFlowIsOnUnlessTurnedOff(t *testing.T) {
	for _, c := range []struct {
		name         string
		fast, boring bool
		explicit     []string
		env          string
		want         bool
		fails        bool
	}{
		{name: "default", fast: true, want: true},
		{name: "-boring", fast: true, boring: true, explicit: []string{"boring"}},
		{name: "-fast=false", explicit: []string{"fast"}},
		{name: "-fast", fast: true, explicit: []string{"fast"}, want: true},
		{name: "GIRDLE_BORING=1", fast: true, env: "1"},
		{name: "GIRDLE_BORING=0", fast: true, env: "0", want: true},
		{name: "-fast beats GIRDLE_BORING", fast: true, explicit: []string{"fast"}, env: "1", want: true},
		{name: "-boring=false beats GIRDLE_BORING", fast: true, explicit: []string{"boring"}, env: "1", want: true},
		{name: "-fast -boring", fast: true, boring: true, explicit: []string{"fast", "boring"}, fails: true},
		{name: "GIRDLE_BORING=maybe", fast: true, env: "maybe", fails: true},
	} {
		explicit := map[string]bool{}
		for _, f := range c.explicit {
			explicit[f] = true
		}
		got, err := fastFlowOn(c.fast, c.boring, explicit, c.env)
		if (err != nil) != c.fails || got != c.want {
			t.Errorf("%s: fast flow %v, error %v", c.name, got, err)
		}
	}
}
