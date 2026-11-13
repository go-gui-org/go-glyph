package glyph

import (
	"math"
	"testing"
)

// parseSizeFromStyle must be the same resolver as resolveFontSize, so every
// backend clamps and falls back the same way.
func TestParseSizeFromStyleMatchesResolveFontSize(t *testing.T) {
	nan := float32(math.NaN())
	styles := []TextStyle{
		{Size: float32(math.Inf(1))},
		{Size: nan, FontName: "Sans 11"},
		{Size: 1e30},
		{Size: 0.01},
		{Size: 12},
		{},
	}
	for _, s := range styles {
		if got, want := parseSizeFromStyle(s), resolveFontSize(s); got != want {
			t.Errorf("parseSizeFromStyle(%+v) = %v, resolveFontSize = %v",
				s, got, want)
		}
	}
}
