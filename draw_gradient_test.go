//go:build android || linux || darwin || windows

package glyph

import "testing"

// gradientTestRenderer lays out text and returns a recording renderer, or
// skips when the host resolves no font file for it.
func gradientTestRenderer(t *testing.T, text string) (Layout,
	*Renderer, *recordingBackend) {

	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	t.Cleanup(ctx.Free)
	layout, err := ctx.LayoutText(text, TextConfig{
		Style: TextStyle{FontName: "Sans 32", Color: Color{0, 0, 0, 255}},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}
	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	t.Cleanup(r.Free)
	return layout, r, b
}

// TestVerticalGradientStripsFollowGlyphTop: each vertical-gradient strip
// must take the color at its own Y. An underscore sits below the
// baseline, near the bottom of the line box, so its strips must be light.
// Before the fix the strip Y was measured from the line top, so the
// underscore got the dark colors from the top of the gradient.
func TestVerticalGradientStripsFollowGlyphTop(t *testing.T) {
	layout, r, b := gradientTestRenderer(t, "_")
	r.DrawLayoutWithGradient(layout, 0, 0, blackToWhiteStops(GradientVertical))
	if len(b.drawCalls) == 0 {
		t.Fatal("no textured quads drawn")
	}
	for i, dc := range b.drawCalls {
		// The strip center in layout coords; the draw origin is 0.
		mid := dc.Dst.Y + dc.Dst.Height*0.5
		lineTop := float32(layout.Items[0].Y - layout.Items[0].Ascent)
		want := clamp01((mid - lineTop) / layout.VisualHeight)
		got := float32(dc.Color.R) / 255
		if d := got - want; d > 0.05 || d < -0.05 {
			t.Errorf("strip %d at y=%.1f: t=%.2f, want %.2f",
				i, mid, got, want)
		}
	}
}

// TestGradientDoesNotTintColorEmoji: a color emoji keeps its own colors
// under a gradient. The quad color must stay white, as it is without a
// gradient; a gradient color multiplies into the emoji bitmap.
func TestGradientDoesNotTintColorEmoji(t *testing.T) {
	for _, dir := range []GradientDirection{
		GradientHorizontal, GradientVertical, GradientDiagonal,
	} {
		layout, r, b := gradientTestRenderer(t, "\U0001F600")
		if !layout.Items[0].UseOriginalColor {
			t.Skip("no color emoji font on this host; skipping")
		}
		r.DrawLayoutWithGradient(layout, 0, 0, blackToWhiteStops(dir))
		if len(b.drawCalls) == 0 {
			t.Fatalf("dir %v: no textured quads drawn", dir)
		}
		white := Color{255, 255, 255, 255}
		for i, dc := range b.drawCalls {
			if dc.Color != white {
				t.Errorf("dir %v quad %d: color %+v, want %+v",
					dir, i, dc.Color, white)
			}
		}
	}
}
