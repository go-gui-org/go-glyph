package glyph

import "math"

// Built-in rasterization of box-drawing characters (U+2500-257F), block
// elements (U+2580-259F) and Powerline separators (U+E0B0-E0B3).
//
// Rendering these through the font produces visibly uneven TUI frames
// (issue #99). Three causes stack: the sub-pixel phase of the cell origin
// splits a 1px stem across two columns in some cells but not others; a
// font's box glyph need not span the full advance or line box, so
// neighbours gap at the join; and the stem thickness comes from the font
// design at that ppem, so it is not an integer number of pixels.
//
// Drawing them here fixes all three at once. Every dimension below is in
// whole physical pixels, every stem is centered by integer division (so the
// same weight lands at the same offset in every cell), and the drawn extent
// is the full cell box (so neighbours abut). The result is also independent
// of the font, which is why a frame looks the same whichever monospace face
// the user picked.
//
// This file is deliberately free of build tags: it takes a dense 8-bit
// coverage buffer and touches nothing platform-specific, so the geometry is
// testable everywhere and the WASM renderer can adopt it later.

// Codepoint ranges drawn by this file.
const (
	boxLineLo  = 0x2500
	boxLineHi  = 0x257F
	boxBlockLo = 0x2580
	boxBlockHi = 0x259F
	boxPowerLo = 0xE0B0
	boxPowerHi = 0xE0B3
)

// boxKind classifies a codepoint into the families this file draws.
type boxKind uint8

const (
	boxKindNone boxKind = iota
	boxKindLine
	boxKindBlock
	boxKindPowerline
)

// boxGlyphKind classifies cp. A table entry of 0 means "not drawable", so a
// hole in a range falls back to the font glyph rather than drawing nothing.
func boxGlyphKind(cp rune) boxKind {
	switch {
	case cp >= boxLineLo && cp <= boxLineHi:
		if boxLineTable[cp-boxLineLo] == 0 {
			return boxKindNone
		}
		return boxKindLine
	case cp >= boxBlockLo && cp <= boxBlockHi:
		if boxBlockTable[cp-boxBlockLo] == 0 {
			return boxKindNone
		}
		return boxKindBlock
	case cp >= boxPowerLo && cp <= boxPowerHi:
		return boxKindPowerline
	}
	return boxKindNone
}

// boxMetrics is the complete input to drawBoxGlyph. Everything is in whole
// physical pixels, so the same metrics always produce a byte-identical
// bitmap — which is what lets hashBoxGlyph key the atlas on this struct
// alone, with no reference to the font or the sub-pixel bin.
type boxMetrics struct {
	cp    rune
	kind  boxKind
	cellW int // Cell width, physical px.
	cellH int // Cell height, physical px.
	top   int // Physical px from the baseline up to the cell top.
	light int // Light stem thickness.
	heavy int // Heavy stem thickness.
	gap   int // Clear space between the two stems of a double line.
}

// boxLightDivisor sets the light stroke weight as a fraction of the cell
// height: 1px up to an 18px cell, 2px to 30px, 3px to 42px. The weight
// tracks the cell, not the font design, so it is the same integer in every
// cell at a given size and scale. kitty and Ghostty scale theirs the same
// way.
const boxLightDivisor = 12

// boxMetricsFor resolves the cell box and stroke weights for one glyph. ok
// is false when the codepoint is outside the built-in ranges, the style
// opts out, or the resulting cell is unusable — every one of which falls
// back to the font glyph.
func boxMetricsFor(item Item, g Glyph, cp rune, scale float32) (boxMetrics, bool) {
	if item.Style.NoBuiltinBoxGlyphs {
		return boxMetrics{}, false
	}
	kind := boxGlyphKind(cp)
	if kind == boxKindNone {
		return boxMetrics{}, false
	}
	// Powerline lives in the private use area, where a patched Nerd Font
	// legitimately owns the codepoints and draws E0B0-E0B3 to match the rest
	// of its E0Bx family. Only synthesize when the font had nothing:
	// GlyphID 0 with Shaped set is an authoritative .notdef from this item's
	// own face, so the alternative here is tofu, not a different look.
	if kind == boxKindPowerline {
		if item.FontPath == "" || g.GlyphID != 0 || !g.Shaped {
			return boxMetrics{}, false
		}
	}
	// A stroked run rasterizes twice — once for the outline, once for the
	// fill (draw_atlas.go). Synthesizing only the fill would seat a built-in
	// block inside a font-derived outline, so leave stroked runs entirely to
	// the font. Keyed on Item.HasStroke rather than the strokeWidth
	// parameter so the two passes cannot disagree.
	if item.HasStroke {
		return boxMetrics{}, false
	}

	m := boxMetrics{cp: cp, kind: kind}

	// Default cell: the glyph's own advance, rounded up so a fractional
	// advance overlaps its neighbour by a fraction of a pixel rather than
	// leaving a nick. Grid callers override with their real cell, which is
	// the only way to get an exact abutment.
	m.cellW = pxCeil(float32(g.XAdvance) * scale)
	if item.Style.CellWidth > 0 {
		m.cellW = pxRound(item.Style.CellWidth * scale)
	}
	m.cellH = pxRound(float32(item.Ascent+item.Descent) * scale)
	if item.Style.CellHeight > 0 {
		m.cellH = pxRound(item.Style.CellHeight * scale)
	}
	if m.cellW < 1 || m.cellH < 1 {
		return boxMetrics{}, false
	}
	// Oversized cells fall back to the font rather than erroring into the
	// negative cache. Reachable around 150pt at 2x, where the font's own
	// glyph is perfectly good.
	if m.cellW > MaxGlyphSize || m.cellH > MaxGlyphSize {
		return boxMetrics{}, false
	}

	// Derive the top bearing from the bottom edge, not from the ascent, so
	// the cell bottom sits exactly on baseline+descent. With line height
	// equal to cell height that makes vertically adjacent cells share an
	// edge, which is what joins a column of U+2502 into one line.
	descPx := pxRound(float32(item.Descent) * scale)
	m.top = min(max(m.cellH-descPx, 0), m.cellH)

	// light <= limit by construction, so heavy >= light follows without a
	// further clamp: min(max(2L, L+1), limit) >= min(L+1, limit) >= L.
	limit := max(1, min(m.cellW, m.cellH)/2)
	m.light = min(max(1, (m.cellH+boxLightDivisor/2)/boxLightDivisor), limit)
	m.heavy = min(max(m.light*2, m.light+1), limit)
	// Shrink the double-line gap until the pair fits. Below a 3px cell a
	// double collapses toward a single stem; unreachable at any usable size.
	m.gap = m.light
	for 2*m.light+m.gap > min(m.cellW, m.cellH) && m.gap > 1 {
		m.gap--
	}
	return m, true
}

