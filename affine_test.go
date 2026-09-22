package glyph

import (
	"math"
	"testing"
)

const transformEpsilon = float32(0.0001)

func near(a, b float32) bool {
	return float32(math.Abs(float64(a-b))) < transformEpsilon
}

func TestAffineIdentity(t *testing.T) {
	tr := AffineIdentity()
	if tr.XX != 1 || tr.XY != 0 || tr.YX != 0 || tr.YY != 1 || tr.X0 != 0 || tr.Y0 != 0 {
		t.Errorf("identity fields: %+v", tr)
	}
	x, y := tr.Apply(3.5, -2.0)
	if !near(x, 3.5) || !near(y, -2.0) {
		t.Errorf("identity apply: got (%v, %v), want (3.5, -2.0)", x, y)
	}
}

func TestAffineRotationQuarterTurn(t *testing.T) {
	tr := AffineRotation(float32(math.Pi) * 0.5)
	x, y := tr.Apply(1.0, 0.0)
	if !near(x, 0.0) || !near(y, 1.0) {
		t.Errorf("rotation: got (%v, %v), want (0, 1)", x, y)
	}
}

func TestAffineTranslation(t *testing.T) {
	tr := AffineTranslation(5.0, -2.0)
	x, y := tr.Apply(3.0, 4.0)
	if !near(x, 8.0) || !near(y, 2.0) {
		t.Errorf("translation: got (%v, %v), want (8, 2)", x, y)
	}
}

func TestAffineSkew(t *testing.T) {
	tr := AffineSkew(0.5, -0.25)
	x, y := tr.Apply(4.0, 2.0)
	if !near(x, 5.0) || !near(y, 1.0) {
		t.Errorf("skew: got (%v, %v), want (5, 1)", x, y)
	}
}

func TestAffineMultiplyOrder(t *testing.T) {
	rot := AffineRotation(float32(math.Pi) * 0.5)
	trans := AffineTranslation(5.0, 0.0)

	// trans after rot: rotate (1,0)->(0,1), then shift to (5,1).
	x, y := trans.Multiply(rot).Apply(1.0, 0.0)
	if !near(x, 5.0) || !near(y, 1.0) {
		t.Errorf("trans*rot: got (%v, %v), want (5, 1)", x, y)
	}

	// rot after trans: shift (1,0)->(6,0), then rotate to (0,6).
	// Order matters: the two compositions must differ.
	x, y = rot.Multiply(trans).Apply(1.0, 0.0)
	if !near(x, 0.0) || !near(y, 6.0) {
		t.Errorf("rot*trans: got (%v, %v), want (0, 6)", x, y)
	}
}

func TestAffineMultiplyMatchesNestedApply(t *testing.T) {
	a := AffineSkew(0.25, 0.0).Multiply(AffineScale(2.0, 3.0))
	b := AffineTranslation(7.0, -4.0)
	combined := a.Multiply(b)
	for _, p := range [][2]float32{{0, 0}, {1, 2}, {-3, 5}} {
		bx, by := b.Apply(p[0], p[1])
		wantX, wantY := a.Apply(bx, by)
		gotX, gotY := combined.Apply(p[0], p[1])
		if !near(gotX, wantX) || !near(gotY, wantY) {
			t.Fatalf("p=%v: got (%v, %v), want (%v, %v)",
				p, gotX, gotY, wantX, wantY)
		}
	}
}

func TestAffineScale(t *testing.T) {
	x, y := AffineScale(2.0, -3.0).Apply(4.0, 5.0)
	if !near(x, 8.0) || !near(y, -15.0) {
		t.Errorf("scale: got (%v, %v), want (8, -15)", x, y)
	}
}

func TestAffineIsIdentity(t *testing.T) {
	if !AffineIdentity().IsIdentity() {
		t.Error("identity must report IsIdentity")
	}
	if AffineTranslation(0, 0).IsIdentity() == false {
		// Translation by zero is exactly identity.
		t.Error("zero translation must report IsIdentity")
	}
	if AffineRotation(0.5).IsIdentity() {
		t.Error("rotation must not report IsIdentity")
	}
	if AffineIdentity().Multiply(AffineIdentity()).IsIdentity() == false {
		t.Error("identity*identity must report IsIdentity")
	}
}

func TestAffineIsFinite(t *testing.T) {
	if !AffineIdentity().IsFinite() {
		t.Error("identity must be finite")
	}
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	if (AffineTransform{XX: nan, YY: 1}).IsFinite() {
		t.Error("NaN field must not be finite")
	}
	if (AffineTransform{XX: 1, X0: inf}).IsFinite() {
		t.Error("Inf field must not be finite")
	}
	if !finiteF32(1.5) || finiteF32(nan) || finiteF32(inf) {
		t.Error("finiteF32 mismatch")
	}
}

func TestAffineInverse(t *testing.T) {
	cases := []AffineTransform{
		AffineIdentity(),
		AffineTranslation(5.0, -2.0),
		AffineScale(2.0, 0.5),
		AffineRotation(0.7),
		AffineTranslation(3, 4).Multiply(AffineRotation(1.1)),
	}
	for _, tr := range cases {
		inv, ok := tr.Inverse()
		if !ok {
			t.Fatalf("Inverse(%+v): ok=false, want true", tr)
		}
		// tr after inv must map every test point back to itself.
		round := tr.Multiply(inv)
		for _, p := range [][2]float32{{0, 0}, {1, 2}, {-3, 5}} {
			gotX, gotY := round.Apply(p[0], p[1])
			if !near(gotX, p[0]) || !near(gotY, p[1]) {
				t.Fatalf("Inverse(%+v): round trip p=%v got (%v, %v)",
					tr, p, gotX, gotY)
			}
		}
	}

	singular := []AffineTransform{
		{},
		AffineScale(0, 1),
		{XX: 1, XY: 2, YX: 2, YY: 4}, // det = 0
		{XX: float32(math.NaN()), YY: 1},
		{XX: float32(math.Inf(1)), YY: 1},
	}
	for _, tr := range singular {
		if _, ok := tr.Inverse(); ok {
			t.Errorf("Inverse(%+v): ok=true, want false", tr)
		}
	}
}

func TestAffineRotationAround(t *testing.T) {
	// Quarter turn around (1,0) moves (2,0) to (1,1).
	tr := AffineRotationAround(float32(math.Pi)*0.5, 1.0, 0.0)
	x, y := tr.Apply(2.0, 0.0)
	if !near(x, 1.0) || !near(y, 1.0) {
		t.Errorf("rotate around: got (%v, %v), want (1, 1)", x, y)
	}
	// The pivot itself must not move.
	x, y = tr.Apply(1.0, 0.0)
	if !near(x, 1.0) || !near(y, 0.0) {
		t.Errorf("pivot moved: got (%v, %v), want (1, 0)", x, y)
	}
}
