package glyph

import "math"

// Block elements (U+2580-259F) and Powerline separators (U+E0B0-E0B3), plus
// the small signed-distance helpers the arcs, diagonals and Powerline
// shapes share. See boxdraw.go for why these are drawn here at all.

// Block-table opcodes.
const (
	boxBlockFill  = 1 // Fill k eighths inward from an edge.
	boxBlockQuads = 2 // Fill a quadrant mask.
	boxBlockShade = 3 // Uniform coverage fill.
)

// Edges for boxBlockFill.
const (
	boxEdgeBottom = 0
	boxEdgeLeft   = 1
	boxEdgeTop    = 2
	boxEdgeRight  = 3
)

// Quadrant mask bits.
const (
	quadTL = 1 << iota
	quadTR
	quadBL
	quadBR
)

// bb packs one block-table entry.
func bb(param, op, edge uint16) uint16 { return param | op<<4 | edge<<7 }

func blockParam(e uint16) int   { return int(e & 15) }
func blockOp(e uint16) uint16   { return (e >> 4) & 7 }
func blockEdge(e uint16) uint16 { return (e >> 7) & 3 }

// drawBoxBlock renders one U+2580-259F codepoint.
func drawBoxBlock(dst []byte, m boxMetrics, e uint16) {
	switch blockOp(e) {
	case boxBlockFill:
		drawBlockFill(dst, m, blockParam(e), blockEdge(e))
	case boxBlockQuads:
		drawBlockQuadrants(dst, m, blockParam(e))
	case boxBlockShade:
		// Shades are a uniform coverage fill, not the font's stipple, so
		// they stay flat at any size and never moire under scaling. The
		// values are the nominal 25 / 50 / 75 percent.
		fillRect(dst, m, 0, 0, m.cellW, m.cellH,
			byte(min(blockParam(e), 3))*0x40)
	}
}

// eighthEdge rounds the boundary k eighths along an extent. Both a fill and
// its complement derive their edge from this one value — a bottom fill of k
// starts where a top fill of 8-k ends — so U+2580 above U+2584 tiles the
// cell exactly instead of overlapping by the rounding of each half.
func eighthEdge(extent, k int) int { return (extent*k + 4) / 8 }

// drawBlockFill fills k eighths of the cell inward from one edge. Monotone
// in k, and exactly the full extent at k == 8 so a run of U+2588 has no
// seam.
func drawBlockFill(dst []byte, m boxMetrics, k int, edge uint16) {
	switch edge {
	case boxEdgeBottom:
		fillRect(dst, m, 0, eighthEdge(m.cellH, 8-k), m.cellW, m.cellH, 255)
	case boxEdgeTop:
		fillRect(dst, m, 0, 0, m.cellW, eighthEdge(m.cellH, k), 255)
	case boxEdgeLeft:
		fillRect(dst, m, 0, 0, eighthEdge(m.cellW, k), m.cellH, 255)
	case boxEdgeRight:
		fillRect(dst, m, eighthEdge(m.cellW, 8-k), 0, m.cellW, m.cellH, 255)
	}
}

// drawBlockQuadrants fills the quadrants named by mask. The right and
// bottom halves take the remainder of the split, so left+right is exactly
// the cell whatever the parity of the cell size.
func drawBlockQuadrants(dst []byte, m boxMetrics, mask int) {
	xm, ym := m.cellW/2, m.cellH/2
	if mask&quadTL != 0 {
		fillRect(dst, m, 0, 0, xm, ym, 255)
	}
	if mask&quadTR != 0 {
		fillRect(dst, m, xm, 0, m.cellW, ym, 255)
	}
	if mask&quadBL != 0 {
		fillRect(dst, m, 0, ym, xm, m.cellH, 255)
	}
	if mask&quadBR != 0 {
		fillRect(dst, m, xm, ym, m.cellW, m.cellH, 255)
	}
}

