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
