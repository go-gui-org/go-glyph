package glyph

import (
	"math"
	"testing"
)

func TestHasComplexShaping(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"ascii", "fi ligature", false},
		{"latin accents", "café naïve", false},
		{"cjk", "日本語", false},
		{"arabic", "لا", true},
		{"arabic in latin", "hello لا world", true},
		{"arabic presentation form", "ﻻ", true},
		{"devanagari", "क्ष", true},
		{"syriac", "ܐ", true},
		{"nko", "ߒߞߏ", true},
		{"adlam", "𞤀𞤣𞤤𞤢𞤥", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasComplexShaping(tt.in); got != tt.want {
				t.Errorf("hasComplexShaping(%q) = %v, want %v",
					tt.in, got, tt.want)
			}
		})
	}
}

// prefixMeasure turns per-cluster contributions into the cumulative
// prefix-width function detectAbsorbed expects, standing in for the
// browser's measureText.
func prefixMeasure(deltas []float64) func(int) float64 {
	return func(n int) float64 {
		var w float64
		for i := 0; i < n; i++ {
			w += deltas[i]
		}
		return w
	}
}

func TestDetectAbsorbed(t *testing.T) {
	tests := []struct {
		name   string
		deltas []float64
		want   []bool // nil means "no cluster absorbed"
	}{
		{"empty run", nil, nil},
		{"single cluster", []float64{10}, nil},
		{
			// A lam-alef: the alef adds nothing to the shaped run.
			name:   "zero delta absorbed",
			deltas: []float64{12, 0},
			want:   []bool{false, true},
		},
		{
			// The ligature is narrower than the isolated lead form, so
			// the delta goes negative — still absorbed.
			name:   "negative delta absorbed",
			deltas: []float64{18, -4},
			want:   []bool{false, true},
		},
		{
			// An "fi" ligature: narrower than f+i apart, but the "i"
			// still advances. Browsers stop the caret there; so do we.
			name:   "discretionary ligature not absorbed",
			deltas: []float64{7, 3.5},
			want:   nil,
		},
		{
			// Kerning shaves a fraction of a pixel; not an absorption.
			name:   "kerning not absorbed",
			deltas: []float64{10, 9.6},
			want:   nil,
		},
		{
			name:   "run of absorbed clusters",
			deltas: []float64{20, 0, 0, 9},
			want:   []bool{false, true, true, false},
		},
		{
			// The whole tail of the run folds into the lead cluster.
			name:   "all absorbed after first",
			deltas: []float64{10, 0, 0},
			want:   []bool{false, true, true},
		},
		{
			// NaN widths (a pathological measure function) must not
			// crash or mark absorption: NaN <= eps is false, so the
			// cluster is treated as visible.
			name:   "nan width not absorbed",
			deltas: []float64{10, math.NaN()},
			want:   nil,
		},
		{
			// A leading zero-width cluster keeps its own position: there
			// is no predecessor to absorb it.
			name:   "first cluster never absorbed",
			deltas: []float64{0, 8},
			want:   nil,
		},
		{
			// Sub-epsilon jitter (a rounded-off zero advance) counts as
			// absorbed.
			name:   "sub-epsilon delta absorbed",
			deltas: []float64{10, ligatureEpsilon / 2},
			want:   []bool{false, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectAbsorbed(len(tt.deltas),
				prefixMeasure(tt.deltas), ligatureEpsilon)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("detectAbsorbed = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("detectAbsorbed = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("detectAbsorbed = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestDetectAbsorbedMeasuresPrefixes pins the contract with the caller:
// every prefix length from 1..count is measured exactly once, so the
// wasm side can size its substring slicing accordingly.
func TestDetectAbsorbedMeasuresPrefixes(t *testing.T) {
	var seen []int
	detectAbsorbed(4, func(n int) float64 {
		seen = append(seen, n)
		return float64(n) * 10
	}, ligatureEpsilon)
	want := []int{1, 2, 3, 4}
	if len(seen) != len(want) {
		t.Fatalf("prefix lengths measured = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("prefix lengths measured = %v, want %v", seen, want)
		}
	}
}
