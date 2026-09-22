package glyph

import (
	"math"
	"testing"
)

// TestSanitizeScale is the regression test for NaN and infinite scale
// factors. The WASM NewContext checked scaleFactor <= 0, which is false
// for NaN, so a NaN scale reached scaleInv.
func TestSanitizeScale(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	cases := []struct{ in, want float32 }{
		{2, 2}, {0.5, 0.5}, {0, 1}, {-1, 1}, {nan, 1}, {inf, 1}, {-inf, 1},
	}
	for _, c := range cases {
		if got := sanitizeScale(c.in); got != c.want {
			t.Errorf("sanitizeScale(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestParseFontSizeShared pins the shared size parser. Native truncated
// "12.5" to 12; WASM took "12px" as 12 through fmt.Sscanf.
func TestParseFontSizeShared(t *testing.T) {
	cases := []struct {
		name string
		want float32
	}{
		{"Sans 12.5", 12.5},
		{"Sans 12px", 0},
		{"Sans Inf", 0},
		{"Sans NaN", 0},
		{"Sans -3", 0},
		{"Sans 0x10", 0},
		{"Sans 1e400", 0},
		{"Sans  14 ", 14},
		{"14", 14},
		{"", 0},
	}
	for _, c := range cases {
		if got := parseSizeFromFontName(c.name); got != c.want {
			t.Errorf("parseSizeFromFontName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
	if got := parseFamilyFromFontName("Fira Code Light 10.5"); got != "Fira Code" {
		t.Errorf("parseFamilyFromFontName = %q, want Fira Code", got)
	}
}

func TestParseFamilyFromFontNameAllocs(t *testing.T) {
	// The style-word map is built once at package init, not per call.
	n := testing.AllocsPerRun(100, func() {
		_ = parseFamilyFromFontName("Sans Bold 12")
	})
	// strings.Fields and ToLower("Bold") allocate; the old per-call map
	// literal added several more.
	if n > 3 {
		t.Errorf("parseFamilyFromFontName allocs = %v, want <= 3", n)
	}
}

func TestResolveFontSize(t *testing.T) {
	nan := float32(math.NaN())
	cases := []struct {
		style TextStyle
		want  float32
	}{
		{TextStyle{Size: 20}, 20},
		{TextStyle{Size: nan, FontName: "Sans 11"}, 11},
		{TextStyle{Size: float32(math.Inf(1))}, 16},
		{TextStyle{Size: -2}, 16},
	}
	for _, c := range cases {
		if got := resolveFontSize(c.style); got != c.want {
			t.Errorf("resolveFontSize(%+v) = %v, want %v", c.style, got, c.want)
		}
	}
}