// pxCeil and pxRound convert a pixel dimension to an int, saturating rather
// than converting out of range. Go leaves a float-to-int conversion
// implementation-defined when the value does not fit, and both the scale
// factor (caller-supplied to NewRenderer) and the advance (font-supplied)
// can be NaN or absurd. Saturating to 0 or MaxGlyphSize+1 lands on the
// cell-size rejections in boxMetricsFor, which fall back to the font.
func pxCeil(v float32) int  { return clampPx(math.Ceil(float64(v))) }
func pxRound(v float32) int { return clampPx(math.Round(float64(v))) }

func clampPx(v float64) int {
	// NaN fails both comparisons and lands on 0, which boxMetricsFor rejects.
	switch {
	case v > float64(MaxGlyphSize)+1:
		return MaxGlyphSize + 1
	case v > 0:
		return int(v)
	}
	return 0
}

// boxGlyphKeyDomain namespaces built-in box-glyph cache keys away from the
// two font-glyph key shapes, both of which fold a string (the cluster text
// or the font path) and a float32 into the key. This path folds only
// integers behind a domain tag.
const boxGlyphKeyDomain = uint64(0x676c79706862786e) // "glyphbxn"

// hashBoxGlyph folds the complete determinant of the bitmap into key.
//
// Three things are deliberately absent. The sub-pixel bin: the bitmap is
// phase-independent by construction, and folding the bin in would store
// four identical copies and reintroduce the phase variation this feature
// exists to remove. The font identity: the bitmap depends only on cell
// geometry, so two faces at one cell size legitimately share an entry — the
// same property that makes frames look identical across fonts. And the
// scale factor: it is fully absorbed into the physical-pixel fields below,
// so a scale change lands on a different key on its own.
func hashBoxGlyph(key uint64, m boxMetrics) uint64 {
	key = fnvHashU64(key, boxGlyphKeyDomain)
	key = fnvHashU64(key, uint64(m.cp))
	key = fnvHashU64(key, uint64(m.cellW))
	key = fnvHashU64(key, uint64(m.cellH))
	key = fnvHashU64(key, uint64(m.top))
	key = fnvHashU64(key, uint64(m.light))
	key = fnvHashU64(key, uint64(m.heavy))
	key = fnvHashU64(key, uint64(m.gap))
	return key
}

// boxSink receives one glyph's geometry in cell-local physical pixels,
// origin at the cell's top-left corner. Splitting the geometry from the
// pixels is what lets the same code serve both backends: the native path
// writes an 8-bit coverage buffer for the atlas (covSink), and the WASM path
// replays the identical primitives as Canvas2D calls, which needs no atlas
// and no per-glyph image upload (issue #101).
//
// Rectangles carry a coverage byte because the shade blocks (U+2591-2593)
// are a partial fill; every other primitive is opaque.
type boxSink interface {
	fillRect(x0, y0, x1, y1 int, v byte)
	strokeSeg(ax, ay, bx, by, w float32)
	// strokeArcQuad strokes the quarter of the circle (cx, cy, r) that lies
	// on the named side of the center: xLow selects x <= cx, yLow y <= cy.
	strokeArcQuad(cx, cy, r, w float32, xLow, yLow bool)
	fillTri(ax, ay, bx, by, cx, cy float32)
}

// drawBoxGlyph renders m into dst, a dense 8-bit coverage buffer of exactly
// cellW*cellH bytes that the caller has already cleared.
func drawBoxGlyph(dst []byte, m boxMetrics) {
	if len(dst) < m.cellW*m.cellH {
		return
	}
	drawBoxGlyphTo(covSink{dst: dst, m: m}, m)
}

