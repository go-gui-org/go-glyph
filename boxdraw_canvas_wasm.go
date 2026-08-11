//go:build js && wasm

package glyph

import (
	"math"
	"syscall/js"
)

// Canvas2D implementation of boxSink. The web backend has no rasterizer —
// it redraws text with fillText every frame — so the built-in box geometry
// reaches it as drawing commands rather than as an atlas bitmap (issue
// #101). Rectangles and paths are Canvas2D's own primitives, so nothing is
// uploaded per glyph and nothing is cached.
//
// Coordinates arrive in cell-local physical pixels and leave in the logical
// units the canvas transform expects: the cell origin is a whole physical
// pixel (boxCellOrigin), and every dimension inside the cell is a whole
// number of physical pixels by construction, so the drawn edges land on
// device pixel boundaries at any device pixel ratio. That is the same
// guarantee the atlas path gets, arrived at without an atlas.
type canvasSink struct {
	ctx js.Value
	m   boxMetrics
	ox  int     // Cell origin, physical px.
	oy  int     // Cell origin, physical px.
	inv float32 // 1 / scaleFactor: physical px -> logical units.
	// alpha is the globalAlpha the caller set for this run. The shade
	// blocks scale it, so it has to be restored afterwards.
	alpha float64
}

// reset re-aims an existing sink at another cell. The sink lives on the
// Renderer and is re-aimed rather than reallocated, so a screen of box art
// allocates nothing per glyph.
func (s *canvasSink) reset(ctx js.Value, m boxMetrics, ox, oy int,
	inv float32, alpha float64) {

	s.ctx, s.m = ctx, m
	s.ox, s.oy, s.inv, s.alpha = ox, oy, inv, alpha
}

// px maps a cell-local physical x to a logical canvas coordinate.
func (s *canvasSink) px(x float32) float64 {
	return float64((float32(s.ox) + x) * s.inv)
}

func (s *canvasSink) py(y float32) float64 {
	return float64((float32(s.oy) + y) * s.inv)
}

func (s *canvasSink) fillRect(x0, y0, x1, y1 int, v byte) {
	// Clamped to the cell for the same reason the buffer sink clamps: an arm
	// is drawn as a rectangle running to the far side of its junction, which
	// can overshoot, and a cell must never paint into its neighbour.
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, s.m.cellW), min(y1, s.m.cellH)
	if x0 >= x1 || y0 >= y1 || v == 0 {
		return
	}
	// A partial fill (the U+2591-2593 shades) modulates the run's alpha
	// rather than its color, which keeps it correct under a gradient
	// fillStyle as well as a flat one.
	if v < 255 {
		s.ctx.Set("globalAlpha", s.alpha*float64(v)/255)
	}
	s.ctx.Call("fillRect",
		s.px(float32(x0)), s.py(float32(y0)),
		float64(float32(x1-x0)*s.inv), float64(float32(y1-y0)*s.inv))
	if v < 255 {
		s.ctx.Set("globalAlpha", s.alpha)
	}
}

func (s *canvasSink) strokeSeg(ax, ay, bx, by, w float32) {
	s.clipCell()
	s.ctx.Call("beginPath")
	s.ctx.Call("moveTo", s.px(ax), s.py(ay))
	s.ctx.Call("lineTo", s.px(bx), s.py(by))
	s.stroke(w)
}

func (s *canvasSink) strokeArcQuad(cx, cy, r, w float32, xLow, yLow bool) {
	// Canvas angles run clockwise from +x with y pointing down, so each
	// quadrant is a half-pi sweep starting at the axis that enters it.
	var start float64
	switch {
	case xLow && yLow: // Up and to the left of the center.
		start = math.Pi
	case xLow: // Down and to the left.
		start = math.Pi / 2
	case yLow: // Up and to the right.
		start = 3 * math.Pi / 2
	default: // Down and to the right.
		start = 0
	}
	s.clipCell()
	s.ctx.Call("beginPath")
	s.ctx.Call("arc", s.px(cx), s.py(cy), float64(r*s.inv),
		start, start+math.Pi/2)
	s.stroke(w)
}

func (s *canvasSink) fillTri(ax, ay, bx, by, cx, cy float32) {
	s.ctx.Call("beginPath")
	s.ctx.Call("moveTo", s.px(ax), s.py(ay))
	s.ctx.Call("lineTo", s.px(bx), s.py(by))
	s.ctx.Call("lineTo", s.px(cx), s.py(cy))
	s.ctx.Call("closePath")
	s.ctx.Call("fill")
}

// clipCell confines the next path to this cell and saves the state that
// stroke restores. A stroke has width perpendicular to its path, so a
// diagonal running corner to corner, or an arc tangent to a cell edge,
// spills half a stroke past the cell — into the neighbour, which owns those
// pixels. The coverage sink gets this for free by writing a cell-sized
// buffer.
func (s *canvasSink) clipCell() {
	s.ctx.Call("save")
	s.ctx.Call("beginPath")
	s.ctx.Call("rect", s.px(0), s.py(0),
		float64(float32(s.m.cellW)*s.inv),
		float64(float32(s.m.cellH)*s.inv))
	s.ctx.Call("clip")
}

// stroke closes out a path at the given physical-pixel width and drops the
// clip clipCell installed. Butt caps match the coverage path, which shades
// by distance to the segment itself and so ends square on the endpoint.
// strokeStyle is taken from fillStyle because the caller only ever set the
// latter: the box glyph is part of the fill pass, and stroking here is an
// implementation detail of drawing a curve, not the run's outline. Copying
// the value also carries a gradient fillStyle across unchanged.
func (s *canvasSink) stroke(w float32) {
	s.ctx.Set("strokeStyle", s.ctx.Get("fillStyle"))
	s.ctx.Set("lineWidth", float64(w*s.inv))
	s.ctx.Set("lineCap", "butt")
	s.ctx.Call("stroke")
	s.ctx.Call("restore")
}
