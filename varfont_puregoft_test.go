//go:build linux || darwin || windows

package glyph

import (
	"math"
	"os"
	"testing"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// sfPath is the macOS system font, a variable font with wght, opsz (17..28,
// default 28) and GRAD axes.
const sfPath = "/System/Library/Fonts/SFNS.ttf"

func needSF(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(sfPath); err != nil {
		t.Skip("needs the macOS system font")
	}
}

// shapedWidth returns the advance width, in px, of text shaped with f.
func shapedWidth(t *testing.T, f ftFont, text string) float64 {
	t.Helper()
	buf := f.shape(text)
	if buf == nil {
		t.Fatal("shape returned nil")
	}
	defer releaseShapeBuffer(buf)
	w := 0.0
	for _, p := range buf.Pos {
		w += float64(p.XAdvance) / 64
	}
	return w
}

// TestAutoOpszMatchesCoreText is the regression test for the optical size
// gap. CoreText sets opsz to the point size. Before the fix, the pure-Go path
// shaped SF with its default instance (opsz 28) at every size, so UI text was
// up to 11% narrower than the same text in native apps.
//
// The reference widths were measured on macOS 27 with
// CTFontCreateUIFontForLanguage(.system) at scale 1 (CTLineGetTypographicBounds).
func TestAutoOpszMatchesCoreText(t *testing.T) {
	needSF(t)
	const text = "Trump Spends Entire Summit Fangirling Over"
	coreText := map[float64]float64{13: 271.10, 15: 306.65, 26: 498.22}
	for size, want := range coreText {
		// Scale 2 (Retina): the opsz is the point size, not the px size.
		for _, scale := range []float32{1, 2} {
			px := size * float64(scale)
			f := openFTFont(sfPath, px, varSpecFor(TextStyle{}, scale))
			if f.face == nil {
				t.Fatal("SF did not open")
			}
			got := shapedWidth(t, f, text) / float64(scale)
			if math.Abs(got/want-1) > 0.01 {
				t.Errorf("%v pt at scale %v: width %.2f, CoreText %.2f",
					size, scale, got, want)
			}
		}
	}
}

// TestVariationAxesApplied checks that TextStyle.Features.VariationAxes
// reaches the shaper and the outlines. Before the fix it was only hashed into
// the layout cache key and had no effect.
func TestVariationAxesApplied(t *testing.T) {
	needSF(t)
	style := func(wght float32) TextStyle {
		return TextStyle{Features: &FontFeatures{
			VariationAxes: []FontAxis{{Tag: "wght", Value: wght}}}}
	}
	regular := openFTFont(sfPath, 16, varSpecFor(style(400), 1))
	bold := openFTFont(sfPath, 16, varSpecFor(style(800), 1))
	if regular.cf == bold.cf {
		t.Fatal("wght 400 and 800 share one face")
	}
	if w, b := shapedWidth(t, regular, "Hamburgefonstiv"),
		shapedWidth(t, bold, "Hamburgefonstiv"); b <= w {
		t.Errorf("wght 800 width %.2f, want more than wght 400 width %.2f", b, w)
	}

	ink := func(v varSpec) int {
		ro := rasterOpts{vary: v}
		res, _ := renderMonoRun(nil, sfPath, 16, 0, "H", "", 0, false, ro)
		if res == nil {
			t.Fatal("no raster")
		}
		s := 0
		for i := 3; i < len(res.data); i += 4 {
			s += int(res.data[i])
		}
		return s
	}
	if r, b := ink(varSpecFor(style(400), 1)), ink(varSpecFor(style(800), 1)); b <= r {
		t.Errorf("wght 800 ink %d, want more than wght 400 ink %d", b, r)
	}
}

// TestLayoutUsesInstance checks the public path: LayoutText of SF at 13 pt
// shapes with the opsz 17 instance, and the renderer loads the same face for
// the item.
func TestLayoutUsesInstance(t *testing.T) {
	needSF(t)
	ctx, err := NewContext(2)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()
	f := ctx.createFTFont(TextStyle{FontName: "System Font 13"})
	defer f.close()
	if f.path != sfPath {
		t.Skipf("System resolves to %q, not SF", f.path)
	}
	if f.cf.base == nil {
		t.Fatal("13 pt SF shaped with the default instance")
	}
	if got := f.cf.face.Coords(); len(got) == 0 {
		t.Fatal("instance face has no coords")
	}
	item := Item{Style: TextStyle{FontName: "System Font 13"}, FontPath: f.path}
	_, px, _, _ := resolveFTFontParams(item.Style, 2)
	if cf := loadFace(item.FontPath, px, varSpecFor(item.Style, 2)); cf != f.cf {
		t.Error("renderer face is not the face layout shaped with")
	}
}

// testAxes is a synthetic fvar: wght 100..900 (400) and opsz 10..40 (28).
var testAxes = []tables.VariationAxisRecord{
	{Tag: ot.MustNewTag("wght"), Minimum: 100, Default: 400, Maximum: 900},
	{Tag: opszTag, Minimum: 10, Default: 28, Maximum: 40},
}

// TestDesignValues checks the auto opsz, the caller override, clamping, and
// the default detection.
func TestDesignValues(t *testing.T) {
	cf := &cachedFace{axes: testAxes}
	auto := varSpec{scale: 2}
	tests := []struct {
		name   string
		size   float64
		v      varSpec
		wght   float32
		opsz   float32
		isDflt bool
	}{
		{"auto opsz is pt", 26, auto, 400, 13, false},
		{"rounded to half pt", 26.6, auto, 400, 13.5, false},
		{"default opsz", 56, auto, 400, 28, true},
		{"clamped low", 8, auto, 400, 10, false},
		{"clamped high", 200, auto, 400, 40, false},
		{"no scale: no auto", 26, varSpec{}, 400, 28, true},
		{"caller opsz wins", 26, varSpec{scale: 2,
			axes: []FontAxis{{Tag: "opsz", Value: 20}}}, 400, 20, false},
		{"caller wght", 56, varSpec{scale: 2,
			axes: []FontAxis{{Tag: "wght", Value: 700}}}, 700, 28, false},
		{"NaN to min", 56, varSpec{scale: 2,
			axes: []FontAxis{{Tag: "wght", Value: float32(math.NaN())}}}, 100, 28, false},
		{"bad tag ignored", 56, varSpec{scale: 2,
			axes: []FontAxis{{Tag: "wg", Value: 700}}}, 400, 28, true},
		{"unknown tag ignored", 56, varSpec{scale: 2,
			axes: []FontAxis{{Tag: "wdth", Value: 50}}}, 400, 28, true},
	}
	for _, tc := range tests {
		key, isDflt := cf.designValues(tc.size, tc.v)
		if key[0] != tc.wght || key[1] != tc.opsz || isDflt != tc.isDflt {
			t.Errorf("%s: wght %v opsz %v default %v, want %v %v %v", tc.name,
				key[0], key[1], isDflt, tc.wght, tc.opsz, tc.isDflt)
		}
	}
}

// TestInstanceCache checks that one set of values gives one shared
// instance, that the default values give the base face, and that the map
// stays bounded.
func TestInstanceCache(t *testing.T) {
	needSF(t)
	base := loadCachedFace(sfPath)
	if base == nil || len(base.axes) == 0 {
		t.Fatal("SF has no variation axes")
	}
	v := varSpec{scale: 1}
	if a, b := loadFace(sfPath, 13, v), loadFace(sfPath, 13.1, v); a != b || a == base {
		t.Error("13 and 13.1 px do not share one instance")
	}
	if cf := loadFace(sfPath, 28, v); cf != base {
		t.Error("opsz 28 (the default) did not give the base face")
	}
	if cf := loadFace(sfPath, 13, varSpec{}); cf != base {
		t.Error("no varSpec did not give the base face")
	}
	for i := range 3 * maxFaceInstances {
		w := varSpec{axes: []FontAxis{{Tag: "wght", Value: 100 + float32(i)}}}
		loadFace(sfPath, 13, w)
	}
	base.instMu.Lock()
	n := len(base.instances)
	base.instMu.Unlock()
	if n > maxFaceInstances {
		t.Errorf("%d instances cached, want at most %d", n, maxFaceInstances)
	}
}

// TestStaticFontNoInstance checks that a font without fvar is never
// instanced.
func TestStaticFontNoInstance(t *testing.T) {
	path, _, _ := resolveTestGlyph(t, "H")
	base := loadCachedFace(path)
	if base == nil {
		t.Fatal("test font did not load")
	}
	if len(base.axes) != 0 {
		t.Skipf("test font %q is variable", path)
	}
	if cf := loadFace(path, 13, varSpec{scale: 1,
		axes: []FontAxis{{Tag: "wght", Value: 700}}}); cf != base {
		t.Error("static font got an instance")
	}
}