// drawBoxGlyphTo emits m's geometry into s.
func drawBoxGlyphTo(s boxSink, m boxMetrics) {
	if m.cellW < 1 || m.cellH < 1 {
		return
	}
	switch m.kind {
	case boxKindLine:
		drawBoxLine(s, m, boxLineTable[m.cp-boxLineLo])
	case boxKindBlock:
		drawBoxBlock(s, m, boxBlockTable[m.cp-boxBlockLo])
	case boxKindPowerline:
		drawPowerline(s, m)
	case boxKindNone:
	}
}

// boxCellOrigin places a glyph's cell box from the pen position of its
// baseline origin, in logical units, at the given scale.
//
// Rounding the origin to a whole physical pixel is half of the anti-banding
// guarantee (centerSpan is the other half): the geometry inside the cell is
// already phase-independent, so the run only stays aligned if every cell
// starts on a pixel boundary. Subtracting top seats the cell bottom on
// baseline+descent, the same seating loadBoxGlyphFT gets from its zero left
// bearing and top bearing of m.top.
func boxCellOrigin(x, y, scale float32, m boxMetrics) (int, int) {
	return pxRoundOrigin(x * scale), pxRoundOrigin(y*scale) - m.top
}

// boxSnapOriginX snaps a cell's physical-pixel origin to the nearest whole
// cell slot measured from baseX, the origin of the run the cell belongs to.
//
// Rounding each origin independently (boxCellOrigin, or computeDrawOrigin on
// the atlas path) is only enough when the caller draws one cell per call.
// Inside a coalesced run the pen steps by the font's *fractional* advance, so
// consecutive rounded origins step by alternating floor and ceil of that
// advance while the bitmap stays a constant cellW wide — wherever the step
// exceeds the width the cells fail to meet and a 1px gap opens in the rule
// (issue #102). Quantizing the origin to a multiple of cellW makes the step
// and the bitmap width the same integer by construction, which is the same
// argument that makes the bitmap itself uniform.
//
// Nearest slot rather than a running counter: the run's advance and the
// caller's cell width need not agree exactly, and picking the nearest slot
// is self-correcting instead of accumulating that disagreement. It also
// bounds the displacement from the unsnapped origin at half a cell.
func boxSnapOriginX(baseX, originX, cellW int) int {
	if cellW <= 0 {
		return originX
	}
	// Through float64 and back through the same clamp pxRoundOrigin uses: the
	// atlas path derives originX from an unclamped float32 coordinate, and a
	// NaN or absurd origin must not make the conversion back to int
	// implementation-defined.
	n := math.Round(float64(originX-baseX) / float64(cellW))
	return pxRoundOrigin(float32(float64(baseX) + n*float64(cellW)))
}

// pxRoundOrigin rounds a pixel coordinate to an int. Unlike pxRound it keeps
// the sign — a cell scrolled off the top or left of the viewport has a
// negative origin, and clamping that to zero would drag it back into view.
// The magnitude limit is far past any real canvas and only exists so a NaN
// or absurd coordinate cannot make the conversion implementation-defined.
func pxRoundOrigin(v float32) int {
	const limit = 1 << 24
	f := math.Round(float64(v))
	switch {
	case math.IsNaN(f):
		return 0
	case f > limit:
		return limit
	case f < -limit:
		return -limit
	}
	return int(f)
}

// ---------------------------------------------------------------------------
// Spans and bands
// ---------------------------------------------------------------------------

// span is a half-open pixel interval [lo, hi).
type span struct{ lo, hi int }

func (s span) empty() bool { return s.hi <= s.lo }

// centerSpan places a run of t pixels inside an extent of e pixels. Integer
// division only, and a pure function of (e, t): cell N and cell N+1 place
// the stem at the same offset from their own origins, so once the cell
// origins are whole physical pixels the stems land in the same physical
// column. That is the whole anti-banding guarantee.
func centerSpan(e, t int) span {
	t = min(max(t, 1), e)
	lo := max((e-t)/2, 0)
	return span{lo, min(lo+t, e)}
}

func spanUnion(a, b span) span {
	switch {
	case a.empty():
		return b
	case b.empty():
		return a
	}
	return span{min(a.lo, b.lo), max(a.hi, b.hi)}
}

// Arm weights, two bits each in the line table.
const (
	armNone   = 0
	armLight  = 1
	armHeavy  = 2
	armDouble = 3
)

// band is a stroke's cross-section: one rail for a light or heavy stroke,
// two for a double. Single strokes set rail[0] == rail[1] and n == 1, which
// lets every junction rule in drawDoubleLine apply unchanged to the mixed
// single/double codepoints such as U+2552.
type band struct {
	rail [2]span
	n    int
}

// centerBand places a band of the given arm weight in extent pixels.
func centerBand(extent int, w uint16, m boxMetrics) band {
	switch w {
	case armDouble:
		total := min(2*m.light+m.gap, extent)
		base := centerSpan(extent, total).lo
		lo := span{base, min(base+m.light, extent)}
		hi := span{max(base+total-m.light, lo.lo), min(base+total, extent)}
		return band{rail: [2]span{lo, hi}, n: 2}
	case armHeavy:
		s := centerSpan(extent, m.heavy)
		return band{rail: [2]span{s, s}, n: 1}
	default:
		s := centerSpan(extent, m.light)
		return band{rail: [2]span{s, s}, n: 1}
	}
}

// armSpan is the cross-section of a single arm of the given weight. Unlike
// centerBand it collapses a double to its full outer extent, which the
// simple (no-double) path never asks for.
func armSpan(extent int, w uint16, m boxMetrics) span {
	if w == armNone {
		return span{}
	}
	b := centerBand(extent, w, m)
	return spanUnion(b.rail[0], b.rail[1])
}

