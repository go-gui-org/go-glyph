//go:build android || linux || darwin || windows

package glyph

import (
	"bytes"
	"testing"
)

// resolveTestGlyph loads the default font, shapes a single-glyph string, and
// returns its path, pixel size, and glyph id. It skips the test when no font
// is installed or the character is uncovered.
func resolveTestGlyph(t *testing.T, ch string) (path string, size float64, gid uint32) {
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
	g := uint32(buf.Info[0].Glyph)
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

// TestRenderGlyphByIDStrokedMatchesShaped extends the by-id equivalence check
// to the stroked path: a stroked glyph rasterized by id must match the stroked
// shaped rasterization byte for byte.
func TestRenderGlyphByIDStrokedMatchesShaped(t *testing.T) {
	path, size, gid := resolveTestGlyph(t, "H")
	const sw = 1.5

	byID := renderGlyphByID(path, size, sw, 0, gid)
	if byID == nil {
		t.Fatal("renderGlyphByID (stroked) produced no ink for H")
	}
	shaped, _ := renderStrokedRun(path, size, sw, 0, "H", "", 0, false)
	if shaped == nil {
		t.Fatal("renderStrokedRun produced no ink for H")
	}
	if byID.w != shaped.w || byID.h != shaped.h ||
		byID.left != shaped.left || byID.top != shaped.top {
		t.Fatalf("stroked cell geometry mismatch: byID=%dx%d@(%d,%d) shaped=%dx%d@(%d,%d)",
			byID.w, byID.h, byID.left, byID.top,
			shaped.w, shaped.h, shaped.left, shaped.top)
	}
	if !bytes.Equal(byID.data, shaped.data) {
		t.Error("stroked pixel data mismatch between by-id and shaped")
	}
}

// TestDrawLayoutPlacedStream drives the documented per-glyph placement pattern
// (size placements to len(Glyphs), fill by GlyphInfo.Index) end-to-end through
// the stream, confirming DrawLayoutPlaced accepts the shaped glyph count and
// rasterizes ink.
func TestDrawLayoutPlacedStream(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Wave", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	positions := layout.GlyphPositions()
	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{X: -9999, Y: -9999}
	}
	for _, p := range positions {
		placements[p.Index] = GlyphPlacement{X: p.X + 50, Y: p.Y + 50}
	}

	b := newMockBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.DrawLayoutPlaced(layout, placements)
	r.Commit()

	inked := false
	for _, tx := range b.textures {
		for _, v := range tx {
			if v != 0 {
				inked = true
				break
			}
		}
	}
	if !inked {
		t.Error("DrawLayoutPlaced rasterized no ink (length guard rejected placements?)")
	}
}

// TestByIDCacheKeyDistinguishesSize guards a cache collision: when the font
// size is encoded in FontName (Style.Size == 0), two items sharing a GlyphID,
// FontPath and Ascent but differing in size must not share an atlas cell.
// Ascent is held equal on purpose so targetH cannot mask the bug — FontName is
// the only size discriminator.
func TestByIDCacheKeyDistinguishesSize(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")

	b := newMockBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	item := func(fontName string) Item {
		return Item{
			Style:    TextStyle{FontName: fontName},
			FontPath: path,
			Ascent:   20,
		}
	}
	g := Glyph{GlyphID: gid, Index: 0, Codepoint: 1}

	big := r.getOrLoadGlyph("H", item("Sans 40"), g, 0, 0)
	small := r.getOrLoadGlyph("H", item("Sans 10"), g, 0, 0)

	if big.Width == small.Width && big.Height == small.Height {
		t.Errorf("40px and 10px glyphs share a cell (%dx%d): by-id cache key ignores font size",
			big.Width, big.Height)
	}
	if big.Height <= small.Height {
		t.Errorf("40px cell (%dx%d) not larger than 10px cell (%dx%d)",
			big.Width, big.Height, small.Width, small.Height)
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

// TestLigatureCaretSkipsInterior verifies that a cluster absorbed into a
// ligature is not a valid caret stop — carets land only at ligature
// boundaries. It probes a few ligating strings and skips if none ligate on
// this host (no covering font / ligatures disabled).
func TestLigatureCaretSkipsInterior(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	for _, s := range []string{"لا", "لام", "ffi", "fi"} {
		layout, err := ctx.LayoutText(s, TextConfig{})
		if err != nil {
			continue
		}
		valid := make(map[int]bool)
		for _, p := range layout.GetValidCursorPositions() {
			valid[p] = true
		}

		foundLigature := false
		for _, it := range layout.Items {
			hasReal := false
			for j := it.GlyphStart; j < it.GlyphStart+it.GlyphCount; j++ {
				if layout.Glyphs[j].GlyphID != 0 {
					hasReal = true
					break
				}
			}
			if !hasReal {
				continue // run not shaped/covered on this host
			}
			// An absorbed cluster emits a zero-id, ~zero-advance placeholder
			// inside an otherwise-shaped item. (A .notdef box has id 0 but a
			// real advance, so the advance guard excludes it.)
			for j := it.GlyphStart; j < it.GlyphStart+it.GlyphCount; j++ {
				g := layout.Glyphs[j]
				if g.GlyphID == 0 && g.XAdvance < 0.01 {
					foundLigature = true
					if valid[int(g.Index)] {
						t.Errorf("%q: caret allowed inside ligature at byte %d",
							s, g.Index)
					}
				}
			}
		}
		if foundLigature {
			return // exercised at least one ligature
		}
	}
	t.Skip("no ligature formed on this host; skipping")
}

// TestCombiningMarkPositioned verifies the stream carries HarfBuzz mark
// offsets: a base+combining-mark grapheme shapes to multiple glyphs sharing one
// cluster, and the mark glyph must carry a nonzero x/y offset so it overlaps
// the base instead of advancing past it. Skips where the host font precomposes
// the sequence or lacks mark positioning.
func TestCombiningMarkPositioned(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	candidates := []string{
		"e\u0301",      // e + combining acute
		"a\u0300",      // a + combining grave
		"o\u0308",      // o + combining diaeresis
		"n\u0303",      // n + combining tilde
		"\u0628\u064e", // Arabic beh + fatha
	}
	for _, s := range candidates {
		layout, err := ctx.LayoutText(s, TextConfig{})
		if err != nil {
			continue
		}
		byCluster := make(map[uint32][]Glyph)
		for _, g := range layout.Glyphs {
			if g.GlyphID != 0 {
				byCluster[g.Index] = append(byCluster[g.Index], g)
			}
		}
		for _, gs := range byCluster {
			if len(gs) < 2 {
				continue // precomposed or single-glyph: no separate mark
			}
			for _, g := range gs {
				if g.XOffset != 0 || g.YOffset != 0 {
					return // mark carries a positioning offset — verified
				}
			}
		}
	}
	t.Skip("no positioned combining mark on this host; skipping")
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
