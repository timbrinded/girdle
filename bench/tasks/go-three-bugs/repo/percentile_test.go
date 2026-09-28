package stats

import "testing"

func TestPercentile(t *testing.T) {
	xs := []float64{15, 20, 35, 40, 50}
	cases := []struct {
		p    float64
		want float64
	}{
		{5, 15}, {30, 20}, {40, 20}, {50, 35}, {100, 50},
	}
	for _, c := range cases {
		if got := Percentile(xs, c.p); got != c.want {
			t.Errorf("Percentile(%v) = %v, want %v", c.p, got, c.want)
		}
	}
}
