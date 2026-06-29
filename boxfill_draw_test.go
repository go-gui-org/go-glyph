//go:build darwin && !glyph_pango

package glyph

import (
	"testing"
)

// capturingBackend records DrawTexturedQuad dst rects so a test can inspect
// the on-screen size of rendered glyphs. DPIScale reports retina (2.0) to
// match the typical macOS display the terminal renders on.
type capturingBackend struct {
	mockBackend
	quads []Rect
}

func (c *capturingBackend) DrawTexturedQuad(id TextureID, src, dst Rect, col Color) {
	c.quads = append(c.quads, dst)
}

func (c *capturingBackend) DrawTexturedQuadTransformed(id TextureID, src, dst Rect, col Color, t AffineTransform) {
	c.quads = append(c.quads, dst)
}

func (c *capturingBackend) DPIScale() float32 { return 2.0 }

// largestQuad returns the biggest-area dst rect captured (the emoji glyph;
// ignores tiny decoration quads).
func largestQuad(qs []Rect) Rect {
	var best Rect
	for _, q := range qs {
		if q.Width*q.Height > best.Width*best.Height {
			best = q
		}
	}
	return best
}

// TestBoxFillFillsCellBox confirms the EmojiBoxWidth hint scales a color glyph
// to fill the reserved cell box. On retina the default advance-clamped sizing
// scales the glyph to the box width. To stay robust across CI font/DPI
// differences, it drives the emoji with a box narrower than its natural size,
// so EmojiBoxWidth is always the binding constraint and the result is
// deterministic: filled width == boxW, and smaller than the default sizing.
// Menlo is used because it ships with every macOS (the emoji itself always
// resolves to the system color font regardless of the base text font).
func TestBoxFillFillsCellBox(t *testing.T) {
	const emoji = "\U0001f680" // 🚀
	base := TextStyle{FontName: "Menlo 12", Color: Color{0, 0, 0, 255}}

	draw := func(boxW float32) Rect {
		be := &capturingBackend{mockBackend: mockBackend{textures: map[TextureID][]byte{}}}
		ts, err := NewTextSystem(be)
		if err != nil {
			t.Skip(err)
		}
		defer ts.Free()
		st := base
		st.EmojiBoxWidth = boxW
		if err := ts.DrawText(0, 50, emoji, TextConfig{Style: st}); err != nil {
			t.Fatal(err)
		}
		ts.Commit()
		return largestQuad(be.quads)
	}

	defaultQuad := draw(0)
	if defaultQuad.Width == 0 {
		t.Fatal("no emoji quad captured")
	}
	// A box narrower than the natural emoji width is always the binding
	// constraint, so box-fill must scale the glyph down to exactly boxW.
	boxW := defaultQuad.Width * 0.5
	filledQuad := draw(boxW)
	t.Logf("default=%.2fx%.2f  boxW=%.2f  boxfill=%.2fx%.2f",
		defaultQuad.Width, defaultQuad.Height, boxW,
		filledQuad.Width, filledQuad.Height)

	if d := filledQuad.Width - boxW; d > 1 || d < -1 {
		t.Errorf("box-fill width = %.2f, want ≈ boxW %.2f", filledQuad.Width, boxW)
	}
	if filledQuad.Width >= defaultQuad.Width {
		t.Errorf("box-fill did not constrain emoji: filled=%.2f default=%.2f",
			filledQuad.Width, defaultQuad.Width)
	}
}
