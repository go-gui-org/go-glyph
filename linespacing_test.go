package glyph

import (
	"math"
	"testing"
)

func TestTypoLineBox(t *testing.T) {
	const em = 2048.0 // font units: scale 1
	tests := []struct {
		name              string
		asc, desc, gap    float64
		wantAsc, wantDesc float64
		wantHeightOverEm  float64
	}{
		// Segoe UI: 1491+431+269 = 2191 < 1.15 em floor (2355.2); the
		// floor's extra 433.2 splits evenly above and below.
		{"segoe ui floored", 1491, 431, 269, 1707.6, 647.6, 1.15},
		// Sum above the floor: the gap alone splits.
		{"gap split", 1600, 500, 400, 1800, 700, 2500.0 / em},
		{"no gap above floor", 1900, 600, 0, 1900, 600, 2500.0 / em},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, d := typoLineBox(tc.asc, tc.desc, tc.gap, em)
			if math.Abs(a-tc.wantAsc) > 1e-9 || math.Abs(d-tc.wantDesc) > 1e-9 {
				t.Errorf("typoLineBox = (%v, %v), want (%v, %v)",
					a, d, tc.wantAsc, tc.wantDesc)
			}
			if h := (a + d) / em; math.Abs(h-tc.wantHeightOverEm) > 1e-9 {
				t.Errorf("line height %v em, want %v", h, tc.wantHeightOverEm)
			}
		})
	}
}

func TestRecommendedLineHeight(t *testing.T) {
	const em = 16.0
	tests := []struct {
		name                     string
		ascent, descent, leading float64
		want                     float64
	}{
		{"font hint honored", 14, 4, 3, 21},     // 21 > 1.15*16=18.4
		{"floored when no gap", 10, 3, 0, 18.4}, // 13 < 18.4 → floor
		{"floored on synth 1em", 12.8, 3.2, 0, 18.4},
		{"exact hint above floor", 15, 5, 0, 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := recommendedLineHeight(tc.ascent, tc.descent, tc.leading, em)
			if got != tc.want {
				t.Errorf("recommendedLineHeight(%v,%v,%v,%v) = %v, want %v",
					tc.ascent, tc.descent, tc.leading, em, got, tc.want)
			}
		})
	}
}
