package stats

import (
	"slices"
	"testing"
)

func TestMean(t *testing.T) {
	if got := Mean([]float64{2, 4, 6}); got != 4 {
		t.Fatalf("Mean = %v, want 4", got)
	}
	if got := Mean(nil); got != 0 {
		t.Fatalf("Mean(nil) = %v, want 0", got)
	}
}

func TestMedianOdd(t *testing.T) {
	if got := Median([]float64{9, 1, 5}); got != 5 {
		t.Fatalf("Median = %v, want 5", got)
	}
}

func TestMedianEven(t *testing.T) {
	if got := Median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Fatalf("Median = %v, want 2.5", got)
	}
}

func TestMedianDoesNotModifyInput(t *testing.T) {
	xs := []float64{3, 1, 2}
	Median(xs)
	if !slices.Equal(xs, []float64{3, 1, 2}) {
		t.Fatalf("Median modified its input: %v", xs)
	}
}
