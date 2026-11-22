//go:build linux || darwin || windows

package glyph

import (
	"bytes"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Synthetic outlines for the stem-snapping tests (issue 191). With upem 1000
// and a 20 px render size, 50 font units are 1 device pixel, so the test
// shapes can be given in pixels and converted with hintTestUnits.
const (
	hintTestUpem = 1000
	hintTestSize = 20.0
)

// hintTestUnits converts device pixels to font units at hintTestSize.
func hintTestUnits(px float64) float32 {
	return float32(px * hintTestUpem / hintTestSize)
}

// rectContour appends one closed rectangle contour from x0 to x1 (pixels),
// 0 to 10 px tall. cw selects clockwise order in Y-up font space (the
// TrueType outer-contour direction); false gives the CFF direction.
func rectContour(segs []font.Segment, x0, x1 float64, cw bool) []font.Segment {
	l, r := hintTestUnits(x0), hintTestUnits(x1)
	b, t := float32(0), hintTestUnits(10)
	pts := [4]ot.SegmentPoint{{X: l, Y: b}, {X: l, Y: t}, {X: r, Y: t}, {X: r, Y: b}}
	if !cw {
		pts = [4]ot.SegmentPoint{{X: l, Y: b}, {X: r, Y: b}, {X: r, Y: t}, {X: l, Y: t}}
	}
	segs = append(segs, font.Segment{Op: ot.SegmentOpMoveTo,
		Args: [3]ot.SegmentPoint{pts[0]}})
	for _, p := range pts[1:] {
		segs = append(segs, font.Segment{Op: ot.SegmentOpLineTo,
			Args: [3]ot.SegmentPoint{p}})
	}
	return segs
}

// rasterHintTest rasterizes segs at size with or without hinting.
func rasterHintTest(t *testing.T, segs []font.Segment, size float64,
	hint bool) *rasterResult {
	t.Helper()
	cf := &cachedFace{upem: hintTestUpem}
	res, _ := rasterizeGlyphs(nil, cf, size, 0,
		[]placedGlyph{{segs: segs}}, rasterOpts{hint: hint})
	if res == nil {
		t.Fatal("rasterizeGlyphs returned nil")
	}
	return res
}

// midRowAlpha returns the alpha of every column on the middle row of res.
func midRowAlpha(res *rasterResult) []byte {
	row := res.h / 2
	out := make([]byte, res.w)
	for c := range res.w {
		out[c] = res.data[(row*res.w+c)*4+3]
	}
	return out
}

// countColumns returns how many columns are solid (alpha >= 250) and how
// many are gray (alpha strictly between 5 and 250).
func countColumns(alpha []byte) (solid, gray int) {
	for _, a := range alpha {
		switch {
		case a >= 250:
			solid++
		case a > 5:
			gray++
		}
	}
	return solid, gray
}

// TestHintSnapsStemToOnePixel is the core regression test for issue 191: a
// 1.2 px stem that starts at x = 0.3 px covers two gray columns without
// hinting. With hinting it must be one solid column and no gray.
func TestHintSnapsStemToOnePixel(t *testing.T) {
	segs := rectContour(nil, 0.3, 1.5, true)

	raw := midRowAlpha(rasterHintTest(t, segs, hintTestSize, false))
	if _, gray := countColumns(raw); gray == 0 {
		t.Fatalf("control: unhinted stem has no gray columns: %v", raw)
	}

	got := midRowAlpha(rasterHintTest(t, segs, hintTestSize, true))
	solid, gray := countColumns(got)
	if solid != 1 || gray != 0 {
		t.Errorf("hinted stem: solid=%d gray=%d, want 1 and 0 (row %v)",
			solid, gray, got)
	}
}

// TestHintRoundsStemWidth checks the snapped width. A width within
// hintWidthSnap of a whole number is rounded to it (all solid, no gray). A
// hairline grows to one pixel. Any other width is kept: the left edge is
// solid and only the right edge is gray.
func TestHintRoundsStemWidth(t *testing.T) {
	cases := []struct {
		name      string
		x0, x1    float64
		wantSolid int
		wantGray  int
	}{
		{"near 2px (2.2)", 0.3, 2.5, 2, 0},
		{"near 3px (2.8)", 0.5, 3.3, 3, 0},
		{"hairline 0.6px", 0.5, 1.1, 1, 0},
		{"between (1.4)", 0.3, 1.7, 1, 1},
		{"between (2.5)", 0.6, 3.1, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segs := rectContour(nil, tc.x0, tc.x1, true)
			got := midRowAlpha(rasterHintTest(t, segs, hintTestSize, true))
			solid, gray := countColumns(got)
			if solid != tc.wantSolid || gray != tc.wantGray {
				t.Errorf("solid=%d gray=%d, want %d and %d (row %v)",
					solid, gray, tc.wantSolid, tc.wantGray, got)
			}
		})
	}
}

