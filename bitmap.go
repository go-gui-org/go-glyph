//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

/*
#include <ft2build.h>
#include FT_FREETYPE_H
*/
import "C"
import (
	"fmt"
	"math"
	"unsafe"
)

// FTBitmapToBitmap converts a raw FreeType bitmap (GRAY, MONO, LCD,
// or BGRA) into a uniform 32-bit RGBA Bitmap.
//
// Supported pixel modes:
//   - GRAY: RGB=255, A=gamma[gray]. For tinting via vertex color.
//   - MONO: 1-bit expanded to 0 or 255 alpha.
//   - LCD: 3x-width subpixel bitmap flattened to RGBA.
//   - BGRA: Color emoji bitmaps, BGRA→RGBA swizzle.
func FTBitmapToBitmap(bmp *C.FT_Bitmap, ftFace C.FT_Face, targetHeight int) (Bitmap, error) {
	if bmp.buffer == nil || bmp.width == 0 || bmp.rows == 0 {
		return Bitmap{}, fmt.Errorf("empty bitmap")
	}

	width := int(bmp.width)
	height := int(bmp.rows)
	const channels = 4

	outLen := int64(width) * int64(height) * channels
	if outLen > int64(math.MaxInt32) || outLen <= 0 {
		return Bitmap{}, fmt.Errorf("bitmap size overflow: %dx%d", width, height)
	}

	// Allocation deferred for LCD mode (different output width).
	var data []byte
	if int(bmp.pixel_mode) != int(C.FT_PIXEL_MODE_LCD) {
		data = make([]byte, outLen)
	}

	pitchPositive := bmp.pitch >= 0
	absPitch := int(bmp.pitch)
	if !pitchPositive {
		absPitch = -absPitch
	}

	bufPtr := unsafe.Pointer(bmp.buffer)

	switch int(bmp.pixel_mode) {
	case int(C.FT_PIXEL_MODE_GRAY):
		for y := range height {
			srcY := y
			if !pitchPositive {
				srcY = height - 1 - y
			}
			row := unsafe.Add(bufPtr, srcY*absPitch)
			for x := range width {
				val := *(*byte)(unsafe.Add(row, x))
				i := (y*width + x) * 4
				data[i+0] = 255
				data[i+1] = 255
				data[i+2] = 255
				data[i+3] = gammaTable[val]
			}
		}

	case int(C.FT_PIXEL_MODE_MONO):
		for y := range height {
			srcY := y
			if !pitchPositive {
				srcY = height - 1 - y
			}
			row := unsafe.Add(bufPtr, srcY*absPitch)
			for x := range width {
				b := *(*byte)(unsafe.Add(row, x>>3))
				bit := 7 - (x & 7)
				var val byte
				if (b>>bit)&1 != 0 {
					val = 255
				}
				i := (y*width + x) * 4
				data[i+0] = 255
				data[i+1] = 255
				data[i+2] = 255
				data[i+3] = val
			}
		}

	case int(C.FT_PIXEL_MODE_LCD):
		if width < 3 {
			return Bitmap{}, fmt.Errorf("invalid LCD bitmap width: %d", width)
		}
		logicalWidth := width / 3
		newLen := logicalWidth * height * 4
		data = make([]byte, newLen)

		for y := range height {
			srcY := y
			if !pitchPositive {
				srcY = height - 1 - y
			}
			row := unsafe.Add(bufPtr, srcY*absPitch)
			for x := range logicalWidth {
				r := *(*byte)(unsafe.Add(row, x*3+0))
				g := *(*byte)(unsafe.Add(row, x*3+1))
				b := *(*byte)(unsafe.Add(row, x*3+2))
				avg := (int(r) + int(g) + int(b)) / 3

				i := (y*logicalWidth + x) * 4
				data[i+0] = r
				data[i+1] = g
				data[i+2] = b
				data[i+3] = byte(avg)
			}
		}
		width = logicalWidth

	case int(C.FT_PIXEL_MODE_BGRA):
		if width > MaxGlyphSize || height > MaxGlyphSize {
			return Bitmap{}, fmt.Errorf(
				"emoji bitmap exceeds max size %dx%d: %dx%d",
				MaxGlyphSize, MaxGlyphSize, width, height)
		}
		for y := range height {
			srcY := y
			if !pitchPositive {
				srcY = height - 1 - y
			}
			row := unsafe.Add(bufPtr, srcY*absPitch)
			for x := range width {
				px := unsafe.Add(row, x*4)
				i := (y*width + x) * 4
				data[i+0] = *(*byte)(unsafe.Add(px, 2)) // R
				data[i+1] = *(*byte)(unsafe.Add(px, 1)) // G
				data[i+2] = *(*byte)(unsafe.Add(px, 0)) // B
				data[i+3] = *(*byte)(unsafe.Add(px, 3)) // A
			}
		}

	default:
		return Bitmap{}, fmt.Errorf("unsupported FT pixel mode: %d", bmp.pixel_mode)
	}

	return Bitmap{
		Width:    width,
		Height:   height,
		Channels: channels,
		Data:     data,
	}, nil
}
