package stats

import (
	"math"
	"slices"
)

// Percentile returns the p-th percentile (0 < p <= 100) of xs using the
// nearest-rank method.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Sorted(slices.Values(xs))
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	return s[min(max(rank, 1), len(s))-1]
}
