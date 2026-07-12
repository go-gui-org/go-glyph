package glyph

import "testing"

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
