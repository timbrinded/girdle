package checkpoint

import "testing"

func TestReproduceClaimsNothingWithoutJev(t *testing.T) {
	d := Reproduce(t.Context(), nil, ReproduceState{Command: "pytest -k x", Output: "no tests ran"})
	if d.Ran || d.Error == "" {
		t.Errorf("Reproduce without Jev = %+v; want not ran, with the error recorded", d)
	}
}

func TestMustChangeKeepsEveryMentionWithoutJev(t *testing.T) {
	mentions := []string{"a.go:1: old()", "CHANGELOG.md:3: renamed old"}
	d := MustChange(t.Context(), nil, "rename old to new", "old", mentions)
	if len(d.Must) != len(mentions) || d.Error == "" {
		t.Errorf("MustChange without Jev = %+v", d)
	}
}
