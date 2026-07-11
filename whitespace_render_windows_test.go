//go:build windows

package glyph

import (
	"testing"
)

// TestSpaceNoInk guards the whitespace-dot regression: a space between
// emoji (or anywhere) must not pick up a stray glyph from a CJK fallback
// font whose U+0020 carries a spurious contour.
func TestSpaceNoInk(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()

	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 512, 512)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	item := Item{Style: TextStyle{Size: 32}, Ascent: 32, Descent: 8}
	res, err := loadGlyphFT(atlas, " ", item, 0, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached.Width != 0 || res.Cached.Height != 0 {
		t.Errorf("space rasterized to %dx%d ink; want empty (stray dot)",
			res.Cached.Width, res.Cached.Height)
	}
}
