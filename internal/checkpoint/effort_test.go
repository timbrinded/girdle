package checkpoint

import "testing"

func TestFit(t *testing.T) {
	cases := []struct {
		want      Effort
		supported []Effort
		got       Effort
	}{
		{EffortLow, Efforts, EffortLow},
		{EffortMedium, []Effort{EffortLow, EffortHigh}, EffortHigh},
		{EffortLow, []Effort{EffortHigh}, EffortHigh},
		{EffortMax, []Effort{EffortNone, EffortLow, EffortMedium}, EffortMedium},
		// None is never a fallback, only an answer to asking for it.
		{EffortLow, []Effort{EffortNone}, ""},
		{EffortNone, []Effort{EffortNone, EffortLow}, EffortNone},
		{EffortNone, []Effort{EffortLow, EffortHigh}, EffortLow},
		{EffortHigh, nil, ""},
	}
	for _, c := range cases {
		if got := c.want.Fit(c.supported); got != c.got {
			t.Errorf("%s.Fit(%v) = %q, want %q", c.want, c.supported, got, c.got)
		}
	}
}
