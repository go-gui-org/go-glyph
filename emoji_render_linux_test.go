//go:build linux && !android

package glyph

import "testing"

// TestEmojiRendersColor verifies loadGlyphFT rasterizes a color-emoji
// glyph as an actual color (BGRA) bitmap for a UseOriginalColor item,
// rather than falling through to a monochrome glyph in the text font.
func TestEmojiRendersColor(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no emoji fallback font installed")
	}
	hasEmoji := false
	for _, p := range ctx.fallbackPaths {
		f := newFTFontFromPath(ctx.ftLib, p, 32)
		if f.isColorFont() && f.hasGlyphs("\U0001F600") {
			hasEmoji = true
		}
		f.close()
	}
	if !hasEmoji {
		t.Skip("no color-emoji font covers 😀")
	}

	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 512, 512)
	if err != nil {
		t.Fatalf("NewGlyphAtlas: %v", err)
	}
	defer atlas.Free()

	item := Item{
		Style:            TextStyle{Size: 32},
		Ascent:           32,
		Descent:          8,
		UseOriginalColor: true,
	}
	res, err := loadGlyphFT(atlas, "\U0001F600", item, 0, 1.0)
	if err != nil {
		t.Fatalf("loadGlyphFT: %v", err)
	}
	cg := res.Cached
	if cg.Width == 0 || cg.Height == 0 {
		t.Fatal("emoji rasterized to empty bitmap")
	}

	page := atlas.Pages[cg.Page]
	stride := page.Width * 4
	colored := 0
	for row := 0; row < cg.Height; row++ {
		for col := 0; col < cg.Width; col++ {
			off := (cg.Y+row)*stride + (cg.X+col)*4
			r, g, b, a := page.StagingBack[off], page.StagingBack[off+1],
				page.StagingBack[off+2], page.StagingBack[off+3]
			if a > 0 && (r != g || g != b) {
				colored++
			}
		}
	}
	if colored == 0 {
		t.Errorf("emoji glyph has no colored pixels (%dx%d) — rendered "+
			"monochrome instead of color", cg.Width, cg.Height)
	} else {
		t.Logf("emoji colored pixels: %d of %d", colored, cg.Width*cg.Height)
	}
}
