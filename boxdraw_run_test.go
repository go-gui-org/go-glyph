//go:build linux || darwin || windows

package glyph

import (
	"math"
	"strings"
	"testing"
)

// boxRunLayout builds a single-item layout of n copies of ch, laid out the
// way a terminal coalesces a same-styled span into one draw call: the item
// declares a cell, but the pen still steps by the font's *fractional*
// advance. cellW is logical, and is used both as the declared cell and as
// the per-glyph advance, which is what a grid caller does (go-term takes
// cellW = TextWidth("M")).
func boxRunLayout(ch string, n int, cellW float32) Layout {
	cells := make([]string, n)
	for i := range cells {
		cells[i] = ch
	}
	return boxRunLayoutMixed(cells, cellW)
}

// boxRunLayoutMixed is boxRunLayout for a run whose cells are not all the
// same character.
func boxRunLayoutMixed(cells []string, cellW float32) Layout {
	n := len(cells)
	text := strings.Join(cells, "")
	item := Item{
		Style: TextStyle{
			FontName:   "monospace 12",
			CellWidth:  cellW,
			CellHeight: 20,
		},
		Ascent:     15,
		Descent:    5,
		GlyphStart: 0,
		GlyphCount: n,
		Length:     len(text),
		Color:      Color{255, 255, 255, 255},
	}
	glyphs := make([]Glyph, n)
	off := 0
	for i, c := range cells {
		glyphs[i] = Glyph{
			XAdvance:  float64(cellW),
			Index:     uint32(off),
			Codepoint: uint32(len(c)),
		}
		off += len(c)
	}
	return Layout{Text: text, Items: []Item{item}, Glyphs: glyphs}
}

// drawRunDst renders l twice (the first pass populates the atlas) and
// returns the Dst rects of the second pass.
func drawRunDst(t *testing.T, l Layout, scale float32) []Rect {
	t.Helper()
	be := newRecordingBackend()
	r, err := NewRenderer(be, scale)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Free)

	r.DrawLayout(l, 0, 40)
	r.Commit()
	be.drawCalls = nil
	r.DrawLayout(l, 0, 40)
	r.Commit()

	dst := make([]Rect, len(be.drawCalls))
	for i, c := range be.drawCalls {
		dst[i] = c.Dst
	}
	return dst
}

// assertRunTiles checks adjacent cells abut exactly: each cell starts where
// its predecessor ends, and every cell is the same width. Tolerance is a
// tenth of a physical pixel expressed in logical units: the assertion is
// about whole-pixel tiling, not about float32 round-trips through scaleInv.
func assertRunTiles(t *testing.T, dst []Rect, scale float32) {
	t.Helper()
	tol := 0.1 / scale
	for i := 1; i < len(dst); i++ {
		want := dst[i-1].X + dst[i-1].Width
		if math.Abs(float64(dst[i].X-want)) > float64(tol) {
			t.Errorf("cell %d starts at %v, want %v (cell %d ends there); "+
				"gap/overlap of %v logical px", i, dst[i].X, want, i-1,
				dst[i].X-want)
		}
		if dst[i].Width != dst[0].Width {
			t.Errorf("cell %d is %v wide, want %v", i, dst[i].Width,
				dst[0].Width)
		}
	}
}

// TestBoxRunTilesAtFractionalAdvance is issue #102: inside one coalesced run
// the pen steps by the font's fractional advance, so each origin used to
// floor to an alternating whole pixel while every bitmap stayed a constant
// cellW wide — roughly a third of the boundaries opened a 1px gap. Adjacent
// cells must now tile exactly.
//
// The advance here is deliberately fractional; boxTestItem's integer advance
// of 11 cannot reproduce the defect.
func TestBoxRunTilesAtFractionalAdvance(t *testing.T) {
	const cellW = 7.219 // Monospace 12 on the reporter's box.

	for _, scale := range []float32{1, 1.5, 2} {
		l := boxRunLayout("─", 8, cellW)
		dst := drawRunDst(t, l, scale)
		if len(dst) != 8 {
			t.Fatalf("scale %v: got %d draw calls, want 8", scale, len(dst))
		}
		assertRunTiles(t, dst, scale)
	}
}

// TestBoxRunMixedCodepointsTile covers a coalesced run whose box cells are
// not all the same character — the frame case, where one draw call mixes ─
// with ▄ and │. Every built-in bitmap is a full cellW wide with a zero left
// bearing regardless of codepoint, so a mixed run must tile exactly like a
// homogeneous one.
func TestBoxRunMixedCodepointsTile(t *testing.T) {
	const cellW = 7.219

	l := boxRunLayoutMixed([]string{"─", "▄", "─", "▄", "─", "▄", "─", "▄"},
		cellW)
	dst := drawRunDst(t, l, 2)
	if len(dst) != 8 {
		t.Fatalf("got %d draw calls, want 8", len(dst))
	}
	assertRunTiles(t, dst, 2)
}

