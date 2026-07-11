//go:build android || linux || darwin || windows

package glyph

import (
	"bytes"
	"testing"
)

// resolveTestGlyph loads the default font, shapes a single-glyph string, and
// returns its path, pixel size, and glyph id. It skips the test when no font
// is installed or the character is uncovered.
func resolveTestGlyph(t *testing.T, ch string) (path string, size float64, gid uint16) {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	t.Cleanup(ctx.Free)

	var item Item
	family, fontSize, bold, italic := resolveFTFontParams(item.Style, 1.0)
	paths := fontFallbackPaths(ftFontPathsSingleton, family, bold, italic)
	if len(paths) == 0 {
		t.Skip("no font installed; skipping by-id render test")
	}
	path = paths[0]

	cf := loadCachedFace(path)
	if cf == nil {
		t.Skipf("could not load face %q", path)
	}
	buf := shapeWith(cf, fontSize, ch)
	if buf == nil || len(buf.Info) != 1 {
		t.Skipf("%q did not shape to a single glyph", ch)
	}
	g := uint16(buf.Info[0].Glyph)
	if g == 0 {
		t.Skipf("%q not covered by %q", ch, path)
	}
	return path, fontSize, g
}

// TestRenderGlyphByIDMatchesShaped checks that rasterizing a glyph directly by
// id produces the exact same cell as shaping the run and rasterizing — for a
// single glyph with no HarfBuzz offset, the two paths must be byte-identical.
func TestRenderGlyphByIDMatchesShaped(t *testing.T) {
	path, size, gid := resolveTestGlyph(t, "H")

	byID := renderGlyphByID(path, size, 0, 0, gid)
	if byID == nil {
		t.Fatal("renderGlyphByID produced no ink for H")
	}
	shaped, _ := renderMonoRun(path, size, 0, "H", "", 0, false)
	if shaped == nil {
		t.Fatal("renderMonoRun produced no ink for H")
	}

	if byID.w != shaped.w || byID.h != shaped.h ||
		byID.left != shaped.left || byID.top != shaped.top {
		t.Fatalf("cell geometry mismatch: byID=%dx%d@(%d,%d) shaped=%dx%d@(%d,%d)",
			byID.w, byID.h, byID.left, byID.top,
			shaped.w, shaped.h, shaped.left, shaped.top)
	}
	if !bytes.Equal(byID.data, shaped.data) {
		t.Error("pixel data mismatch between by-id and shaped rasterization")
	}
}

// TestLoadGlyphByIDFT verifies the by-id load path uploads a non-empty cell to
// the atlas.
func TestLoadGlyphByIDFT(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")

	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatalf("NewGlyphAtlas: %v", err)
	}
	defer atlas.Free()

	res, err := loadGlyphByIDFT(atlas, path, gid, Item{}, 0, 0, 1.0)
	if err != nil {
		t.Fatalf("loadGlyphByIDFT: %v", err)
	}
	if res.Cached.Width == 0 || res.Cached.Height == 0 {
		t.Errorf("expected a non-empty cached glyph, got %dx%d",
			res.Cached.Width, res.Cached.Height)
	}
}

// TestLayoutPopulatesGlyphStream verifies that plain Latin layout now emits a
// shaped-glyph stream: one glyph per cluster carrying a resolved GlyphID, an
// item FontPath, and advances that sum to the item width.
func TestLayoutPopulatesGlyphStream(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Hello", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || len(layout.Glyphs) == 0 {
		t.Fatal("empty layout")
	}
	if layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	// Plain Latin has no ligatures: one glyph per grapheme cluster.
	if len(layout.Glyphs) != 5 {
		t.Errorf("glyph count = %d, want 5 for \"Hello\"", len(layout.Glyphs))
	}
	nonzero := 0
	for _, g := range layout.Glyphs {
		if g.GlyphID != 0 {
			nonzero++
		}
	}
	if nonzero != len(layout.Glyphs) {
		t.Errorf("%d/%d glyphs carry a GlyphID, want all",
			nonzero, len(layout.Glyphs))
	}

	// Each item's glyph advances must sum to its width (the pen/CharRect
	// invariant the stream must preserve).
	for i, item := range layout.Items {
		var sum float64
		for j := item.GlyphStart; j < item.GlyphStart+item.GlyphCount; j++ {
			sum += layout.Glyphs[j].XAdvance
		}
		if diff := sum - item.Width; diff < -1e-6 || diff > 1e-6 {
			t.Errorf("item %d: glyph advance sum %.6f != width %.6f",
				i, sum, item.Width)
		}
	}
}

// TestRenderStreamMatchesLegacy renders the same layout end-to-end two ways —
// through the by-glyph-id stream, and with GlyphIDs zeroed to force the legacy
// text-reshape path — and asserts the uploaded atlas pixels are identical. For
// non-contextual Latin the two paths must produce the same glyphs in the same
// order, so the packed atlas pages match byte for byte.
func TestRenderStreamMatchesLegacy(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Hello", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}
	streamed := false
	for _, g := range layout.Glyphs {
		if g.GlyphID != 0 {
			streamed = true
			break
		}
	}
	if !streamed {
		t.Skip("no glyph ids resolved; nothing to compare")
	}

	renderCapture := func(l Layout) map[TextureID][]byte {
		b := newMockBackend()
		r, err := NewRenderer(b, 1.0)
		if err != nil {
			t.Fatalf("NewRenderer: %v", err)
		}
		defer r.Free()
		r.DrawLayout(l, 10, 20)
		r.Commit()
		return b.textures
	}

	// Zeroing GlyphID forces byID=false → the text-reshape path.
	legacy := layout
	legacy.Glyphs = append([]Glyph(nil), layout.Glyphs...)
	for i := range legacy.Glyphs {
		legacy.Glyphs[i].GlyphID = 0
	}

	newTex := renderCapture(layout)
	oldTex := renderCapture(legacy)

	if len(newTex) != len(oldTex) {
		t.Fatalf("texture count differs: stream=%d legacy=%d",
			len(newTex), len(oldTex))
	}
	for id, nd := range newTex {
		od, ok := oldTex[id]
		if !ok {
			t.Fatalf("texture %d present in stream render, missing in legacy", id)
		}
		if !bytes.Equal(nd, od) {
			t.Errorf("texture %d pixels differ between stream and legacy render", id)
		}
	}
}

// TestLayoutArabicStream verifies Arabic shapes through the stream path: with a
// covering font the run yields resolved glyph ids, and ligature absorption can
// make the glyph count differ from the cluster count.
func TestLayoutArabicStream(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	const arabic = "سلام"
	layout, err := ctx.LayoutText(arabic, TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	streamed := false
	for _, g := range layout.Glyphs {
		if g.GlyphID != 0 {
			streamed = true
			break
		}
	}
	if !streamed {
		t.Skip("no installed font covers Arabic; skipping")
	}

	// Advance invariant must hold for the joined/ligated run too.
	for i, item := range layout.Items {
		var sum float64
		for j := item.GlyphStart; j < item.GlyphStart+item.GlyphCount; j++ {
			sum += layout.Glyphs[j].XAdvance
		}
		if diff := sum - item.Width; diff < -1e-6 || diff > 1e-6 {
			t.Errorf("item %d: glyph advance sum %.6f != width %.6f",
				i, sum, item.Width)
		}
	}
}