// ---------------------------------------------------------------------------
// Coverage-buffer sink
// ---------------------------------------------------------------------------

// covSink writes a dense 8-bit coverage buffer of cellW*cellH bytes, which
// is what the atlas path expands to RGBA. Every primitive combines with max,
// so an antialiased edge drawn earlier is never dimmed by a later hard-edged
// fill.
type covSink struct {
	dst []byte
	m   boxMetrics
}

func (s covSink) fillRect(x0, y0, x1, y1 int, v byte) {
	m := s.m
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, m.cellW), min(y1, m.cellH)
	if x0 >= x1 || y0 >= y1 || v == 0 {
		return
	}
	for y := y0; y < y1; y++ {
		row := s.dst[y*m.cellW : y*m.cellW+m.cellW]
		for x := x0; x < x1; x++ {
			if v > row[x] {
				row[x] = v
			}
		}
	}
}

// strokeSeg walks the whole cell and shades by distance to the segment.
// Scanning every pixel costs nothing worth optimizing at cell sizes and
// keeps the antialiased ends exact.
func (s covSink) strokeSeg(ax, ay, bx, by, w float32) {
	m := s.m
	half := w * 0.5
	for y := range m.cellH {
		py := float32(y) + 0.5
		for x := range m.cellW {
			px := float32(x) + 0.5
			d := segDist(px, py, ax, ay, bx, by) - half
			if v := coverAt(d); v > s.dst[y*m.cellW+x] {
				s.dst[y*m.cellW+x] = v
			}
		}
	}
}

// strokeArcQuad shades the quarter annulus. Painting only the one quadrant
// keeps the ring out of the three quadrants the straight arms own.
func (s covSink) strokeArcQuad(cx, cy, r, w float32, xLow, yLow bool) {
	m := s.m
	half := w * 0.5
	for y := range m.cellH {
		py := float32(y) + 0.5
		if (py <= cy) != yLow {
			continue
		}
		for x := range m.cellW {
			px := float32(x) + 0.5
			if (px <= cx) != xLow {
				continue
			}
			d := absF32(hypotF32(px-cx, py-cy)-r) - half
			if v := coverAt(d); v > s.dst[y*m.cellW+x] {
				s.dst[y*m.cellW+x] = v
			}
		}
	}
}

// fillTri shades the intersection of the triangle's three half-planes. Each
// edge distance is exact because the edges are straight, so the maximum is
// the signed distance to the triangle.
func (s covSink) fillTri(ax, ay, bx, by, cx, cy float32) {
	m := s.m
	for y := range m.cellH {
		py := float32(y) + 0.5
		for x := range m.cellW {
			px := float32(x) + 0.5
			d := max(
				halfPlane(px, py, ax, ay, bx, by, cx, cy),
				halfPlane(px, py, bx, by, cx, cy, ax, ay),
				halfPlane(px, py, cx, cy, ax, ay, bx, by))
			if v := coverAt(d); v > s.dst[y*m.cellW+x] {
				s.dst[y*m.cellW+x] = v
			}
		}
	}
}

// fillSpans fills the rectangle formed by a longitudinal run and a
// cross-section, oriented by horiz.
func fillSpans(s boxSink, horiz bool, run, cross span, v byte) {
	if horiz {
		s.fillRect(run.lo, cross.lo, run.hi, cross.hi, v)
		return
	}
	s.fillRect(cross.lo, run.lo, cross.hi, run.hi, v)
}

// ---------------------------------------------------------------------------
// U+2500-257F
// ---------------------------------------------------------------------------

// Line-table opcodes.
const (
	boxOpPlain     = 0
	boxOpArc       = 1
	boxOpDiagUp    = 2 // U+2571 "/"
	boxOpDiagDown  = 3 // U+2572 "\"
	boxOpDiagCross = 4
)

// be packs one line-table entry. Arm weights are armNone/Light/Heavy/Double,
// dash is 0 (solid), 1 (double dash), 2 (triple) or 3 (quadruple).
func be(n, e, s, w, dash, op uint16) uint16 {
	return w | e<<2 | n<<4 | s<<6 | dash<<8 | op<<10
}

func armW(e uint16) uint16  { return e & 3 }
func armE(e uint16) uint16  { return (e >> 2) & 3 }
func armN(e uint16) uint16  { return (e >> 4) & 3 }
func armS(e uint16) uint16  { return (e >> 6) & 3 }
func boxOp(e uint16) uint16 { return (e >> 10) & 7 }

// dashRuns is the number of dashes across the cell: the table stores 1, 2
// or 3 for the double, triple and quadruple dashed forms.
func dashRuns(e uint16) int { return int((e>>8)&3) + 1 }

// drawBoxLine renders one U+2500-257F codepoint.
func drawBoxLine(s boxSink, m boxMetrics, e uint16) {
	switch op := boxOp(e); op {
	case boxOpArc:
		drawBoxArc(s, m, e)
		return
	case boxOpDiagUp, boxOpDiagDown, boxOpDiagCross:
		drawBoxDiagonal(s, m, op)
		return
	}
	if armN(e) == armDouble || armE(e) == armDouble ||
		armS(e) == armDouble || armW(e) == armDouble {
		drawDoubleLine(s, m, e)
		return
	}
	drawSimpleLine(s, m, e)
}

