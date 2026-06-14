//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

import "testing"

func newTestContext(t *testing.T) *Context {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	return ctx
}

func TestLayoutSimpleText(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: -1, Align: AlignLeft},
	}
	l, err := ctx.LayoutText("Hello World", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Items) == 0 {
		t.Error("no items")
	}
	if len(l.CharRects) != len("Hello World") {
		t.Errorf("char_rects=%d, want %d", len(l.CharRects), len("Hello World"))
	}
}

func TestLayoutEmptyText(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	l, err := ctx.LayoutText("", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Items) != 0 {
		t.Errorf("items=%d, want 0", len(l.Items))
	}
}

func TestLayoutWrapping(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: 50, Wrap: WrapWord},
	}
	l, err := ctx.LayoutText("This is a long text that should wrap", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Lines) < 2 {
		t.Errorf("lines=%d, want >= 2", len(l.Lines))
	}
}

func TestLayoutLineSpacingIncreasesHeightAndOffsetsLines(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	baseCfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: 80, Wrap: WrapWord},
	}
	spacedCfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{
			Width:       80,
			Wrap:        WrapWord,
			LineSpacing: 12,
		},
	}

	base, err := ctx.LayoutText("alpha beta gamma delta", baseCfg)
	if err != nil {
		t.Fatalf("base LayoutText: %v", err)
	}
	spaced, err := ctx.LayoutText("alpha beta gamma delta", spacedCfg)
	if err != nil {
		t.Fatalf("spaced LayoutText: %v", err)
	}
	if len(base.Lines) < 2 || len(spaced.Lines) < 2 {
		t.Fatalf("expected wrapped multi-line layouts, got %d and %d lines", len(base.Lines), len(spaced.Lines))
	}

	wantExtra := float32(len(base.Lines)-1) * 12
	if got := spaced.Height - base.Height; got < wantExtra-0.5 || got > wantExtra+0.5 {
		t.Fatalf("spaced.Height - base.Height = %f, want about %f", got, wantExtra)
	}
	if got := spaced.Lines[1].Rect.Y - base.Lines[1].Rect.Y; got < 11.5 || got > 12.5 {
		t.Fatalf("second line Y delta = %f, want about 12", got)
	}
}

func TestLayoutMarkup(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style:     TextStyle{FontName: "Sans 20"},
		UseMarkup: true,
	}
	l, err := ctx.LayoutText(`<span foreground="#FF0000">Red</span>`, cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Items) == 0 {
		t.Fatal("no items")
	}
	item := l.Items[0]
	if item.Color.R != 255 || item.Color.G != 0 || item.Color.B != 0 {
		t.Errorf("color=%+v, want red", item.Color)
	}
}

func TestTextHeightNoDraw(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 30"},
		Block: BlockStyle{Width: -1, Align: AlignLeft},
	}
	l, err := ctx.LayoutText("Hello", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if l.VisualHeight <= 0 || l.VisualWidth <= 0 {
		t.Errorf("visual WxH: %fx%f", l.VisualWidth, l.VisualHeight)
	}
	if l.Height <= 0 || l.Width <= 0 {
		t.Errorf("logical WxH: %fx%f", l.Width, l.Height)
	}
	if l.VisualHeight < 10.0 {
		t.Errorf("visual height=%f, want >= 10 for 30pt font", l.VisualHeight)
	}
}

func TestLetterSpacingWider(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	text := "ABCDEF"
	baseCfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	wideCfg := TextConfig{Style: TextStyle{FontName: "Sans 20", LetterSpacing: 5}}

	base, err := ctx.LayoutText(text, baseCfg)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	wide, err := ctx.LayoutText(text, wideCfg)
	if err != nil {
		t.Fatalf("wide: %v", err)
	}
	if wide.Width <= base.Width {
		t.Errorf("wide=%f <= base=%f", wide.Width, base.Width)
	}
}

func TestLetterSpacingNarrower(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	text := "ABCDEF"
	baseCfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	tightCfg := TextConfig{Style: TextStyle{FontName: "Sans 20", LetterSpacing: -1}}

	base, err := ctx.LayoutText(text, baseCfg)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	tight, err := ctx.LayoutText(text, tightCfg)
	if err != nil {
		t.Fatalf("tight: %v", err)
	}
	if tight.Width >= base.Width {
		t.Errorf("tight=%f >= base=%f", tight.Width, base.Width)
	}
}

func TestLetterSpacingZero(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	text := "ABCDEF"
	baseCfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	zeroCfg := TextConfig{Style: TextStyle{FontName: "Sans 20", LetterSpacing: 0}}

	base, err := ctx.LayoutText(text, baseCfg)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	zero, err := ctx.LayoutText(text, zeroCfg)
	if err != nil {
		t.Fatalf("zero: %v", err)
	}
	if zero.Width != base.Width {
		t.Errorf("zero=%f != base=%f", zero.Width, base.Width)
	}
}

