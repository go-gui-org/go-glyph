package glyph

import (
	"math"
	"testing"
)

// testBoxMetrics resolves metrics for cp at an exact w x h cell, going
// through boxMetricsFor so the resolution path is exercised too.
func testBoxMetrics(t *testing.T, cp rune, w, h int) boxMetrics {
	t.Helper()
	item := Item{
		Style:    TextStyle{CellWidth: float32(w), CellHeight: float32(h)},
		FontPath: "/test/font.ttf",
		Ascent:   float64(h) * 0.8,
		Descent:  float64(h) * 0.2,
	}
	// Shaped with GlyphID 0 is a .notdef, which is what unlocks the
	// Powerline range; the other ranges ignore it.
	g := Glyph{XAdvance: float64(w), Shaped: true}
	m, ok := boxMetricsFor(item, g, cp, 1)
	if !ok {
		t.Fatalf("boxMetricsFor(%U, %dx%d) = not ok", cp, w, h)
	}
	return m
}

// renderBox draws cp into a fresh buffer.
func renderBox(t *testing.T, cp rune, w, h int) ([]byte, boxMetrics) {
	t.Helper()
	m := testBoxMetrics(t, cp, w, h)
	dst := make([]byte, m.cellW*m.cellH)
	drawBoxGlyph(dst, m)
	return dst, m
}

// colRange returns the first and last columns holding any coverage.
func colRange(dst []byte, m boxMetrics) (int, int) {
	lo, hi := -1, -1
	for x := range m.cellW {
		for y := range m.cellH {
			if dst[y*m.cellW+x] != 0 {
				if lo < 0 {
					lo = x
				}
				hi = x
				break
			}
		}
	}
	return lo, hi
}

func rowRange(dst []byte, m boxMetrics) (int, int) {
	lo, hi := -1, -1
	for y := range m.cellH {
		for x := range m.cellW {
			if dst[y*m.cellW+x] != 0 {
				if lo < 0 {
					lo = y
				}
				hi = y
				break
			}
		}
	}
	return lo, hi
}

func TestBoxStrokeWeightIsInteger(t *testing.T) {
	for h := 4; h <= 64; h++ {
		w := max(2, h/2)
		m := testBoxMetrics(t, '│', w, h)
		limit := max(1, min(w, h)/2)
		switch {
		case m.light < 1:
			t.Fatalf("h=%d: light = %d, want >= 1", h, m.light)
		case m.light > limit:
			t.Fatalf("h=%d: light = %d, want <= %d", h, m.light, limit)
		case m.heavy < m.light:
			t.Fatalf("h=%d: heavy %d < light %d", h, m.heavy, m.light)
		case m.heavy > limit:
			t.Fatalf("h=%d: heavy = %d, want <= %d", h, m.heavy, limit)
		}
		// The two rails of a double stay inside the cell and in order at
		// every size, including the sizes too small for the nominal gap.
		for _, ext := range []int{w, h} {
			b := centerBand(ext, armDouble, m)
			if b.rail[0].lo < 0 || b.rail[1].hi > ext ||
				b.rail[0].lo > b.rail[1].lo {
				t.Fatalf("h=%d ext=%d: double band %+v out of range", h, ext, b)
			}
		}
	}
}

// TestBoxStemsAlignAcrossCells is the anti-banding invariant: the stem
// position is a pure function of the cell width and the stroke weight, so
// every cell in a column places it identically.
func TestBoxStemsAlignAcrossCells(t *testing.T) {
	for w := 2; w <= 24; w++ {
		dst, m := renderBox(t, '│', w, 23)
		lo, hi := colRange(dst, m)
		want := centerSpan(m.cellW, m.light)
		if lo != want.lo || hi != want.hi-1 {
			t.Fatalf("w=%d: stem columns [%d,%d], want [%d,%d)",
				w, lo, hi+1, want.lo, want.hi)
		}
	}
}

