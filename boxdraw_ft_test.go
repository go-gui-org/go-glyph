//go:build linux || darwin || windows

package glyph

import "testing"

// boxTestItem builds an item that resolves to the built-in path for a
// single box-drawing rune laid out in an 11x23 physical cell.
func boxTestItem(text string) (Item, Glyph) {
	item := Item{
		Style:      TextStyle{FontName: "monospace 12", CellWidth: 11, CellHeight: 23},
		FontPath:   "/test/font.ttf",
		Ascent:     18,
		Descent:    5,
		GlyphCount: 1,
		Length:     len(text),
	}
	g := Glyph{XAdvance: 11, Codepoint: uint32(len(text)), GlyphID: 7, Shaped: true}
	return item, g
}

func newBoxTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Free)
	return r
}

// TestBoxGlyphIsCellSized checks the atlas entry covers exactly the cell
// with a zero left bearing, which is what makes the emitted quad tile.
func TestBoxGlyphIsCellSized(t *testing.T) {
	r := newBoxTestRenderer(t)
	item, g := boxTestItem("─")

	cg := r.getOrLoadGlyph("─", item, g, 0, 0)
	if cg.Width != 11 || cg.Height != 23 {
		t.Fatalf("glyph is %dx%d, want 11x23", cg.Width, cg.Height)
	}
	if cg.Left != 0 || cg.Top != 18 {
		t.Fatalf("bearings = (%d,%d), want (0,18)", cg.Left, cg.Top)
	}
}

// TestBoxGlyphCacheIgnoresBin is the point of leaving the sub-pixel bin out
// of the key: all four phases resolve to one atlas entry, so a run of cells
// cannot alternate between a crisp and a split stem.
func TestBoxGlyphCacheIgnoresBin(t *testing.T) {
	r := newBoxTestRenderer(t)
	item, g := boxTestItem("│")

	before := len(r.cache)
	var first CachedGlyph
	for bin := range SubpixelBins {
		cg := r.getOrLoadGlyph("│", item, g, bin, 0)
		if bin == 0 {
			first = cg
		} else if cg != first {
			t.Fatalf("bin %d returned %+v, want %+v", bin, cg, first)
		}
	}
	if got := len(r.cache) - before; got != 1 {
		t.Fatalf("%d cache entries for four bins, want 1", got)
	}
}

// TestBoxGlyphKeyTracksScale checks a DPI change lands on a different key,
// since the scale is only present in the key through the pixel dimensions.
func TestBoxGlyphKeyTracksScale(t *testing.T) {
	item, g := boxTestItem("─")

	m1, ok1 := boxMetricsFor(item, g, '─', 1)
	m2, ok2 := boxMetricsFor(item, g, '─', 2)
	if !ok1 || !ok2 {
		t.Fatal("metrics did not resolve at both scales")
	}
	if hashBoxGlyph(fnvOffsetBasis, m1) == hashBoxGlyph(fnvOffsetBasis, m2) {
		t.Fatal("1x and 2x share a cache key")
	}
}

// TestBoxGlyphKeyNamespace checks the built-in key cannot alias the font
// key for the same glyph, which would swap one for the other in the atlas.
func TestBoxGlyphKeyNamespace(t *testing.T) {
	item, g := boxTestItem("─")
	m, _ := boxMetricsFor(item, g, '─', 1)

	boxKey := hashBoxGlyph(fnvOffsetBasis, m)

	byID := fnvHashU64(fnvOffsetBasis, uint64(g.GlyphID))
	byID = fnvHashString(byID, item.FontPath)
	byID = hashGlyphStyle(byID, item, 0, 18, 0)

	byText := fnvHashString(fnvOffsetBasis, "─")
	byText = hashGlyphStyle(byText, item, 0, 18, 0)

	if boxKey == byID || boxKey == byText {
		t.Fatal("built-in key collides with a font-glyph key")
	}
}

// TestBoxGlyphOptOutUsesFontPath checks NoBuiltinBoxGlyphs routes the glyph
// back through the font, producing a different cache entry.
func TestBoxGlyphOptOutUsesFontPath(t *testing.T) {
	item, g := boxTestItem("─")
	if _, ok := boxMetricsFor(item, g, '─', 1); !ok {
		t.Fatal("built-in path not taken by default")
	}
	item.Style.NoBuiltinBoxGlyphs = true
	if _, ok := boxMetricsFor(item, g, '─', 1); ok {
		t.Fatal("NoBuiltinBoxGlyphs did not disable the built-in path")
	}
}

// BenchmarkBoxGlyphSynthesis measures a cold miss: the renderer cache is
// dropped each iteration but the atlas scratch buffers stay warm, so the
// synthesis path itself should not allocate.
func BenchmarkBoxGlyphSynthesis(b *testing.B) {
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Free()

	item, g := boxTestItem("┼")
	m, ok := boxMetricsFor(item, g, '┼', 1)
	if !ok {
		b.Fatal("metrics did not resolve")
	}
	if _, err := loadBoxGlyphFT(r.atlas, m); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := loadBoxGlyphFT(r.atlas, m); err != nil {
			b.Fatal(err)
		}
	}
}
