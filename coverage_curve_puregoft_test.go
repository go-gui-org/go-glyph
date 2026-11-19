//go:build linux || darwin || windows

package glyph

import "testing"

// atlasAlpha returns the alpha bytes of cg, row by row, as the atlas page
// holds them.
func atlasAlpha(r *Renderer, cg CachedGlyph) []byte {
	page := r.atlas.Pages[cg.Page]
	out := make([]byte, 0, cg.Width*cg.Height)
	for y := range cg.Height {
		row := ((cg.Y+y)*page.Width + cg.X) * 4
		for x := range cg.Width {
			out = append(out, page.StagingBack[row+x*4+3])
		}
	}
	return out
}

// TestCoverageCurveByTextColor is the regression test for issue #189. The
// pure-Go raster path wrote raw coverage for every text color, so light text
// on a dark background looked thin and dim. Light text must now get the
// coverageLight curve, and dark text must keep the raw coverage. The two
// must be separate cache entries, because one atlas raster cannot serve both.
func TestCoverageCurveByTextColor(t *testing.T) {
	// Stem darkening (on for macOS) gives each luminance bucket its own
	// outline, so the light raster would not be the dark raster with the
	// curve on top. This test is about the curve only; darkening has its own
	// tests in darken_puregoft_test.go.
	saved := stemDarkening
	stemDarkening = false
	defer func() { stemDarkening = saved }()

	path, _, gid := resolveTestGlyph(t, "H")
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	item := Item{Style: TextStyle{FontName: "Sans 13"}, FontPath: path, Ascent: 13}
	g := Glyph{GlyphID: gid, Codepoint: 1}

	dark := r.getOrLoadGlyph("H", item, g, 1, 0, curveFor(Color{0, 0, 0, 255}))
	light := r.getOrLoadGlyph("H", item, g, 1, 0, curveFor(Color{255, 255, 255, 255}))
	if dark.Width == 0 || light.Width == 0 {
		t.Fatal("glyph did not rasterize")
	}
	if dark == light {
		t.Fatal("dark and light text share one atlas entry")
	}
	if len(r.cache) != 2 {
		t.Fatalf("cache holds %d entries, want 2", len(r.cache))
	}
	if dark.Width != light.Width || dark.Height != light.Height {
		t.Fatalf("sizes differ: %dx%d vs %dx%d",
			dark.Width, dark.Height, light.Width, light.Height)
	}

	da, la := atlasAlpha(r, dark), atlasAlpha(r, light)
	partial := 0
	for i := range da {
		if la[i] != coverageLight[da[i]] {
			t.Fatalf("pixel %d: light alpha %d, want coverageLight[%d] = %d",
				i, la[i], da[i], coverageLight[da[i]])
		}
		if da[i] > 0 && da[i] < 255 {
			partial++
		}
	}
	// Subpixel bin 1 puts the stems off the pixel grid, so edges are
	// partial. Without them the test would prove nothing.
	if partial == 0 {
		t.Fatal("no partial-coverage pixels; the curve was not exercised")
	}
}

// TestInsertRasterLeavesColorGlyphs checks that the curve touches only mono
// coverage masks. A color emoji raster holds real color in its alpha, so a
// remap would change how it looks.
func TestInsertRasterLeavesColorGlyphs(t *testing.T) {
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	for _, mono := range []bool{false, true} {
		data := []byte{10, 20, 30, 128}
		res, err := insertRaster(r.atlas, &rasterResult{data: data, w: 1, h: 1,
			mono: mono}, curveLight)
		if err != nil {
			t.Fatal(err)
		}
		got := atlasAlpha(r, res.Cached)[0]
		want := byte(128)
		if mono {
			want = coverageLight[128]
		}
		if got != want {
			t.Errorf("mono=%v: alpha %d, want %d", mono, got, want)
		}
	}
}

// TestFillCurve checks that the fill curve follows the color the emit pass
// tints with: item.Color, the gradient, or none for original-color glyphs.
func TestFillCurve(t *testing.T) {
	white := Item{Color: Color{255, 255, 255, 255}}
	if got := fillCurve(white, false, curveNone); got != curveLight {
		t.Errorf("white text: %d, want curveLight", got)
	}
	if got := fillCurve(white, true, curveMid); got != curveMid {
		t.Errorf("gradient: %d, want the gradient curve", got)
	}
	emoji := Item{Color: Color{255, 255, 255, 255}, UseOriginalColor: true}
	if got := fillCurve(emoji, true, curveLight); got != curveNone {
		t.Errorf("original color: %d, want curveNone", got)
	}
}

// TestDrawLayoutCurveFollowsColor checks the wiring in drawLayoutImpl: the
// fill curve follows item.Color and the stroke curve follows
// item.StrokeColor. A change of one color must add atlas entries for that
// pass only.
func TestDrawLayoutCurveFollowsColor(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	layout, err := ctx.LayoutText("H", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	black, white := Color{0, 0, 0, 255}, Color{255, 255, 255, 255}
	setColors := func(fill, stroke Color) {
		for i := range layout.Items {
			layout.Items[i].Color = fill
			layout.Items[i].StrokeColor = stroke
			layout.Items[i].HasStroke = true
			layout.Items[i].StrokeWidth = 1
		}
	}

	setColors(black, black)
	r.DrawLayout(layout, 10, 20)
	base := len(r.cache) // one fill and one stroke entry per glyph
	if base == 0 {
		t.Fatal("nothing rasterized")
	}

	// Same colors again: every glyph is a cache hit.
	r.DrawLayout(layout, 10, 20)
	if len(r.cache) != base {
		t.Fatalf("redraw grew cache %d -> %d", base, len(r.cache))
	}

	// Light stroke only: new stroke entries, fills reused.
	setColors(black, white)
	r.DrawLayout(layout, 10, 20)
	afterStroke := len(r.cache)
	if afterStroke != base+base/2 {
		t.Fatalf("light stroke: cache %d, want %d", afterStroke, base+base/2)
	}

	// Light fill too: new fill entries, light strokes reused.
	setColors(white, white)
	r.DrawLayout(layout, 10, 20)
	if len(r.cache) != base*2 {
		t.Fatalf("light fill: cache %d, want %d", len(r.cache), base*2)
	}
}