// TestBoxRunsAbut checks the drawn extent reaches both cell edges, so
// neighbouring cells join with no nick.
func TestBoxRunsAbut(t *testing.T) {
	dst, m := renderBox(t, '─', 11, 23)
	rows := centerSpan(m.cellH, m.light)
	for y := rows.lo; y < rows.hi; y++ {
		if dst[y*m.cellW] != 255 || dst[y*m.cellW+m.cellW-1] != 255 {
			t.Fatalf("U+2500 row %d: left=%d right=%d, want both 255",
				y, dst[y*m.cellW], dst[y*m.cellW+m.cellW-1])
		}
	}

	dst, m = renderBox(t, '│', 11, 23)
	cols := centerSpan(m.cellW, m.light)
	for x := cols.lo; x < cols.hi; x++ {
		if dst[x] != 255 || dst[(m.cellH-1)*m.cellW+x] != 255 {
			t.Fatalf("U+2502 col %d: top=%d bottom=%d, want both 255",
				x, dst[x], dst[(m.cellH-1)*m.cellW+x])
		}
	}
}

func TestBoxBlocksTileTheCell(t *testing.T) {
	const w, h = 11, 23

	full, m := renderBox(t, '█', w, h)
	for i, v := range full {
		if v != 255 {
			t.Fatalf("U+2588 pixel %d = %d, want 255", i, v)
		}
	}

	// Upper half above lower half must partition the cell exactly.
	upper, _ := renderBox(t, '▀', w, h)
	lower, _ := renderBox(t, '▄', w, h)
	for i := range m.cellW * m.cellH {
		if (upper[i] != 0) == (lower[i] != 0) {
			t.Fatalf("pixel %d: upper=%d lower=%d, want exactly one set",
				i, upper[i], lower[i])
		}
	}

	// Same for the left and right halves, and for the quadrant pair.
	left, _ := renderBox(t, '▌', w, h)
	right, _ := renderBox(t, '▐', w, h)
	tl, _ := renderBox(t, '▘', w, h)
	tr, _ := renderBox(t, '▝', w, h)
	for i := range m.cellW * m.cellH {
		if (left[i] != 0) == (right[i] != 0) {
			t.Fatalf("pixel %d: left/right halves overlap or gap", i)
		}
		if tl[i] != 0 && tr[i] != 0 {
			t.Fatalf("pixel %d: upper quadrants overlap", i)
		}
	}

	// The bottom eighths grow strictly with k.
	prev := 0
	for cp := '▁'; cp <= '█'; cp++ {
		dst, mm := renderBox(t, cp, w, h)
		lo, _ := rowRange(dst, mm)
		height := mm.cellH - lo
		if height <= prev {
			t.Fatalf("%U: filled height %d, want > %d", cp, height, prev)
		}
		prev = height
	}
}

func TestBoxShadeLevels(t *testing.T) {
	for i, cp := range []rune{'░', '▒', '▓'} {
		dst, _ := renderBox(t, cp, 11, 23)
		want := byte(i+1) * 0x40
		for j, v := range dst {
			if v != want {
				t.Fatalf("%U pixel %d = %#x, want %#x", cp, j, v, want)
			}
		}
	}
}

// TestBoxCoverageAllCodepoints walks every codepoint in the built-in
// ranges. U+2500-257F and U+2580-259F are fully assigned, so each must be
// drawable and each must put ink somewhere inside the cell.
func TestBoxCoverageAllCodepoints(t *testing.T) {
	for cp := rune(boxLineLo); cp <= boxPowerHi; cp++ {
		if cp > boxBlockHi && cp < boxPowerLo {
			continue
		}
		if k := boxGlyphKind(cp); k == boxKindNone {
			t.Fatalf("%U: kind none, want a table entry", cp)
		}
		m := boxMetrics{cp: cp, kind: boxGlyphKind(cp),
			cellW: 11, cellH: 23, light: 2, heavy: 4, gap: 2}
		dst := make([]byte, m.cellW*m.cellH+8)
		for i := range dst {
			dst[i] = 0
		}
		drawBoxGlyph(dst, m)
		ink := false
		for _, v := range dst[:m.cellW*m.cellH] {
			if v != 0 {
				ink = true
				break
			}
		}
		if !ink {
			t.Fatalf("%U: no coverage", cp)
		}
		for i, v := range dst[m.cellW*m.cellH:] {
			if v != 0 {
				t.Fatalf("%U: wrote %d past the cell at +%d", cp, v, i)
			}
		}
	}
}

