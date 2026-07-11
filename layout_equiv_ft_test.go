//go:build linux || darwin || windows

package glyph

import "testing"

// TestLayoutEquivalenceFT runs the shared cross-platform layout
// invariant suite against the FreeType+HarfBuzz backend, mirroring the
// Pango (layout_test.go) and CoreText (layout_darwin_test.go) runs.
func TestLayoutEquivalenceFT(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	runLayoutEquivCases(t, LayoutEquivCases(), ctx.LayoutText)
}

func TestLayoutRichTextEmojiUseOriginalColor(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no emoji fallback font installed; skipping")
	}

	hasEmoji := false
	for _, p := range ctx.fallbackPaths {
		f := newFTFontFromPath(ctx.ftLib, p, 16)
		if f.isColorFont() && f.hasGlyphs("\U0001F600") {
			hasEmoji = true
		}
		f.close()
	}
	if !hasEmoji {
		t.Skip("no color-emoji font covers \U0001F600; skipping")
	}

	rt := RichText{Runs: []StyleRun{
		{Text: "a\U0001F600b", Style: TextStyle{FontName: "Sans 20"}},
	}}
	layout, err := ctx.LayoutRichText(rt, TextConfig{})
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	found := false
	for _, it := range layout.Items {
		if it.UseOriginalColor {
			found = true
		}
	}
	if !found {
		t.Error("rich-text emoji layout produced no UseOriginalColor item")
	}
}
