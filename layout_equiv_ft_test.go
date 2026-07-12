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

	// Filter with the cheap cmap coverage cache first (mirroring production
	// probeFallback) and only full-parse a color candidate that maps the emoji;
	// scanning the whole fallback set with newFTFontFromPath parsed every system
	// font and spiked peak RSS enough to OOM-kill the macOS CI runner.
	hasEmoji := false
	for _, p := range ctx.fallbackPaths {
		cov := loadCoverage(p)
		if cov == nil || !cov.color || !cov.covers("\U0001F600") {
			continue
		}
		f := newFTFontFromPath(ctx.ftLib, p, 16)
		ok := f.isColorFont() && f.hasGlyphs("\U0001F600")
		f.close()
		if ok {
			hasEmoji = true
			break
		}
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
