//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

import "testing"

func TestFreeTypeInit(t *testing.T) {
	lib, err := InitFreeType()
	if err != nil {
		t.Fatalf("FT_Init_FreeType: %v", err)
	}
	defer lib.Close()
}

func TestPangoFontMapCreation(t *testing.T) {
	fm := NewPangoFT2FontMap()
	defer fm.Close()
	fm.SetResolution(72, 72)

	ctx := fm.CreateContext()
	defer ctx.Close()

	if ctx.Ptr() == nil {
		t.Fatal("PangoContext is nil")
	}
}

func TestPangoLayoutCreation(t *testing.T) {
	fm := NewPangoFT2FontMap()
	defer fm.Close()
	fm.SetResolution(72, 72)

	ctx := fm.CreateContext()
	defer ctx.Close()

	layout := NewPangoLayout(ctx)
	defer layout.Close()

	layout.SetText("Hello")
	if layout.Ptr() == nil {
		t.Fatal("PangoLayout is nil")
	}
}

func TestPangoFontDescription(t *testing.T) {
	desc := NewPangoFontDescFromString("Sans 12")
	defer desc.Close()
	desc.SetSize(24 * PangoScale)
}
