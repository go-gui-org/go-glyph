//go:build linux || darwin || windows

package glyph

import (
	"testing"

	"github.com/go-text/typesetting/font"
)

// colorGlyphBox turns font-unit extents into the bitmap box at the requested
// size. An sbix glyph drops by sbixBaselineDrop, as CoreText draws it.
func TestColorGlyphBox(t *testing.T) {
	// Apple Color Emoji: upem 800, a 1 em square with its bottom on the
	// baseline per the sbix table.
	apple := font.GlyphExtents{XBearing: 0, YBearing: 800, Width: 800, Height: -800}
	// Noto Color Emoji (CBDT): upem 2048, a box wider than 1 em that
	// reaches below the baseline.
	noto := font.GlyphExtents{XBearing: 0, YBearing: 1900, Width: 2550, Height: -2400}

	tests := []struct {
		name string
		ext  font.GlyphExtents
		ok   bool
		size float64
		upem uint16
		sbix bool
		want colorBox
	}{
		{"sbix drops 0.125 em", apple, true, 40, 800, true,
			colorBox{w: 40, h: 40, left: 0, top: 35}},
		{"same extents without sbix", apple, true, 40, 800, false,
			colorBox{w: 40, h: 40, left: 0, top: 40}},
		{"cbdt bearings", noto, true, 20.48, 2048, false,
			colorBox{w: 26, h: 24, left: 0, top: 19}},
	}
	for _, tc := range tests {
		got, ok := colorGlyphBox(tc.ext, tc.ok, tc.size, tc.upem, tc.sbix)
		if !ok || got != tc.want {
			t.Errorf("%s: got %+v ok=%v, want %+v", tc.name, got, ok, tc.want)
		}
	}

	// Missing or empty extents fall back to the strike's own box.
	bad := []struct {
		name string
		ext  font.GlyphExtents
		ok   bool
		size float64
	}{
		{"no extents", apple, false, 40},
		{"zero width", font.GlyphExtents{YBearing: 800, Height: -800}, true, 40},
		{"zero height", font.GlyphExtents{YBearing: 800, Width: 800}, true, 40},
		{"huge size", apple, true, 1e9},
	}
	for _, tc := range bad {
		if got, ok := colorGlyphBox(tc.ext, tc.ok, tc.size, 800, true); ok {
			t.Errorf("%s: got %+v ok=true, want ok=false", tc.name, got)
		}
	}
}
