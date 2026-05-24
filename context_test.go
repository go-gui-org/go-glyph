//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

import (
	"runtime"
	"strings"
	"testing"
)

func TestContextCreation(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	ctx.Free()
}

func TestFontHeightSanity(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	}
	h, err := ctx.FontHeight(cfg)
	if err != nil {
		t.Fatalf("FontHeight: %v", err)
	}
	if h < 15.0 || h > 40.0 {
		t.Errorf("Sans 20 height=%f, want 15-40", h)
	}
}

func TestFontHeightPixels(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20px"},
	}
	h, err := ctx.FontHeight(cfg)
	if err != nil {
		t.Fatalf("FontHeight: %v", err)
	}
	if h < 18.0 || h > 30.0 {
		t.Errorf("Sans 20px height=%f, want 18-30", h)
	}
}

func TestFontMetrics(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	}
	m, err := ctx.FontMetrics(cfg)
	if err != nil {
		t.Fatalf("FontMetrics: %v", err)
	}
	if m.Ascender <= 0 {
		t.Errorf("ascender=%f, want > 0", m.Ascender)
	}
	if m.Descender <= 0 {
		t.Errorf("descender=%f, want > 0", m.Descender)
	}
	if m.Height <= 0 {
		t.Errorf("height=%f, want > 0", m.Height)
	}
}

func TestFontHeightCaching(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	defer ctx.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	}
	h1, err := ctx.FontHeight(cfg)
	if err != nil {
		t.Fatalf("FontHeight first call: %v", err)
	}
	h2, err := ctx.FontHeight(cfg)
	if err != nil {
		t.Fatalf("FontHeight second call: %v", err)
	}
	if h1 != h2 {
		t.Errorf("cached mismatch: %f != %f", h1, h2)
	}
}

func TestResolveFontName(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	defer ctx.Free()

	name, err := ctx.ResolveFontName("Sans 12")
	if err != nil {
		t.Fatalf("ResolveFontName: %v", err)
	}
	if name == "" {
		t.Error("resolved name is empty")
	}
}

func TestResolveFontName_MonospaceDescriptor(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Skip("Pango/FreeType not available")
	}
	defer ctx.Free()

	name, err := ctx.ResolveFontName("Monospace 12")
	if err != nil {
		t.Fatalf("ResolveFontName monospace: %v", err)
	}
	if name == "" {
		t.Error("resolved monospace name is empty")
	}
}

func TestIsMonospaceName_KnownKeywords(t *testing.T) {
	cases := []struct {
		family string
		want   bool
	}{
		{"JetBrains Mono", true},
		{"Courier New", true},
		{"Consolas", true},
		{"VS Code", true},
		{"Terminal", true},
		{"Typewriter Pro", true},
		{"Fixed Sys", true},
		{"Menlo", true},
		{"Monaco", true},
		{"Arial", false},
		{"Helvetica", false},
		{"Times New Roman", false},
		{"Georgia", false},
	}
	for _, c := range cases {
		if got := isMonospaceName(c.family); got != c.want {
			t.Errorf("isMonospaceName(%q) = %v, want %v", c.family, got, c.want)
		}
	}
}

func TestIsMonospaceName_CaseInsensitive(t *testing.T) {
	cases := []string{"JETBRAINS MONO", "jetbrains mono", "JetBrains Mono", "MENLO", "menlo"}
	for _, fam := range cases {
		if !isMonospaceName(fam) {
			t.Errorf("isMonospaceName(%q) = false, want true", fam)
		}
	}
}

func TestIsMonospaceName_Empty(t *testing.T) {
	if isMonospaceName("") {
		t.Error("isMonospaceName(\"\") = true, want false")
	}
}

func TestResolveFamilyAlias_MonospaceAppendsFallback(t *testing.T) {
	result := resolveFamilyAlias("MyFont", true)
	if !strings.HasPrefix(result, "MyFont") {
		t.Errorf("result %q does not start with primary family", result)
	}
	if !strings.Contains(result, ",") {
		t.Errorf("result %q has no fallback appended", result)
	}
	// Monospace fallbacks must not include proportional families.
	proportional := map[string]bool{"SF Pro": true, "System Font": true, "Segoe UI": true, "Sans": true}
	for fam := range proportional {
		if strings.Contains(result, fam) {
			t.Errorf("monospace result %q contains proportional fallback %q", result, fam)
		}
	}
}

func TestResolveFamilyAlias_ProportionalAppendsFallback(t *testing.T) {
	result := resolveFamilyAlias("MyFont", false)
	if !strings.HasPrefix(result, "MyFont") {
		t.Errorf("result %q does not start with primary family", result)
	}
	if !strings.Contains(result, ",") {
		t.Errorf("result %q has no fallback appended", result)
	}
	// Proportional fallbacks must not include monospace families.
	monospace := map[string]bool{"Menlo": true, "Consolas": true, "monospace": true}
	for fam := range monospace {
		if strings.Contains(result, fam) {
			t.Errorf("proportional result %q contains monospace fallback %q", result, fam)
		}
	}
}

func TestResolveFamilyAlias_NoDuplicateWhenFamilyMatchesFallback(t *testing.T) {
	// Pick the first fallback for the current platform so we can test
	// that it is not appended a second time when fam already names it.
	var primary string
	switch runtime.GOOS {
	case "darwin":
		primary = "Menlo"
	default:
		primary = "monospace"
	}
	result := resolveFamilyAlias(primary, true)
	parts := strings.Split(result, ", ")
	seen := map[string]int{}
	for _, p := range parts {
		seen[strings.TrimSpace(p)]++
	}
	if seen[primary] > 1 {
		t.Errorf("resolveFamilyAlias(%q, true) = %q: %q appears %d times, want 1",
			primary, result, primary, seen[primary])
	}
}
