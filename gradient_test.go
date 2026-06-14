package glyph

import "testing"

func TestLerpColorEndpoints(t *testing.T) {
	a := Color{0, 0, 0, 255}
	b := Color{255, 255, 255, 255}

	c0 := LerpColor(a, b, 0.0)
	if c0.R != 0 || c0.G != 0 || c0.B != 0 || c0.A != 255 {
		t.Errorf("lerp t=0: %+v", c0)
	}
	c1 := LerpColor(a, b, 1.0)
	if c1.R != 255 || c1.G != 255 || c1.B != 255 || c1.A != 255 {
		t.Errorf("lerp t=1: %+v", c1)
	}
}

func TestLerpColorMidpoint(t *testing.T) {
	a := Color{0, 0, 0, 0}
	b := Color{200, 100, 50, 250}
	c := LerpColor(a, b, 0.5)
	if c.R != 100 || c.G != 50 || c.B != 25 || c.A != 125 {
		t.Errorf("lerp mid: %+v", c)
	}
}

func TestLerpColorClampsT(t *testing.T) {
	a := Color{10, 20, 30, 40}
	b := Color{110, 120, 130, 140}

	cNeg := LerpColor(a, b, -5.0)
	if cNeg.R != a.R || cNeg.G != a.G {
		t.Errorf("lerp t<0: %+v", cNeg)
	}
	cOver := LerpColor(a, b, 10.0)
	if cOver.R != b.R || cOver.G != b.G {
		t.Errorf("lerp t>1: %+v", cOver)
	}
}

func TestGradientColorAtEmptyStops(t *testing.T) {
	c := GradientColorAt(nil, 0.5)
	if c.R != 0 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("empty stops: %+v", c)
	}
}

func TestGradientColorAtSingleStop(t *testing.T) {
	stops := []GradientStop{{Color: Color{100, 150, 200, 255}, Position: 0.5}}
	c0 := GradientColorAt(stops, 0.0)
	if c0.R != 100 {
		t.Errorf("single stop t=0: R=%d", c0.R)
	}
	c1 := GradientColorAt(stops, 1.0)
	if c1.R != 100 {
		t.Errorf("single stop t=1: R=%d", c1.R)
	}
}

func TestGradientColorAtTwoStops(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{0, 0, 0, 255}, Position: 0.0},
		{Color: Color{200, 100, 50, 255}, Position: 1.0},
	}
	c := GradientColorAt(stops, 0.5)
	if c.R != 100 || c.G != 50 || c.B != 25 {
		t.Errorf("two stops mid: %+v", c)
	}
}

func TestGradientColorAtFourStops(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{255, 0, 0, 255}, Position: 0.0},
		{Color: Color{0, 255, 0, 255}, Position: 0.33},
		{Color: Color{0, 0, 255, 255}, Position: 0.66},
		{Color: Color{255, 255, 255, 255}, Position: 1.0},
	}
	c0 := GradientColorAt(stops, 0.0)
	if c0.R != 255 || c0.G != 0 {
		t.Errorf("4-stop t=0: %+v", c0)
	}
	c1 := GradientColorAt(stops, 0.33)
	if c1.R != 0 || c1.G != 255 {
		t.Errorf("4-stop t=0.33: %+v", c1)
	}
	c3 := GradientColorAt(stops, 1.0)
	if c3.R != 255 || c3.G != 255 || c3.B != 255 {
		t.Errorf("4-stop t=1: %+v", c3)
	}
}

func TestGradientColorAtBeforeFirstStop(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{50, 100, 150, 200}, Position: 0.3},
		{Color: Color{200, 200, 200, 255}, Position: 0.8},
	}
	c := GradientColorAt(stops, 0.0)
	if c.R != 50 || c.G != 100 {
		t.Errorf("before first: %+v", c)
	}
}

func TestGradientColorAtAfterLastStop(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{50, 100, 150, 200}, Position: 0.2},
		{Color: Color{200, 200, 200, 255}, Position: 0.7},
	}
	c := GradientColorAt(stops, 1.0)
	if c.R != 200 || c.A != 255 {
		t.Errorf("after last: %+v", c)
	}
}

func TestGradientColorAtNegativeT(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{100, 0, 0, 255}, Position: 0.0},
		{Color: Color{200, 0, 0, 255}, Position: 1.0},
	}
	c := GradientColorAt(stops, -5.0)
	if c.R != 100 {
		t.Errorf("negative t: R=%d, want 100", c.R)
	}
}

func TestGradientColorAtLargeT(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{100, 0, 0, 255}, Position: 0.0},
		{Color: Color{200, 0, 0, 255}, Position: 1.0},
	}
	c := GradientColorAt(stops, 100.0)
	if c.R != 200 {
		t.Errorf("large t: R=%d, want 200", c.R)
	}
}

