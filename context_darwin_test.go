//go:build darwin && !glyph_pango

package glyph

import "testing"

// TestFontMetricsDarwin mirrors the Pango-build TestFontMetrics under
// the default CoreText build. The Pango variant in context_test.go is
// gated by !darwin || glyph_pango, so the metrics path here is
// otherwise untested in CI.
func TestFontMetricsDarwin(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	m, err := ctx.FontMetrics(cfg)
	if err != nil {
		t.Fatalf("FontMetrics: %v", err)
	}
	if m.Ascender <= 0 || m.Descender <= 0 || m.Height <= 0 {
		t.Errorf("expected positive metrics, got %+v", m)
	}
}

// TestFontMetricsCacheHit verifies that repeated FontMetrics calls
// with identical style params return identical values (correctness
// invariant for the params-keyed metrics cache).
func TestFontMetricsCacheHit(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	m1, err := ctx.FontMetrics(cfg)
	if err != nil {
		t.Fatalf("FontMetrics first: %v", err)
	}
	m2, err := ctx.FontMetrics(cfg)
	if err != nil {
		t.Fatalf("FontMetrics second: %v", err)
	}
	if m1 != m2 {
		t.Errorf("cache mismatch: %+v vs %+v", m1, m2)
	}
}

func TestFontHeightDarwin_PositiveResult(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	h, err := ctx.FontHeight(cfg)
	if err != nil {
		t.Fatalf("FontHeight: %v", err)
	}
	if h <= 0 {
		t.Errorf("FontHeight = %v, want > 0", h)
	}
}

func TestFontMetrics_DifferentStylesIndependent(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	small := TextConfig{Style: TextStyle{FontName: "Sans 12"}}
	large := TextConfig{Style: TextStyle{FontName: "Sans 24"}}
	ms, err := ctx.FontMetrics(small)
	if err != nil {
		t.Fatalf("FontMetrics small: %v", err)
	}
	ml, err := ctx.FontMetrics(large)
	if err != nil {
		t.Fatalf("FontMetrics large: %v", err)
	}
	if ms == ml {
		t.Error("different sizes returned identical metrics — possible cache key collision")
	}
	if ml.Height <= ms.Height {
		t.Errorf("larger font should have taller height: size12=%v size24=%v", ms.Height, ml.Height)
	}
}

func BenchmarkFontMetrics(b *testing.B) {
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Fatal(err)
	}
	defer ctx.Free()
	cfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	if _, err := ctx.FontMetrics(cfg); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ctx.FontMetrics(cfg)
	}
}
