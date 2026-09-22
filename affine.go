package glyph

import "math"

// AffineTransform encodes a 2D affine transform matrix:
//
//	[ XX  XY  X0 ]
//	[ YX  YY  Y0 ]
//	[  0   0   1 ]
//
// The zero value is the zero matrix, not identity.
// Use AffineIdentity for the identity transform.
type AffineTransform struct {
	XX float32
	XY float32
	YX float32
	YY float32
	X0 float32
	Y0 float32
}

// Apply maps a point through the affine transform.
func (a AffineTransform) Apply(x, y float32) (float32, float32) {
	return a.XX*x + a.XY*y + a.X0, a.YX*x + a.YY*y + a.Y0
}

// IsIdentity reports whether the transform is exactly identity.
// Exact compare is correct here: the constructors produce exact
// values and Multiply preserves them, so the fast path triggers
// reliably. Near-identity input (for example rotation by 2*pi)
// takes the slow path, which still draws correctly.
func (a AffineTransform) IsIdentity() bool {
	return a == AffineTransform{XX: 1, YY: 1}
}

// IsFinite reports whether all six fields are finite (no NaN,
// no infinite values). A transform that is not finite maps
// every point it touches to garbage, so callers must drop it
// before it reaches a backend.
func (a AffineTransform) IsFinite() bool {
	return finiteF32(a.XX) && finiteF32(a.XY) &&
		finiteF32(a.YX) && finiteF32(a.YY) &&
		finiteF32(a.X0) && finiteF32(a.Y0)
}

// finiteF32 reports whether v is finite (no NaN, no infinite).
func finiteF32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

// AffineIdentity returns an identity transform.
func AffineIdentity() AffineTransform {
	return AffineTransform{XX: 1, YY: 1}
}

// AffineRotation returns a rotation transform in radians around
// the origin. To rotate around another point, use
// AffineRotationAround.
func AffineRotation(angle float32) AffineTransform {
	c := float32(math.Cos(float64(angle)))
	s := float32(math.Sin(float64(angle)))
	return AffineTransform{XX: c, XY: -s, YX: s, YY: c}
}

// AffineRotationAround returns a rotation by angle in radians
// around the point (cx, cy). It equals translate(cx, cy) after
// rotate(angle) after translate(-cx, -cy).
func AffineRotationAround(angle, cx, cy float32) AffineTransform {
	return AffineTranslation(cx, cy).Multiply(
		AffineRotation(angle).Multiply(AffineTranslation(-cx, -cy)))
}

// AffineTranslation returns a translation transform.
func AffineTranslation(dx, dy float32) AffineTransform {
	return AffineTransform{XX: 1, YY: 1, X0: dx, Y0: dy}
}

// AffineSkew returns a shear transform with direct skew factors.
func AffineSkew(skewX, skewY float32) AffineTransform {
	return AffineTransform{XX: 1, YY: 1, XY: skewX, YX: skewY}
}

// AffineScale returns a scale by (sx, sy) around the origin.
func AffineScale(sx, sy float32) AffineTransform {
	return AffineTransform{XX: sx, YY: sy}
}

// Multiply returns the composition a after b: b runs first,
// then a. Result maps point p as:
// Multiply(a, b).Apply(p) == a.Apply(b.Apply(p)).
// Order matters: Multiply(a, b) is not Multiply(b, a).
func (a AffineTransform) Multiply(b AffineTransform) AffineTransform {
	return AffineTransform{
		XX: a.XX*b.XX + a.XY*b.YX,
		XY: a.XX*b.XY + a.XY*b.YY,
		YX: a.YX*b.XX + a.YY*b.YX,
		YY: a.YX*b.XY + a.YY*b.YY,
		X0: a.XX*b.X0 + a.XY*b.Y0 + a.X0,
		Y0: a.YX*b.X0 + a.YY*b.Y0 + a.Y0,
	}
}

// Inverse returns the inverse transform. ok is false when the
// matrix has no inverse (zero or non-finite determinant).
func (a AffineTransform) Inverse() (inv AffineTransform, ok bool) {
	det := a.XX*a.YY - a.XY*a.YX
	if det == 0 || !finiteF32(det) {
		return AffineTransform{}, false
	}
	invDet := 1 / det
	if !finiteF32(invDet) {
		return AffineTransform{}, false
	}
	return AffineTransform{
		XX: a.YY * invDet,
		XY: -a.XY * invDet,
		YX: -a.YX * invDet,
		YY: a.XX * invDet,
		X0: (a.XY*a.Y0 - a.YY*a.X0) * invDet,
		Y0: (a.YX*a.X0 - a.XX*a.Y0) * invDet,
	}, true
}
