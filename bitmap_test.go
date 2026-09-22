package glyph

import (
	"math"
	"testing"
)

func TestCheckAllocationSize(t *testing.T) {
	tests := []struct {
		w, h, ch int
		wantErr  bool
	}{
		{100, 100, 4, false},
		{0, 100, 4, true},
		{100, 0, 4, true},
		{-1, 100, 4, true},
		{100, -1, 4, true},
		{50000, 50000, 4, true}, // exceeds 1GB
	}
	for _, tt := range tests {
		_, err := checkAllocationSize(tt.w, tt.h, tt.ch)
		if (err != nil) != tt.wantErr {
			t.Errorf("checkAllocationSize(%d,%d,%d): err=%v, wantErr=%v",
				tt.w, tt.h, tt.ch, err, tt.wantErr)
		}
	}
}

func TestCheckAllocationSizeValid(t *testing.T) {
	size, err := checkAllocationSize(1024, 1024, 4)
	if err != nil {
		t.Fatal(err)
	}
	if size != 1024*1024*4 {
		t.Errorf("size = %d, want %d", size, 1024*1024*4)
	}
}

func TestScaleBitmapBicubicIdentity(t *testing.T) {
	// 2x2 RGBA bitmap.
	src := []byte{
		255, 0, 0, 255, 0, 255, 0, 255,
		0, 0, 255, 255, 255, 255, 0, 255,
	}

	// Scale to same size.
	dst := ScaleBitmapBicubic(src, 2, 2, 2, 2)
	if dst == nil {
		t.Fatal("ScaleBitmapBicubic returned nil")
	}
	if len(dst) != 2*2*4 {
		t.Fatalf("dst len = %d, want %d", len(dst), 2*2*4)
	}

	// Corners should roughly match originals.
	// Allow tolerance due to Catmull-Rom boundary effects.
	tolerance := byte(50)
	if absDiffByte(dst[0], 255) > tolerance {
		t.Errorf("pixel(0,0) R=%d, want ~255", dst[0])
	}
	if absDiffByte(dst[3], 255) > tolerance {
		t.Errorf("pixel(0,0) A=%d, want ~255", dst[3])
	}
}

func TestScaleBitmapBicubicUpscale(t *testing.T) {
	// 1x1 solid white.
	src := []byte{255, 255, 255, 255}
	dst := ScaleBitmapBicubic(src, 1, 1, 4, 4)
	if dst == nil {
		t.Fatal("nil result")
	}
	if len(dst) != 4*4*4 {
		t.Fatalf("len = %d, want %d", len(dst), 4*4*4)
	}

	// All pixels should be solid white.
	for i := 0; i < len(dst); i += 4 {
		if dst[i] != 255 || dst[i+1] != 255 || dst[i+2] != 255 || dst[i+3] != 255 {
			t.Errorf("pixel at offset %d: RGBA=(%d,%d,%d,%d), want solid white",
				i/4, dst[i], dst[i+1], dst[i+2], dst[i+3])
			break
		}
	}
}

func TestScaleBitmapBicubicDownscale(t *testing.T) {
	// 4x4 solid red.
	src := make([]byte, 4*4*4)
	for i := 0; i < len(src); i += 4 {
		src[i] = 200
		src[i+1] = 50
		src[i+2] = 50
		src[i+3] = 255
	}

	dst := ScaleBitmapBicubic(src, 4, 4, 2, 2)
	if dst == nil {
		t.Fatal("nil result")
	}

	// Downscaled solid color should remain approximately same.
	tolerance := byte(30)
	for i := 0; i < len(dst); i += 4 {
		if absDiffByte(dst[i], 200) > tolerance ||
			absDiffByte(dst[i+1], 50) > tolerance ||
			absDiffByte(dst[i+3], 255) > tolerance {
			t.Errorf("pixel at %d: RGBA=(%d,%d,%d,%d), want ~(200,50,50,255)",
				i/4, dst[i], dst[i+1], dst[i+2], dst[i+3])
			break
		}
	}
}

func TestScaleBitmapBicubicZero(t *testing.T) {
	if ScaleBitmapBicubic(nil, 0, 0, 0, 0) != nil {
		t.Error("expected nil for zero dimensions")
	}
	if ScaleBitmapBicubic([]byte{1, 2, 3, 4}, 1, 1, 0, 0) != nil {
		t.Error("expected nil for zero dst dimensions")
	}
}

