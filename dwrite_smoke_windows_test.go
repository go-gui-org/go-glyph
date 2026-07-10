//go:build windows

package glyph

import (
	"errors"
	"math"
	"testing"
	"unsafe"
)

// TestColorGlyphRunLayout pins the binary layout of
// _DWRITE_COLOR_GLYPH_RUN1 to the COM ABI it aliases. Unlike the smoke
// test, it needs no font and no DirectWrite runtime, so it guards the
// load-bearing field offsets on every Windows build — including CI
// hosts without Segoe UI Emoji, where the smoke test skips. A wrong
// offset here silently mis-decodes color runs (the CGo->syscall port
// regression: glyphImageFormat read 4 bytes early, discarding layers).
func TestColorGlyphRunLayout(t *testing.T) {
	if got := unsafe.Sizeof(_DWRITE_GLYPH_RUN{}); got != 48 {
		t.Errorf("sizeof(_DWRITE_GLYPH_RUN) = %d, want 48", got)
	}
	var r _DWRITE_COLOR_GLYPH_RUN1
	checks := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"GlyphRunDescription", unsafe.Offsetof(r.GlyphRunDescription), 48},
		{"BaselineOriginX", unsafe.Offsetof(r.BaselineOriginX), 56},
		{"BaselineOriginY", unsafe.Offsetof(r.BaselineOriginY), 60},
		{"RunColor", unsafe.Offsetof(r.RunColor), 64},
		{"PaletteIndex", unsafe.Offsetof(r.PaletteIndex), 80},
		{"GlyphImageFormat", unsafe.Offsetof(r.GlyphImageFormat), 88},
		{"MeasuringMode", unsafe.Offsetof(r.MeasuringMode), 92},
		{"sizeof", unsafe.Sizeof(r), 96},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s offset/size = %d, want %d", c.name, c.got, c.want)
		}
	}
}

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
		var alphaSum, chromatic int
		for i := 0; i+3 < len(bmp.Data); i += 4 {
			alphaSum += int(bmp.Data[i+3])
			// A colored glyph must contain at least one chromatic
			// pixel (channels not all equal). If the color path
			// silently falls back to a monochrome foreground fill,
			// every opaque pixel is gray/white and this stays zero —
			// the exact regression from the CGo->syscall port.
			r8, g8, b8 := bmp.Data[i], bmp.Data[i+1], bmp.Data[i+2]
			hi, lo := max(r8, max(g8, b8)), min(r8, min(g8, b8))
			if bmp.Data[i+3] > 0 && hi-lo > 24 {
				chromatic++
			}
		}
		if alphaSum == 0 {
			t.Errorf("U+%04X: bitmap has zero total alpha", r)
		}
		if chromatic == 0 {
			t.Errorf("U+%04X: no chromatic pixels — color path likely "+
				"fell back to a monochrome outline", r)
		}
		t.Logf("U+%04X: %dx%d left=%d top=%d alphaSum=%d chromatic=%d",
			r, bmp.Width, bmp.Height, left, top, alphaSum, chromatic)
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