// ---------------------------------------------------------------------------
// gradientColorForGlyph
// ---------------------------------------------------------------------------

func TestGradientForGlyphNil(t *testing.T) {
	c := gradientColorForGlyph(nil, 50, 50, 20, 0, 0, 100, 100)
	if c.R != 0 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("nil gradient: %+v", c)
	}
}

func TestGradientForGlyphHorizontal(t *testing.T) {
	grad := &GradientConfig{
		Direction: GradientHorizontal,
		Stops: []GradientStop{
			{Color: Color{0, 0, 0, 255}, Position: 0.0},
			{Color: Color{255, 0, 0, 255}, Position: 1.0},
		},
	}
	// Left edge → t=0 → black.
	cl := gradientColorForGlyph(grad, 0, 0, 0, 0, 0, 100, 100)
	if cl.R != 0 {
		t.Errorf("left edge: R=%d, want 0", cl.R)
	}
	// Right edge → t=1 → red.
	cr := gradientColorForGlyph(grad, 100, 0, 0, 0, 0, 100, 100)
	if cr.R != 255 {
		t.Errorf("right edge: R=%d, want 255", cr.R)
	}
	// Midpoint → t=0.5 → R=~127.
	cm := gradientColorForGlyph(grad, 50, 0, 0, 0, 0, 100, 100)
	if cm.R < 120 || cm.R > 135 {
		t.Errorf("midpoint: R=%d, want ~127", cm.R)
	}
}

func TestGradientForGlyphVertical(t *testing.T) {
	grad := &GradientConfig{
		Direction: GradientVertical,
		Stops: []GradientStop{
			{Color: Color{0, 0, 0, 255}, Position: 0.0},
			{Color: Color{0, 255, 0, 255}, Position: 1.0},
		},
	}
	ascent := float32(20)
	// Top (cy = ascent) → t=0 → black.
	ct := gradientColorForGlyph(grad, 0, 20, ascent, 0, 0, 100, 100)
	if ct.G != 0 {
		t.Errorf("top: G=%d, want 0", ct.G)
	}
	// Bottom (cy = ascent + gradH) → t=1 → green.
	cb := gradientColorForGlyph(grad, 0, 120, ascent, 0, 0, 100, 100)
	if cb.G != 255 {
		t.Errorf("bottom: G=%d, want 255", cb.G)
	}
}

func TestGradientForGlyphDiagonal(t *testing.T) {
	grad := &GradientConfig{
		Direction: GradientDiagonal,
		Stops: []GradientStop{
			{Color: Color{0, 0, 0, 255}, Position: 0.0},
			{Color: Color{0, 0, 255, 255}, Position: 1.0},
		},
	}
	ascent := float32(20)
	// Top-left (cx=gradXOff, cy=ascent) → t≈0 → black.
	tl := gradientColorForGlyph(grad, 0, 20, ascent, 0, 0, 100, 100)
	if tl.B > 20 {
		t.Errorf("top-left: B=%d, want ~0", tl.B)
	}
	// Bottom-right (cx=gradXOff+gradW, cy=ascent+gradH) → t≈1 → blue.
	br := gradientColorForGlyph(grad, 100, 120, ascent, 0, 0, 100, 100)
	if br.B < 230 {
		t.Errorf("bottom-right: B=%d, want ~255", br.B)
	}
	// Center → t≈0.5.
	cc := gradientColorForGlyph(grad, 50, 70, ascent, 0, 0, 100, 100)
	if cc.B < 110 || cc.B > 145 {
		t.Errorf("center: B=%d, want ~127", cc.B)
	}
}

func TestGradientForGlyphDiagonalZeroExtent(t *testing.T) {
	grad := &GradientConfig{
		Direction: GradientDiagonal,
		Stops: []GradientStop{
			{Color: Color{100, 0, 0, 255}, Position: 0.0},
		},
	}
	// Zero width and height → t stays 0 → returns first stop color.
	c := gradientColorForGlyph(grad, 0, 0, 0, 0, 0, 0, 0)
	if c.R != 100 {
		t.Errorf("zero extent: R=%d, want 100", c.R)
	}
}

func TestGradientColorAtCoincidentPositions(t *testing.T) {
	stops := []GradientStop{
		{Color: Color{255, 0, 0, 255}, Position: 0.5},
		{Color: Color{0, 0, 255, 255}, Position: 0.5},
	}
	c := GradientColorAt(stops, 0.5)
	if c.R != 255 || c.B != 0 {
		t.Errorf("coincident: %+v", c)
	}
}
