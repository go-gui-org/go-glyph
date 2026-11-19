//go:build linux || darwin || windows

package glyph

import (
	"math"
	"os"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// segBounds returns the box of every point of segs (control points too).
func segBounds(segs []font.Segment) (x0, y0, x1, y1 float64) {
	x0, y0 = math.Inf(1), math.Inf(1)
	x1, y1 = math.Inf(-1), math.Inf(-1)
	for si := range segs {
		for _, p := range segs[si].ArgsSlice() {
			x0, x1 = math.Min(x0, float64(p.X)), math.Max(x1, float64(p.X))
			y0, y1 = math.Min(y0, float64(p.Y)), math.Max(y1, float64(p.Y))
		}
	}
	return x0, y0, x1, y1
}

// TestEmboldenSquareGrows checks that both contour directions grow outward
// by the strength on each side: TrueType (clockwise) and CFF
// (counter-clockwise) outlines must both get heavier, not thinner.
func TestEmboldenSquareGrows(t *testing.T) {
	for _, cw := range []bool{true, false} {
		segs := rectContour(nil, 2, 6, cw) // 100..300 x 0..500 font units
		emboldenSegs(segs, 10, 20)
		x0, y0, x1, y1 := segBounds(segs)
		want := [4]float64{90, -20, 310, 520}
		got := [4]float64{x0, y0, x1, y1}
		for i := range want {
			if math.Abs(got[i]-want[i]) > 1e-3 {
				t.Errorf("cw=%v: bounds = %v, want %v", cw, got, want)
				break
			}
		}
	}
}

// TestEmboldenShrinksCounter checks that an inner contour (a counter, which
// runs the other way) gets smaller, so the ink around it gets wider.
func TestEmboldenShrinksCounter(t *testing.T) {
	// Outer 0..10 px, clockwise. The counter is 3..7 px, counter-clockwise.
	segs := rectContour(nil, 0, 10, true)
	nOuter := len(segs)
	segs = rectContour(segs, 3, 7, false)
	emboldenSegs(segs, 10, 10)
	ox0, _, ox1, _ := segBounds(segs[:nOuter])
	ix0, _, ix1, _ := segBounds(segs[nOuter:])
	if ox0 != -10 || ox1 != 510 {
		t.Errorf("outer x = %v..%v, want -10..510", ox0, ox1)
	}
	if ix0 != 160 || ix1 != 340 {
		t.Errorf("counter x = %v..%v, want 160..340", ix0, ix1)
	}
}

// TestEmboldenClosingDuplicate checks a contour whose last point repeats
// its first: the two copies must move together, or the contour would get a
// gap or a sliver at its start.
func TestEmboldenClosingDuplicate(t *testing.T) {
	segs := rectContour(nil, 2, 6, true)
	segs = append(segs, font.Segment{Op: ot.SegmentOpLineTo,
		Args: [3]ot.SegmentPoint{segs[0].Args[0]}})
	emboldenSegs(segs, 10, 10)
	if first, last := segs[0].Args[0], segs[len(segs)-1].Args[0]; first != last {
		t.Errorf("first %v and closing %v points differ", first, last)
	}
}

// TestEmboldenDegenerate checks that contours with too few distinct points
// stay as they are and do not produce NaN.
func TestEmboldenDegenerate(t *testing.T) {
	p := ot.SegmentPoint{X: 5, Y: 5}
	segs := []font.Segment{
		{Op: ot.SegmentOpMoveTo, Args: [3]ot.SegmentPoint{p}},
		{Op: ot.SegmentOpLineTo, Args: [3]ot.SegmentPoint{p}},
		{Op: ot.SegmentOpLineTo, Args: [3]ot.SegmentPoint{{X: 9, Y: 5}}},
	}
	emboldenSegs(segs, 10, 10)
	for si := range segs {
		for _, q := range segs[si].ArgsSlice() {
			if math.IsNaN(float64(q.X)) || math.IsNaN(float64(q.Y)) {
				t.Fatalf("NaN point in %v", segs)
			}
		}
	}
	emboldenSegs(nil, 10, 10) // must not panic
}

// TestEmboldenNoAllocs keeps the common path (a contour of fewer than 256
// points) free of heap allocations.
func TestEmboldenNoAllocs(t *testing.T) {
	segs := rectContour(nil, 0, 10, true)
	segs = rectContour(segs, 3, 7, false)
	if n := testing.AllocsPerRun(100, func() { emboldenSegs(segs, 1, 1) }); n != 0 {
		t.Errorf("emboldenSegs allocates %v times, want 0", n)
	}
}

// TestDarkenAmount checks the cap, the per-bucket order (lighter text gets
// at least as much) and the zero cases.
func TestDarkenAmount(t *testing.T) {
	if ax, ay := darkenAmount(0, curveLight); ax != 0 || ay != 0 {
		t.Errorf("size 0: got %v, %v, want 0, 0", ax, ay)
	}
	if ax, ay := darkenAmount(math.NaN(), curveLight); ax != 0 || ay != 0 {
		t.Errorf("size NaN: got %v, %v, want 0, 0", ax, ay)
	}
	ax, ay := darkenAmount(1000, curveLight)
	if ax > darkenMaxPx || ay > darkenMaxPx {
		t.Errorf("size 1000: got %v, %v, want at most %v", ax, ay, darkenMaxPx)
	}
	n, _ := darkenAmount(14, curveNone)
	m, _ := darkenAmount(14, curveMid)
	l, _ := darkenAmount(14, curveLight)
	if !(n > 0 && n <= m && m <= l) {
		t.Errorf("size 14: none %v, mid %v, light %v, want 0 < none <= mid <= light",
			n, m, l)
	}
	if _, ay := darkenAmount(14, curveNone); ay <= n {
		t.Errorf("size 14: ay %v, want more than ax %v", ay, n)
	}
}

// TestRasterizeDarkenOnlyFills checks that rasterizeGlyphs darkens a fill
// when asked, and leaves a stroke alone (a stroke has its own width).
func TestRasterizeDarkenOnlyFills(t *testing.T) {
	cf := &cachedFace{upem: hintTestUpem}
	ink := func(stroke float64, darken bool) int {
		segs := rectContour(nil, 2, 6, true)
		res, _ := rasterizeGlyphs(nil, cf, hintTestSize, stroke,
			[]placedGlyph{{segs: segs}}, rasterOpts{darken: darken})
		if res == nil {
			t.Fatal("rasterizeGlyphs returned nil")
		}
		s := 0
		for i := 3; i < len(res.data); i += 4 {
			s += int(res.data[i])
		}
		return s
	}
	if plain, dark := ink(0, false), ink(0, true); dark <= plain {
		t.Errorf("fill: darkened ink %d, want more than %d", dark, plain)
	}
	if plain, dark := ink(1, false), ink(1, true); dark != plain {
		t.Errorf("stroke: darkened ink %d, want %d (unchanged)", dark, plain)
	}
}

// TestStemDarkeningInk compares the ink of darkened text with the ink of the
// same text drawn by Core Graphics with font smoothing on, as AppKit and
// mygo draw it. The reference values were measured on macOS 27 with
// CTFontCreateUIFontForLanguage(.system) (SFNS.ttf), "Hamburgefonstiv",
// an alpha-only bitmap, subpixel positioning on, and fill gray 0 (black),
// 0.75 (mid) and 1 (white). Ink is the sum of alpha / 255. Our ink is taken
// after the coverage curve, because that is what goes on screen.
//
// A large miss means the darkening factors or the gamma curves changed. A
// new SFNS.ttf can move both sides a little, hence the tolerance.
func TestStemDarkeningInk(t *testing.T) {
	const path = "/System/Library/Fonts/SFNS.ttf"
	if _, err := os.Stat(path); err != nil {
		t.Skip("needs the macOS system font")
	}
	coreGraphics := map[float64][3]float64{
		13: {320.91, 356.85, 393.21},
		16: {506.56, 562.18, 612.18},
		20: {763.16, 859.48, 939.23},
		28: {1459.32, 1639.10, 1639.10},
	}
	const tolerance = 0.08
	for size, want := range coreGraphics {
		for _, c := range []coverageCurve{curveNone, curveMid, curveLight} {
			// vary as a Renderer at scale 1 sets it: the auto opsz instance,
			// which is what Core Graphics draws.
			ro := rasterOpts{hint: true, darken: true, curve: c,
				vary: varSpec{scale: 1}}
			res, _ := renderMonoRun(nil, path, size, 0, "Hamburgefonstiv", "", 0,
				false, ro)
			if res == nil {
				t.Fatalf("size %v: no raster", size)
			}
			applyCoverageCurve(res.data, c)
			s := 0
			for i := 3; i < len(res.data); i += 4 {
				s += int(res.data[i])
			}
			ratio := float64(s) / 255 / want[c]
			if math.Abs(ratio-1) > tolerance {
				t.Errorf("size %v curve %d: ink is %.3f of Core Graphics, want 1±%v",
					size, c, ratio, tolerance)
			}
		}
	}
}