// boxBlockTable encodes U+2580-259F, indexed by cp-0x2580.
var boxBlockTable = [32]uint16{
	0x00: bb(4, boxBlockFill, boxEdgeTop),    // 2580 ▀
	0x01: bb(1, boxBlockFill, boxEdgeBottom), // 2581 ▁
	0x02: bb(2, boxBlockFill, boxEdgeBottom), // 2582 ▂
	0x03: bb(3, boxBlockFill, boxEdgeBottom), // 2583 ▃
	0x04: bb(4, boxBlockFill, boxEdgeBottom), // 2584 ▄
	0x05: bb(5, boxBlockFill, boxEdgeBottom), // 2585 ▅
	0x06: bb(6, boxBlockFill, boxEdgeBottom), // 2586 ▆
	0x07: bb(7, boxBlockFill, boxEdgeBottom), // 2587 ▇
	0x08: bb(8, boxBlockFill, boxEdgeBottom), // 2588 █
	// The left series counts down: 2589 is seven eighths, 258F is one.
	0x09: bb(7, boxBlockFill, boxEdgeLeft),           // 2589 ▉
	0x0A: bb(6, boxBlockFill, boxEdgeLeft),           // 258A ▊
	0x0B: bb(5, boxBlockFill, boxEdgeLeft),           // 258B ▋
	0x0C: bb(4, boxBlockFill, boxEdgeLeft),           // 258C ▌
	0x0D: bb(3, boxBlockFill, boxEdgeLeft),           // 258D ▍
	0x0E: bb(2, boxBlockFill, boxEdgeLeft),           // 258E ▎
	0x0F: bb(1, boxBlockFill, boxEdgeLeft),           // 258F ▏
	0x10: bb(4, boxBlockFill, boxEdgeRight),          // 2590 ▐
	0x11: bb(1, boxBlockShade, 0),                    // 2591 ░
	0x12: bb(2, boxBlockShade, 0),                    // 2592 ▒
	0x13: bb(3, boxBlockShade, 0),                    // 2593 ▓
	0x14: bb(1, boxBlockFill, boxEdgeTop),            // 2594 ▔
	0x15: bb(1, boxBlockFill, boxEdgeRight),          // 2595 ▕
	0x16: bb(quadBL, boxBlockQuads, 0),               // 2596 ▖
	0x17: bb(quadBR, boxBlockQuads, 0),               // 2597 ▗
	0x18: bb(quadTL, boxBlockQuads, 0),               // 2598 ▘
	0x19: bb(quadTL|quadBL|quadBR, boxBlockQuads, 0), // 2599 ▙
	0x1A: bb(quadTL|quadBR, boxBlockQuads, 0),        // 259A ▚
	0x1B: bb(quadTL|quadTR|quadBL, boxBlockQuads, 0), // 259B ▛
	0x1C: bb(quadTL|quadTR|quadBR, boxBlockQuads, 0), // 259C ▜
	0x1D: bb(quadTR, boxBlockQuads, 0),               // 259D ▝
	0x1E: bb(quadTR|quadBL, boxBlockQuads, 0),        // 259E ▞
	0x1F: bb(quadTR|quadBL|quadBR, boxBlockQuads, 0), // 259F ▟
}

// ---------------------------------------------------------------------------
// Powerline
// ---------------------------------------------------------------------------

// drawPowerline renders U+E0B0-E0B3: a solid triangle pointing right or
// left, or the same two edges stroked as a thin chevron. Reached only when
// the font had no glyph (see boxMetricsFor), so this never overrides a
// patched Nerd Font's own separators.
func drawPowerline(dst []byte, m boxMetrics) {
	w, h := float32(m.cellW), float32(m.cellH)
	var ax, ay, bx, by, cx, cy float32
	switch m.cp {
	case 0xE0B0, 0xE0B1: // Pointing right.
		ax, ay = 0, 0
		bx, by = w, h*0.5
		cx, cy = 0, h
	default: // E0B2, E0B3, pointing left.
		ax, ay = w, 0
		bx, by = 0, h*0.5
		cx, cy = w, h
	}
	filled := m.cp == 0xE0B0 || m.cp == 0xE0B2
	half := float32(m.light) * 0.5

	for y := range m.cellH {
		for x := range m.cellW {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			var d float32
			if filled {
				// Intersection of three half-planes: each edge distance is
				// exact because the edges are straight, so the maximum is the
				// signed distance to the triangle.
				d = max(
					halfPlane(px, py, ax, ay, bx, by, cx, cy),
					halfPlane(px, py, bx, by, cx, cy, ax, ay),
					halfPlane(px, py, cx, cy, ax, ay, bx, by))
			} else {
				d = min(
					segDist(px, py, ax, ay, bx, by),
					segDist(px, py, bx, by, cx, cy)) - half
			}
			if v := coverAt(d); v > dst[y*m.cellW+x] {
				dst[y*m.cellW+x] = v
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Signed-distance helpers
// ---------------------------------------------------------------------------

// coverAt converts a signed distance in pixels (negative inside) to 8-bit
// coverage with a one-pixel linear ramp centred on the edge. Exact enough
// for the straight edges and circular arcs this file draws, and it needs
// neither a rasterizer nor an allocation.
func coverAt(sd float32) byte {
	v := (0.5 - sd) * 255
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return byte(v + 0.5)
}

// halfPlane returns the signed distance from p to the line ab, positive on
// the side away from the reference point r.
func halfPlane(px, py, ax, ay, bx, by, rx, ry float32) float32 {
	ex, ey := bx-ax, by-ay
	l := hypotF32(ex, ey)
	if l == 0 {
		return hypotF32(px-ax, py-ay)
	}
	// Normal pointing away from r.
	nx, ny := ey/l, -ex/l
	if nx*(rx-ax)+ny*(ry-ay) > 0 {
		nx, ny = -nx, -ny
	}
	return nx*(px-ax) + ny*(py-ay)
}

// segDist returns the distance from p to the segment ab.
func segDist(px, py, ax, ay, bx, by float32) float32 {
	ex, ey := bx-ax, by-ay
	l2 := ex*ex + ey*ey
	if l2 == 0 {
		return hypotF32(px-ax, py-ay)
	}
	t := min(max(((px-ax)*ex+(py-ay)*ey)/l2, 0), 1)
	return hypotF32(px-(ax+t*ex), py-(ay+t*ey))
}

func hypotF32(x, y float32) float32 {
	return float32(math.Sqrt(float64(x*x + y*y)))
}
