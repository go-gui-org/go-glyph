package glyph

import (
	"math"
	"testing"
)

// The WASM backend consumes the box geometry through boxSink instead of the
// coverage buffer (issue #101). Its own drawing cannot be tested here — no
// browser — so these tests pin the two contracts it relies on: that the sink
// protocol carries the whole glyph, and that a cell origin lands on a whole
// physical pixel.

// recSink records every primitive it is handed and can replay them into a
// coverage buffer.
type recSink struct {
	m     boxMetrics
	rects []recRect
	segs  []recSeg
	arcs  []recArc
	tris  []recTri
}

type recRect struct {
	x0, y0, x1, y1 int
	v              byte
}

type recSeg struct{ ax, ay, bx, by, w float32 }

type recArc struct {
	cx, cy, r, w float32
	xLow, yLow   bool
}

type recTri struct{ ax, ay, bx, by, cx, cy float32 }

func (s *recSink) fillRect(x0, y0, x1, y1 int, v byte) {
	s.rects = append(s.rects, recRect{x0, y0, x1, y1, v})
}

func (s *recSink) strokeSeg(ax, ay, bx, by, w float32) {
	s.segs = append(s.segs, recSeg{ax, ay, bx, by, w})
}

func (s *recSink) strokeArcQuad(cx, cy, r, w float32, xLow, yLow bool) {
	s.arcs = append(s.arcs, recArc{cx, cy, r, w, xLow, yLow})
}

func (s *recSink) fillTri(ax, ay, bx, by, cx, cy float32) {
	s.tris = append(s.tris, recTri{ax, ay, bx, by, cx, cy})
}

func (s *recSink) count() int {
	return len(s.rects) + len(s.segs) + len(s.arcs) + len(s.tris)
}

// replay draws the recording into a fresh coverage buffer, in the order the
// primitives arrived.
func (s *recSink) replay() []byte {
	dst := make([]byte, s.m.cellW*s.m.cellH)
	cov := covSink{dst: dst, m: s.m}
	for _, r := range s.rects {
		cov.fillRect(r.x0, r.y0, r.x1, r.y1, r.v)
	}
	for _, g := range s.segs {
		cov.strokeSeg(g.ax, g.ay, g.bx, g.by, g.w)
	}
	for _, a := range s.arcs {
		cov.strokeArcQuad(a.cx, a.cy, a.r, a.w, a.xLow, a.yLow)
	}
	for _, t := range s.tris {
		cov.fillTri(t.ax, t.ay, t.bx, t.by, t.cx, t.cy)
	}
	return dst
}

// boxTestCodepoints lists every codepoint the built-in path draws.
func boxTestCodepoints() []rune {
	var cps []rune
	for cp := rune(boxLineLo); cp <= boxPowerHi; cp++ {
		if cp > boxBlockHi && cp < boxPowerLo {
			continue
		}
		if boxGlyphKind(cp) != boxKindNone {
			cps = append(cps, cp)
		}
	}
	return cps
}

// A sink recording carries the whole glyph: replaying it reproduces the
// buffer the atlas path draws, byte for byte. That is what makes the
// Canvas2D sink a port of the geometry rather than a second implementation
// of it.
func TestBoxSinkReplayMatchesBuffer(t *testing.T) {
	for _, cell := range [][2]int{{9, 18}, {10, 20}, {21, 43}} {
		w, h := cell[0], cell[1]
		for _, cp := range boxTestCodepoints() {
			m := testBoxMetrics(t, cp, w, h)

			rec := &recSink{m: m}
			drawBoxGlyphTo(rec, m)
			if rec.count() == 0 {
				t.Errorf("%U at %dx%d: no primitives emitted", cp, w, h)
				continue
			}

			want := make([]byte, m.cellW*m.cellH)
			drawBoxGlyph(want, m)
			got := rec.replay()
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%U at %dx%d: replay differs at px %d "+
						"(row %d, col %d): got %d, want %d",
						cp, w, h, i, i/m.cellW, i%m.cellW, got[i], want[i])
				}
			}
		}
	}
}