// edgeInk reports whether any pixel on the named cell edge is covered.
func edgeInk(dst []byte, m boxMetrics, dir byte) bool {
	switch dir {
	case 'n':
		for x := range m.cellW {
			if dst[x] != 0 {
				return true
			}
		}
	case 's':
		for x := range m.cellW {
			if dst[(m.cellH-1)*m.cellW+x] != 0 {
				return true
			}
		}
	case 'w':
		for y := range m.cellH {
			if dst[y*m.cellW] != 0 {
				return true
			}
		}
	case 'e':
		for y := range m.cellH {
			if dst[y*m.cellW+m.cellW-1] != 0 {
				return true
			}
		}
	}
	return false
}

// TestBoxArmsReachTheirEdges checks that every codepoint puts ink on
// exactly the cell edges its table entry claims arms on. This is what makes
// a frame join: an arm that stops short leaves a nick at the cell boundary.
func TestBoxArmsReachTheirEdges(t *testing.T) {
	for cp := rune(boxLineLo); cp <= boxLineHi; cp++ {
		e := boxLineTable[cp-boxLineLo]
		if boxOp(e) != boxOpPlain {
			continue // Arcs and diagonals are checked separately.
		}
		if (e>>8)&3 != 0 {
			// A dashed line ends its cell on a gap by design: that is what
			// makes the pattern periodic across the cell boundary rather
			// than doubling the ink at every join.
			continue
		}
		dst, m := renderBox(t, cp, 11, 23)
		for _, c := range []struct {
			dir  byte
			want bool
		}{
			{'n', armN(e) != armNone},
			{'e', armE(e) != armNone},
			{'s', armS(e) != armNone},
			{'w', armW(e) != armNone},
		} {
			if got := edgeInk(dst, m, c.dir); got != c.want {
				t.Fatalf("%U: edge %c ink = %v, want %v", cp, c.dir, got, c.want)
			}
		}
	}
	// The arcs turn a corner, so they reach exactly their two arm edges.
	for _, tc := range []struct {
		cp    rune
		edges string
	}{{'╭', "es"}, {'╮', "ws"}, {'╯', "wn"}, {'╰', "en"}} {
		dst, m := renderBox(t, tc.cp, 11, 23)
		for _, dir := range []byte{'n', 'e', 's', 'w'} {
			want := false
			for i := range len(tc.edges) {
				if tc.edges[i] == dir {
					want = true
				}
			}
			if got := edgeInk(dst, m, dir); got != want {
				t.Fatalf("%U: edge %c ink = %v, want %v", tc.cp, dir, got, want)
			}
		}
	}
}

// TestBoxDoubleJunctions pins the shapes the rail model exists to get
// right: the two rails of a double must stay separated, and a tee must
// break the rail on the side its arm arrives from.
func TestBoxDoubleJunctions(t *testing.T) {
	const w, h = 11, 23

	// U+2551 draws two separated vertical rails.
	dst, m := renderBox(t, '║', w, h)
	runs := 0
	prev := false
	for x := range m.cellW {
		on := dst[x] != 0
		if on && !prev {
			runs++
		}
		prev = on
	}
	if runs != 2 {
		t.Fatalf("U+2551: %d vertical rails, want 2", runs)
	}

	// U+2563 breaks its left rail between the two horizontal rails and
	// leaves the right rail continuous.
	dst, m = renderBox(t, '╣', w, h)
	vb := centerBand(m.cellW, armDouble, m)
	hb := centerBand(m.cellH, armDouble, m)
	midY := (hb.rail[0].hi + hb.rail[1].lo) / 2
	if v := dst[midY*m.cellW+vb.rail[0].lo]; v != 0 {
		t.Fatalf("U+2563: left rail covered at the break, got %d", v)
	}
	if v := dst[midY*m.cellW+vb.rail[1].lo]; v == 0 {
		t.Fatalf("U+2563: right rail broken, want continuous")
	}

	// U+2566 keeps the top rail whole and breaks the bottom one.
	dst, m = renderBox(t, '╦', w, h)
	midX := (vb.rail[0].hi + vb.rail[1].lo) / 2
	if v := dst[hb.rail[0].lo*m.cellW+midX]; v == 0 {
		t.Fatalf("U+2566: top rail broken, want continuous")
	}
	if v := dst[hb.rail[1].lo*m.cellW+midX]; v != 0 {
		t.Fatalf("U+2566: bottom rail covered at the break, got %d", v)
	}

	// U+256C leaves all four corners of the junction clear.
	dst, m = renderBox(t, '╬', w, h)
	if v := dst[midY*m.cellW+midX]; v != 0 {
		t.Fatalf("U+256C: junction centre covered, got %d", v)
	}

	// A single stroke crossing a double passes through unbroken.
	dst, m = renderBox(t, '╪', w, h)
	vs := centerSpan(m.cellW, m.light)
	for y := range m.cellH {
		if dst[y*m.cellW+vs.lo] == 0 {
			t.Fatalf("U+256A: vertical broken at row %d", y)
		}
	}
}

