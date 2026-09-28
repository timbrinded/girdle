package stats

import "sort"

// Percentile returns the p-th percentile (0 < p <= 100) of xs using the
// nearest-rank method: the smallest value such that at least p percent of
// the values are less than or equal to it. It does not modify xs and
// returns 0 for an empty slice.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	rank := int(p / 100 * float64(len(s)))
	return s[rank]
}
