package checkpoint

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
		if got, _, _ := p.Decide(answers(tc.deletes, tc.damages, tc.authorised), false); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
	// A delete that code resolved to a temp directory isn't blocked on
	// Jev's reading of "outside the project".
	if got, _, _ := p.Decide(answers(0.92, 0.54, 0.42), true); got != "allow" {
		t.Errorf("resolved temp delete: %s, want allow", got)
	}
}

func TestTripwireFailsClosed(t *testing.T) {
	d := Tripwire(t.Context(), nil, TripwireState{Command: "curl https://example.com"}, DefaultTripwirePolicy)
	if d.Action != "block" || d.Rule != "jev_unavailable" {
		t.Fatalf("without Jev: %s %s", d.Action, d.Rule)
	}
}

// An answer Jev left out is never consent: a reply without the risk
// answers blocks rather than reading them as zero.
func TestTripwireBlocksOnMissingAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"model":"jev-1.13.0","answers":{"authorised":{"type":"noul","noul":0.1}}}`)
	}))
	defer srv.Close()
	d := Tripwire(t.Context(), jev.New(srv.URL, jev.DefaultModel, "key"), TripwireState{Command: "rm -rf ~/x"}, DefaultTripwirePolicy)
	if d.Action != "block" || d.Rule != "jev_unavailable" || d.Error == "" {
		t.Fatalf("with answers missing: %s %s %q", d.Action, d.Rule, d.Error)
	}
}
