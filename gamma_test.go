package glyph

import "testing"

// TestCoverageTablesMonotonic checks that each correction table keeps 0 and
// 255 fixed and never goes down. A table that goes down would make a pixel
// with more ink look lighter than a pixel with less ink.
func TestCoverageTablesMonotonic(t *testing.T) {
	for name, tab := range map[string]*[256]byte{
		"mid":   &coverageMid,
		"light": &coverageLight,
	} {
		if tab[0] != 0 || tab[255] != 255 {
			t.Errorf("%s: ends = (%d, %d), want (0, 255)", name, tab[0], tab[255])
		}
		for i := 1; i < 256; i++ {
			if tab[i] < tab[i-1] {
				t.Errorf("%s: tab[%d]=%d < tab[%d]=%d", name, i, tab[i], i-1, tab[i-1])
			}
		}
		// Both curves raise partial coverage (light text needs more ink).
		if tab[128] <= 128 {
			t.Errorf("%s: tab[128] = %d, want > 128", name, tab[128])
		}
	}
	// The light curve must raise coverage more than the mid curve.
	if coverageLight[128] <= coverageMid[128] {
		t.Errorf("light[128]=%d <= mid[128]=%d", coverageLight[128], coverageMid[128])
	}
}

// TestCurveFor checks the luminance buckets. Dark text keeps the raw
// coverage. Light text gets the strongest raise.
func TestCurveFor(t *testing.T) {
	cases := []struct {
		name string
		c    Color
		want coverageCurve
	}{
		{"black", Color{0, 0, 0, 255}, curveNone},
		{"dark gray", Color{60, 60, 60, 255}, curveNone},
		{"mid gray", Color{160, 160, 160, 255}, curveMid},
		{"light gray", Color{220, 220, 220, 255}, curveLight},
		{"white", Color{255, 255, 255, 255}, curveLight},
		// Pure blue is dark (luminance 0.07) although its channel is full.
		{"blue", Color{0, 0, 255, 255}, curveNone},
		// Pure yellow is light (luminance 0.93).
		{"yellow", Color{255, 255, 0, 255}, curveLight},
	}
	for _, tc := range cases {
		if got := curveFor(tc.c); got != tc.want {
			t.Errorf("%s: curveFor(%v) = %d, want %d", tc.name, tc.c, got, tc.want)
		}
	}
}

// TestApplyCoverageCurve checks that only the alpha bytes change, that
// curveNone changes nothing, and that the remap does not allocate.
func TestApplyCoverageCurve(t *testing.T) {
	// A nil or short buffer has no alpha byte; the remap must not panic.
	applyCoverageCurve(nil, curveLight)
	short := []byte{1, 2, 3}
	applyCoverageCurve(short, curveLight)
	if short[0] != 1 || short[1] != 2 || short[2] != 3 {
		t.Fatalf("short buffer changed: %v", short)
	}

	px := []byte{255, 255, 255, 128, 255, 255, 255, 0, 255, 255, 255, 255}
	orig := append([]byte(nil), px...)

	applyCoverageCurve(px, curveNone)
	for i := range px {
		if px[i] != orig[i] {
			t.Fatalf("curveNone changed byte %d: %d -> %d", i, orig[i], px[i])
		}
	}

	applyCoverageCurve(px, curveLight)
	want := []byte{255, 255, 255, coverageLight[128], 255, 255, 255, 0,
		255, 255, 255, 255}
	for i := range px {
		if px[i] != want[i] {
			t.Errorf("byte %d = %d, want %d", i, px[i], want[i])
		}
	}

	if n := testing.AllocsPerRun(100, func() {
		applyCoverageCurve(px, curveMid)
	}); n != 0 {
		t.Errorf("applyCoverageCurve allocates %v per run, want 0", n)
	}
}

// TestGradientCurve checks that a gradient takes the curve of its mean stop
// color, and that no stops means no correction.
func TestGradientCurve(t *testing.T) {
	if got := gradientCurve(nil); got != curveNone {
		t.Errorf("no stops: %d, want curveNone", got)
	}
	white := GradientStop{Color: Color{255, 255, 255, 255}}
	black := GradientStop{Color: Color{0, 0, 0, 255}, Position: 1}
	if got := gradientCurve([]GradientStop{white, white}); got != curveLight {
		t.Errorf("white..white: %d, want curveLight", got)
	}
	// Mean of black and white is sRGB 127: luminance 0.21, so dark.
	if got := gradientCurve([]GradientStop{white, black}); got != curveNone {
		t.Errorf("white..black: %d, want curveNone", got)
	}
}
