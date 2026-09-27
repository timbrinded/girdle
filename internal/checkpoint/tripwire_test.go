package checkpoint

import (
	"testing"

	"github.com/timbrinded/girdle/internal/jev"
)

func TestTripwirePolicy(t *testing.T) {
	p := DefaultTripwirePolicy
	answers := func(deletes, damages, authorised float64) map[string]jev.Answer {
		return map[string]jev.Answer{
			"deletes_outside": {Noul: deletes}, "force_push_shared": {Noul: 0.01}, "leaks_secret": {Noul: 0.01},
			"damages_system": {Noul: damages}, "authorised": {Noul: authorised},
		}
	}
	for _, tc := range []struct {
		name                         string
		deletes, damages, authorised float64
		want                         string
	}{
		{"catastrophic", 0.98, 0.97, 0.05, "block"},
		{"safe", 0.03, 0.2, 0.5, "allow"},
		{"damage alone is only logged", 0.1, 0.45, 0.1, "allow"},
		{"the user asked for it", 0.95, 0.9, 0.9, "allow"},
	} {
		if got, _, _ := p.Decide(answers(tc.deletes, tc.damages, tc.authorised)); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestTripwireFailsClosed(t *testing.T) {
	d := Tripwire(t.Context(), nil, TripwireState{Command: "curl https://example.com"}, DefaultTripwirePolicy)
	if d.Action != "block" || d.Rule != "jev_unavailable" {
		t.Fatalf("without Jev: %s %s", d.Action, d.Rule)
	}
}
