//go:build darwin && !ios

package glyph

import (
	"math"
	"os"
	"testing"
)

// appleColorEmoji is the system color-emoji font. Every test here measures
// against what CoreText (and so Safari and Chrome on macOS) does with it.
const appleColorEmoji = "/System/Library/Fonts/Apple Color Emoji.ttc"

// emojiTestContext returns a Context, or skips when Apple Color Emoji is
// missing or not among the color fallbacks.
func emojiTestContext(t *testing.T) *Context {
	t.Helper()
	if _, err := os.Stat(appleColorEmoji); err != nil {
		t.Skipf("Apple Color Emoji not installed: %v", err)
	}
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	t.Cleanup(ctx.Free)
	return ctx
}

// emojiAdvanceIn returns the summed advance of the color-emoji glyphs in a
// layout, and how many there were.
func emojiAdvanceIn(l Layout) (adv float64, n int) {
	for _, it := range l.Items {
		if !it.UseOriginalColor {
			continue
		}
		for gi := it.GlyphStart; gi < it.GlyphStart+it.GlyphCount; gi++ {
			adv += l.Glyphs[gi].XAdvance
			n++
		}
	}
	return adv, n
}

// A color emoji advances by the emoji font's own advance (Apple Color Emoji:
// 800/800 units, so 1 em), as browsers do. It used to take one line height
// of the text font (1.18 em for SF), which left a visible gap after it.
func TestEmojiAdvanceIsOneEm(t *testing.T) {
	ctx := emojiTestContext(t)
	const size = 20
	l, err := ctx.LayoutText("(\U0001F513)", TextConfig{
		Style: TextStyle{FontName: "Sans", Size: size},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	adv, n := emojiAdvanceIn(l)
	if n != 1 {
		t.Fatalf("emoji glyphs = %d, want 1", n)
	}
	if math.Abs(adv-size) > 0.5 {
		t.Errorf("emoji advance = %v, want %v (1 em)", adv, size)
	}
}

// Rich text marks emoji clusters color while the run font stays the text
// font, so the advance must still come from the color font.
func TestRichTextEmojiAdvanceIsOneEm(t *testing.T) {
	ctx := emojiTestContext(t)
	const size = 20
	rt := RichText{Runs: []StyleRun{
		{Text: "(\U0001F513)", Style: TextStyle{FontName: "Sans", Size: size}},
	}}
	l, err := ctx.LayoutRichText(rt, TextConfig{})
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	adv, n := emojiAdvanceIn(l)
	if n != 1 {
		t.Fatalf("emoji glyphs = %d, want 1", n)
	}
	if math.Abs(adv-size) > 0.5 {
		t.Errorf("emoji advance = %v, want %v (1 em)", adv, size)
	}
}

// The emoji bitmap is rasterized at the requested em size and placed by the
// font's own bearings. CoreText draws an Apple Color Emoji glyph as a 1 em
// square from 0.125 em below the baseline to 0.875 em above it (measured
// with CTFontGetBoundingRectsForGlyphs and by drawing the glyph). The old
// path returned the raw strike bitmap with a guessed top, and the drawer
// stretched it over the text font's line box.
func TestEmojiBitmapPlacement(t *testing.T) {
	emojiTestContext(t)
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 512, 512)
	if err != nil {
		t.Fatalf("NewGlyphAtlas: %v", err)
	}
	defer atlas.Free()

	const size = 100
	item := Item{
		Style:            TextStyle{Size: size},
		Ascent:           97,
		Descent:          21,
		UseOriginalColor: true,
	}
	res, err := loadGlyphFT(atlas, nil, "\U0001F513", "", 0, item, 0, 1.0)
	if err != nil {
		t.Fatalf("loadGlyphFT: %v", err)
	}
	cg := res.Cached
	near1 := func(got, want int) bool { return got >= want-1 && got <= want+1 }
	if !near1(cg.Width, size) || !near1(cg.Height, size) {
		t.Errorf("bitmap = %dx%d, want %dx%d (1 em)",
			cg.Width, cg.Height, size, size)
	}
	if !near1(cg.Left, 0) {
		t.Errorf("Left = %d, want 0", cg.Left)
	}
	if want := size * 7 / 8; !near1(cg.Top, want) {
		t.Errorf("Top = %d, want %d (0.875 em above the baseline)",
			cg.Top, want)
	}
}
