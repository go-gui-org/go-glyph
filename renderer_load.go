//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

/*
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_STROKER_H
#include FT_GLYPH_H
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// LoadGlyphConfig holds parameters for glyph rasterization.
type LoadGlyphConfig struct {
	Face         C.FT_Face // Borrowed from Pango; do not free.
	Index        uint32
	TargetHeight int
	SubpixelBin  int // 0..3 → 0, 0.25, 0.5, 0.75 px shift.
}

// LoadGlyphResult holds the output of a glyph load operation.
type LoadGlyphResult struct {
	Cached        CachedGlyph
	ResetOccurred bool
	ResetPage     int
}

// LoadGlyph rasterizes a glyph via FreeType and inserts it
// into the atlas.
//
// Hybrid hinting strategy:
//   - High DPI (>= 2.0): FT_LOAD_TARGET_LCD for subpixel.
//   - Low DPI (< 2.0): FT_LOAD_TARGET_LIGHT for auto-hinting.
//
// Subpixel positioning: bins 0–3 shift the outline by
// 0/0.25/0.5/0.75 pixels before rendering.
func LoadGlyph(atlas *GlyphAtlas, cfg LoadGlyphConfig,
	scaleFactor float32) (LoadGlyphResult, error) {

	isHighDPI := scaleFactor >= 2.0
	var targetFlag C.FT_Int32
	if isHighDPI {
		targetFlag = C.FT_Int32(FTLoadTargetLCD)
	} else {
		targetFlag = C.FT_Int32(FTLoadTargetLight)
	}

	shouldShift := cfg.SubpixelBin > 0

	// Base flags: color (for emoji) + target mode.
	flags := C.FT_Int32(C.FT_LOAD_COLOR) | targetFlag

	if !shouldShift {
		flags |= C.FT_Int32(C.FT_LOAD_RENDER)
	} else {
		flags |= C.FT_Int32(C.FT_LOAD_NO_BITMAP)
	}

	if C.FT_Load_Glyph(cfg.Face, C.FT_UInt(cfg.Index), flags) != 0 {
		if !shouldShift {
			return LoadGlyphResult{}, fmt.Errorf(
				"FT_Load_Glyph failed for index 0x%x", cfg.Index)
		}
		// Bitmap-only font (e.g. color emoji) — fall back to
		// direct render without subpixel shift.
		fallback := C.FT_Int32(C.FT_LOAD_RENDER|C.FT_LOAD_COLOR) | targetFlag
		if C.FT_Load_Glyph(cfg.Face, C.FT_UInt(cfg.Index), fallback) != 0 {
			return LoadGlyphResult{}, fmt.Errorf(
				"FT_Load_Glyph failed for index 0x%x", cfg.Index)
		}
		shouldShift = false
	}

	glyph := cfg.Face.glyph

	if shouldShift {
		shift := C.FT_Pos(cfg.SubpixelBin * FTSubpixelUnit)
		C.FT_Outline_Translate(&glyph.outline, shift, 0)

		var renderMode C.FT_Render_Mode
		if isHighDPI {
			renderMode = C.FT_RENDER_MODE_LCD
		} else {
			renderMode = C.FT_RENDER_MODE_NORMAL
		}
		if C.FT_Render_Glyph(glyph, renderMode) != 0 {
			// Fallback: reload with FT_LOAD_RENDER.
			fallback := C.FT_Int32(C.FT_LOAD_RENDER|C.FT_LOAD_COLOR) | targetFlag
			if C.FT_Load_Glyph(cfg.Face, C.FT_UInt(cfg.Index), fallback) != 0 {
				return LoadGlyphResult{},
					fmt.Errorf("FT_Render_Glyph failed and fallback failed")
			}
		}
	}

	ftBitmap := &glyph.bitmap
	if ftBitmap.buffer == nil || ftBitmap.width == 0 || ftBitmap.rows == 0 {
		return LoadGlyphResult{}, nil // space or empty glyph
	}

	bmp, err := FTBitmapToBitmap(ftBitmap, cfg.Face, cfg.TargetHeight)
	if err != nil {
		return LoadGlyphResult{}, err
	}

	var left, top int
	if int(ftBitmap.pixel_mode) == int(C.FT_PIXEL_MODE_BGRA) {
		left = 0
		top = bmp.Height
	} else {
		left = int(glyph.bitmap_left)
		top = int(glyph.bitmap_top)
	}

	cached, resetOccurred, resetPage, err := atlas.InsertBitmap(bmp, left, top)
	if err != nil {
		return LoadGlyphResult{}, err
	}

	return LoadGlyphResult{
		Cached:        cached,
		ResetOccurred: resetOccurred,
		ResetPage:     resetPage,
	}, nil
}

// LoadStrokedGlyph rasterizes a stroked (outline-only) glyph
// via FT_Stroker and inserts it into the atlas.
func LoadStrokedGlyph(atlas *GlyphAtlas, stroker FTStroker,
	cfg LoadGlyphConfig, strokeRadius int64,
	scaleFactor float32) (LoadGlyphResult, error) {

	isHighDPI := scaleFactor >= 2.0
	var targetFlag C.FT_Int32
	if isHighDPI {
		targetFlag = C.FT_Int32(FTLoadTargetLCD)
	} else {
		targetFlag = C.FT_Int32(FTLoadTargetLight)
	}

	// Load outline only.
	flags := C.FT_Int32(C.FT_LOAD_NO_BITMAP) | targetFlag
	if C.FT_Load_Glyph(cfg.Face, C.FT_UInt(cfg.Index), flags) != 0 {
		return LoadGlyphResult{},
			fmt.Errorf("FT_Load_Glyph failed for stroked glyph 0x%x", cfg.Index)
	}

	glyphSlot := cfg.Face.glyph

	// Get independent glyph copy from slot.
	var ftGlyph C.FT_Glyph
	if C.FT_Get_Glyph(glyphSlot, &ftGlyph) != 0 {
		return LoadGlyphResult{}, fmt.Errorf("FT_Get_Glyph failed")
	}
	defer func() { C.FT_Done_Glyph(ftGlyph) }()

	// Apply stroke.
	if C.FT_Glyph_Stroke(&ftGlyph, stroker.ptr, 1) != 0 {
		return LoadGlyphResult{}, fmt.Errorf("FT_Glyph_Stroke failed")
	}

	// Convert to bitmap.
	var renderMode C.FT_Render_Mode
	if isHighDPI {
		renderMode = C.FT_RENDER_MODE_LCD
	} else {
		renderMode = C.FT_RENDER_MODE_NORMAL
	}
	if C.FT_Glyph_To_Bitmap(&ftGlyph, renderMode, nil, 1) != 0 {
		return LoadGlyphResult{}, fmt.Errorf("FT_Glyph_To_Bitmap failed")
	}

	// Cast to BitmapGlyph.
	bmpGlyph := (*C.FT_BitmapGlyphRec)(unsafe.Pointer(ftGlyph))
	ftBitmap := &bmpGlyph.bitmap

	if ftBitmap.buffer == nil || ftBitmap.width == 0 || ftBitmap.rows == 0 {
		return LoadGlyphResult{}, nil // empty glyph
	}

	bmp, err := FTBitmapToBitmap(ftBitmap, cfg.Face, cfg.TargetHeight)
	if err != nil {
		return LoadGlyphResult{}, err
	}

	cached, resetOccurred, resetPage, err := atlas.InsertBitmap(
		bmp, int(bmpGlyph.left), int(bmpGlyph.top))
	if err != nil {
		return LoadGlyphResult{}, err
	}

	return LoadGlyphResult{
		Cached:        cached,
		ResetOccurred: resetOccurred,
		ResetPage:     resetPage,
	}, nil
}
