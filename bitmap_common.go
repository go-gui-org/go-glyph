package glyph

import (
	"fmt"
	"math"
)

// MaxGlyphSize caps individual glyph bitmaps to 256x256 pixels. This
// prevents a single oversized color emoji (some CBDT fonts ship 128px
// strikes) from consuming a disproportionate fraction of the atlas
// page. 256 supports up to ~256pt at 1x DPI; at 2x Retina the
// effective limit is ~128pt, which covers all practical UI sizes.
const MaxGlyphSize = 256

// maxAllocationSize is the 1GB allocation limit.
const maxAllocationSize = 1024 * 1024 * 1024

// Bitmap holds RGBA pixel data for a rasterized glyph. Channels must be
// 4; copyBitmapToPage rejects anything else.
type Bitmap struct {
	Data     []byte
	Width    int
	Height   int
	Channels int // Must be 4 (RGBA).
}

// checkAllocationSize validates width*height*channels does not overflow
// or exceed 1GB. Each dimension is validated before multiplying: the
// naive product wraps to a small positive on 64-bit ints for hostile
// inputs and would pass the limit checks below.
func checkAllocationSize(w, h, channels int) (int64, error) {
	if w <= 0 || h <= 0 || channels <= 0 {
		return 0, fmt.Errorf("invalid allocation size: %dx%dx%d",
			w, h, channels)
	}
	if int64(w) > math.MaxInt64/int64(h)/int64(channels) {
		return 0, fmt.Errorf("allocation size overflows int64: %dx%dx%d",
			w, h, channels)
	}
	size := int64(w) * int64(h) * int64(channels)
	if size > int64(math.MaxInt32) {
		return 0, fmt.Errorf("allocation too large: %d bytes exceeds %d-byte limit",
			size, int64(math.MaxInt32))
	}
	if size > maxAllocationSize {
		return 0, fmt.Errorf("allocation exceeds 1GB limit: %d bytes",
			size)
	}
	return size, nil
}

// validRenderSize reports whether a pixel font size is safe to shape and
// rasterize. It rejects zero, negative, NaN, and infinite sizes (which
// collapse or explode the device-space math) and caps absurd sizes before
// the int32( size*64 ) shaping scale or the raster bounds can overflow.
// Oversized-but-valid sizes still refuse later at maxNaturalGlyphDim.
func validRenderSize(size float64) bool {
	if math.IsNaN(size) || math.IsInf(size, 0) {
		return false
	}
	return size > 0 && size <= maxRenderSize
}

// maxRenderSize caps the pixel font size accepted for shaping. shaping
// scales positions by size*64 into int32; this cap keeps that product
// far from overflow while exceeding any practical UI size.
const maxRenderSize = 1 << 20

// maxNaturalGlyphDim bounds the transient full-size raster allocated on
// the oversize path before downscaling to MaxGlyphSize. Beyond it the
// glyph refuses (blank cell) instead of allocating tens of MB for one
// glyph.
const maxNaturalGlyphDim = 2048

// fitGlyphDims scales w0,h0 to fit within MaxGlyphSize, preserving
// aspect. It returns the fitted dimensions and the applied scale.
func fitGlyphDims(w0, h0 int) (dstW, dstH int, s float64) {
	m := max(w0, h0)
	if m <= MaxGlyphSize || m <= 0 {
		return max(w0, 1), max(h0, 1), 1
	}
	s = float64(MaxGlyphSize) / float64(m)
	return max(1, int(math.Round(float64(w0)*s))),
		max(1, int(math.Round(float64(h0)*s))), s
}

// scaleOffset scales a pixel bearing by the downscale factor, rounding
// to nearest instead of truncating so the shrunken bitmap stays
// centered on its pen origin.
func scaleOffset(v int, s float64) int {
	return int(math.Round(float64(v) * s))
}

// cubicHermite evaluates a Catmull-Rom spline at parameter t.
func cubicHermite(p0, p1, p2, p3, t float32) float32 {
	a := -0.5*p0 + 1.5*p1 - 1.5*p2 + 0.5*p3
	b := p0 - 2.5*p1 + 2.0*p2 - 0.5*p3
	c := -0.5*p0 + 0.5*p2
	d := p1
	return a*t*t*t + b*t*t + c*t + d
}

