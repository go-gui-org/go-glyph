//go:build linux || darwin || windows

package glyph

import (
	"math"
	"testing"
)

// openFTFont takes raw pixel sizes, so it must keep its int32 26.6 scale
// positive and bounded for NaN, Inf and huge inputs.
func TestOpenFTFontScaleBounded(t *testing.T) {
	ctx, err := NewContext(1)
	if err != nil {
		t.Skipf("NewContext: %v", err)
	}
	defer ctx.Free()
	var path string
	for _, p := range ctx.fontPaths {
		if p != "" {
			path, _ = splitFacePath(p)
			break
		}
	}
	if path == "" {
		t.Skip("no installed fonts")
	}
	for _, size := range []float64{1e12, math.Inf(1), math.NaN()} {
		f := openFTFont(path, size)
		if f.cf == nil {
			t.Skipf("font %s did not load", path)
		}
		if f.scale <= 0 || f.scale > int32(maxRenderSize*64) {
			t.Errorf("openFTFont(size=%v).scale = %d, want in (0, %d]",
				size, f.scale, int32(maxRenderSize*64))
		}
	}
}

// Out-of-range sizes are clamped, so layout geometry stays finite.
func TestLayoutTextSizeClamped(t *testing.T) {
	ctx, err := NewContext(1)
	if err != nil {
		t.Skipf("NewContext: %v", err)
	}
	defer ctx.Free()
	lay := func(size float32) Layout {
		t.Helper()
		l, err := ctx.LayoutText("abc", TextConfig{
			Style: TextStyle{FontName: "Sans", Size: size},
		})
		if err != nil {
			t.Fatalf("LayoutText(size=%v): %v", size, err)
		}
		return l
	}
	over, atMax := lay(600), lay(500)
	if math.Abs(float64(over.Height-atMax.Height)) > 0.5 {
		t.Errorf("Height at 600 = %v, at 500 = %v, want equal",
			over.Height, atMax.Height)
	}
	for _, size := range []float32{float32(math.Inf(1)), 1e30} {
		l := lay(size)
		for _, v := range []float32{l.Width, l.Height} {
			if math.IsInf(float64(v), 0) || math.IsNaN(float64(v)) {
				t.Errorf("size %v: non-finite geometry W=%v H=%v",
					size, l.Width, l.Height)
			}
		}
	}
}