func TestBoxDashRuns(t *testing.T) {
	for _, tc := range []struct {
		cp   rune
		want int
	}{{'╌', 2}, {'┄', 3}, {'┈', 4}, {'╎', 2}, {'┆', 3}, {'┊', 4}} {
		dst, m := renderBox(t, tc.cp, 21, 23)
		horiz := armW(boxLineTable[tc.cp-boxLineLo]) != armNone
		runs, prev := 0, false
		n := m.cellW
		if !horiz {
			n = m.cellH
		}
		for i := range n {
			var v byte
			if horiz {
				v = dst[centerSpan(m.cellH, m.light).lo*m.cellW+i]
			} else {
				v = dst[i*m.cellW+centerSpan(m.cellW, m.light).lo]
			}
			on := v != 0
			if on && !prev {
				runs++
			}
			prev = on
		}
		if runs != tc.want {
			t.Fatalf("%U: %d dashes, want %d", tc.cp, runs, tc.want)
		}
	}
}

func TestBoxDegenerateCells(t *testing.T) {
	// Tiny cells must stay in bounds and not panic.
	for _, wh := range [][2]int{{1, 1}, {1, 5}, {5, 1}, {2, 2}, {3, 2}} {
		for cp := rune(boxLineLo); cp <= boxPowerHi; cp++ {
			if cp > boxBlockHi && cp < boxPowerLo {
				continue
			}
			if boxGlyphKind(cp) == boxKindNone {
				continue
			}
			m := testBoxMetrics(t, cp, wh[0], wh[1])
			dst := make([]byte, m.cellW*m.cellH)
			drawBoxGlyph(dst, m)
		}
	}

	// Unusable and oversized cells fall back to the font.
	base := Item{Ascent: 10, Descent: 2}
	if _, ok := boxMetricsFor(base, Glyph{XAdvance: 0}, '─', 1); ok {
		t.Fatal("zero advance: want fallback to the font")
	}
	huge := Item{
		Style:   TextStyle{CellWidth: MaxGlyphSize + 1, CellHeight: 20},
		Ascent:  16,
		Descent: 4,
	}
	if _, ok := boxMetricsFor(huge, Glyph{XAdvance: 10}, '─', 1); ok {
		t.Fatal("oversized cell: want fallback to the font")
	}
}