// Rectangles are the primitive the Canvas2D sink draws unclipped, so they
// have to stay inside the cell after that sink's own clamp — i.e. clamping
// must never turn an overshoot into a different rectangle than the buffer
// path draws. Checked here as an invariant on the emitted geometry: a
// rectangle may run to a cell edge but must not start beyond one.
func TestBoxSinkRectsStayInCell(t *testing.T) {
	for _, cp := range boxTestCodepoints() {
		m := testBoxMetrics(t, cp, 10, 20)
		rec := &recSink{m: m}
		drawBoxGlyphTo(rec, m)
		for _, r := range rec.rects {
			if r.x0 >= m.cellW || r.y0 >= m.cellH || r.x1 < 0 || r.y1 < 0 {
				t.Errorf("%U: rect (%d,%d)-(%d,%d) is wholly outside "+
					"the %dx%d cell", cp, r.x0, r.y0, r.x1, r.y1,
					m.cellW, m.cellH)
			}
		}
	}
}

// The cell origin is half the anti-banding guarantee: neighbouring cells
// must start on whole physical pixels whatever the fractional pen position,
// and the cell must sit above the baseline by its top bearing.
func TestBoxCellOrigin(t *testing.T) {
	m := testBoxMetrics(t, '│', 10, 20)

	tests := []struct {
		name       string
		x, y       float32
		scale      float32
		wantX      int
		wantYDelta int // Relative to -m.top.
	}{
		{"integer at 1x", 40, 60, 1, 40, 60},
		{"fractional rounds up", 40.5, 60.5, 1, 41, 61},
		{"fractional rounds down", 40.4, 60.4, 1, 40, 60},
		{"integer at 2x", 40, 60, 2, 80, 120},
		{"fractional at 2x", 40.25, 60.25, 2, 81, 121},
		// Off-screen cells keep their sign; a half rounds away from zero.
		{"negative scrolls off", -12.5, -3, 1, -13, -3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotX, gotY := boxCellOrigin(tc.x, tc.y, tc.scale, m)
			if gotX != tc.wantX {
				t.Errorf("x = %d, want %d", gotX, tc.wantX)
			}
			if want := tc.wantYDelta - m.top; gotY != want {
				t.Errorf("y = %d, want %d", gotY, want)
			}
		})
	}

	// A run of cells at a fractional pitch still starts every cell on a
	// pixel, which is what keeps the stems in one column.
	const pitch = 10.5
	for i := range 8 {
		x, _ := boxCellOrigin(float32(i)*pitch, 0, 1, m)
		if want := int(float32(i)*pitch + 0.5); x != want {
			t.Errorf("cell %d origin = %d, want %d", i, x, want)
		}
	}
}

// pxRoundOrigin sits between the pen position (user text data) and the int
// conversion (implementation-defined out of range), so the NaN/Inf/oversize
// cases must land on the clamps instead of corrupting the canvas transform.
func TestPxRoundOriginClamps(t *testing.T) {
	limit := 1 << 24
	tests := []struct {
		name string
		v    float32
		want int
	}{
		{"NaN", float32(math.NaN()), 0},
		{"+Inf", float32(math.Inf(1)), limit},
		{"-Inf", float32(math.Inf(-1)), -limit},
		{"beyond the limit", 1e10, limit},
		{"below -limit", -1e10, -limit},
		{"exactly the limit", float32(limit), limit},
		{"negative keeps its sign", -3.2, -3},
		{"half rounds away from zero", -12.5, -13},
	}
	for _, tc := range tests {
		if got := pxRoundOrigin(tc.v); got != tc.want {
			t.Errorf("pxRoundOrigin(%v) = %d, want %d", tc.v, got, tc.want)
		}
	}
}
