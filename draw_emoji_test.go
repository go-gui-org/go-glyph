//go:build android || linux || darwin || windows

package glyph

import "testing"

// The rasterizer sizes a color emoji bitmap to the em and sets its bearings
// from the font, so both draw paths draw it as is, as a browser does. They
// used to stretch it over the text font's line box, which drew it too big
// and too low. The shared helper is the one source of that geometry.
func TestEmojiQuadBox(t *testing.T) {
	// 20px bitmap, top 17px above the baseline, in a 16+4 line.
	cg := CachedGlyph{Width: 20, Height: 20, Left: 1, Top: 17}

	// Fits the advance: the bitmap's own box, unscaled.
	dx, dy, w, h := emojiQuadBox(cg, 1, 16, 4, 0, 20)
	if dx != 1 || dy != -17 || w != 20 || h != 20 {
		t.Errorf("own box = (%v, %v, %v, %v), want (1, -17, 20, 20)",
			dx, dy, w, h)
	}

	// Ascent and descent do not matter outside a grid cell.
	dx, dy, w, h = emojiQuadBox(cg, 1, 30, 10, 0, 20)
	if dx != 1 || dy != -17 || w != 20 || h != 20 {
		t.Errorf("tall line = (%v, %v, %v, %v), want (1, -17, 20, 20)",
			dx, dy, w, h)
	}

	// Wider than the 10px advance: scale 0.5 about the pen origin.
	dx, dy, w, h = emojiQuadBox(cg, 1, 16, 4, 0, 10)
	if !near(dx, 0.5) || !near(dy, -8.5) || !near(w, 10) || !near(h, 10) {
		t.Errorf("advance fit = (%v, %v, %v, %v), want (0.5, -8.5, 10, 10)",
			dx, dy, w, h)
	}

	// Physical pixels at scale 2 become logical units.
	dx, dy, w, h = emojiQuadBox(cg, 0.5, 16, 4, 0, 20)
	if !near(dx, 0.5) || !near(dy, -8.5) || !near(w, 10) || !near(h, 10) {
		t.Errorf("scaled = (%v, %v, %v, %v), want (0.5, -8.5, 10, 10)",
			dx, dy, w, h)
	}

	// Grid box: fills a 10-wide cell and the 16+4 line, centered on both
	// axes.
	cg = CachedGlyph{Width: 10, Height: 40, Left: 2, Top: 36}
	dx, dy, w, h = emojiQuadBox(cg, 1, 16, 4, 10, 100)
	if !near(dx, 2.5) || !near(dy, -16) || !near(w, 5) || !near(h, 20) {
		t.Errorf("box fit = (%v, %v, %v, %v), want (2.5, -16, 5, 20)",
			dx, dy, w, h)
	}
}

// DrawLayoutPlaced must put an emoji where DrawLayout puts it: at the
// bitmap's own bearings from the placement point.
func TestPlacedEmojiMatchesLayoutGeometry(t *testing.T) {
	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	cg := CachedGlyph{Width: 20, Height: 20, Left: 1, Top: 17, Page: 0}
	r.emitPlacedQuad(cg, GlyphPlacement{X: 50, Y: 60},
		Color{255, 255, 255, 255}, 16, 4, true, 20)
	if len(b.drawCalls) != 1 {
		t.Fatalf("draw calls = %d, want 1", len(b.drawCalls))
	}
	want := Rect{X: 51, Y: 43, Width: 20, Height: 20}
	if got := b.drawCalls[0].Dst; got != want {
		t.Errorf("dst = %+v, want %+v", got, want)
	}
}