func TestVerticalLayoutDimensions(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("ABC", TextConfig{
		Style:       TextStyle{FontName: "Sans 12"},
		Orientation: OrientationVertical,
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if l.VisualHeight <= l.VisualWidth {
		t.Errorf("vertical: height=%f <= width=%f", l.VisualHeight, l.VisualWidth)
	}
}

func TestHorizontalLayoutDimensions(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("ABC", TextConfig{
		Style:       TextStyle{FontName: "Sans 12"},
		Orientation: OrientationHorizontal,
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if l.VisualWidth <= l.VisualHeight {
		t.Errorf("horizontal: width=%f <= height=%f", l.VisualWidth, l.VisualHeight)
	}
}

func TestVerticalGlyphAdvances(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("AB", TextConfig{
		Style:       TextStyle{FontName: "Sans 12"},
		Orientation: OrientationVertical,
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Glyphs) < 2 {
		t.Skip("fewer than 2 glyphs")
	}
	g := l.Glyphs[0]
	if g.XAdvance != 0 {
		t.Errorf("vertical glyph x_advance=%f, want 0", g.XAdvance)
	}
	if g.YAdvance == 0 {
		t.Error("vertical glyph y_advance=0, want non-zero")
	}
}

func TestLogAttrsExtraction(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	text := "Hello"
	l, err := ctx.LayoutText(text, TextConfig{
		Style: TextStyle{FontName: "Sans 12"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	// log_attrs should have len = text.len + 1.
	if len(l.LogAttrs) != len(text)+1 {
		t.Errorf("log_attrs=%d, want %d", len(l.LogAttrs), len(text)+1)
	}
}

func TestGlyphPositionsCount(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("ABC", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	positions := l.GlyphPositions()
	if len(positions) != len(l.Glyphs) {
		t.Errorf("positions=%d, glyphs=%d", len(positions), len(l.Glyphs))
	}
	if len(positions) != 3 {
		t.Errorf("positions=%d, want 3", len(positions))
	}
}

func TestGlyphPositionsAdvances(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("AB", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	pos := l.GlyphPositions()
	if len(pos) != 2 {
		t.Fatalf("positions=%d, want 2", len(pos))
	}
	if pos[0].X < 0 {
		t.Errorf("first glyph x=%f, want >= 0", pos[0].X)
	}
	if pos[0].Advance <= 0 {
		t.Errorf("first advance=%f, want > 0", pos[0].Advance)
	}
	if pos[1].X <= pos[0].X {
		t.Errorf("second x=%f <= first x=%f", pos[1].X, pos[0].X)
	}
}

func TestGlyphPositionsEmpty(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if pos := l.GlyphPositions(); len(pos) != 0 {
		t.Errorf("empty layout positions=%d, want 0", len(pos))
	}
}

func TestGlyphPositionsIndex(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("Hello", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	for i, pos := range l.GlyphPositions() {
		if pos.Index != i {
			t.Errorf("position[%d].index=%d", i, pos.Index)
		}
	}
}

func TestEmojiUseOriginalColor(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("\U0001F600", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Items) == 0 {
		t.Skip("no items for emoji")
	}
	found := false
	for _, item := range l.Items {
		if item.UseOriginalColor {
			found = true
			break
		}
	}
	if !found {
		t.Error("emoji should have UseOriginalColor=true")
	}
}

func TestEmojiAscentDescentPositive(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	l, err := ctx.LayoutText("\U0001F600\U0001F680", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	for _, item := range l.Items {
		if item.UseOriginalColor {
			if item.Ascent <= 0 {
				t.Errorf("emoji ascent=%f, want > 0", item.Ascent)
			}
			if item.Descent < 0 {
				t.Errorf("emoji descent=%f, want >= 0", item.Descent)
			}
		}
	}
}

func TestRichTextLayout(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()

	rt := RichText{
		Runs: []StyleRun{
			{Text: "Hello ", Style: TextStyle{
				FontName: "Sans 20",
				Color:    Color{255, 0, 0, 255},
			}},
			{Text: "World", Style: TextStyle{
				FontName: "Sans 20",
				Color:    Color{0, 0, 255, 255},
			}},
		},
	}
	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: -1},
	}
	l, err := ctx.LayoutRichText(rt, cfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(l.Items) == 0 {
		t.Error("no items")
	}
	if l.Width <= 0 {
		t.Errorf("width=%f, want > 0", l.Width)
	}
}

func TestLayoutPangoEquivalence(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Free()
	runLayoutEquivCases(t, LayoutEquivCases(), ctx.LayoutText)
}
