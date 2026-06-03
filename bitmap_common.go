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

// Bitmap holds RGBA pixel data for a rasterized glyph.
type Bitmap struct {
	Data     []byte
	Width    int
	Height   int
	Channels int // Always 4 (RGBA).
}

// checkAllocationSize validates width*height*channels doesn't overflow
// or exceed 1GB.
func checkAllocationSize(w, h, channels int) (int64, error) {
	size := int64(w) * int64(h) * int64(channels)
	if size <= 0 {
		return 0, fmt.Errorf("invalid allocation size: %dx%dx%d",
			w, h, channels)
	}
	if size > int64(math.MaxInt32) {
		return 0, fmt.Errorf("allocation overflow: %d bytes", size)
	}
	if size > maxAllocationSize {
		return 0, fmt.Errorf("allocation exceeds 1GB limit: %d bytes",
			size)
	}
	return size, nil
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
// (Catmull-Rom) interpolation with premultiplied alpha.
func ScaleBitmapBicubic(src []byte, srcW, srcH, dstW, dstH int) []byte {
	if dstW <= 0 || dstH <= 0 || srcW <= 0 || srcH <= 0 {
		return nil
	}
	dstSize := int64(dstW) * int64(dstH) * 4
	if dstSize > int64(math.MaxInt32) || dstSize <= 0 {
		return nil
	}

	dst := make([]byte, dstSize)
	xScale := float32(srcW) / float32(dstW)
	yScale := float32(srcH) / float32(dstH)

	for y := range dstH {
		srcY := float32(y) * yScale
		y0 := int(srcY)
		yDiff := srcY - float32(y0)

		for x := range dstW {
			srcX := float32(x) * xScale
			x0 := int(srcX)
			xDiff := srcX - float32(x0)

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

			dst[dstIdx+0] = byte(max(0, min(finalR, 255)))
			dst[dstIdx+1] = byte(max(0, min(finalG, 255)))
			dst[dstIdx+2] = byte(max(0, min(finalB, 255)))
			dst[dstIdx+3] = byte(finalA)
		}
	}
	return dst
}
