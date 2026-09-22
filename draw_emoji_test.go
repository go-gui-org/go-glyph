//go:build android || linux || darwin || windows

package glyph

import "testing"

// A color emoji bitmap rarely matches the line height, so both draw paths
// scale it to ascent+descent and center it in the line box. The shared
// helper is the one source of that geometry.
func TestEmojiQuadBox(t *testing.T) {
	// 40px-tall bitmap, top 36px above the baseline, into a 16+4 line.
	cg := CachedGlyph{Width: 10, Height: 40, Left: 2, Top: 36}

	// Height-limited: scale 0.5, so 5x20 fills the line top to bottom.
	dx, dy, w, h := emojiQuadBox(cg, 1, 16, 4, 0, 100)
	if dx != 1 || dy != -16 || w != 5 || h != 20 {
		t.Errorf("height fit = (%v, %v, %v, %v), want (1, -16, 5, 20)",
			dx, dy, w, h)
	}

	// Advance-limited: the width may not pass the 4px advance, so the
	// scale is 0.4 and the 16px-tall result centers vertically.
	dx, dy, w, h = emojiQuadBox(cg, 1, 16, 4, 0, 4)
	if !near(dx, 0.8) || !near(dy, -14) || !near(w, 4) || !near(h, 16) {
		t.Errorf("advance fit = (%v, %v, %v, %v), want (0.8, -14, 4, 16)",
			dx, dy, w, h)
	}

	// Grid box: fills a 10-wide cell, centered on both axes.
	dx, dy, w, h = emojiQuadBox(cg, 1, 16, 4, 10, 100)
	if !near(dx, 2.5) || !near(dy, -16) || !near(w, 5) || !near(h, 20) {
		t.Errorf("box fit = (%v, %v, %v, %v), want (2.5, -16, 5, 20)",
			dx, dy, w, h)
	}

	// Already line height: the bitmap metrics pass through unscaled.
	cg = CachedGlyph{Width: 10, Height: 20, Left: 2, Top: 17}
	dx, dy, w, h = emojiQuadBox(cg, 1, 16, 4, 0, 100)
	if dx != 2 || dy != -17 || w != 10 || h != 20 {
		t.Errorf("no-op = (%v, %v, %v, %v), want (2, -17, 10, 20)",
			dx, dy, w, h)
	}
}

// DrawLayoutPlaced must put a scaled emoji where DrawLayout puts it: the
// top of the line box at a height-limited fit. Before the fix the placed
// path computed top = -Top*scale + h - ascent, which put a 36px-Top
// bitmap 2px low.
func TestPlacedEmojiMatchesLayoutGeometry(t *testing.T) {
	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	cg := CachedGlyph{Width: 10, Height: 40, Left: 2, Top: 36, Page: 0}
	r.emitPlacedQuad(cg, GlyphPlacement{X: 50, Y: 60},
		Color{255, 255, 255, 255}, 16, 4, true, 100)
	if len(b.drawCalls) != 1 {
		t.Fatalf("draw calls = %d, want 1", len(b.drawCalls))
	}
	want := Rect{X: 51, Y: 44, Width: 5, Height: 20}
	if got := b.drawCalls[0].Dst; got != want {
		t.Errorf("dst = %+v, want %+v", got, want)
	}
}
