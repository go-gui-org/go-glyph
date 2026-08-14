//go:build android || linux || darwin || windows

package glyph

import "testing"

// inkBoundsFor lays text out and measures its ink box, skipping the test
// when the host has no usable font for it.
func inkBoundsFor(t *testing.T, ctx *Context, text string,
	cfg TextConfig) (Layout, Rect) {
	t.Helper()
	layout, err := ctx.LayoutText(text, cfg)
	if err != nil {
		t.Fatalf("LayoutText(%q): %v", text, err)
	}
	ink, ok := ctx.LayoutInkBounds(&layout)
	if !ok {
		t.Skipf("no measurable ink for %q on this host", text)
	}
	return layout, ink
}

// TestLayoutInkBoundsInsideAdvanceBox asserts the core invariant: ink
// never escapes the advance box the layout reports, and for a glyph with
// no descender ("H") it sits strictly inside it vertically. That gap is
// exactly what centring on the advance box gets wrong.
func TestLayoutInkBoundsInsideAdvanceBox(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{Size: 64}}
	layout, ink := inkBoundsFor(t, ctx, "H", cfg)

	if ink.Width <= 0 || ink.Height <= 0 {
		t.Fatalf("degenerate ink box: %+v", ink)
	}
	if ink.X < -0.5 || ink.X+ink.Width > layout.Width+0.5 {
		t.Errorf("ink escapes advance width: ink %+v, width %v",
			ink, layout.Width)
	}
	if ink.Y < -0.5 || ink.Y+ink.Height > layout.Height+0.5 {
		t.Errorf("ink escapes advance height: ink %+v, height %v",
			ink, layout.Height)
	}
	// "H" has no descender, so the advance box must have slack below it.
	if bottomSlack := layout.Height - (ink.Y + ink.Height); bottomSlack <= 1 {
		t.Errorf("expected descent slack under 'H', got %v (ink %+v, "+
			"height %v)", bottomSlack, ink, layout.Height)
	}
}

// TestLayoutInkBoundsScalesWithSize asserts the box is measured, not
// approximated: doubling the font size doubles the ink box.
func TestLayoutInkBoundsScalesWithSize(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	_, small := inkBoundsFor(t, ctx, "H", TextConfig{Style: TextStyle{Size: 32}})
	_, large := inkBoundsFor(t, ctx, "H", TextConfig{Style: TextStyle{Size: 64}})

	ratio := large.Height / small.Height
	if ratio < 1.9 || ratio > 2.1 {
		t.Errorf("ink height ratio for 2x size = %v, want ~2 "+
			"(small %+v, large %+v)", ratio, small, large)
	}
}

// TestLayoutInkBoundsMultiLine asserts the union spans every line: a
// two-line layout reports a taller ink box than a single "H", still
// inside the advance box.
func TestLayoutInkBoundsMultiLine(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{Size: 32}}
	_, single := inkBoundsFor(t, ctx, "H", cfg)
	layout, multi := inkBoundsFor(t, ctx, "H\nj", cfg)

	if multi.Height <= single.Height {
		t.Errorf("multi-line ink height %v not taller than single-line %v",
			multi.Height, single.Height)
	}
	// A descender glyph may carry a negative left bearing ("j"), so only
	// the right edge is bounded by the advance.
	if multi.X+multi.Width > layout.Width+0.5 {
		t.Errorf("multi-line ink escapes advance width: ink %+v, width %v",
			multi, layout.Width)
	}
}

// TestLayoutInkBoundsDescender asserts a descender ("j") paints below the
// baseline slack of a flat-bottomed glyph ("H") at the same size: the
// bottom of "j"'s ink sits lower than the bottom of "H"'s ink.
func TestLayoutInkBoundsDescender(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{Size: 48}}
	_, inkH := inkBoundsFor(t, ctx, "H", cfg)
	_, inkJ := inkBoundsFor(t, ctx, "j", cfg)

	if inkJ.Y+inkJ.Height <= inkH.Y+inkH.Height {
		t.Errorf("'j' ink bottom %v not below 'H' ink bottom %v",
			inkJ.Y+inkJ.Height, inkH.Y+inkH.Height)
	}
}

// TestLayoutInkBoundsBlankText covers the blank-cluster guard: a run that
// paints nothing reports no ink instead of a box around the pen.
func TestLayoutInkBoundsBlankText(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("   ", TextConfig{Style: TextStyle{Size: 32}})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if ink, ok := ctx.LayoutInkBounds(&layout); ok {
		t.Errorf("blank text reported ink %+v, want ok=false", ink)
	}
}

// TestLayoutInkBoundsNil covers the nil/empty guards.
func TestLayoutInkBoundsNil(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	if _, ok := ctx.LayoutInkBounds(nil); ok {
		t.Error("nil layout reported ok=true")
	}
	if _, ok := ctx.LayoutInkBounds(&Layout{}); ok {
		t.Error("empty layout reported ok=true")
	}
	var nilCtx *Context
	if _, ok := nilCtx.LayoutInkBounds(&Layout{}); ok {
		t.Error("nil context reported ok=true")
	}
}

// TestTextSystemInkBounds exercises the TextSystem entry point, including
// its cache path and the vertical/nil/empty guards.
func TestTextSystemInkBounds(t *testing.T) {
	ts, err := NewTextSystem(newMockBackend())
	if err != nil {
		t.Fatalf("NewTextSystem: %v", err)
	}
	defer ts.Free()

	cfg := TextConfig{Style: TextStyle{Size: 48}}
	ink, ok := ts.InkBounds("H", cfg)
	if !ok {
		t.Skip("no measurable ink for 'H' on this host")
	}
	if ink.Width <= 0 || ink.Height <= 0 {
		t.Fatalf("degenerate ink box: %+v", ink)
	}
	// Second call hits the layout cache and must agree with the first.
	ink2, ok := ts.InkBounds("H", cfg)
	if !ok {
		t.Fatal("cached layout lost its ink")
	}
	if ink != ink2 {
		t.Errorf("cached ink %+v differs from fresh %+v", ink2, ink)
	}

	vert := cfg
	vert.Orientation = OrientationVertical
	if _, ok := ts.InkBounds("H", vert); ok {
		t.Error("vertical orientation reported ok=true")
	}
	if _, ok := ts.InkBounds("", cfg); ok {
		t.Error("empty text reported ok=true")
	}
	var nilTS *TextSystem
	if _, ok := nilTS.InkBounds("H", cfg); ok {
		t.Error("nil TextSystem reported ok=true")
	}
}
