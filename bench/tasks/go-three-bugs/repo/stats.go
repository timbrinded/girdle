package stats

import "sort"

// Mean returns the arithmetic mean of xs. It returns 0 for an empty slice.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs)-1)
}

// Median returns the median of xs without modifying xs. It returns 0 for an
// empty slice.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := xs
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return s[n/2]
}
