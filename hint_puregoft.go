//go:build linux || darwin || windows

package glyph

import (
	"math"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Light x-axis hinting for the pure-Go raster path (issue 191).
//
// x/image/vector draws the outline exactly where the font puts it. At a
// fractional scale (125%, 150%) a vertical stem often starts between two
// pixels, so its coverage spreads over two gray columns. Native rasterizers
// snap stems to the pixel grid, and their stems are one solid column.
//
// This pass finds the vertical stems of one glyph, moves each stem edge to a
// whole device pixel, and moves the rest of the outline by linear
// interpolation between the snapped edges. Only x changes. The advance
// widths and the layout do not change, so text keeps its width and caret
// positions. Only the bitmap is different.
//
// The snap is in device pixels. The draw origin is a whole pixel and the
// subpixel fraction is in penX (see Renderer.computeDrawOrigin), and the
// bitmap offset in rasterizeGlyphs is a whole pixel too. So a whole x out of
// mapPt is a pixel boundary on screen, for every subpixel bin.

// hintMaxSize is the largest render size (device px) that gets hinting.
// Above it a stem is many pixels wide and a half-pixel blur is not visible,
// so the raw outline is better.
const hintMaxSize = 48

// Edge and stem detection limits. These are first values; tune them
// against screenshots on Windows at 125%.
const (
	// hintEdgeSlope is the largest |dx|/|dy| of a vertical edge, about 3
	// degrees. Italic stems lean more and are not snapped.
	hintEdgeSlope = 0.05
	// hintEdgeMinLen is the shortest vertical edge, as a fraction of the
	// render size. Shorter edges are serifs and joins, not stems.
	hintEdgeMinLen = 0.1
	// hintStemMaxWidth is the widest stem, as a fraction of the render size.
	// A wider ink run is a bowl or a wide shape, not a stem.
	hintStemMaxWidth = 0.3
	// hintEdgeMerge is the distance (px) at which two edges with the same
	// ink side count as one edge (for example the stem of 'l' and the stem
	// of a joined serif).
	hintEdgeMerge = 0.1
)

// Capacity limits. They keep every buffer a fixed-size array on the stack,
// so the hint pass does not allocate. A glyph with more edges or stems is
// hinted only for the first ones (left to right).
const (
	hintMaxEdges = 32
	hintMaxStems = 8
)

// xHint is a piecewise-linear map from raw device x to hinted device x.
// src holds the raw x of each control point (strictly increasing) and dst
// the snapped x. n == 0 is the identity map.
type xHint struct {
	n   int
	src [2 * hintMaxStems]float64
	dst [2 * hintMaxStems]float64
}

// apply maps one raw device x to its hinted position. Left of the first
// control point and right of the last one, the outline moves by the nearest
// control point's delta, so side bearings move as a block. Between control
// points the map is linear.
func (h *xHint) apply(x float64) float64 {
	n := h.n
	if n == 0 {
		return x
	}
	if x <= h.src[0] {
		return x + h.dst[0] - h.src[0]
	}
	if x >= h.src[n-1] {
		return x + h.dst[n-1] - h.src[n-1]
	}
	for k := 0; k+1 < n; k++ {
		if x < h.src[k+1] {
			t := (x - h.src[k]) / (h.src[k+1] - h.src[k])
			return h.dst[k] + t*(h.dst[k+1]-h.dst[k])
		}
	}
	return x + h.dst[n-1] - h.src[n-1] // not reached
}

// hintEdge is one vertical outline edge in device px. start is true when the
// ink is on the right of the edge (a stem's left side).
type hintEdge struct {
	x     float64
	start bool
}

// buildXHint finds the vertical stems of g and returns the map that snaps
// them. scale converts font units to device px; size is the render size in
// px. It returns the identity map when g has no stem to snap.
func buildXHint(g placedGlyph, scale, size float64) xHint {
	var h xHint
	if len(g.segs) == 0 || !(scale > 0) || !(size > 0) {
		return h
	}
	devX := func(fx float32) float64 { return g.penX + g.xOff + float64(fx)*scale }

	// The ink side of an edge depends on the contour direction, and the
	// direction differs between TrueType (clockwise outer contours) and CFF
	// (counter-clockwise). The outer contour has the largest area, so its
	// sign gives the direction for the whole glyph. Inner contours (counters)
	// run the other way, which gives the correct ink side for them too.
	inkRightWhenUp := outerContourClockwise(g.segs)

	var edges [hintMaxEdges]hintEdge
	ne := 0
	minLen := hintEdgeMinLen * size
	// consider records the segment from cur through args (its control
	// points, then its end point) when it is a long, near-vertical edge.
	consider := func(cur ot.SegmentPoint, args []ot.SegmentPoint) {
		end := args[len(args)-1]
		x0, x1 := devX(cur.X), devX(end.X)
		dy := float64(end.Y-cur.Y) * scale
		if math.Abs(dy) < minLen || ne == hintMaxEdges {
			return
		}
		// Every point of the segment (control points too) must stay within
		// the slope limit of the start x, so a curve that bulges sideways is
		// not taken for a straight stem.
		lim := hintEdgeSlope * math.Abs(dy)
		for _, p := range args {
			if math.Abs(devX(p.X)-x0) > lim {
				return
			}
		}
		e := hintEdge{x: (x0 + x1) / 2, start: (dy > 0) == inkRightWhenUp}
		ne = insertEdge(&edges, ne, e)
	}

	// A contour is closed by an implicit line from its last point back to
	// its MoveTo point. That line is often a stem edge (the left side of a
	// counter-clockwise rectangle), so it is checked too.
	var cur, start ot.SegmentPoint
	open := false
	closeContour := func() {
		if open && cur != start {
			consider(cur, []ot.SegmentPoint{start})
		}
	}
	for si := range g.segs {
		seg := g.segs[si]
		if seg.Op == ot.SegmentOpMoveTo {
			closeContour()
			cur, start, open = seg.Args[0], seg.Args[0], true
			continue
		}
		args := seg.ArgsSlice()
		consider(cur, args)
		cur = args[len(args)-1]
	}
	closeContour()

	// Pair each stem start with the nearest stem end on its right. If a
	// second start comes first, the first start is not a simple stem (for
	// example overlapping contours), so it is skipped.
	maxW := hintStemMaxWidth * size
	var prevR, prevRawR float64
	have := false
	for i := 0; i < ne; i++ {
		if !edges[i].start {
			continue
		}
		j := i + 1
		if j >= ne || edges[j].start {
			continue
		}
		l, r := edges[i].x, edges[j].x
		w := r - l
		if !(w > 0) || w > maxW {
			continue
		}
		sl, sr := snapStem(l, w)
		if have {
			// The gap to the previous stem must not close up or run backward.
			// A gap of one pixel or more keeps at least one clear pixel.
			minGap := 0.0
			if l-prevRawR >= 1 {
				minGap = 1
			}
			if sl-prevR < minGap {
				continue
			}
		}
		if h.n+2 > len(h.src) {
			break
		}
		h.src[h.n], h.dst[h.n] = l, sl
		h.src[h.n+1], h.dst[h.n+1] = r, sr
		h.n += 2
		prevR, prevRawR, have = sr, r, true
		i = j
	}
	return h
}

// snapStem returns the hinted left and right edges of a stem with raw left
// edge l and width w (device px).
//
// The width is rounded to a whole number of pixels (at least 1, so a
// hairline stays visible) and the stem is centered on its raw center, then
// put on the grid. Both edges land on pixel boundaries, so the stem is solid
// with no gray column.
//
// Before #198 a width far from a whole pixel kept its raw width and only the
// left edge moved to the grid, to keep the stem's ink. That made bowls lean:
// in an "o" the left wall got a crisp outer edge and the right wall a crisp
// inner edge, so the two walls did not mirror each other. Both edges now snap.
// The cost is weight: a 1.4 px stem draws as 1 px. That is the trade classic
// x-stem hinting makes too.
func snapStem(l, w float64) (sl, sr float64) {
	sw := math.Max(1, math.Round(w))
	sl = math.Round(l + (w-sw)/2)
	return sl, sl + sw
}

// insertEdge adds e to the first n entries of edges, which are sorted by x,
// and returns the new count. An edge within hintEdgeMerge of an edge with the
// same ink side is merged into it (the mean x). Insertion sort: n is small
// and sort.Slice would allocate.
func insertEdge(edges *[hintMaxEdges]hintEdge, n int, e hintEdge) int {
	for k := range n {
		if edges[k].start == e.start && math.Abs(edges[k].x-e.x) <= hintEdgeMerge {
			edges[k].x = (edges[k].x + e.x) / 2
			return n
		}
	}
	k := n
	for k > 0 && edges[k-1].x > e.x {
		edges[k] = edges[k-1]
		k--
	}
	edges[k] = e
	return n + 1
}

// outerContourClockwise reports whether the contour with the largest area
// runs clockwise in Y-up font space (the TrueType outer direction). The
// shoelace sum uses only on-curve end points, which is enough for the sign.
func outerContourClockwise(segs []font.Segment) bool {
	var area, best float64
	var start, prev ot.SegmentPoint
	open := false
	closeContour := func() {
		if !open {
			return
		}
		area += float64(prev.X)*float64(start.Y) - float64(start.X)*float64(prev.Y)
		if math.Abs(area) > math.Abs(best) {
			best = area
		}
		area, open = 0, false
	}
	for si := range segs {
		seg := segs[si]
		if seg.Op == ot.SegmentOpMoveTo {
			closeContour()
			start, prev, open = seg.Args[0], seg.Args[0], true
			continue
		}
		args := seg.ArgsSlice()
		p := args[len(args)-1]
		area += float64(prev.X)*float64(p.Y) - float64(p.X)*float64(prev.Y)
		prev = p
	}
	closeContour()
	// A positive shoelace sum is counter-clockwise in Y-up space.
	return best <= 0
}

// hintAtScale reports whether a glyph loaded at scaleFactor gets stem hinting
// when the renderer asks for it (hint). Hinting runs only at a fractional
// scale such as 125% or 150% (#198), the case it was added for (issue 191).
// At a whole scale the x-only snap moves ink but not advances, so spacing
// looks uneven, and the go-gui#1334 reporter preferred the raw outline there.
// An invalid scale counts as 1, as it does everywhere else (sanitizeScale).
func hintAtScale(hint bool, scaleFactor float32) bool {
	s := float64(sanitizeScale(scaleFactor))
	return hint && s != math.Trunc(s)
}
