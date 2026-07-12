//go:build darwin

package glyph

import (
	"os"
	"testing"
)

// TestShapeRecoversAATPanic guards against a process-crashing panic in
// go-text's AAT morx shaper (typesetting v0.3.4) for certain macOS system
// fonts. ftFont.shape must recover and return nil instead of propagating.
// Skips per-font when the font is absent (varies by macOS version).
func TestShapeRecoversAATPanic(t *testing.T) {
	cases := []struct {
		path string
		text string
	}{
		{"/System/Library/Fonts/GeezaPro.ttc", "ً"},
		{"/System/Library/Fonts/Supplemental/Raanana.ttc", "יִ"},
		{"/System/Library/Fonts/Supplemental/NewPeninimMT.ttc", "יִ"},
	}
	ran := false
	for _, c := range cases {
		if _, err := os.Stat(c.path); err != nil {
			continue
		}
		ran = true
		f := newFTFontFromPath(FTLibrary{}, c.path, 16)
		if f.face == nil {
			t.Fatalf("failed to load %s", c.path)
		}
		// Neither shaping entry point may panic (return values are irrelevant;
		// a shaper failure yields no glyphs). Cover both: the layout/probe path
		// (ftFont.shape via hasGlyphs) and the render path (shapeWith).
		_ = f.hasGlyphs(c.text)
		if cf := loadCachedFace(c.path); cf != nil {
			_ = shapeWith(cf, 16, c.text)
		}
	}
	if !ran {
		t.Skip("no known AAT-crashing fonts present on this system")
	}
}
