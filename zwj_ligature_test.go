//go:build darwin && !glyph_pango

package glyph

import "testing"

// TestEmojiLigatureFragmentsMerge guards against CoreText decomposing a single
// ligated color emoji (couple/kiss ZWJ sequences) into a zero-advance lead
// fragment plus a trailing glyph. shapeTextClusters must coalesce those into
// one cluster spanning the whole grapheme so re-rasterizing the substring
// reforms the ligature instead of rendering a standalone, overflowing heart.
func TestEmojiLigatureFragmentsMerge(t *testing.T) {
	font := newCTFont(TextStyle{FontName: "JetBrainsMono Nerd Font 12"}, 1)
	if font.ref == 0 {
		font = newCTFont(TextStyle{FontName: "Menlo 12"}, 1)
	}
	if font.ref == 0 {
		t.Skip("no usable font")
	}
	cases := map[string]string{
		"couple same skin":  "\U0001f469\U0001f3ff\u200d\u2764\ufe0f\u200d\U0001f468\U0001f3ff",
		"couple mixed skin": "\U0001f469\U0001f3ff\u200d\u2764\ufe0f\u200d\U0001f468\U0001f3fb",
		"kiss mixed skin":   "\U0001f469\U0001f3ff\u200d\u2764\ufe0f\u200d\U0001f48b\u200d\U0001f468\U0001f3fb",
	}
	for name, s := range cases {
		cs := shapeTextClusters(font, s)
		if len(cs) != 1 {
			t.Errorf("%s: got %d clusters, want 1 (ligature split)", name, len(cs))
			continue
		}
		if cs[0].byteStart != 0 || cs[0].byteLen != len(s) {
			t.Errorf("%s: cluster span [%d,%d), want [0,%d)", name,
				cs[0].byteStart, cs[0].byteStart+cs[0].byteLen, len(s))
		}
		if cs[0].advance <= 0 {
			t.Errorf("%s: merged advance = %.1f, want > 0", name, cs[0].advance)
		}
	}
}