func TestBoxMetricsGating(t *testing.T) {
	base := Item{
		Style:   TextStyle{CellWidth: 11, CellHeight: 23},
		Ascent:  18,
		Descent: 5,
	}
	g := Glyph{XAdvance: 11}

	if _, ok := boxMetricsFor(base, g, 'A', 1); ok {
		t.Fatal("U+0041: want fallback to the font")
	}
	opt := base
	opt.Style.NoBuiltinBoxGlyphs = true
	if _, ok := boxMetricsFor(opt, g, '─', 1); ok {
		t.Fatal("NoBuiltinBoxGlyphs: want fallback to the font")
	}
	stroked := base
	stroked.HasStroke = true
	if _, ok := boxMetricsFor(stroked, g, '─', 1); ok {
		t.Fatal("stroked run: want fallback to the font")
	}

	// Powerline is drawn only when the font had no glyph of its own.
	pl := base
	pl.FontPath = "/some/font.ttf"
	if _, ok := boxMetricsFor(pl, Glyph{XAdvance: 11, GlyphID: 42, Shaped: true},
		0xE0B0, 1); ok {
		t.Fatal("E0B0 with a font glyph: want the font's own separator")
	}
	if _, ok := boxMetricsFor(pl, Glyph{XAdvance: 11}, 0xE0B0, 1); ok {
		t.Fatal("E0B0 unshaped: want fallback to the font")
	}
	if _, ok := boxMetricsFor(pl, Glyph{XAdvance: 11, Shaped: true},
		0xE0B0, 1); !ok {
		t.Fatal("E0B0 notdef: want the built-in separator")
	}
}

func TestBoxMetricsCellDerivation(t *testing.T) {
	item := Item{Ascent: 18, Descent: 5}

	// A fractional advance rounds up, so neighbours overlap rather than gap.
	m, ok := boxMetricsFor(item, Glyph{XAdvance: 8.4}, '─', 1)
	if !ok || m.cellW != 9 {
		t.Fatalf("cellW = %d (ok=%v), want 9", m.cellW, ok)
	}
	if m.cellH != 23 {
		t.Fatalf("cellH = %d, want 23", m.cellH)
	}
	if m.top != 18 {
		t.Fatalf("top = %d, want 18", m.top)
	}

	// Scale multiplies through into whole physical pixels.
	m2, _ := boxMetricsFor(item, Glyph{XAdvance: 8.4}, '─', 2)
	if m2.cellW != 17 || m2.cellH != 46 {
		t.Fatalf("at 2x: cell = %dx%d, want 17x46", m2.cellW, m2.cellH)
	}

	// The style override wins over the advance.
	item.Style.CellWidth = 12
	item.Style.CellHeight = 24
	m3, _ := boxMetricsFor(item, Glyph{XAdvance: 8.4}, '─', 1)
	if m3.cellW != 12 || m3.cellH != 24 {
		t.Fatalf("override: cell = %dx%d, want 12x24", m3.cellW, m3.cellH)
	}
}

// TestBoxHashDistinct asserts hashBoxGlyph is injective over the space of
// metrics the renderer can produce — the bitmap is a pure function of these
// fields, so a collision would show one codepoint's art in another's cell.
func TestBoxHashDistinct(t *testing.T) {
	seen := make(map[uint64]boxMetrics)
	for cp := rune(boxLineLo); cp <= boxPowerHi; cp++ {
		if cp > boxBlockHi && cp < boxPowerLo {
			continue
		}
		for w := 4; w <= 20; w += 3 {
			for h := 8; h <= 48; h += 5 {
				m := boxMetrics{cp: cp, kind: boxKindLine, cellW: w, cellH: h,
					top: h * 4 / 5, light: max(1, h/12),
					heavy: max(2, h/6), gap: max(1, h/12)}
				k := hashBoxGlyph(fnvOffsetBasis, m)
				if prev, dup := seen[k]; dup && prev != m {
					t.Fatalf("collision: %+v and %+v", prev, m)
				}
				seen[k] = m
			}
		}
	}
	if len(seen) < 1000 {
		t.Fatalf("only %d distinct keys, expected the full grid", len(seen))
	}
}