// TestHintKeepsStemWeight is the regression test for stems that looked too
// thin: rounding a 1.4 px stem down to 1 px removed 30% of its ink, so 'l'
// and 'i' looked thin next to stems rounded up. The total coverage of a
// stem whose width is not near a whole pixel must stay within 5% of the
// unhinted coverage.
func TestHintKeepsStemWeight(t *testing.T) {
	for _, w := range []float64{1.4, 1.5, 1.6, 2.4} {
		segs := rectContour(nil, 0.3, 0.3+w, true)
		sum := func(hint bool) (s int) {
			for _, a := range midRowAlpha(rasterHintTest(t, segs, hintTestSize, hint)) {
				s += int(a)
			}
			return s
		}
		raw, got := sum(false), sum(true)
		if d := float64(got-raw) / float64(raw); d < -0.05 || d > 0.05 {
			t.Errorf("w=%.1f: hinted ink %d vs raw %d (%+.0f%%), want within 5%%",
				w, got, raw, d*100)
		}
	}
}

// TestHintCFFOrientation checks that a counter-clockwise outer contour (the
// CFF direction) snaps the same as the TrueType direction.
func TestHintCFFOrientation(t *testing.T) {
	tt := rasterHintTest(t, rectContour(nil, 0.3, 1.5, true), hintTestSize, true)
	cff := rasterHintTest(t, rectContour(nil, 0.3, 1.5, false), hintTestSize, true)
	if tt.w != cff.w || tt.left != cff.left || !bytes.Equal(tt.data, cff.data) {
		t.Errorf("CFF raster differs from TrueType raster:\n tt=%v\ncff=%v",
			midRowAlpha(tt), midRowAlpha(cff))
	}
}

// TestHintKeepsCounterOpen checks two stems with a narrow gap between them,
// like the letter m: both stems snap, and the gap stays at least one clear
// column, so the stems do not merge.
func TestHintKeepsCounterOpen(t *testing.T) {
	segs := rectContour(nil, 0.3, 1.5, true)
	segs = rectContour(segs, 2.7, 3.9, true) // 1.2 px gap
	got := midRowAlpha(rasterHintTest(t, segs, hintTestSize, true))
	solid, gray := countColumns(got)
	if solid != 2 || gray != 0 {
		t.Fatalf("solid=%d gray=%d, want 2 and 0 (row %v)", solid, gray, got)
	}
	// The two solid columns must not touch.
	for c := 1; c < len(got); c++ {
		if got[c] >= 250 && got[c-1] >= 250 {
			t.Errorf("stems merged at columns %d-%d (row %v)", c-1, c, got)
		}
	}
}

// TestHintOffKeepsRawRaster checks that hint=false, and a size above
// hintMaxSize, give the same bytes as the unhinted raster.
func TestHintOffKeepsRawRaster(t *testing.T) {
	segs := rectContour(nil, 0.3, 1.5, true)
	// Above hintMaxSize: same outline at a larger size, hint on vs off.
	big := float64(hintMaxSize) + 4
	on := rasterHintTest(t, segs, big, true)
	off := rasterHintTest(t, segs, big, false)
	if on.w != off.w || on.left != off.left || !bytes.Equal(on.data, off.data) {
		t.Error("hinting changed a raster above hintMaxSize")
	}
}

// TestHintIgnoresSlantedStem checks that an italic (slanted) stem is not an
// edge, so the map stays the identity and the raster is unchanged.
func TestHintIgnoresSlantedStem(t *testing.T) {
	l0, l1 := hintTestUnits(0.3), hintTestUnits(2.3) // 2 px lean over 10 px
	r0, r1 := hintTestUnits(1.5), hintTestUnits(3.5)
	top := hintTestUnits(10)
	segs := []font.Segment{
		{Op: ot.SegmentOpMoveTo, Args: [3]ot.SegmentPoint{{X: l0, Y: 0}}},
		{Op: ot.SegmentOpLineTo, Args: [3]ot.SegmentPoint{{X: l1, Y: top}}},
		{Op: ot.SegmentOpLineTo, Args: [3]ot.SegmentPoint{{X: r1, Y: top}}},
		{Op: ot.SegmentOpLineTo, Args: [3]ot.SegmentPoint{{X: r0, Y: 0}}},
	}
	g := placedGlyph{segs: segs}
	if h := buildXHint(g, hintTestSize/hintTestUpem, hintTestSize); h.n != 0 {
		t.Errorf("slanted stem made %d control points, want 0", h.n)
	}
}

