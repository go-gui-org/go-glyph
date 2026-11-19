//go:build windows

package glyph

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSegoeUITypoLineBox verifies Segoe UI, which lacks OS/2
// USE_TYPO_METRICS and has an hhea ascent inflated for stacked diacritics,
// gets its line box from the typo metrics on Windows (issue #195): a
// 1.15 em advance with half-leading, not the 1.33 em hhea box.
func TestSegoeUITypoLineBox(t *testing.T) {
	segoe := filepath.Join(os.Getenv("WINDIR"), "Fonts", "segoeui.ttf")
	if _, err := os.Stat(segoe); err != nil {
		t.Skip("segoeui.ttf not installed")
	}
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	const size = 16.0
	cfg := TextConfig{Style: TextStyle{FontName: "Segoe UI", Size: size}}
	layout, err := ctx.LayoutText("Ag\nAg\nAg", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 ||
		!strings.EqualFold(filepath.Base(layout.Items[0].FontPath), "segoeui.ttf") {
		t.Skip("Segoe UI did not resolve to segoeui.ttf")
	}

	m, err := ctx.FontMetrics(cfg)
	if err != nil {
		t.Fatalf("FontMetrics: %v", err)
	}
	// typo 1491/431/269 @ upem 2048: floor 1.15 em, extra split evenly.
	const upem = 2048.0
	wantLH := minLineHeightRatio * size
	half := (wantLH - (1491+431)*size/upem) / 2
	wantAsc := 1491*size/upem + half
	near := func(got float32, want float64) bool {
		return math.Abs(float64(got)-want) < 0.01
	}
	if !near(m.LineHeight, wantLH) || !near(m.Ascender, wantAsc) ||
		!near(m.Ascender+m.Descender, wantLH) || m.LineGap != 0 {
		t.Errorf("FontMetrics = %+v, want LineHeight %.3f Ascender %.3f "+
			"Height %.3f LineGap 0", m, wantLH, wantAsc, wantLH)
	}

	if len(layout.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(layout.Lines))
	}
	for i := 1; i < len(layout.Lines); i++ {
		dy := layout.Lines[i].Rect.Y - layout.Lines[i-1].Rect.Y
		if !near(dy, wantLH) {
			t.Errorf("line %d advance %.3f, want %.3f", i, dy, wantLH)
		}
	}
	if it := layout.Items[0]; !near(float32(it.Ascent), wantAsc) {
		t.Errorf("Item.Ascent %.3f, want %.3f", it.Ascent, wantAsc)
	}
}