// drawSimpleLine handles every light/heavy combination, including the ones
// that mix weights on a single axis (U+253D and friends). Each arm is a
// rectangle from its own cell edge to the far side of the perpendicular
// junction, so the corner fills as the union of the two arms and no
// junction case needs enumerating.
func drawSimpleLine(s boxSink, m boxMetrics, e uint16) {
	an, ae, as, aw := armN(e), armE(e), armS(e), armW(e)

	if (e>>8)&3 != 0 {
		drawDashedLine(s, m, e)
		return
	}

	// Each arm's cross-section: the vertical arms are measured across the
	// cell width, the horizontal ones across the height. Empty for an absent
	// arm, which spanUnion and fillRect both absorb.
	spN := armSpan(m.cellW, an, m)
	spS := armSpan(m.cellW, as, m)
	spE := armSpan(m.cellH, ae, m)
	spW := armSpan(m.cellH, aw, m)

	// Junction extents. With no arm on an axis the notional light stem
	// position stands in, which is what makes the half lines (U+2574-257F)
	// stop at the cell center.
	cx := centerSpan(m.cellW, m.light)
	if an != armNone || as != armNone {
		cx = spanUnion(spN, spS)
	}
	cy := centerSpan(m.cellH, m.light)
	if ae != armNone || aw != armNone {
		cy = spanUnion(spE, spW)
	}

	fillSpans(s, true, span{0, cx.hi}, spW, 255)
	fillSpans(s, true, span{cx.lo, m.cellW}, spE, 255)
	fillSpans(s, false, span{0, cy.hi}, spN, 255)
	fillSpans(s, false, span{cy.lo, m.cellH}, spS, 255)
}

// drawDashedLine renders U+2504-250B and U+254C-254F. The dash boundaries
// are integer divisions of the cell extent, so they are identical in every
// cell and a dashed run phase-aligns exactly like a solid one.
func drawDashedLine(s boxSink, m boxMetrics, e uint16) {
	horiz := armW(e) != armNone || armE(e) != armNone
	n := dashRuns(e)

	var cross span
	var length int
	if horiz {
		cross = armSpan(m.cellH, max(armW(e), armE(e)), m)
		length = m.cellW
	} else {
		cross = armSpan(m.cellW, max(armN(e), armS(e)), m)
		length = m.cellH
	}

	// The gap shrinks with the dash period so a dense dash in a narrow cell
	// still leaves visible ink: at four dashes across an 11px cell a
	// light-width gap would eat most of the first dash.
	gapPx := max(1, min(m.light, length/n/2))
	for i := range n {
		lo := i * length / n
		hi := (i+1)*length/n - gapPx
		if hi <= lo {
			hi = min(lo+1, length)
		}
		fillSpans(s, horiz, span{lo, hi}, cross, 255)
	}
}

// drawDoubleLine handles U+2550-256C, where an axis carries either a single
// light stroke or a double, never a mix within the axis.
//
// The model is a pair of rails per band. A rail that has no arm in its
// travel direction terminates on a rail of the perpendicular band, and
// which one depends on whether the codepoint is a corner or a tee:
//
//   - Corner (one vertical arm, one horizontal arm): outer rail pairs with
//     outer rail and inner with inner, which is what makes U+2554 join
//     top-to-left and U+255A join top-to-right.
//   - Tee (arms on both sides of the perpendicular axis): the rail stops at
//     the near rail of the band it meets, covering it — so U+2563's
//     horizontals reach the left vertical rail and stop.
//
// Separately, a rail that runs through a double band is interrupted between
// that band's two rails when an arm is present on its own side. Both bands
// must be double for the split to apply: a single stroke crossing a double
// (U+256A, U+256B) passes through unbroken.
func drawDoubleLine(s boxSink, m boxMetrics, e uint16) {
	an, ae, as, aw := armN(e), armE(e), armS(e), armW(e)

	hasV := an != armNone || as != armNone
	hasH := ae != armNone || aw != armNone

	vWeight, hWeight := uint16(armLight), uint16(armLight)
	if an != armNone {
		vWeight = an
	} else if as != armNone {
		vWeight = as
	}
	if ae != armNone {
		hWeight = ae
	} else if aw != armNone {
		hWeight = aw
	}
	vb := centerBand(m.cellW, vWeight, m)
	hb := centerBand(m.cellH, hWeight, m)

	// Outer rail indices. Travelling down, the top rail is the outer one;
	// travelling right, the left rail is.
	outerH, outerV := 0, 0
	if an != armNone && as == armNone {
		outerH = 1
	}
	if aw != armNone && ae == armNone {
		outerV = 1
	}
	isCorner := (an != armNone) != (as != armNone) &&
		(ae != armNone) != (aw != armNone)

	splittable := vb.n == 2 && hb.n == 2
	vGap := span{hb.rail[0].hi, hb.rail[1].lo}
	hGap := span{vb.rail[0].hi, vb.rail[1].lo}

	if hasH {
		for j := range hb.n {
			r := hb.rail[j]
			run := span{0, m.cellW}
			if aw == armNone {
				run.lo = vb.rail[partnerRail(j, outerH, outerV, vb.n, isCorner, true)].lo
			}
			if ae == armNone {
				run.hi = vb.rail[partnerRail(j, outerH, outerV, vb.n, isCorner, false)].hi
			}
			split := splittable &&
				((j == 0 && an != armNone) || (j == 1 && as != armNone))
			emitRail(s, true, run, r, hGap, split)
		}
	}
	if hasV {
		for i := range vb.n {
			r := vb.rail[i]
			run := span{0, m.cellH}
			if an == armNone {
				run.lo = hb.rail[partnerRail(i, outerV, outerH, hb.n, isCorner, true)].lo
			}
			if as == armNone {
				run.hi = hb.rail[partnerRail(i, outerV, outerH, hb.n, isCorner, false)].hi
			}
			split := splittable &&
				((i == 0 && aw != armNone) || (i == 1 && ae != armNone))
			emitRail(s, false, run, r, vGap, split)
		}
	}
}

