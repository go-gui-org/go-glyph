//go:build android || linux || darwin || windows

package glyph

import (
	"strconv"
	"testing"
)

func decoratedItem(t *testing.T, text string, size int) Item {
	t.Helper()
	ctx := newReviewContext(t)
	l, err := ctx.LayoutText(text, TextConfig{Style: TextStyle{
		FontName:      "Sans " + strconv.Itoa(size),
		Underline:     true,
		Strikethrough: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Items) == 0 {
		t.Fatal("no items")
	}
	return l.Items[0]
}

// TestDecorationScalesWithFontSize is the regression test for fixed 1px
// underline and strikethrough lines: a 64px run got the same hairline as
// a 12px run. The lines now come from the font's post and OS/2 metrics.
func TestDecorationScalesWithFontSize(t *testing.T) {
	small := decoratedItem(t, "Hello", 12)
	big := decoratedItem(t, "Hello", 64)

	if !(big.UnderlineThickness > small.UnderlineThickness) {
		t.Errorf("underline thickness: 64px %v, 12px %v; want it to grow",
			big.UnderlineThickness, small.UnderlineThickness)
	}
	if !(big.StrikethroughThickness > small.StrikethroughThickness) {
		t.Errorf("strikethrough thickness: 64px %v, 12px %v; want it to grow",
			big.StrikethroughThickness, small.StrikethroughThickness)
	}
	if !(big.UnderlineOffset > small.UnderlineOffset) {
		t.Errorf("underline offset: 64px %v, 12px %v; want it to grow",
			big.UnderlineOffset, small.UnderlineOffset)
	}
	for _, it := range []Item{small, big} {
		// The underline sits below the baseline, the strikethrough
		// above it and below the ascent.
		if top := it.UnderlineOffset - it.UnderlineThickness; top < 0 {
			t.Errorf("underline top %v above the baseline", top)
		}
		stTop := it.StrikethroughOffset - it.StrikethroughThickness
		if stTop <= 0 || stTop >= it.Ascent {
			t.Errorf("strikethrough top %v not between baseline and "+
				"ascent %v", stTop, it.Ascent)
		}
	}
}

// TestDecorationMinimumOnePixel checks that a tiny run still gets a
// visible line: the thickness never drops below one device pixel.
func TestDecorationMinimumOnePixel(t *testing.T) {
	it := decoratedItem(t, "Hello", 4)
	if it.UnderlineThickness < 1 || it.StrikethroughThickness < 1 {
		t.Errorf("thickness below 1px: underline %v, strikethrough %v",
			it.UnderlineThickness, it.StrikethroughThickness)
	}
}

// TestDecorationCoversEmoji checks parity with WASM (and browsers): an
// underlined run keeps its underline across a color emoji instead of
// breaking at the emoji item.
func TestDecorationCoversEmoji(t *testing.T) {
	ctx := newReviewContext(t)
	l, err := ctx.LayoutText("a\U0001F600b", TextConfig{Style: TextStyle{
		FontName:  "Sans 16",
		Underline: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	sawEmoji := false
	for _, it := range l.Items {
		sawEmoji = sawEmoji || it.UseOriginalColor
		if !it.HasUnderline {
			t.Errorf("item %q (emoji %v) lost its underline",
				l.Text[it.StartIndex:it.StartIndex+it.Length],
				it.UseOriginalColor)
		}
	}
	if !sawEmoji {
		t.Skip("no color emoji font on this host")
	}
}

func TestNewDecorationMetrics(t *testing.T) {
	// Font values: underline top 2px below the baseline, 1.5px thick;
	// strikethrough top 5px above it, 1px thick.
	m := newDecorationMetrics(-2, 1.5, 5, 1, 16, 1)
	if m.ulThick != 1.5 || m.ulOffset != 3.5 {
		t.Errorf("underline = (%v, %v), want offset 3.5 thickness 1.5",
			m.ulOffset, m.ulThick)
	}
	if m.stThick != 1 || m.stOffset != 6 {
		t.Errorf("strikethrough = (%v, %v), want offset 6 thickness 1",
			m.stOffset, m.stThick)
	}

	// Missing metrics (zero thickness) fall back to em-based values.
	fb := newDecorationMetrics(0, 0, 0, 0, 20, 1)
	if fb.ulThick <= 0 || fb.stThick <= 0 || fb.ulOffset <= 0 ||
		fb.stOffset <= fb.stThick {
		t.Errorf("fallback metrics not usable: %+v", fb)
	}

	// Thickness is clamped to the minimum and the top edge stays put.
	c := newDecorationMetrics(-2, 0.2, 5, 0.2, 16, 1)
	if c.ulThick != 1 || c.ulOffset-c.ulThick != 2 {
		t.Errorf("clamped underline = (%v, %v), want top 2 thickness 1",
			c.ulOffset, c.ulThick)
	}
}
