//go:build linux || darwin || windows

package glyph

import (
	"math"
	"runtime"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Stem darkening for the pure-Go raster path.
//
// macOS draws text with "font smoothing" on by default. Core Graphics makes
// each glyph a little heavier before it fills it, and it makes light text
// heavier than dark text. Measured on SF at 13-28 px (CoreText, alpha-only
// bitmap, smoothing on against smoothing off), the total ink goes up by about
// 15% for black text and about 38% for white text. Text drawn by AppKit, and
// by frameworks that call Core Graphics such as mygo, has this weight. The
// x/image/vector fill has none of it, so next to native apps our text looks
// thin, most of all light text on a dark background.
//
// This pass copies the effect: it moves the outline of each glyph outward by
// a fraction of a pixel before the fill. The amount grows with the font size
// (with a cap) and with the luminance bucket of the text color. Vertical
// strokes get a little more than horizontal ones, as on macOS. Advances do
// not change, so layout, caret positions and hit testing do not change. Only
// the bitmap is heavier.
//
// It is on for macOS only. Users on Windows and Linux expect the weight of
// their own platform, which has no such darkening.

// stemDarkening turns the pass on. A var, not a const, so tests can turn it
// on or off on any OS.
var stemDarkening = runtime.GOOS == "darwin"

// Darkening amounts, as the outward move of each outline edge in device px
// per px of font size. The base values are the ones Pathfinder uses to copy
// macOS (x 0.0121, y 1.25 times that). They are then scaled per luminance
// bucket, so that the total ink of the fill after the coverage curve (see
// gamma.go) matches the Core Graphics ink. See TestStemDarkeningInk.
const (
	darkenBaseX = 0.0121
	darkenBaseY = darkenBaseX * 1.25
	// darkenMaxPx caps the move of one edge, so large text does not get bold.
	darkenMaxPx = 0.3
)

// darkenScale holds the per-bucket multiplier of the base amounts. Dark
// text keeps raw coverage (curveNone) and so needs the full macOS amount.
// Light text already gets heavier edges from its gamma curve, so it needs
// less outline growth to reach the same ink.
var darkenScale = [...]float64{
	curveNone:  0.65,
	curveMid:   0.8,
	curveLight: 0.85,
}

// rasterOpts carries the per-load raster settings from the glyph loaders to
// rasterizeGlyphs.
type rasterOpts struct {
	// hint turns on x-axis stem snapping (issue 191).
	hint bool
	// darken turns on stem darkening. The loaders set it from stemDarkening.
	darken bool
	// curve is the coverage curve of the text color. It selects the
	// darkening amount. The curve itself is applied later, in insertRaster.
	curve coverageCurve
}

// darkenAmount returns the outward move of an edge, in device px, for text
// of the given size (device px) and coverage curve: ax for the left and
// right edges (it makes vertical stems wider), ay for the top and bottom
// edges.
func darkenAmount(size float64, curve coverageCurve) (ax, ay float64) {
	if !(size > 0) || int(curve) >= len(darkenScale) {
		return 0, 0
	}
	k := darkenScale[curve]
	ax = math.Min(darkenBaseX*size, darkenMaxPx) * k
	ay = math.Min(darkenBaseY*size, darkenMaxPx) * k
	return ax, ay
}

// darkenGlyphs moves the outline of every glyph outward by the darkening
// amount for size and curve. scale converts font units to device px. It
// writes into each glyph's segments, which must belong to the caller (the
// raster path gets new segment slices from GlyphData for every load).
func darkenGlyphs(glyphs []placedGlyph, scale, size float64, curve coverageCurve) {
	ax, ay := darkenAmount(size, curve)
	if ax <= 0 && ay <= 0 || !(scale > 0) {
		return
	}
	// The move is done in font units, before hinting and the device map.
	sx, sy := ax/scale, ay/scale
	for i := range glyphs {
		emboldenSegs(glyphs[i].segs, sx, sy)
	}
}

// emboldenSegs moves every point of the outline outward by sx (in x) and sy
// (in y), in font units, in place. "Outward" comes from the direction of the
// outer contour (clockwise in Y-up space for TrueType, counter-clockwise for
// CFF). Inner contours run the other way, so the counters of a glyph get
// smaller, which is what a heavier glyph needs.
func emboldenSegs(segs []font.Segment, sx, sy float64) {
	if len(segs) == 0 {
		return
	}
	cw := outerContourClockwise(segs)
	for start := 0; start < len(segs); {
		end := start + 1
		for end < len(segs) && segs[end].Op != ot.SegmentOpMoveTo {
			end++
		}
		// A contour without an opening MoveTo (malformed font) is left as
		// it is; addOutline drops it anyway.
		if segs[start].Op == ot.SegmentOpMoveTo {
			emboldenContour(segs[start:end], sx, sy, cw)
		}
		start = end
	}
}

// emboldenContour moves the points of one contour (segs[0] is its MoveTo).
//
// The method is the one of FreeType's FT_Outline_EmboldenXY. Each point,
// control points too, is a vertex of the control polygon. A point moves
// along the bisector of its two adjacent edges, by the length that moves
// both edges outward by the strength (a miter join). The move is limited
// on short edges, so a small detail does not fold over itself. Control
// points of a curve move with the polygon, which moves the curve by about
// the same amount.
func emboldenContour(segs []font.Segment, sx, sy float64, cw bool) {
	// A copy of the original points: the move of a point depends on its
	// neighbors as they were before any point moved. Most contours have
	// fewer than 256 points, so the buffer stays on the stack and the
	// common path does not allocate.
	var buf [256]ot.SegmentPoint
	pts := buf[:0]
	for si := range segs {
		pts = append(pts, segs[si].ArgsSlice()...)
	}
	n := len(pts)
	// Many fonts close a contour with a last point equal to the first. It
	// is one vertex of the ring, not two.
	m := n
	if m > 1 && pts[m-1] == pts[0] {
		m--
	}
	if m < 3 {
		return
	}

	k := 0
	for si := range segs {
		args := segs[si].ArgsSlice()
		for ai := range args {
			v := k
			if v >= m {
				v = 0 // the closing duplicate moves with the first point
			}
			dx, dy := emboldenShift(pts[:m], v, sx, sy, cw)
			args[ai].X = pts[k].X + float32(dx)
			args[ai].Y = pts[k].Y + float32(dy)
			k++
		}
	}
}

// emboldenShift returns the move of ring point v. Neighbors equal to the
// point are skipped, because an edge of zero length has no direction.
func emboldenShift(ring []ot.SegmentPoint, v int, sx, sy float64,
	cw bool) (dx, dy float64) {

	m := len(ring)
	p := ring[v]
	prev, next := -1, -1
	for i := 1; i < m; i++ {
		if j := (v - i + m) % m; ring[j] != p {
			prev = j
			break
		}
	}
	for i := 1; i < m; i++ {
		if j := (v + i) % m; ring[j] != p {
			next = j
			break
		}
	}
	if prev < 0 || next < 0 {
		return 0, 0
	}
	inX := float64(p.X - ring[prev].X)
	inY := float64(p.Y - ring[prev].Y)
	outX := float64(ring[next].X - p.X)
	outY := float64(ring[next].Y - p.Y)
	lIn := math.Hypot(inX, inY)
	lOut := math.Hypot(outX, outY)
	inX, inY = inX/lIn, inY/lIn
	outX, outY = outX/lOut, outY/lOut

	// d is 1 + cos of the turn. Near -1 the outline turns back on itself
	// (a spike) and the bisector is not defined, so the point stays.
	d := inX*outX + inY*outY
	if d <= -0.9375 {
		return 0, 0
	}
	d++

	// The sum of the two directions, turned by 90 degrees to the ink-free
	// side. For a clockwise (TrueType) outer contour in Y-up space, the
	// outside of an edge with direction (x, y) is (-y, x).
	shX, shY := inY+outY, inX+outX
	q := outX*inY - outY*inX
	if cw {
		shX = -shX
		q = -q
	} else {
		shY = -shY
	}
	// Limit the move to the shorter edge, as FreeType does, so a short edge
	// at a sharp corner does not move past its neighbor.
	l := math.Min(lIn, lOut)
	if sx*q <= l*d {
		dx = shX * sx / d
	} else {
		dx = shX * l / q
	}
	if sy*q <= l*d {
		dy = shY * sy / d
	} else {
		dy = shY * l / q
	}
	return dx, dy
}
