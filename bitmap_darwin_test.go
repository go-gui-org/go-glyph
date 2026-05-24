//go:build darwin && !glyph_pango

package glyph

import "testing"

func newTestAtlas(t *testing.T) *GlyphAtlas {
	t.Helper()
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 512, 512)
	if err != nil {
		t.Fatalf("NewGlyphAtlas: %v", err)
	}
	return atlas
}

func makeLoadItem() Item {
	return Item{
		Style:   TextStyle{FontName: "Sans 16"},
		Ascent:  20,
		Descent: 5,
	}
}

// ---------------------------------------------------------------------------
// loadGlyphCG
// ---------------------------------------------------------------------------

func TestLoadGlyphCG_GlyphIDZero_TextPath_NoError(t *testing.T) {
	atlas := newTestAtlas(t)
	defer atlas.Free()

	result, err := loadGlyphCG(atlas, "A", makeLoadItem(), 0, 0, 1.0)
	if err != nil {
		t.Fatalf("loadGlyphCG: %v", err)
	}
	// A real glyph should produce a non-empty result.
	_ = result
}

func TestLoadGlyphCG_GlyphIDZero_EmptyString_NoError(t *testing.T) {
	atlas := newTestAtlas(t)
	defer atlas.Free()

	// Empty string renders a minimal bitmap in C (w/h clamped to 1).
	// The guard that skips empty text lives in getOrLoadGlyph, not here.
	// Verify no error is returned.
	_, err := loadGlyphCG(atlas, "", makeLoadItem(), 0, 0, 1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadGlyphCG_SubpixelBins_NoError(t *testing.T) {
	atlas := newTestAtlas(t)
	defer atlas.Free()

	item := makeLoadItem()
	for bin := 0; bin < 4; bin++ {
		_, err := loadGlyphCG(atlas, "g", item, 0, bin, 2.0)
		if err != nil {
			t.Errorf("bin=%d: %v", bin, err)
		}
	}
}

// ---------------------------------------------------------------------------
// loadStrokedGlyphCG
// ---------------------------------------------------------------------------

func TestLoadStrokedGlyphCG_ExcessiveStrokeWidth_ReturnsEmpty(t *testing.T) {
	atlas := newTestAtlas(t)
	defer atlas.Free()

	// physStroke = 2e6 * 1.0 = 2e6 > 1e6 → early return.
	result, err := loadStrokedGlyphCG(atlas, "A", makeLoadItem(), 0, 2e6, 0, 1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (LoadGlyphResult{}) {
		t.Errorf("excessive stroke: got %+v, want zero result", result)
	}
}

func TestLoadStrokedGlyphCG_NormalStroke_NoError(t *testing.T) {
	atlas := newTestAtlas(t)
	defer atlas.Free()

	_, err := loadStrokedGlyphCG(atlas, "A", makeLoadItem(), 0, 2.0, 0, 1.0)
	if err != nil {
		t.Fatalf("loadStrokedGlyphCG: %v", err)
	}
}

func TestLoadStrokedGlyphCG_GlyphIDNonZero_NoError(t *testing.T) {
	atlas := newTestAtlas(t)
	defer atlas.Free()

	// glyphID != 0 still falls through to text-based stroke rendering.
	_, err := loadStrokedGlyphCG(atlas, "A", makeLoadItem(), 42, 1.5, 0, 1.0)
	if err != nil {
		t.Fatalf("loadStrokedGlyphCG (glyphID=42): %v", err)
	}
}