// TestBoxRunSnapStaysNearPen checks the snapping never drags a cell more
// than half a cell from where the pen put it, which is what keeps a run
// anchored to the text it belongs to.
func TestBoxRunSnapStaysNearPen(t *testing.T) {
	const cellW = 7.219
	l := boxRunLayout("─", 8, cellW)
	dst := drawRunDst(t, l, 2)

	for i, d := range dst {
		pen := float32(i) * cellW
		if math.Abs(float64(d.X-pen)) > float64(cellW)/2+0.5 {
			t.Errorf("cell %d drawn at %v, pen was at %v", i, d.X, pen)
		}
	}
}

// TestBoxRunSlotFollowsThePen checks the slot a cell snaps to comes from
// where the pen actually is, not from a count of box glyphs seen so far. A
// run that alternates box and non-box characters is the case that separates
// them: with a running counter the second box would land in slot 1 instead
// of slot 2, dragging the whole rule half a cell left of its text.
//
// The interleaved cells are marked unknown so they emit nothing and the run
// is observable as box quads alone: what is under test is that the pen still
// walks past them and the next box lands two slots on, not one.
func TestBoxRunSlotFollowsThePen(t *testing.T) {
	const (
		cellW = 7.219
		scale = 2
	)
	// pxRound(7.219 * 2) = 14 physical, 7 logical.
	const slotLogical = 14 / float32(scale)

	l := boxRunLayoutMixed([]string{"─", "A", "─", "A", "─", "A", "─", "A"},
		cellW)
	for i := 1; i < len(l.Glyphs); i += 2 {
		l.Glyphs[i].Index |= PangoGlyphUnknownFlag
	}
	dst := drawRunDst(t, l, scale)
	if len(dst) != 4 {
		t.Fatalf("got %d draw calls, want 4 (one per box glyph)", len(dst))
	}
	for n, d := range dst {
		want := float32(2*n) * slotLogical // Box glyphs sit at 0, 2, 4, 6.
		if d.X != want {
			t.Errorf("box %d at %v, want slot %d at %v", n, d.X, 2*n, want)
		}
	}
}

// TestBoxRunWithoutDeclaredCellIsUnchanged pins the gate: with no
// Style.CellWidth the cell is derived from the advance with pxCeil, which is
// deliberately a pixel generous so cells overlap rather than gap. Nothing
// snaps, and placement matches the plain pen positions.
func TestBoxRunWithoutDeclaredCellIsUnchanged(t *testing.T) {
	const cellW = 7.219
	l := boxRunLayout("─", 8, cellW)
	l.Items[0].Style.CellWidth = 0

	dst := drawRunDst(t, l, 2)
	if len(dst) != 8 {
		t.Fatalf("got %d draw calls, want 8", len(dst))
	}
	for i, d := range dst {
		// Unsnapped placement: floor of the quarter-pixel-snapped pen.
		pen := float64(float32(i)*cellW) * 2.0
		want := float32(math.Floor(math.Round(pen*4.0)/4.0)) / 2.0
		if d.X != want {
			t.Errorf("cell %d at %v, want unsnapped %v", i, d.X, want)
		}
	}
}

// TestBoxSnapOriginX covers the helper directly.
func TestBoxSnapOriginX(t *testing.T) {
	tests := []struct {
		name                string
		base, origin, cellW int
		want                int
	}{
		{"already on a slot", 0, 21, 7, 21},
		{"rounds down", 0, 22, 7, 21},
		{"rounds up", 0, 26, 7, 28},
		{"half rounds away from zero", 0, 24, 8, 24},
		{"non-zero base", 5, 26, 7, 26},
		{"negative offset scrolled left", 0, -22, 7, -21},
		{"negative base", -14, -1, 7, 0},
		{"zero cell passes through", 0, 22, 0, 22},
		{"negative cell passes through", 0, 22, -3, 22},
		// Both clamped inputs at the ±2^24 limit, with a round that would
		// carry the result past it: the return must clamp, not wrap.
		{"output clamps at the limit", 1 << 24, -(1 << 24), 3, -(1 << 24)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := boxSnapOriginX(tt.base, tt.origin, tt.cellW)
			if got != tt.want {
				t.Errorf("boxSnapOriginX(%d, %d, %d) = %d, want %d",
					tt.base, tt.origin, tt.cellW, got, tt.want)
			}
		})
	}
}