// partnerRail picks the rail of the perpendicular band that rail idx
// terminates on. For a corner it is the matching outer/inner rail; for a
// tee it is the near rail in the direction of travel (low when the run is
// being clipped at its high end, and vice versa).
func partnerRail(idx, outerSelf, outerOther, otherN int, isCorner, low bool) int {
	if otherN == 1 {
		return 0
	}
	if isCorner {
		if idx == outerSelf {
			return outerOther
		}
		return 1 - outerOther
	}
	if low {
		return otherN - 1
	}
	return 0
}

// emitRail draws one rail, optionally interrupted over gap.
func emitRail(s boxSink, horiz bool, run, cross, gap span, split bool) {
	if !split || gap.empty() {
		fillSpans(s, horiz, run, cross, 255)
		return
	}
	fillSpans(s, horiz, span{run.lo, min(run.hi, gap.lo)}, cross, 255)
	fillSpans(s, horiz, span{max(run.lo, gap.hi), run.hi}, cross, 255)
}

// drawBoxArc renders the rounded corners U+256D-2570 as two straight light
// arms plus a quarter annulus joining them.
func drawBoxArc(s boxSink, m boxMetrics, e uint16) {
	an, ae, as, aw := armN(e), armE(e), armS(e), armW(e)

	vs := centerSpan(m.cellW, m.light)
	hs := centerSpan(m.cellH, m.light)
	// Stem center lines, in pixel-center coordinates.
	vc := float32(vs.lo+vs.hi) * 0.5
	hc := float32(hs.lo+hs.hi) * 0.5

	// The circle center sits diagonally inward from the corner the arc
	// turns, so the radius is limited by the room between the stem center
	// and the two cell edges the arms run to. Without that clamp a tall
	// narrow cell puts the center outside the cell and the arc leaves the
	// box entirely.
	hroom := float32(m.cellW) - vc
	if aw != armNone {
		hroom = vc
	}
	vroom := float32(m.cellH) - hc
	if an != armNone {
		vroom = hc
	}
	// Pull the radius in by one stem width so a short straight run survives
	// at each cell edge: the arc then meets a neighbouring U+2500 as a full
	// rectangle instead of as its own tangent pixel.
	r := min(hroom, vroom, float32(min(m.cellW, m.cellH))*0.5)
	r = max(r-float32(m.light), float32(m.light))

	cx, cy := vc-r, hc-r
	if ae != armNone {
		cx = vc + r
	}
	if as != armNone {
		cy = hc + r
	}

	// Straight runs are rounded outward so they overlap the arc's tangent
	// pixel rather than leaving a hairline gap at the junction.
	if aw != armNone {
		fillSpans(s, true, span{0, ceilInt(cx)}, hs, 255)
	}
	if ae != armNone {
		fillSpans(s, true, span{floorInt(cx), m.cellW}, hs, 255)
	}
	if an != armNone {
		fillSpans(s, false, span{0, ceilInt(cy)}, vs, 255)
	}
	if as != armNone {
		fillSpans(s, false, span{floorInt(cy), m.cellH}, vs, 255)
	}

	// The quarter the arc occupies lies on the side of the center away from
	// the arms.
	s.strokeArcQuad(cx, cy, r, float32(m.light), ae != armNone, as != armNone)
}

func floorInt(v float32) int { return int(math.Floor(float64(v))) }
func ceilInt(v float32) int  { return int(math.Ceil(float64(v))) }

// drawBoxDiagonal renders U+2571-2573 corner to corner. U+2573 is two
// independent strokes rather than one distance field over both segments, so
// the pixels either side of the crossing take the stronger of the two
// coverages instead of a joint one — the same max-combining rule the arcs
// already meet their arms with, and a difference of a fraction of a pixel.
func drawBoxDiagonal(s boxSink, m boxMetrics, op uint16) {
	w, h := float32(m.cellW), float32(m.cellH)
	if op == boxOpDiagUp || op == boxOpDiagCross {
		s.strokeSeg(0, h, w, 0, float32(m.light))
	}
	if op == boxOpDiagDown || op == boxOpDiagCross {
		s.strokeSeg(0, 0, w, h, float32(m.light))
	}
}

