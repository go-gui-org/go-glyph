//go:build darwin && !glyph_pango

package glyph

import "testing"

func newTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	backend := newMockBackend()
	r, err := NewRendererWithConfig(backend, 1.0, 512, 512, RendererConfig{})
	if err != nil {
		t.Fatalf("NewRendererWithConfig: %v", err)
	}
	return r
}

func makeTestItem() Item {
	return Item{
		Style:  TextStyle{FontName: "Sans 16"},
		Ascent: 20,
	}
}

// ---------------------------------------------------------------------------
// getOrLoadGlyph — cache key with GlyphID
// ---------------------------------------------------------------------------

func TestGetOrLoadGlyph_DifferentGlyphIDs_DifferentCacheEntries(t *testing.T) {
	r := newTestRenderer(t)
	defer r.Free()

	item := makeTestItem()
	g1 := Glyph{Index: 0, Codepoint: 1, GlyphID: 1}
	g2 := Glyph{Index: 0, Codepoint: 1, GlyphID: 2}

	r.getOrLoadGlyph("A", item, g1, 0, 0)
	r.getOrLoadGlyph("A", item, g2, 0, 0)

	if len(r.cache) != 2 {
		t.Errorf("cache len=%d, want 2 (distinct GlyphIDs must produce distinct keys)",
			len(r.cache))
	}
}

func TestGetOrLoadGlyph_SameGlyphID_CacheHit(t *testing.T) {
	r := newTestRenderer(t)
	defer r.Free()

	item := makeTestItem()
	g := Glyph{Index: 0, Codepoint: 1, GlyphID: 5}

	r.getOrLoadGlyph("A", item, g, 0, 0)
	if len(r.cache) != 1 {
		t.Fatalf("after first call, cache len=%d, want 1", len(r.cache))
	}

	r.getOrLoadGlyph("A", item, g, 0, 0)
	if len(r.cache) != 1 {
		t.Errorf("after second call, cache len=%d, want 1 (should be a cache hit)",
			len(r.cache))
	}
}

func TestGetOrLoadGlyph_GlyphIDZero_FeaturesHashedSeparately(t *testing.T) {
	r := newTestRenderer(t)
	defer r.Free()

	g := Glyph{Index: 0, Codepoint: 1, GlyphID: 0}

	// No features.
	itemPlain := makeTestItem()
	r.getOrLoadGlyph("A", itemPlain, g, 0, 0)

	// Same text, same font, but with an OpenType feature — must produce a
	// different cache key so shaped vs. unshaped renders don't collide.
	feat := &FontFeatures{OpenTypeFeatures: []FontFeature{{Tag: "liga", Value: 1}}}
	itemFeat := Item{
		Style:  TextStyle{FontName: "Sans 16", Features: feat},
		Ascent: 20,
	}
	r.getOrLoadGlyph("A", itemFeat, g, 0, 0)

	if len(r.cache) != 2 {
		t.Errorf("cache len=%d, want 2 (feature hash should distinguish entries)",
			len(r.cache))
	}
}

func TestGetOrLoadGlyph_EmptyText_ReturnsEmpty(t *testing.T) {
	r := newTestRenderer(t)
	defer r.Free()

	item := makeTestItem()
	// Glyph whose byte range is out of bounds → glyphText returns "".
	g := Glyph{Index: 100, Codepoint: 1, GlyphID: 0}
	cg := r.getOrLoadGlyph("A", item, g, 0, 0)
	if cg != (CachedGlyph{}) {
		t.Errorf("out-of-bounds glyph: got %+v, want zero CachedGlyph", cg)
	}
	if len(r.cache) != 0 {
		t.Errorf("cache len=%d, want 0 (empty text skips caching)", len(r.cache))
	}
}