func TestCubicHermite(t *testing.T) {
	// At t=0, result should be p1.
	v := cubicHermite(0, 100, 200, 300, 0)
	if math.Abs(float64(v-100)) > 0.001 {
		t.Errorf("cubicHermite(t=0) = %f, want 100", v)
	}
	// At t=1, result should be p2.
	v = cubicHermite(0, 100, 200, 300, 1)
	if math.Abs(float64(v-200)) > 0.001 {
		t.Errorf("cubicHermite(t=1) = %f, want 200", v)
	}
}

func TestGetPixelRGBAPremul(t *testing.T) {
	// 1x1 pixel: R=200, G=100, B=50, A=128
	src := []byte{200, 100, 50, 128}
	r, g, b, a := getPixelRGBAPremul(src, 1, 1, 0, 0)

	// Premultiply: f = 128/255 ≈ 0.502
	f := float32(128) / 255.0
	expectR := float32(200) * f
	expectG := float32(100) * f
	expectB := float32(50) * f

	tolerance := float32(0.5)
	if absDiffF32(r, expectR) > tolerance {
		t.Errorf("R = %f, want %f", r, expectR)
	}
	if absDiffF32(g, expectG) > tolerance {
		t.Errorf("G = %f, want %f", g, expectG)
	}
	if absDiffF32(b, expectB) > tolerance {
		t.Errorf("B = %f, want %f", b, expectB)
	}
	if absDiffF32(a, 128) > tolerance {
		t.Errorf("A = %f, want 128", a)
	}
}

func TestGetPixelRGBAPremulClamped(t *testing.T) {
	src := []byte{255, 255, 255, 255}
	// Out of bounds should clamp to edge.
	r, _, _, a := getPixelRGBAPremul(src, 1, 1, 5, 5)
	if r != 255 || a != 255 {
		t.Errorf("clamped pixel: R=%f A=%f, want 255", r, a)
	}
}

func TestGetPixelRGBAPremulEmptyImage(t *testing.T) {
	r, g, b, a := getPixelRGBAPremul(nil, 0, 0, 0, 0)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Error("expected zero for empty image")
	}
}

func TestCheckAllocationSizeOverflow(t *testing.T) {
	// MaxInt32-scale dimensions should trigger overflow.
	_, err := checkAllocationSize(math.MaxInt32, 2, 4)
	if err == nil {
		t.Error("expected overflow error for MaxInt32 * 2 * 4")
	}
}

func TestCheckAllocationSizeExactLimit(t *testing.T) {
	// Exactly 1GB should pass (1024*1024*256*4 = 1GB).
	size, err := checkAllocationSize(1024, 1024*256, 4)
	if err != nil {
		t.Fatalf("expected 1GB allocation to succeed: %v", err)
	}
	if size != 1024*1024*256*4 {
		t.Errorf("size = %d, want %d", size, 1024*1024*256*4)
	}
}

func TestScaleBitmapBicubicTransparent(t *testing.T) {
	// All-zero alpha source.
	src := make([]byte, 4*4*4)
	dst := ScaleBitmapBicubic(src, 4, 4, 2, 2)
	if dst == nil {
		t.Fatal("nil result for transparent source")
	}
	for i := 0; i < len(dst); i += 4 {
		if dst[i+3] != 0 {
			t.Errorf("pixel %d: A=%d, want 0", i/4, dst[i+3])
			break
		}
	}
}

func TestScaleBitmapBicubicOverflowDimensions(t *testing.T) {
	src := []byte{255, 255, 255, 255}
	// Request dimensions that overflow int32.
	dst := ScaleBitmapBicubic(src, 1, 1, math.MaxInt32, 2)
	if dst != nil {
		t.Error("expected nil for overflow dimensions")
	}
}

func TestScaleBitmapBicubicShortSrc(t *testing.T) {
	// 2x2 claimed but only one pixel present: must refuse rather than
	// interpolate zeros at the clamped edges.
	if got := ScaleBitmapBicubic([]byte{1, 2, 3, 4}, 2, 2, 2, 2); got != nil {
		t.Errorf("expected nil for short src, got %d bytes", len(got))
	}
	if got := ScaleBitmapBicubic(nil, 2, 2, 2, 2); got != nil {
		t.Error("expected nil for nil src with nonzero dimensions")
	}
}

