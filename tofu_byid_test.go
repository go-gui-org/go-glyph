//go:build android || linux || darwin || windows

package glyph

import "testing"

// TestShapedNotdefCollapsesByID verifies that clusters shaped to .notdef
// (GlyphID 0, Shaped set) render through the by-id path and share one atlas
// entry per font/size, instead of the text path keying each distinct
// codepoint (and re-shaping the whole run) separately. This is the scroll
// jank fix for unsupported scripts (Chakma, Javanese, …) that render as tofu.
func TestShapedNotdefCollapsesByID(t *testing.T) {
	path, _, _ := resolveTestGlyph(t, "H") // any installed font path

	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	item := Item{
		Style:      TextStyle{FontName: "Sans 20"},
		FontPath:   path,
		Ascent:     20,
		Length:     4,
		GlyphCount: 2,
	}
	// Two different codepoints, both shaped to .notdef in this font.
	g1 := Glyph{GlyphID: 0, Shaped: true, Index: 0, Codepoint: 4}
	g2 := Glyph{GlyphID: 0, Shaped: true, Index: 0, Codepoint: 4}

	r.getOrLoadGlyph("\U00011100", item, g1, 0, 0) // Chakma letter aa
	after1 := len(r.cache)
	r.getOrLoadGlyph("\U0001114C", item, g2, 0, 0) // different Chakma codepoint
	after2 := len(r.cache)

	if after2 != after1 {
		t.Errorf("distinct .notdef codepoints created separate cache entries "+
			"(%d then %d); shaped tofu should share one by-id entry",
			after1, after2)
	}

	// Contrast: without Shaped, the same two texts take the text path and key
	// by text, producing two distinct entries.
	item.FontPath = path
	u1 := Glyph{GlyphID: 0, Shaped: false, Index: 0, Codepoint: 4}
	u2 := Glyph{GlyphID: 0, Shaped: false, Index: 0, Codepoint: 4}
	before := len(r.cache)
	r.getOrLoadGlyph("\U00011200", item, u1, 0, 0)
	r.getOrLoadGlyph("\U00011201", item, u2, 0, 0)
	if len(r.cache) < before+2 {
		t.Errorf("unshaped text-path glyphs unexpectedly shared a cache entry "+
			"(%d -> %d); text path must key per codepoint", before, len(r.cache))
	}
}