// boxLineTable encodes U+2500-257F, indexed by cp-0x2500. Arms are given as
// (north, east, south, west) weights plus a dash count and an opcode.
var boxLineTable = [128]uint16{
	// Straights and dashes.
	0x00: be(0, 1, 0, 1, 0, 0), // 2500 ─
	0x01: be(0, 2, 0, 2, 0, 0), // 2501 ━
	0x02: be(1, 0, 1, 0, 0, 0), // 2502 │
	0x03: be(2, 0, 2, 0, 0, 0), // 2503 ┃
	0x04: be(0, 1, 0, 1, 2, 0), // 2504 ┄
	0x05: be(0, 2, 0, 2, 2, 0), // 2505 ┅
	0x06: be(1, 0, 1, 0, 2, 0), // 2506 ┆
	0x07: be(2, 0, 2, 0, 2, 0), // 2507 ┇
	0x08: be(0, 1, 0, 1, 3, 0), // 2508 ┈
	0x09: be(0, 2, 0, 2, 3, 0), // 2509 ┉
	0x0A: be(1, 0, 1, 0, 3, 0), // 250A ┊
	0x0B: be(2, 0, 2, 0, 3, 0), // 250B ┋
	// Corners.
	0x0C: be(0, 1, 1, 0, 0, 0), // 250C ┌
	0x0D: be(0, 2, 1, 0, 0, 0), // 250D ┍
	0x0E: be(0, 1, 2, 0, 0, 0), // 250E ┎
	0x0F: be(0, 2, 2, 0, 0, 0), // 250F ┏
	0x10: be(0, 0, 1, 1, 0, 0), // 2510 ┐
	0x11: be(0, 0, 1, 2, 0, 0), // 2511 ┑
	0x12: be(0, 0, 2, 1, 0, 0), // 2512 ┒
	0x13: be(0, 0, 2, 2, 0, 0), // 2513 ┓
	0x14: be(1, 1, 0, 0, 0, 0), // 2514 └
	0x15: be(1, 2, 0, 0, 0, 0), // 2515 ┕
	0x16: be(2, 1, 0, 0, 0, 0), // 2516 ┖
	0x17: be(2, 2, 0, 0, 0, 0), // 2517 ┗
	0x18: be(1, 0, 0, 1, 0, 0), // 2518 ┘
	0x19: be(1, 0, 0, 2, 0, 0), // 2519 ┙
	0x1A: be(2, 0, 0, 1, 0, 0), // 251A ┚
	0x1B: be(2, 0, 0, 2, 0, 0), // 251B ┛
	// Left tees.
	0x1C: be(1, 1, 1, 0, 0, 0), // 251C ├
	0x1D: be(1, 2, 1, 0, 0, 0), // 251D ┝
	0x1E: be(2, 1, 1, 0, 0, 0), // 251E ┞
	0x1F: be(1, 1, 2, 0, 0, 0), // 251F ┟
	0x20: be(2, 1, 2, 0, 0, 0), // 2520 ┠
	0x21: be(2, 2, 1, 0, 0, 0), // 2521 ┡
	0x22: be(1, 2, 2, 0, 0, 0), // 2522 ┢
	0x23: be(2, 2, 2, 0, 0, 0), // 2523 ┣
	// Right tees.
	0x24: be(1, 0, 1, 1, 0, 0), // 2524 ┤
	0x25: be(1, 0, 1, 2, 0, 0), // 2525 ┥
	0x26: be(2, 0, 1, 1, 0, 0), // 2526 ┦
	0x27: be(1, 0, 2, 1, 0, 0), // 2527 ┧
	0x28: be(2, 0, 2, 1, 0, 0), // 2528 ┨
	0x29: be(2, 0, 1, 2, 0, 0), // 2529 ┩
	0x2A: be(1, 0, 2, 2, 0, 0), // 252A ┪
	0x2B: be(2, 0, 2, 2, 0, 0), // 252B ┫
	// Down tees.
	0x2C: be(0, 1, 1, 1, 0, 0), // 252C ┬
	0x2D: be(0, 1, 1, 2, 0, 0), // 252D ┭
	0x2E: be(0, 2, 1, 1, 0, 0), // 252E ┮
	0x2F: be(0, 2, 1, 2, 0, 0), // 252F ┯
	0x30: be(0, 1, 2, 1, 0, 0), // 2530 ┰
	0x31: be(0, 1, 2, 2, 0, 0), // 2531 ┱
	0x32: be(0, 2, 2, 1, 0, 0), // 2532 ┲
	0x33: be(0, 2, 2, 2, 0, 0), // 2533 ┳
	// Up tees.
	0x34: be(1, 1, 0, 1, 0, 0), // 2534 ┴
	0x35: be(1, 1, 0, 2, 0, 0), // 2535 ┵
	0x36: be(1, 2, 0, 1, 0, 0), // 2536 ┶
	0x37: be(1, 2, 0, 2, 0, 0), // 2537 ┷
	0x38: be(2, 1, 0, 1, 0, 0), // 2538 ┸
	0x39: be(2, 1, 0, 2, 0, 0), // 2539 ┹
	0x3A: be(2, 2, 0, 1, 0, 0), // 253A ┺
	0x3B: be(2, 2, 0, 2, 0, 0), // 253B ┻
	// Crosses.
	0x3C: be(1, 1, 1, 1, 0, 0), // 253C ┼
	0x3D: be(1, 1, 1, 2, 0, 0), // 253D ┽
	0x3E: be(1, 2, 1, 1, 0, 0), // 253E ┾
	0x3F: be(1, 2, 1, 2, 0, 0), // 253F ┿
	0x40: be(2, 1, 1, 1, 0, 0), // 2540 ╀
	0x41: be(1, 1, 2, 1, 0, 0), // 2541 ╁
	0x42: be(2, 1, 2, 1, 0, 0), // 2542 ╂
	0x43: be(2, 1, 1, 2, 0, 0), // 2543 ╃
	0x44: be(2, 2, 1, 1, 0, 0), // 2544 ╄
	0x45: be(1, 1, 2, 2, 0, 0), // 2545 ╅
	0x46: be(1, 2, 2, 1, 0, 0), // 2546 ╆
	0x47: be(2, 2, 1, 2, 0, 0), // 2547 ╇
	0x48: be(1, 2, 2, 2, 0, 0), // 2548 ╈
	0x49: be(2, 1, 2, 2, 0, 0), // 2549 ╉
	0x4A: be(2, 2, 2, 1, 0, 0), // 254A ╊
	0x4B: be(2, 2, 2, 2, 0, 0), // 254B ╋
	// Double dashes.
	0x4C: be(0, 1, 0, 1, 1, 0), // 254C ╌
	0x4D: be(0, 2, 0, 2, 1, 0), // 254D ╍
	0x4E: be(1, 0, 1, 0, 1, 0), // 254E ╎
	0x4F: be(2, 0, 2, 0, 1, 0), // 254F ╏
	// Doubles.
	0x50: be(0, 3, 0, 3, 0, 0), // 2550 ═
	0x51: be(3, 0, 3, 0, 0, 0), // 2551 ║
	0x52: be(0, 3, 1, 0, 0, 0), // 2552 ╒
	0x53: be(0, 1, 3, 0, 0, 0), // 2553 ╓
	0x54: be(0, 3, 3, 0, 0, 0), // 2554 ╔
	0x55: be(0, 0, 1, 3, 0, 0), // 2555 ╕
	0x56: be(0, 0, 3, 1, 0, 0), // 2556 ╖
	0x57: be(0, 0, 3, 3, 0, 0), // 2557 ╗
	0x58: be(1, 3, 0, 0, 0, 0), // 2558 ╘
	0x59: be(3, 1, 0, 0, 0, 0), // 2559 ╙
	0x5A: be(3, 3, 0, 0, 0, 0), // 255A ╚
	0x5B: be(1, 0, 0, 3, 0, 0), // 255B ╛
	0x5C: be(3, 0, 0, 1, 0, 0), // 255C ╜
	0x5D: be(3, 0, 0, 3, 0, 0), // 255D ╝
	0x5E: be(1, 3, 1, 0, 0, 0), // 255E ╞
	0x5F: be(3, 1, 3, 0, 0, 0), // 255F ╟
	0x60: be(3, 3, 3, 0, 0, 0), // 2560 ╠
	0x61: be(1, 0, 1, 3, 0, 0), // 2561 ╡
	0x62: be(3, 0, 3, 1, 0, 0), // 2562 ╢
	0x63: be(3, 0, 3, 3, 0, 0), // 2563 ╣
	0x64: be(0, 3, 1, 3, 0, 0), // 2564 ╤
	0x65: be(0, 1, 3, 1, 0, 0), // 2565 ╥
	0x66: be(0, 3, 3, 3, 0, 0), // 2566 ╦
	0x67: be(1, 3, 0, 3, 0, 0), // 2567 ╧
	0x68: be(3, 1, 0, 1, 0, 0), // 2568 ╨
	0x69: be(3, 3, 0, 3, 0, 0), // 2569 ╩
	0x6A: be(1, 3, 1, 3, 0, 0), // 256A ╪
	0x6B: be(3, 1, 3, 1, 0, 0), // 256B ╫
	0x6C: be(3, 3, 3, 3, 0, 0), // 256C ╬
	// Arcs.
	0x6D: be(0, 1, 1, 0, 0, boxOpArc), // 256D ╭
	0x6E: be(0, 0, 1, 1, 0, boxOpArc), // 256E ╮
	0x6F: be(1, 0, 0, 1, 0, boxOpArc), // 256F ╯
	0x70: be(1, 1, 0, 0, 0, boxOpArc), // 2570 ╰
	// Diagonals.
	0x71: be(0, 0, 0, 0, 0, boxOpDiagUp),    // 2571 ╱
	0x72: be(0, 0, 0, 0, 0, boxOpDiagDown),  // 2572 ╲
	0x73: be(0, 0, 0, 0, 0, boxOpDiagCross), // 2573 ╳
	// Half lines.
	0x74: be(0, 0, 0, 1, 0, 0), // 2574 ╴
	0x75: be(1, 0, 0, 0, 0, 0), // 2575 ╵
	0x76: be(0, 1, 0, 0, 0, 0), // 2576 ╶
	0x77: be(0, 0, 1, 0, 0, 0), // 2577 ╷
	0x78: be(0, 0, 0, 2, 0, 0), // 2578 ╸
	0x79: be(2, 0, 0, 0, 0, 0), // 2579 ╹
	0x7A: be(0, 2, 0, 0, 0, 0), // 257A ╺
	0x7B: be(0, 0, 2, 0, 0, 0), // 257B ╻
	0x7C: be(0, 2, 0, 1, 0, 0), // 257C ╼
	0x7D: be(1, 0, 2, 0, 0, 0), // 257D ╽
	0x7E: be(0, 1, 0, 2, 0, 0), // 257E ╾
	0x7F: be(2, 0, 1, 0, 0, 0), // 257F ╿
}
