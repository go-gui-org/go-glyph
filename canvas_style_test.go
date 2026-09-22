package glyph

import "testing"

// The WASM text path sets alpha through globalAlpha. The fill style must
// then be opaque: an rgba() style with the same alpha would apply it a
// second time, and a 50% color would draw at 25%.
func TestCSSColorRGBDropsAlpha(t *testing.T) {
	cases := []struct {
		c    Color
		want string
	}{
		{Color{1, 2, 3, 128}, "rgb(1,2,3)"},
		{Color{255, 0, 10, 0}, "rgb(255,0,10)"},
		{Color{0, 0, 0, 255}, "rgb(0,0,0)"},
	}
	for _, tc := range cases {
		// Twice: the second call is served from the one-entry cache.
		for range 2 {
			if got := cssColorRGB(tc.c); got != tc.want {
				t.Errorf("cssColorRGB(%+v) = %q, want %q", tc.c, got, tc.want)
			}
		}
	}
}

// Gradient stops keep their alpha in the style, because globalAlpha is 1
// while a canvas gradient fills. A small alpha must not round to zero.
func TestCSSColorRGBA(t *testing.T) {
	cases := []struct {
		c    Color
		want string
	}{
		{Color{1, 2, 3, 255}, "rgba(1,2,3,1)"},
		{Color{1, 2, 3, 0}, "rgba(1,2,3,0)"},
		{Color{1, 2, 3, 1}, "rgba(1,2,3,0.004)"},
		{Color{1, 2, 3, 128}, "rgba(1,2,3,0.502)"},
		{Color{1, 2, 3, 254}, "rgba(1,2,3,0.996)"},
	}
	for _, tc := range cases {
		if got := cssColorRGBA(tc.c); got != tc.want {
			t.Errorf("cssColorRGBA(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

// Every gradient direction must get a fill mode. Before the fix the
// WASM fill pass handled only vertical and horizontal, so a diagonal
// gradient drew with whatever fillStyle the previous item left.
func TestCanvasFillModeFor(t *testing.T) {
	cases := []struct {
		g    *GradientConfig
		want canvasFillMode
	}{
		{nil, canvasFillFlat},
		{&GradientConfig{}, canvasFillFlat},
		{blackToWhiteStops(GradientVertical), canvasFillGradient},
		{blackToWhiteStops(GradientHorizontal), canvasFillPerGlyph},
		{blackToWhiteStops(GradientDiagonal), canvasFillPerGlyph},
	}
	for _, tc := range cases {
		if got := canvasFillModeFor(tc.g); got != tc.want {
			t.Errorf("canvasFillModeFor(%+v) = %v, want %v", tc.g, got, tc.want)
		}
	}
}

// Under a transform the canvas matrix already holds the draw origin, and
// canvas gradients resolve in the current matrix. The gradient span must
// then stay in layout coords, or the origin Y is applied twice.
func TestCanvasGradientSpanY(t *testing.T) {
	ext := gradientExtents{yOff: 5, h: 40}
	if y0, y1 := ext.canvasSpanY(100, true); y0 != 105 || y1 != 145 {
		t.Errorf("identity span = (%v, %v), want (105, 145)", y0, y1)
	}
	if y0, y1 := ext.canvasSpanY(100, false); y0 != 5 || y1 != 45 {
		t.Errorf("transformed span = (%v, %v), want (5, 45)", y0, y1)
	}
}

func blackToWhiteStops(dir GradientDirection) *GradientConfig {
	return &GradientConfig{
		Direction: dir,
		Stops: []GradientStop{
			{Color: Color{0, 0, 0, 255}, Position: 0},
			{Color: Color{255, 255, 255, 255}, Position: 1},
		},
	}
}

func TestLayoutGradientExtents(t *testing.T) {
	l := Layout{
		VisualWidth: 80, VisualHeight: 30,
		Items: []Item{
			{X: 10, Y: 20, Ascent: 15},
			{X: 4, Y: 40, Ascent: 12},
		},
	}
	got := layoutGradientExtents(&l)
	want := gradientExtents{xOff: 4, yOff: 5, w: 80, h: 30}
	if got != want {
		t.Errorf("extents = %+v, want %+v", got, want)
	}
	// Zero visual size falls back to 1 so t never divides by zero.
	got = layoutGradientExtents(&Layout{})
	if got != (gradientExtents{w: 1, h: 1}) {
		t.Errorf("empty extents = %+v, want w=h=1", got)
	}
}
