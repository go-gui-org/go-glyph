package glyph

import "math"

// AffineTransform encodes a 2D affine transform matrix:
//
//	[ XX  XY  X0 ]
//	[ YX  YY  Y0 ]
//	[  0   0   1 ]
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

// AffineIdentity returns an identity transform.
func AffineIdentity() AffineTransform {
	return AffineTransform{XX: 1, YY: 1}
}

// AffineRotation returns a rotation transform in radians around origin.
func AffineRotation(angle float32) AffineTransform {
	c := float32(math.Cos(float64(angle)))
	s := float32(math.Sin(float64(angle)))
	return AffineTransform{XX: c, XY: -s, YX: s, YY: c}
}

// AffineTranslation returns a translation transform.
func AffineTranslation(dx, dy float32) AffineTransform {
	return AffineTransform{XX: 1, YY: 1, X0: dx, Y0: dy}
}

// AffineSkew returns a shear transform with direct skew factors.
func AffineSkew(skewX, skewY float32) AffineTransform {
	return AffineTransform{XX: 1, YY: 1, XY: skewX, YX: skewY}
}

// Multiply returns the composition of two transforms: a then b.
// Result maps point p as: Multiply(a, b).Apply(p) == a.Apply(b.Apply(p)).
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
