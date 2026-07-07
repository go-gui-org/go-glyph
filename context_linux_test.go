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

// TestEmojiItemUseOriginalColor verifies that laying out a color-emoji
// grapheme yields an item flagged UseOriginalColor, so the renderer
// draws the CBDT bitmap in its own colors (and scaled) instead of
// tinting it with the text color.
func TestEmojiItemUseOriginalColor(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no emoji fallback font installed; skipping")
	}

	// Confirm an emoji font is actually present before asserting.
	hasEmoji := false
	for _, p := range ctx.fallbackPaths {
		f := newFTFontFromPath(ctx.ftLib, p, 16)
		if f.isColorFont() && f.hasGlyphs("\U0001F600") {
			hasEmoji = true
		}
		f.close()
	}
	if !hasEmoji {
		t.Skip("no color-emoji font covers 😀; skipping")
	}

	layout, err := ctx.LayoutText("a\U0001F600b", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	found := false
	for _, it := range layout.Items {
		if it.UseOriginalColor {
			found = true
		}
	}
	if !found {
		t.Error("emoji layout produced no UseOriginalColor item")
	}
}