func TestScaleBitmapBicubicOver1GB(t *testing.T) {
	src := []byte{255, 255, 255, 255}
	// 20000x20000x4 = 1.6GB: under MaxInt32 but over the 1GB cap.
	if got := ScaleBitmapBicubic(src, 1, 1, 20000, 20000); got != nil {
		t.Error("expected nil for dst exceeding the 1GB limit")
	}
}

func TestCheckAllocationSizeInt64Overflow(t *testing.T) {
	// The naive int64 product wraps to a small positive here; the
	// validator must still reject it.
	if _, err := checkAllocationSize(1<<32, 1<<32, 4); err == nil {
		t.Error("expected error for wrapping dimensions")
	}
	if _, err := checkAllocationSize(1, 1, 0); err == nil {
		t.Error("expected error for zero channels")
	}
}

func TestFitGlyphDims(t *testing.T) {
	w, h, s := fitGlyphDims(512, 100)
	if w != 256 || h != 50 || s != 0.5 {
		t.Errorf("fit(512,100) = %dx%d s=%v, want 256x50 s=0.5", w, h, s)
	}
	w, h, s = fitGlyphDims(100, 100)
	if w != 100 || h != 100 || s != 1 {
		t.Errorf("fit(100,100) = %dx%d s=%v, want unchanged", w, h, s)
	}
	if got := scaleOffset(-30, 0.5); got != -15 {
		t.Errorf("scaleOffset(-30, 0.5) = %d, want -15", got)
	}
}

func TestValidRenderSize(t *testing.T) {
	for _, s := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1 << 21} {
		if validRenderSize(s) {
			t.Errorf("validRenderSize(%v) = true, want false", s)
		}
	}
	if !validRenderSize(16) {
		t.Error("validRenderSize(16) = false, want true")
	}
}

func TestScaleBitmapBicubicMidpoint(t *testing.T) {
	// 2x1 black-to-white gradient downscaled to 1x1 must sample the
	// texel center (mid gray), proving center-aligned sampling.
	src := []byte{0, 0, 0, 255, 255, 255, 255, 255}
	dst := ScaleBitmapBicubic(src, 2, 1, 1, 1)
	if dst == nil {
		t.Fatal("nil result")
	}
	if absDiffByte(dst[0], 128) > 2 || dst[3] != 255 {
		t.Errorf("midpoint = (%d,%d,%d,%d), want ~(128,128,128,255)",
			dst[0], dst[1], dst[2], dst[3])
	}
}

func TestCopyBitmapToPageRejectsChannels(t *testing.T) {
	b := newMockBackend()
	atlas, err := NewGlyphAtlas(b, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	bmp := Bitmap{Width: 2, Height: 2, Channels: 3, Data: make([]byte, 2*2*4)}
	if err := copyBitmapToPage(&atlas.Pages[0], bmp, 0, 0); err == nil {
		t.Error("expected error for Channels != 4")
	}
}

func TestEnsureGuardsReturnNil(t *testing.T) {
	b := newMockBackend()
	atlas, err := NewGlyphAtlas(b, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	if atlas.ensureAlpha(0, 4) != nil {
		t.Error("ensureAlpha(0,4) should be nil")
	}
	if atlas.ensureRGBA(-1, 4) != nil {
		t.Error("ensureRGBA(-1,4) should be nil")
	}
	if atlas.ensureRasterizer(0, 0) != nil {
		t.Error("ensureRasterizer(0,0) should be nil")
	}
	if atlas.ensureAlpha(1<<32, 1<<32) != nil {
		t.Error("ensureAlpha(huge) should be nil")
	}
}

func BenchmarkScaleBitmapBicubic(b *testing.B) {
	src := make([]byte, 32*32*4)
	for i := range src {
		src[i] = byte(i % 256)
	}
	b.ResetTimer()
	for b.Loop() {
		ScaleBitmapBicubic(src, 32, 32, 64, 64)
	}
}

func BenchmarkCubicHermite(b *testing.B) {
	for b.Loop() {
		cubicHermite(10, 20, 30, 40, 0.5)
	}
}

func absDiffByte(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}

func absDiffF32(a, b float32) float32 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}
