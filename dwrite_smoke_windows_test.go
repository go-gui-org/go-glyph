//go:build windows

package glyph

import (
	"errors"
	"math"
	"testing"
)

// TestDWriteRasterizerSmoke verifies end-to-end that the DirectWrite
// color glyph path initializes, renders common emoji with non-empty
// bitmaps and sane bearings, and returns errNoColorGlyph for plain
// ASCII. Skips if DWrite init fails (Segoe UI Emoji not installed).
func TestDWriteRasterizerSmoke(t *testing.T) {
	dw, err := newDWriteRasterizer()
	if err != nil {
		t.Skipf("DirectWrite unavailable: %v", err)
	}
	defer dw.Close()

	cases := []rune{
		0x1F600, // 😀 grinning face
		0x1F389, // 🎉 party popper
		0x1F680, // 🚀 rocket
		0x2764,  // ❤ heavy heart
	}
	for _, r := range cases {
		bmp, left, top, err := dw.RenderColorGlyph(32.0, r)
		if err != nil {
			t.Errorf("U+%04X: unexpected error: %v", r, err)
			continue
		}
		if bmp.Width <= 0 || bmp.Height <= 0 {
			t.Errorf("U+%04X: empty bitmap w=%d h=%d",
				r, bmp.Width, bmp.Height)
			continue
		}
		if len(bmp.Data) != bmp.Width*bmp.Height*4 {
			t.Errorf("U+%04X: wrong data length %d for %dx%d",
				r, len(bmp.Data), bmp.Width, bmp.Height)
			continue
		}
		var alphaSum int
		for i := 3; i < len(bmp.Data); i += 4 {
			alphaSum += int(bmp.Data[i])
		}
		if alphaSum == 0 {
			t.Errorf("U+%04X: bitmap has zero total alpha", r)
		}
		t.Logf("U+%04X: %dx%d left=%d top=%d alphaSum=%d",
			r, bmp.Width, bmp.Height, left, top, alphaSum)
	}

	_, _, _, err = dw.RenderColorGlyph(32.0, 'A')
	if !errors.Is(err, errNoColorGlyph) {
		t.Errorf("'A': expected errNoColorGlyph, got %v", err)
	}
}

// TestDWriteRasterizer_CloseNilSafe verifies Close does not panic on
// a nil receiver, and that double-close is safe.
func TestDWriteRasterizer_CloseNilSafe(t *testing.T) {
	var dw *dwriteRasterizer
	dw.Close() // nil receiver

	dw2, err := newDWriteRasterizer()
	if err != nil {
		t.Skipf("DirectWrite unavailable: %v", err)
	}
	dw2.Close()
	dw2.Close() // double-close
}

// TestDWriteRasterizer_RenderColorGlyph_InvalidSize verifies that
// zero, negative, and NaN emSizePx values return errNoColorGlyph
// without crashing.
func TestDWriteRasterizer_RenderColorGlyph_InvalidSize(t *testing.T) {
	dw, err := newDWriteRasterizer()
	if err != nil {
		t.Skipf("DirectWrite unavailable: %v", err)
	}
	defer dw.Close()

	for _, sz := range []float32{0, -1, float32(math.NaN())} {
		_, _, _, err := dw.RenderColorGlyph(sz, 0x1F600)
		if !errors.Is(err, errNoColorGlyph) {
			t.Errorf("emSizePx=%v: want errNoColorGlyph, got %v", sz, err)
		}
	}
}

// TestDWriteRasterizer_RenderColorGlyph_NilReceiver verifies
// RenderColorGlyph on a nil receiver returns an error without
// crashing.
func TestDWriteRasterizer_RenderColorGlyph_NilReceiver(t *testing.T) {
	var dw *dwriteRasterizer
	_, _, _, err := dw.RenderColorGlyph(32.0, 0x1F600)
	if err == nil {
		t.Error("expected error on nil receiver")
	}
}