// getPixelRGBAPremul fetches an RGBA pixel with premultiplied alpha.
func getPixelRGBAPremul(src []byte, w, h, x, y int) (r, g, b, a float32) {
	if w <= 0 || h <= 0 {
		return
	}
	cx := max(0, min(x, w-1))
	cy := max(0, min(y, h-1))
	idx := (cy*w + cx) * 4
	if idx < 0 || idx+3 >= len(src) {
		return
	}
	rr := float32(src[idx+0])
	gg := float32(src[idx+1])
	bb := float32(src[idx+2])
	aa := float32(src[idx+3])
	f := aa / 255.0
	return rr * f, gg * f, bb * f, aa
}

// ScaleBitmapBicubic scales an RGBA bitmap using bicubic
// (Catmull-Rom) interpolation with premultiplied alpha. Samples are
// center-aligned ((x+0.5)*scale-0.5), so integer factors land on texel
// centers, and outputs round to nearest instead of truncating.
// Returns nil when dimensions are invalid, either allocation would
// exceed the 1GB limit, or src is shorter than its dimensions claim
// (which would otherwise silently interpolate zeros at the edges).
func ScaleBitmapBicubic(src []byte, srcW, srcH, dstW, dstH int) []byte {
	if dstW <= 0 || dstH <= 0 || srcW <= 0 || srcH <= 0 {
		return nil
	}
	dstSize, err := checkAllocationSize(dstW, dstH, 4)
	if err != nil {
		return nil
	}
	// Overflow-safe source size first: the product wraps on hostile
	// dims and would pass the length check below.
	if int64(srcW) > math.MaxInt64/int64(srcH)/4 {
		return nil
	}
	if int64(len(src)) < int64(srcW)*int64(srcH)*4 {
		return nil
	}

	dst := make([]byte, dstSize)
	xScale := float64(srcW) / float64(dstW)
	yScale := float64(srcH) / float64(dstH)

	for y := range dstH {
		srcY := (float64(y)+0.5)*yScale - 0.5
		y0 := int(math.Floor(srcY))
		yDiff := float32(srcY - float64(y0))

		for x := range dstW {
			srcX := (float64(x)+0.5)*xScale - 0.5
			x0 := int(math.Floor(srcX))
			xDiff := float32(srcX - float64(x0))

			dstIdx := (y*dstW + x) * 4

			var colR, colG, colB, colA [4]float32

			for i := -1; i <= 2; i++ {
				rowY := y0 + i
				r0, g0, b0, a0 := getPixelRGBAPremul(src, srcW, srcH, x0-1, rowY)
				r1, g1, b1, a1 := getPixelRGBAPremul(src, srcW, srcH, x0+0, rowY)
				r2, g2, b2, a2 := getPixelRGBAPremul(src, srcW, srcH, x0+1, rowY)
				r3, g3, b3, a3 := getPixelRGBAPremul(src, srcW, srcH, x0+2, rowY)

				j := i + 1
				colR[j] = cubicHermite(r0, r1, r2, r3, xDiff)
				colG[j] = cubicHermite(g0, g1, g2, g3, xDiff)
				colB[j] = cubicHermite(b0, b1, b2, b3, xDiff)
				colA[j] = cubicHermite(a0, a1, a2, a3, xDiff)
			}

			finalR := cubicHermite(colR[0], colR[1], colR[2], colR[3], yDiff)
			finalG := cubicHermite(colG[0], colG[1], colG[2], colG[3], yDiff)
			finalB := cubicHermite(colB[0], colB[1], colB[2], colB[3], yDiff)
			finalA := cubicHermite(colA[0], colA[1], colA[2], colA[3], yDiff)

			finalA = max(0, min(finalA, 255))

			if finalA > 0 {
				f := 255.0 / finalA
				finalR *= f
				finalG *= f
				finalB *= f
			}

			dst[dstIdx+0] = byte(max(0, min(float32(math.Round(float64(finalR))), 255)))
			dst[dstIdx+1] = byte(max(0, min(float32(math.Round(float64(finalG))), 255)))
			dst[dstIdx+2] = byte(max(0, min(float32(math.Round(float64(finalB))), 255)))
			dst[dstIdx+3] = byte(max(0, min(float32(math.Round(float64(finalA))), 255)))
		}
	}
	return dst
}
