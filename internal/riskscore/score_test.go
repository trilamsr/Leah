package riskscore

import (
	"math"
	"testing"
)

// TestCompute_Math pins the formula risk = BR × failureRate × log10(max(diff_LOC, 10)) against §2 fixtures.
func TestCompute_Math(t *testing.T) {
	tests := []struct {
		name        string
		br          int
		failureRate float64
		diffLOC     int
		want        float64
	}{
		{"low_br1_fr01_loc100", 1, 0.1, 100, 0.2},
		{"low_br4_fr01_loc100", 4, 0.1, 100, 0.8},
		{"loc_floor_clamps_at_10", 0, 0.0, 1, 0.0},
		{"loc_floor_logs_to_one", 4, 1.0, 1, 4.0},
		{"high_br4_fr05_loc500", 4, 0.5, 500, 4 * 0.5 * math.Log10(500)},
		{"critical_br4_fr08_loc2000", 4, 0.8, 2000, 4 * 0.8 * math.Log10(2000)},
		{"br_zero_zeroes_out", 0, 0.9, 9999, 0.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(tc.br, tc.failureRate, tc.diffLOC)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("Compute(%d,%v,%d)=%v want %v", tc.br, tc.failureRate, tc.diffLOC, got, tc.want)
			}
		})
	}
}

// TestTier_Boundaries pins §3 thresholds: low <2, medium [2,5), high [5,10), critical ≥10.
func TestTier_Boundaries(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0.0, "low"},
		{1.99, "low"},
		{2.0, "medium"},
		{4.99, "medium"},
		{5.0, "high"},
		{9.99, "high"},
		{10.0, "critical"},
		{100.0, "critical"},
	}
	for _, tc := range tests {
		got := Tier(tc.score)
		if got != tc.want {
			t.Fatalf("Tier(%v)=%q want %q", tc.score, got, tc.want)
		}
	}
}
