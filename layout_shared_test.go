package glyph

import "testing"

// ---------------------------------------------------------------------------
// mergeStyles
// ---------------------------------------------------------------------------

func TestMergeStyles_EmptyRunInheritsBase(t *testing.T) {
	base := TextStyle{FontName: "Sans 16", Size: 14, Color: Color{R: 255, A: 255}}
	got := mergeStyles(base, TextStyle{})
	if got.FontName != "Sans 16" {
		t.Errorf("FontName=%q, want %q", got.FontName, "Sans 16")
	}
	if got.Size != 14 {
		t.Errorf("Size=%v, want 14", got.Size)
	}
	if got.Color.R != 255 {
		t.Errorf("Color.R=%d, want 255", got.Color.R)
	}
}

func TestMergeStyles_RunColorOverridesBase(t *testing.T) {
	base := TextStyle{Color: Color{R: 255, A: 255}}
	run := TextStyle{Color: Color{G: 128, A: 255}}
	got := mergeStyles(base, run)
	if got.Color.R != 0 || got.Color.G != 128 || got.Color.A != 255 {
		t.Errorf("Color=%v, want {G:128 A:255}", got.Color)
	}
}

func TestMergeStyles_RunSizeZeroFallsBackToBase(t *testing.T) {
	base := TextStyle{Size: 20}
	got := mergeStyles(base, TextStyle{Size: 0})
	if got.Size != 20 {
		t.Errorf("Size=%v, want 20", got.Size)
	}
}

func TestMergeStyles_RunFontNameOverridesBase(t *testing.T) {
	base := TextStyle{FontName: "Sans 16"}
	run := TextStyle{FontName: "Serif 12"}
	got := mergeStyles(base, run)
	if got.FontName != "Serif 12" {
		t.Errorf("FontName=%q, want %q", got.FontName, "Serif 12")
	}
}
