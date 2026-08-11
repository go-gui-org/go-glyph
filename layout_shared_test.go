//go:build android || linux || darwin || windows || (js && wasm)

package glyph

import (
	"math"
	"testing"
)

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

// A grid caller declares the cell once on cfg.Style; every rich-text run that
// leaves the fields at 0 must see it (#105).
func TestMergeStyles_GridFieldsInheritFromBase(t *testing.T) {
	base := TextStyle{CellWidth: 9.5, CellHeight: 20, EmojiBoxWidth: 19}
	got := mergeStyles(base, TextStyle{})
	if got.CellWidth != 9.5 {
		t.Errorf("CellWidth=%v, want 9.5", got.CellWidth)
	}
	if got.CellHeight != 20 {
		t.Errorf("CellHeight=%v, want 20", got.CellHeight)
	}
	if got.EmojiBoxWidth != 19 {
		t.Errorf("EmojiBoxWidth=%v, want 19", got.EmojiBoxWidth)
	}
}

func TestMergeStyles_RunGridFieldsOverrideBase(t *testing.T) {
	base := TextStyle{CellWidth: 9.5, CellHeight: 20, EmojiBoxWidth: 19}
	run := TextStyle{CellWidth: 12, CellHeight: 24, EmojiBoxWidth: 24}
	got := mergeStyles(base, run)
	if got.CellWidth != 12 {
		t.Errorf("CellWidth=%v, want 12", got.CellWidth)
	}
	if got.CellHeight != 24 {
		t.Errorf("CellHeight=%v, want 24", got.CellHeight)
	}
	if got.EmojiBoxWidth != 24 {
		t.Errorf("EmojiBoxWidth=%v, want 24", got.EmojiBoxWidth)
	}
}

// A run that overrides one grid field still inherits the others, so the
// geometry is declared once on the base style and refined per run.
func TestMergeStyles_PartialGridOverrideInheritsRest(t *testing.T) {
	base := TextStyle{CellWidth: 9.5, CellHeight: 20, EmojiBoxWidth: 19}
	got := mergeStyles(base, TextStyle{CellWidth: 12})
	if got.CellWidth != 12 {
		t.Errorf("CellWidth=%v, want 12", got.CellWidth)
	}
	if got.CellHeight != 20 {
		t.Errorf("CellHeight=%v, want 20 (inherited)", got.CellHeight)
	}
	if got.EmojiBoxWidth != 19 {
		t.Errorf("EmojiBoxWidth=%v, want 19 (inherited)", got.EmojiBoxWidth)
	}
}

// Bad data in the run is treated as unset: negative values fall back to the
// base, and a NaN base only leaks into runs that leave the field at 0 —
// every consumer gates on `> 0`, so NaN and 0 are equivalent downstream.
func TestMergeStyles_BadDataFallsBackOrStaysInert(t *testing.T) {
	base := TextStyle{CellWidth: 9.5}
	got := mergeStyles(base, TextStyle{CellWidth: -1})
	if got.CellWidth != 9.5 {
		t.Errorf("CellWidth=%v, want base 9.5 for negative run value", got.CellWidth)
	}

	nan := float32(math.NaN())
	got = mergeStyles(TextStyle{CellWidth: nan}, TextStyle{})
	if !math.IsNaN(float64(got.CellWidth)) {
		t.Errorf("CellWidth=%v, want NaN to propagate unchanged", got.CellWidth)
	}
	got = mergeStyles(TextStyle{CellWidth: nan}, TextStyle{CellWidth: 7})
	if got.CellWidth != 7 {
		t.Errorf("CellWidth=%v, want run 7 to override NaN base", got.CellWidth)
	}
}

// The bool has no unset sentinel, so the opt-out is the OR of base and run.
func TestMergeStyles_NoBuiltinBoxGlyphsIsSticky(t *testing.T) {
	tests := []struct {
		name      string
		base, run bool
		want      bool
	}{
		{"base only", true, false, true},
		{"run only", false, true, true},
		{"both", true, true, true},
		{"neither", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeStyles(
				TextStyle{NoBuiltinBoxGlyphs: tt.base},
				TextStyle{NoBuiltinBoxGlyphs: tt.run},
			)
			if got.NoBuiltinBoxGlyphs != tt.want {
				t.Errorf("NoBuiltinBoxGlyphs=%v, want %v", got.NoBuiltinBoxGlyphs, tt.want)
			}
		})
	}
}
