//go:build android || linux || darwin || windows

package glyph

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/vector"
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

	byID := renderGlyphByID(nil, path, size, 0, 0, gid)
	if byID == nil {
		t.Fatal("renderGlyphByID produced no ink for H")
	}
	shaped, _ := renderMonoRun(nil, path, size, 0, "H", "", 0, false)
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

	byID := renderGlyphByID(nil, path, size, sw, 0, gid)
	if byID == nil {
		t.Fatal("renderGlyphByID (stroked) produced no ink for H")
	}
	shaped, _ := renderStrokedRun(nil, path, size, sw, 0, "H", "", 0, false)
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

// TestRenderGlyphByIDHugeStrokeRefuses checks that a stroke radius too large
// to fit in an int (or infinite) refuses cleanly. The bounds are float64;
// converting an out-of-range float to int is implementation-defined in Go
// (it gives MinInt64 on amd64), so the raw w0/h0 math could wrap to a small
// positive size and render garbage instead of refusing.
func TestRenderGlyphByIDHugeStrokeRefuses(t *testing.T) {
	path, size, gid := resolveTestGlyph(t, "H")
	for _, sw := range []float64{math.Inf(1), 1e300, 1e12} {
		if got := renderGlyphByID(nil, path, size, sw, 0, gid); got != nil {
			t.Errorf("strokeWidth %g: got %dx%d bitmap, want refusal",
				sw, got.w, got.h)
		}
	}
}

// TestRenderGlyphByIDOversizeDownscales checks that ink larger than
// MaxGlyphSize is shrunk to fit, not cropped to the top-left 256px, and that
// ink beyond maxNaturalGlyphDim refuses instead of allocating a huge buffer.
func TestRenderGlyphByIDOversizeDownscales(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")
	for _, atlasOn := range []bool{false, true} {
		var atlas *GlyphAtlas
		if atlasOn {
			a, err := NewGlyphAtlas(newMockBackend(), 64, 64)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Free()
			atlas = a
		}
		got := renderGlyphByID(atlas, path, 600, 0, 0, gid)
		if got == nil {
			t.Fatalf("atlas=%v: 600px H refused, want downscaled cell", atlasOn)
		}
		if got.w > MaxGlyphSize || got.h > MaxGlyphSize {
			t.Errorf("atlas=%v: cell %dx%d exceeds MaxGlyphSize", atlasOn, got.w, got.h)
		}
		// A cropped H fills its 256px cell to the edge; a downscaled one
		// keeps the aspect, so exactly one side reaches MaxGlyphSize.
		if max(got.w, got.h) != MaxGlyphSize {
			t.Errorf("atlas=%v: cell %dx%d, want longest side %d",
				atlasOn, got.w, got.h, MaxGlyphSize)
		}
		if len(got.data) < got.w*got.h*4 {
			t.Errorf("atlas=%v: data %d bytes, want >= %d", atlasOn,
				len(got.data), got.w*got.h*4)
		}
	}
	if got := renderGlyphByID(nil, path, 5000, 0, 0, gid); got != nil {
		t.Errorf("5000px H: got %dx%d, want refusal past maxNaturalGlyphDim",
			got.w, got.h)
	}
}

// TestRenderGlyphByIDBadStrokeRendersFilled checks that a negative or NaN
// stroke width renders the plain filled glyph, the same as width 0.
func TestRenderGlyphByIDBadStrokeRendersFilled(t *testing.T) {
	path, size, gid := resolveTestGlyph(t, "H")
	want := renderGlyphByID(nil, path, size, 0, 0, gid)
	if want == nil {
		t.Fatal("filled H produced no ink")
	}
	for _, sw := range []float64{-2, math.NaN(), math.Inf(-1)} {
		got := renderGlyphByID(nil, path, size, sw, 0, gid)
		if got == nil || got.w != want.w || got.h != want.h ||
			!bytes.Equal(got.data, want.data) {
			t.Errorf("strokeWidth %g: output differs from the filled glyph", sw)
		}
	}
}

// TestRenderGlyphByIDInvalidSizeRefuses checks that zero, negative, NaN and
// infinite font sizes refuse before they reach the shaping and raster math.
func TestRenderGlyphByIDInvalidSizeRefuses(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")
	for _, s := range []float64{0, -12, math.NaN(), math.Inf(1)} {
		if got := renderGlyphByID(nil, path, s, 0, 0, gid); got != nil {
			t.Errorf("size %g: got %dx%d, want refusal", s, got.w, got.h)
		}
	}
}