// TestBoxMetricsRejectsUnusableScale checks the float-to-int conversions
// saturate instead of relying on implementation-defined behaviour. The scale
// factor comes from the caller (NewRenderer) and the advance from the font,
// so neither is trustworthy; every case here must fall back to the font
// rather than produce a nonsense cell or panic.
func TestBoxMetricsRejectsUnusableScale(t *testing.T) {
	item := Item{Ascent: 18, Descent: 5}
	g := Glyph{XAdvance: 11}

	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for _, tc := range []struct {
		name  string
		item  Item
		g     Glyph
		scale float32
	}{
		{"NaN scale", item, g, nan},
		{"+Inf scale", item, g, inf},
		{"-Inf scale", item, g, float32(math.Inf(-1))},
		{"huge scale", item, g, 1e30},
		{"negative scale", item, g, -2},
		{"zero scale", item, g, 0},
		{"NaN advance", item, Glyph{XAdvance: float64(nan)}, 1},
		{"huge advance", item, Glyph{XAdvance: 1e30}, 1},
		{"huge cell override",
			Item{Style: TextStyle{CellWidth: 1e30, CellHeight: 1e30},
				Ascent: 18, Descent: 5}, g, 1},
	} {
		m, ok := boxMetricsFor(tc.item, tc.g, '─', tc.scale)
		if ok {
			t.Errorf("%s: accepted, cell = %dx%d; want fallback to the font",
				tc.name, m.cellW, m.cellH)
		}
	}

	// A NaN cell override fails the > 0 guard, so it is ignored and the cell
	// derives from the advance — the same path a zero override takes. That is
	// better than dropping to the font glyph, so it is pinned deliberately.
	m, ok := boxMetricsFor(
		Item{Style: TextStyle{CellWidth: nan, CellHeight: nan},
			Ascent: 18, Descent: 5}, g, '─', 1)
	if !ok || m.cellW != 11 || m.cellH != 23 {
		t.Errorf("NaN override: cell = %dx%d (ok=%v), want the "+
			"advance-derived 11x23", m.cellW, m.cellH, ok)
	}
}

// TestBoxGlyphKindBoundaries pins the range edges: one codepoint outside
// each range must not be claimed, since claiming it would replace a real
// font glyph with a blank cell.
func TestBoxGlyphKindBoundaries(t *testing.T) {
	for _, tc := range []struct {
		cp   rune
		want boxKind
	}{
		{0x24FF, boxKindNone},
		{boxLineLo, boxKindLine},
		{boxLineHi, boxKindLine},
		{boxBlockLo, boxKindBlock},
		{boxBlockHi, boxKindBlock},
		{0x25A0, boxKindNone},
		{0xE0AF, boxKindNone},
		{boxPowerLo, boxKindPowerline},
		{boxPowerHi, boxKindPowerline},
		{0xE0B4, boxKindNone},
	} {
		if got := boxGlyphKind(tc.cp); got != tc.want {
			t.Errorf("boxGlyphKind(%U) = %d, want %d", tc.cp, got, tc.want)
		}
	}
}

// TestBoxStyleFieldsReachTheRenderer covers the glyph.go cache-key fold. A
// cached Layout hands its Items' Style straight to the renderer, so a field
// missing from the key would pin the first call's value and silently ignore
// every later change. NoBuiltinBoxGlyphs is packed at bit 24, immediately
// above Orientation's bits 16+, which is the collision worth pinning.
func TestBoxStyleFieldsReachTheRenderer(t *testing.T) {
	ts := &TextSystem{}
	base := TextConfig{Style: TextStyle{FontName: "Monospace 12"}}
	baseKey := ts.getCacheKey("┌", base)

	variants := map[string]TextConfig{
		"CellWidth":          {Style: TextStyle{FontName: "Monospace 12", CellWidth: 11}},
		"CellHeight":         {Style: TextStyle{FontName: "Monospace 12", CellHeight: 23}},
		"NoBuiltinBoxGlyphs": {Style: TextStyle{FontName: "Monospace 12", NoBuiltinBoxGlyphs: true}},
	}
	seen := map[uint64]string{baseKey: "base"}
	for name, cfg := range variants {
		k := ts.getCacheKey("┌", cfg)
		if prev, dup := seen[k]; dup {
			t.Errorf("%s shares a layout cache key with %s", name, prev)
		}
		seen[k] = name
	}

	// Orientation occupies bits 16 and up in the same packed word.
	for o := TextOrientation(0); o < 4; o++ {
		on := base
		on.Orientation = o
		on.Style.NoBuiltinBoxGlyphs = true
		off := base
		off.Orientation = o
		if ts.getCacheKey("┌", on) == ts.getCacheKey("┌", off) {
			t.Errorf("orientation %d: NoBuiltinBoxGlyphs collides with "+
				"the orientation bits", o)
		}
	}
}
