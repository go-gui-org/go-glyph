//go:build linux && !glyph_pango

package glyph

import "testing"

// TestLinuxScriptFallbacks verifies that font discovery populates
// fallbackPaths and that the discovered fallbacks actually cover CJK
// and color-emoji runes — the regression that made those scripts
// render as tofu on the pango-free backend.
func TestLinuxScriptFallbacks(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no CJK/emoji fallback fonts installed; skipping")
	}

	covers := func(runes string) bool {
		for _, p := range ctx.fallbackPaths {
			f := newFTFontFromPath(ctx.ftLib, p, 16)
			if f.face == nil {
				continue
			}
			ok := f.hasGlyphs(runes)
			f.close()
			if ok {
				return true
			}
		}
		return false
	}

	if !covers("中") {
		t.Error("no fallback font covers CJK rune 中")
	}
	if !covers("日") {
		t.Error("no fallback font covers CJK rune 日")
	}
	if !covers("\U0001F600") { // 😀
		t.Error("no fallback font covers emoji 😀")
	}
}
