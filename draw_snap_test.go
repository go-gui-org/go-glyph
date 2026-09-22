//go:build android || linux || darwin || windows

package glyph

import (
	"math"
	"testing"
)

// The atlas raster of a glyph already holds its subpixel offset (the bin
// shifts the outline inside the bitmap). So an upright quad must land on
// a whole device pixel: any leftover fraction either shifts the glyph a
// second time or makes the GPU sample between texels, which blurs it.

func snapTestLayout(t *testing.T, text string, style TextStyle) Layout {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	t.Cleanup(ctx.Free)
	layout, err := ctx.LayoutText(text, TextConfig{Style: style})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}
	return layout
}

func snapTestRenderer(t *testing.T) (*Renderer, *recordingBackend) {
	t.Helper()
	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	t.Cleanup(r.Free)
	return r, b
}

func isWhole(v float32) bool { return v == float32(math.Floor(float64(v))) }

func assertQuadsOnPixelGrid(t *testing.T, b *recordingBackend) {
	t.Helper()
	if len(b.drawCalls) == 0 {
		t.Fatal("no quads drawn")
	}
	for i, c := range b.drawCalls {
		if !isWhole(c.Dst.X) || !isWhole(c.Dst.Y) {
			t.Errorf("quad %d at (%v, %v), want whole pixels",
				i, c.Dst.X, c.Dst.Y)
		}
	}
}

// TestDrawLayoutPlacedSnapsToPixelGrid is the regression test for the
// double subpixel shift in DrawLayoutPlaced: the bin came from the
// fractional placement.X, and the quad was then also placed at that
// fractional X.
func TestDrawLayoutPlacedSnapsToPixelGrid(t *testing.T) {
	layout := snapTestLayout(t, "Hi", TextStyle{})
	r, b := snapTestRenderer(t)

	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{X: 10.5 + float32(i)*12.25, Y: 20.3}
	}
	r.DrawLayoutPlaced(layout, placements)
	assertQuadsOnPixelGrid(t, b)
}

// TestDrawLayoutPlacedSkipsNonFinite checks that a NaN or Inf placement
// never reaches the backend. Before the fix a NaN angle built a NaN
// rotation matrix and a NaN X or Y went straight into the quad.
func TestDrawLayoutPlacedSkipsNonFinite(t *testing.T) {
	layout := snapTestLayout(t, "A", TextStyle{})
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for _, p := range []GlyphPlacement{
		{X: nan, Y: 10}, {X: 10, Y: inf}, {X: 10, Y: 10, Angle: nan},
	} {
		r, b := snapTestRenderer(t)
		placements := make([]GlyphPlacement, len(layout.Glyphs))
		for i := range placements {
			placements[i] = p
		}
		r.DrawLayoutPlaced(layout, placements)
		if len(b.drawCalls) != 0 {
			t.Errorf("placement %+v drew %d quads, want 0",
				p, len(b.drawCalls))
		}
	}
}

// TestDrawLayoutFractionalOriginSnaps is the regression test for glyphs
// that were snapped relative to the layout and then moved by a fractional
// draw origin, so every quad sat between device pixels.
func TestDrawLayoutFractionalOriginSnaps(t *testing.T) {
	layout := snapTestLayout(t, "Hello", TextStyle{})
	r, b := snapTestRenderer(t)
	r.DrawLayout(layout, 10.5, 20.3)
	assertQuadsOnPixelGrid(t, b)
}

// TestDrawLayoutStrokeSnapsWithFill is the regression test for stroke
// outlines drawn at the unsnapped pen position with bin 0, while the fill
// was snapped with its subpixel bin. The outline sat up to half a pixel
// off the fill and was sampled between texels.
func TestDrawLayoutStrokeSnapsWithFill(t *testing.T) {
	layout := snapTestLayout(t, "Stroke", TextStyle{
		FontName:    "Sans 16",
		StrokeWidth: 1.5,
		Color:       Color{0, 0, 0, 255},
		StrokeColor: Color{255, 0, 0, 255},
	})
	r, b := snapTestRenderer(t)
	r.DrawLayout(layout, 3.7, 9.2)
	assertQuadsOnPixelGrid(t, b)
}

// TestDrawLayoutPlacedUploadsBeforeDraw checks that DrawLayoutPlaced,
// like DrawLayout, pushes new glyphs to a RectTextureUpdater backend
// before it emits quads that sample them.
func TestDrawLayoutPlacedUploadsBeforeDraw(t *testing.T) {
	layout := snapTestLayout(t, "Hi", TextStyle{})
	b := newRectMockBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{X: float32(i) * 12, Y: 20}
	}
	r.DrawLayoutPlaced(layout, placements)
	if len(b.rectCalls) == 0 {
		t.Error("DrawLayoutPlaced did not upload new glyphs before drawing")
	}
}