// TestHintMapIsMonotonic checks that the x map never runs backward, so the
// outline cannot fold over itself.
func TestHintMapIsMonotonic(t *testing.T) {
	segs := rectContour(nil, 0.3, 1.5, true)
	segs = rectContour(segs, 1.9, 2.4, true) // 0.4 px gap, 0.5 px stem
	segs = rectContour(segs, 3.45, 5.55, true)
	h := buildXHint(placedGlyph{segs: segs}, hintTestSize/hintTestUpem,
		hintTestSize)
	if h.n == 0 {
		t.Fatal("no control points")
	}
	prev := h.apply(-2)
	for x := -2.0; x <= 8; x += 0.01 {
		y := h.apply(x)
		if y < prev-1e-9 {
			t.Fatalf("map runs backward at x=%.2f: %.4f < %.4f", x, y, prev)
		}
		prev = y
	}
}

// TestBuildXHintNoAllocs checks the per-glyph hint pass does not allocate.
func TestBuildXHintNoAllocs(t *testing.T) {
	segs := rectContour(nil, 0.3, 1.5, true)
	segs = rectContour(segs, 2.7, 3.9, true)
	g := placedGlyph{segs: segs}
	var sink xHint
	allocs := testing.AllocsPerRun(100, func() {
		sink = buildXHint(g, hintTestSize/hintTestUpem, hintTestSize)
	})
	if allocs != 0 {
		t.Errorf("buildXHint allocated %.0f times per run, want 0", allocs)
	}
	_ = sink
}

// TestHintRealFontStemEveryBin renders a real 'l' at each subpixel bin. The
// stem of every bin must have a solid column: the snap works in device
// pixels, whatever the fractional pen offset.
func TestHintRealFontStemEveryBin(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "l")
	const size = 13.75 // 11 px at 125%: stems fall between pixels
	for bin := range SubpixelBins {
		shift := float64(bin) / SubpixelBins
		res := renderGlyphByID(nil, path, size, 0, shift, gid, rasterOpts{hint: true})
		if res == nil {
			t.Fatalf("bin %d: no raster", bin)
		}
		// Without hinting, bins 1-3 put the stem across two gray columns.
		solid, gray := countColumns(midRowAlpha(res))
		if solid < 1 || gray != 0 {
			t.Errorf("bin %d: solid=%d gray=%d, want >=1 and 0 (row %v)",
				bin, solid, gray, midRowAlpha(res))
		}
	}
}

// TestRendererHintingConfig checks the RendererConfig switches and
// SetHinting. Hinting is off by default (go-gui#1334): the x-only snap
// moves stems but not advances, so straight and curved letters in one word
// look out of step (#198).
func TestRendererHintingConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  RendererConfig
		want bool
	}{
		{"default", RendererConfig{}, false},
		{"EnableHinting", RendererConfig{EnableHinting: true}, true},
		{"DisableHinting", RendererConfig{DisableHinting: true}, false},
		// The deprecated opt-out wins, so a caller that set it for v1.28.0
		// keeps unhinted text whatever else it sets.
		{"both", RendererConfig{EnableHinting: true, DisableHinting: true}, false},
	}
	for _, tc := range cases {
		r, err := NewRendererWithConfig(newMockBackend(), 1, 256, 256, tc.cfg)
		if err != nil {
			t.Fatal(err)
		}
		if r.hint != tc.want {
			t.Errorf("%s: hint = %v, want %v", tc.name, r.hint, tc.want)
		}
		r.Free()
	}

	def, err := NewRenderer(newMockBackend(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer def.Free()
	if def.hint {
		t.Error("default Renderer: hint is on, want off")
	}
	def.SetHinting(true)
	if !def.hint {
		t.Error("SetHinting(true): hint is off")
	}
	def.SetHinting(false)
	if def.hint {
		t.Error("SetHinting(false): hint is on")
	}
}