// TestAddOutlineSkipsContourWithoutMoveTo checks that segments before the
// first MoveTo (a malformed font) are dropped, not drawn from the origin.
func TestAddOutlineSkipsContourWithoutMoveTo(t *testing.T) {
	seg := func(op ot.SegmentOp, x, y float32) ot.Segment {
		return ot.Segment{Op: op, Args: [3]ot.SegmentPoint{{X: x, Y: y}}}
	}
	mapPt := func(_ placedGlyph, fx, fy float32) (float64, float64) {
		return float64(fx), float64(fy)
	}
	// Only stray LineTos: nothing may be drawn.
	g := placedGlyph{segs: []ot.Segment{
		seg(ot.SegmentOpLineTo, 8, 0),
		seg(ot.SegmentOpLineTo, 8, 8),
	}}
	rz := vector.NewRasterizer(16, 16)
	addOutline(rz, g, mapPt, 4, 4)
	dst := image.NewAlpha(image.Rect(0, 0, 16, 16))
	rz.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	for _, a := range dst.Pix {
		if a != 0 {
			t.Fatal("contour without MoveTo produced ink")
		}
	}
}

// TestLoadBoxGlyphFTInvalidDims checks that a degenerate box cell is a
// no-op (empty result, no error, no panic) and that a cell too large to
// allocate returns an error.
func TestLoadBoxGlyphFTInvalidDims(t *testing.T) {
	atlas, err := NewGlyphAtlas(newMockBackend(), 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	for _, m := range []boxMetrics{{cp: 0x2500, cellW: 0, cellH: 10},
		{cp: 0x2500, cellW: 10, cellH: -1}} {
		res, err := loadBoxGlyphFT(atlas, m)
		if err != nil || res != (LoadGlyphResult{}) {
			t.Errorf("%dx%d: got (%+v, %v), want empty no-op", m.cellW, m.cellH, res, err)
		}
	}
	huge := boxMetrics{cp: 0x2500, cellW: 1 << 20, cellH: 1 << 20}
	if _, err := loadBoxGlyphFT(atlas, huge); err == nil {
		t.Error("huge cell: want allocation error")
	}
}

// TestPaletteColorForeground verifies the 0xFFFF "use text foreground"
// COLR index resolves to the run's text color, falling back to opaque
// black only when no color was set, and that real/out-of-range palette
// indices resolve normally.
func TestPaletteColorForeground(t *testing.T) {
	fg := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	if got := paletteColor(nil, 0xFFFF, fg); got != fg {
		t.Errorf("0xFFFF = %+v, want fg %+v", got, fg)
	}
	if got := paletteColor(nil, 0xFFFF, color.NRGBA{}); got != (color.NRGBA{A: 255}) {
		t.Errorf("transparent fg fallback = %+v, want opaque black", got)
	}
	pal := []tables.ColorRecord{{Red: 1, Green: 2, Blue: 3, Alpha: 4}}
	want := color.NRGBA{R: 1, G: 2, B: 3, A: 4}
	if got := paletteColor(pal, 0, fg); got != want {
		t.Errorf("palette[0] = %+v, want %+v", got, want)
	}
	if got := paletteColor(pal, 7, fg); got != (color.NRGBA{A: 255}) {
		t.Errorf("out-of-range index = %+v, want opaque black", got)
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

// TestSuperscriptAdvanceIsSmall guards the run-shaping size boundary: a
// superscript reuses the base .ttf at a reduced size (same face pointer), so if
// runs are grouped by face alone it is shaped with the base font and takes a
// base-size advance while rendering small — leaving a gap after the glyph. The
// superscript run's advance must be smaller than the same glyph at base size.
func TestSuperscriptAdvanceIsSmall(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	sups := &FontFeatures{OpenTypeFeatures: []FontFeature{{Tag: "sups", Value: 1}}}
	rt := RichText{Runs: []StyleRun{
		{Text: "x", Style: TextStyle{FontName: "Sans 40"}},
		{Text: "x", Style: TextStyle{FontName: "Sans 40", Features: sups}},
	}}
	layout, err := ctx.LayoutRichText(rt, TextConfig{})
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(layout.Items) < 2 {
		t.Skipf("expected 2 items, got %d (no font on host?)", len(layout.Items))
	}

	base := layout.Items[0].Width
	sup := layout.Items[1].Width
	if base <= 0 {
		t.Skip("base glyph has no advance; skipping")
	}
	if sup >= base {
		t.Errorf("superscript advance %.2f not smaller than base %.2f: run grouped sub/sup at base size",
			sup, base)
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

// TestRichTextScriptFallback verifies LayoutRichText applies script fallback:
// a symbol the run's authored (proportional body) font lacks — ✓ U+2713,
// ✗ U+2717 — must resolve to a covering fallback font's real glyph id rather
// than shaping to .notdef (GlyphID 0) and rendering as tofu. This mirrors the
// fallback LayoutText already performs; the rich-text path previously locked
// each cluster to its run font with no fallback probe.
func TestRichTextScriptFallback(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	rt := RichText{Runs: []StyleRun{
		{Text: "a\u2713b\u2717c", Style: TextStyle{}},
	}}
	layout, err := ctx.LayoutRichText(rt, TextConfig{})
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}
	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no fallback fonts installed; skipping")
	}

	// Locate the checkmark clusters by byte index and confirm each shaped to
	// a real (nonzero) glyph id. Skip a codepoint no installed font covers.
	want := map[int]rune{1: '\u2713', 4: '\u2717'}
	for _, g := range layout.Glyphs {
		r, ok := want[int(g.Index)]
		if !ok {
			continue
		}
		delete(want, int(g.Index))
		covered := false
		for _, p := range ctx.fallbackPaths {
			if cov := loadCoverage(p); cov != nil && cov.covers(string(r)) {
				covered = true
				break
			}
		}
		if !covered {
			continue // no fallback covers this symbol on this host
		}
		if g.GlyphID == 0 {
			t.Errorf("U+%04X shaped to .notdef (tofu); script fallback not applied", r)
		}
	}
}

// TestRichTextScriptFallbackMixedSize guards the per-size fallback font cache:
// two runs of different sizes whose symbol needs the same fallback path must
// each shape at their own size, so the fallback glyph's advance scales with the
// run. Caching the fallback font by path alone would reuse the first run's size
// for the second, baking a wrong-sized advance. The big run's ✓ advance must
// exceed the small run's.
func TestRichTextScriptFallbackMixedSize(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	// Big run first, then small: exercises the cache-reuse ordering.
	rt := RichText{Runs: []StyleRun{
		{Text: "\u2713", Style: TextStyle{FontName: "Sans 40"}},
		{Text: "\u2713", Style: TextStyle{FontName: "Sans 10"}},
	}}
	layout, err := ctx.LayoutRichText(rt, TextConfig{})
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" ||
		len(ctx.fallbackPaths) == 0 {
		t.Skip("no font path / fallback resolved on this host; skipping")
	}

	// Collect the resolved glyph advance for each of the two ✓ clusters, in
	// text order (byte index 0 = big run, byte 3 = small run — ✓ is 3 bytes).
	var bigAdv, smallAdv float64
	var bigGID, smallGID uint32
	for _, g := range layout.Glyphs {
		switch g.Index {
		case 0:
			bigAdv, bigGID = g.XAdvance, g.GlyphID
		case 3:
			smallAdv, smallGID = g.XAdvance, g.GlyphID
		}
	}
	if bigGID == 0 || smallGID == 0 {
		t.Skip("✓ not covered by a fallback on this host; skipping")
	}
	if bigAdv <= smallAdv {
		t.Errorf("40px ✓ advance %.2f not larger than 10px %.2f: fallback font cached by path ignores size",
			bigAdv, smallAdv)
	}
}

// TestLoadGlyphByIDNoInk verifies the by-id path degrades gracefully for a
// glyph with no outline (a space): renderGlyphByID returns nil, so
// loadGlyphByIDFT must yield an empty result without an error — the renderer
// then caches a blank glyph rather than treating the run as a load failure.
func TestLoadGlyphByIDNoInk(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, " ")

	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatalf("NewGlyphAtlas: %v", err)
	}
	defer atlas.Free()

	res, err := loadGlyphByIDFT(atlas, path, gid, Item{}, 0, 0, 1.0)
	if err != nil {
		t.Fatalf("loadGlyphByIDFT on a no-ink glyph errored: %v", err)
	}
	if res.Cached.Width != 0 || res.Cached.Height != 0 {
		t.Errorf("expected an empty cached glyph for a space, got %dx%d",
			res.Cached.Width, res.Cached.Height)
	}
}
