//go:build android

package glyph

/*
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_GLYPH_H
#include FT_STROKER_H
#include <hb.h>
#include <hb-ft.h>
#include <stdlib.h>
#include <string.h>

// ftRenderGlyph rasterizes a text string into an RGBA bitmap using
// FreeType + HarfBuzz. Returns bitmap data (caller must free),
// width and height. When rejectNotdef is non-zero, returns NULL
// with outW=-1 if any glyph is .notdef (missing from font).
static void* ftRenderGlyph(FT_Library lib, const char *fontPath,
    double fontSize, int bold, int italic, double subpixelShift,
    const char *text, int textLen,
    int *outW, int *outH, int *outLeft, int *outTop,
    int rejectNotdef) {

    *outW = 0; *outH = 0; *outLeft = 0; *outTop = 0;

    FT_Face face;
    if (FT_New_Face(lib, fontPath, 0, &face)) return NULL;

    // Bitmap-only fonts (e.g. NotoColorEmoji CBDT) need
    // FT_Select_Size; scalable fonts use FT_Set_Char_Size.
    if (face->num_fixed_sizes > 0) {
        int bestIdx = 0;
        int target = (int)(fontSize + 0.5);
        int bestDiff = abs(face->available_sizes[0].height
                          - target);
        for (int i = 1; i < face->num_fixed_sizes; i++) {
            int diff = abs(face->available_sizes[i].height
                          - target);
            if (diff < bestDiff) {
                bestDiff = diff;
                bestIdx = i;
            }
        }
        FT_Select_Size(face, bestIdx);
    } else {
        FT_F26Dot6 charSize = (FT_F26Dot6)(fontSize * 64.0);
        if (FT_Set_Char_Size(face, 0, charSize, 72, 72)) {
            FT_Done_Face(face);
            return NULL;
        }
    }

    // Use HarfBuzz for shaping.
    hb_font_t *hbFont = hb_ft_font_create_referenced(face);
    hb_buffer_t *buf = hb_buffer_create();
    hb_buffer_add_utf8(buf, text, textLen, 0, textLen);
    hb_buffer_guess_segment_properties(buf);
    hb_shape(hbFont, buf, NULL, 0);

    unsigned int glyphCount;
    hb_glyph_info_t *infos =
        hb_buffer_get_glyph_infos(buf, &glyphCount);
    hb_glyph_position_t *positions =
        hb_buffer_get_glyph_positions(buf, &glyphCount);

    if (glyphCount == 0) {
        hb_buffer_destroy(buf);
        hb_font_destroy(hbFont);
        FT_Done_Face(face);
        return NULL;
    }

    // Check for missing glyphs (.notdef = glyph index 0).
    if (rejectNotdef) {
        for (unsigned int i = 0; i < glyphCount; i++) {
            if (infos[i].codepoint == 0) {
                hb_buffer_destroy(buf);
                hb_font_destroy(hbFont);
                FT_Done_Face(face);
                *outW = -1; // signal: glyph not in font
                return NULL;
            }
        }
    }

    // FT_LOAD_COLOR tells FreeType to prefer CBDT/CBLC color
    // bitmaps (emoji). Outlines are rendered manually only when
    // the glyph is not already a bitmap.
    int loadFlags = FT_LOAD_DEFAULT | FT_LOAD_COLOR;

    // Compute bounding box across all glyphs.
    int pad = 2;
    double penX = subpixelShift;
    int minX = 999999, maxX = -999999;
    int minY = 999999, maxY = -999999;

    for (unsigned int i = 0; i < glyphCount; i++) {
        FT_Load_Glyph(face, infos[i].codepoint, loadFlags);
        if (face->glyph->format == FT_GLYPH_FORMAT_OUTLINE)
            FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL);
        FT_Bitmap *bmp = &face->glyph->bitmap;
        if (bmp->width == 0 || bmp->rows == 0) {
            penX += (double)positions[i].x_advance / 64.0;
            continue;
        }

        int gx = (int)penX + face->glyph->bitmap_left;
        int gy = -face->glyph->bitmap_top;
        if (gx < minX) minX = gx;
        if (gy < minY) minY = gy;
        if (gx + (int)bmp->width > maxX)
            maxX = gx + (int)bmp->width;
        if (gy + (int)bmp->rows > maxY)
            maxY = gy + (int)bmp->rows;

        penX += (double)positions[i].x_advance / 64.0;
    }

    if (minX >= maxX || minY >= maxY) {
        hb_buffer_destroy(buf);
        hb_font_destroy(hbFont);
        FT_Done_Face(face);
        return NULL;
    }

    int w = (maxX - minX) + pad * 2;
    int h = (maxY - minY) + pad * 2;
    if (w < 1) w = 1;
    if (h < 1) h = 1;
    if (w > 256) w = 256;
    if (h > 256) h = 256;

    *outW = w;
    *outH = h;
    *outLeft = minX - pad;
    *outTop = -(minY - pad);

    size_t bytesPerRow = (size_t)w * 4;
    void *data = calloc((size_t)h, bytesPerRow);
    if (!data) {
        hb_buffer_destroy(buf);
        hb_font_destroy(hbFont);
        FT_Done_Face(face);
        return NULL;
    }

    // Second pass: render glyphs into bitmap.
    penX = subpixelShift;
    for (unsigned int i = 0; i < glyphCount; i++) {
        FT_Load_Glyph(face, infos[i].codepoint, loadFlags);
        if (face->glyph->format == FT_GLYPH_FORMAT_OUTLINE)
            FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL);
        FT_Bitmap *bmp = &face->glyph->bitmap;
        if (bmp->width == 0 || bmp->rows == 0) {
            penX += (double)positions[i].x_advance / 64.0;
            continue;
        }

        int gx = (int)penX + face->glyph->bitmap_left - minX + pad;
        int gy = -face->glyph->bitmap_top - minY + pad;
        unsigned char *dst = (unsigned char *)data;
        int isColor = (bmp->pixel_mode == FT_PIXEL_MODE_BGRA);

        for (unsigned int row = 0; row < bmp->rows; row++) {
            int dy = gy + (int)row;
            if (dy < 0 || dy >= h) continue;
            for (unsigned int col = 0; col < bmp->width; col++) {
                int dx = gx + (int)col;
                if (dx < 0 || dx >= w) continue;
                int idx = (dy * w + dx) * 4;
                if (isColor) {
                    // BGRA → RGBA
                    int si = row * (unsigned int)bmp->pitch
                           + col * 4;
                    unsigned char a = bmp->buffer[si + 3];
                    if (a > dst[idx + 3]) {
                        dst[idx + 0] = bmp->buffer[si + 2];
                        dst[idx + 1] = bmp->buffer[si + 1];
                        dst[idx + 2] = bmp->buffer[si + 0];
                        dst[idx + 3] = a;
                    }
                } else {
                    unsigned char alpha =
                        bmp->buffer[row * (unsigned int)bmp->pitch
                                    + col];
                    dst[idx + 0] = 255;
                    dst[idx + 1] = 255;
                    dst[idx + 2] = 255;
                    if (alpha > dst[idx + 3])
                        dst[idx + 3] = alpha;
                }
            }
        }

        penX += (double)positions[i].x_advance / 64.0;
    }

    hb_buffer_destroy(buf);
    hb_font_destroy(hbFont);
    FT_Done_Face(face);
    return data;
}

// ftRenderStrokedGlyph rasterizes a stroked text string.
// rejectNotdef: if non-zero, return NULL with outW=-1 on .notdef.
static void* ftRenderStrokedGlyph(FT_Library lib,
    const char *fontPath, double fontSize,
    int bold, int italic, double strokeWidth,
    double subpixelShift,
    const char *text, int textLen,
    int *outW, int *outH, int *outLeft, int *outTop,
    int rejectNotdef) {

    *outW = 0; *outH = 0; *outLeft = 0; *outTop = 0;

    FT_Face face;
    if (FT_New_Face(lib, fontPath, 0, &face)) return NULL;
    FT_F26Dot6 charSize = (FT_F26Dot6)(fontSize * 64.0);
    if (FT_Set_Char_Size(face, 0, charSize, 72, 72)) {
        FT_Done_Face(face);
        return NULL;
    }

    FT_Stroker stroker;
    if (FT_Stroker_New(lib, &stroker)) {
        FT_Done_Face(face);
        return NULL;
    }
    FT_Stroker_Set(stroker,
        (FT_Fixed)(strokeWidth * 64.0),
        FT_STROKER_LINECAP_ROUND,
        FT_STROKER_LINEJOIN_ROUND, 0);

    // Use HarfBuzz for shaping.
    hb_font_t *hbFont = hb_ft_font_create_referenced(face);
    hb_buffer_t *buf = hb_buffer_create();
    hb_buffer_add_utf8(buf, text, textLen, 0, textLen);
    hb_buffer_guess_segment_properties(buf);
    hb_shape(hbFont, buf, NULL, 0);

    unsigned int glyphCount;
    hb_glyph_info_t *infos =
        hb_buffer_get_glyph_infos(buf, &glyphCount);
    hb_glyph_position_t *positions =
        hb_buffer_get_glyph_positions(buf, &glyphCount);

    if (glyphCount == 0) {
        hb_buffer_destroy(buf);
        hb_font_destroy(hbFont);
        FT_Stroker_Done(stroker);
        FT_Done_Face(face);
        return NULL;
    }

    // Check for missing glyphs (.notdef).
    if (rejectNotdef) {
        for (unsigned int i = 0; i < glyphCount; i++) {
            if (infos[i].codepoint == 0) {
                hb_buffer_destroy(buf);
                hb_font_destroy(hbFont);
                FT_Stroker_Done(stroker);
                FT_Done_Face(face);
                *outW = -1;
                return NULL;
            }
        }
    }

    int extraPad = (int)(strokeWidth + 0.5) + 4;
    double penX = subpixelShift;
    int minX = 999999, maxX = -999999;
    int minY = 999999, maxY = -999999;

    // First pass: compute bounds of stroked glyphs.
    for (unsigned int i = 0; i < glyphCount; i++) {
        FT_Load_Glyph(face, infos[i].codepoint,
            FT_LOAD_DEFAULT);
        FT_Glyph glyph;
        FT_Get_Glyph(face->glyph, &glyph);
        FT_Glyph_StrokeBorder(&glyph, stroker, 0, 1);
        FT_Glyph_To_Bitmap(&glyph, FT_RENDER_MODE_NORMAL, NULL, 1);
        FT_BitmapGlyph bmpGlyph = (FT_BitmapGlyph)glyph;
        FT_Bitmap *bmp = &bmpGlyph->bitmap;

        if (bmp->width > 0 && bmp->rows > 0) {
            int gx = (int)penX + bmpGlyph->left;
            int gy = -bmpGlyph->top;
            if (gx < minX) minX = gx;
            if (gy < minY) minY = gy;
            if (gx + (int)bmp->width > maxX)
                maxX = gx + (int)bmp->width;
            if (gy + (int)bmp->rows > maxY)
                maxY = gy + (int)bmp->rows;
        }
        FT_Done_Glyph(glyph);
        penX += (double)positions[i].x_advance / 64.0;
    }

    if (minX >= maxX || minY >= maxY) {
        hb_buffer_destroy(buf);
        hb_font_destroy(hbFont);
        FT_Stroker_Done(stroker);
        FT_Done_Face(face);
        return NULL;
    }

    int w = (maxX - minX) + extraPad * 2;
    int h = (maxY - minY) + extraPad * 2;
    if (w < 1) w = 1;
    if (h < 1) h = 1;
    if (w > 256) w = 256;
    if (h > 256) h = 256;

    *outW = w;
    *outH = h;
    *outLeft = minX - extraPad;
    *outTop = -(minY - extraPad);

    size_t bytesPerRow = (size_t)w * 4;
    void *data = calloc((size_t)h, bytesPerRow);
    if (!data) {
        hb_buffer_destroy(buf);
        hb_font_destroy(hbFont);
        FT_Stroker_Done(stroker);
        FT_Done_Face(face);
        return NULL;
    }

    // Second pass: render stroked glyphs.
    penX = subpixelShift;
    for (unsigned int i = 0; i < glyphCount; i++) {
        FT_Load_Glyph(face, infos[i].codepoint,
            FT_LOAD_DEFAULT);
        FT_Glyph glyph;
        FT_Get_Glyph(face->glyph, &glyph);
        FT_Glyph_StrokeBorder(&glyph, stroker, 0, 1);
        FT_Glyph_To_Bitmap(&glyph, FT_RENDER_MODE_NORMAL, NULL, 1);
        FT_BitmapGlyph bmpGlyph = (FT_BitmapGlyph)glyph;
        FT_Bitmap *bmp = &bmpGlyph->bitmap;

        if (bmp->width > 0 && bmp->rows > 0) {
            int gx = (int)penX + bmpGlyph->left -
                minX + extraPad;
            int gy = -bmpGlyph->top - minY + extraPad;
            unsigned char *dst = (unsigned char *)data;

            for (unsigned int row = 0; row < bmp->rows; row++) {
                int dy = gy + (int)row;
                if (dy < 0 || dy >= h) continue;
                for (unsigned int col = 0; col < bmp->width; col++) {
                    int dx = gx + (int)col;
                    if (dx < 0 || dx >= w) continue;
                    unsigned char alpha = bmp->buffer[
                        row * (unsigned int)bmp->pitch + col];
                    int idx = (dy * w + dx) * 4;
                    dst[idx + 0] = 255;
                    dst[idx + 1] = 255;
                    dst[idx + 2] = 255;
                    if (alpha > dst[idx + 3])
                        dst[idx + 3] = alpha;
                }
            }
        }
        FT_Done_Glyph(glyph);
        penX += (double)positions[i].x_advance / 64.0;
    }

    hb_buffer_destroy(buf);
    hb_font_destroy(hbFont);
    FT_Stroker_Done(stroker);
    FT_Done_Face(face);
    return data;
}
*/
import "C"
import (
	"unsafe"
)

// ftLibSingleton caches the FT_Library for bitmap rendering.
// Initialized lazily from Context.
var ftLibSingleton C.FT_Library

// ftFontPathsSingleton caches font paths for bitmap rendering.
var ftFontPathsSingleton map[string]string

// ftScriptFallbacksSingleton holds fallback font paths for
// scripts not covered by the primary font (CJK, Arabic, etc.).
var ftScriptFallbacksSingleton []string

// ftColorFallbacksSingleton holds color-emoji font paths (CBDT/CBLC)
// used to render UseOriginalColor glyphs in their own colors.
var ftColorFallbacksSingleton []string

// setFTLib stores the library handle for bitmap rendering. Called
// by NewContext.
func setFTLib(lib C.FT_Library) {
	ftLibSingleton = lib
}

// setFTFontPaths stores the font paths map for bitmap rendering.
// Called by NewContext.
func setFTFontPaths(paths map[string]string) {
	ftFontPathsSingleton = paths
}

// setFTScriptFallbacks stores fallback font paths for scripts
// not in the primary font.
func setFTScriptFallbacks(paths []string) {
	ftScriptFallbacksSingleton = paths
}

// setFTColorFallbacks stores color-emoji font paths for rendering
// UseOriginalColor glyphs. Called by NewContext.
func setFTColorFallbacks(paths []string) {
	ftColorFallbacksSingleton = paths
}

// loadGlyphFT rasterizes a character using FreeType.
func loadGlyphFT(atlas *GlyphAtlas, ch string, item Item,
	subpixelBin int, scaleFactor float32) (LoadGlyphResult, error) {

	family, fontSize, bold, italic := resolveFTFontParams(
		item.Style, scaleFactor)
	paths := fontFallbackPaths(ftFontPathsSingleton,
		family, bold, italic)

	cText := C.CString(ch)
	defer C.free(unsafe.Pointer(cText))

	subpixelShift := C.double(float64(subpixelBin) / 4.0)
	boldInt := C.int(0)
	if bold {
		boldInt = 1
	}
	italicInt := C.int(0)
	if italic {
		italicInt = 1
	}

	var w, h, left, top C.int
	var data unsafe.Pointer

	// Color-emoji items resolve against the script fallbacks first
	// (color fonts are ordered ahead of CJK). The primary text font
	// often carries a monochrome glyph for the same codepoint, which
	// would otherwise win and render as a tinted/blank box.
	if item.UseOriginalColor {
		for _, fp := range ftColorFallbacksSingleton {
			cPath := C.CString(fp)
			data = C.ftRenderGlyph(ftLibSingleton, cPath,
				C.double(fontSize), 0, 0,
				subpixelShift, cText, C.int(len(ch)),
				&w, &h, &left, &top, 1)
			C.free(unsafe.Pointer(cPath))
			if data != nil {
				break
			}
		}
	}

	// Try primary font paths (rejectNotdef=1).
	if data == nil {
		for _, fontPath := range paths {
			cPath := C.CString(fontPath)
			data = C.ftRenderGlyph(ftLibSingleton, cPath,
				C.double(fontSize), boldInt, italicInt,
				subpixelShift, cText, C.int(len(ch)),
				&w, &h, &left, &top, 1)
			C.free(unsafe.Pointer(cPath))
			if data != nil {
				break
			}
		}
	}

	// If .notdef (w==-1), try script fallback fonts.
	if data == nil && int(w) == -1 {
		for _, fp := range ftScriptFallbacksSingleton {
			cPath := C.CString(fp)
			data = C.ftRenderGlyph(ftLibSingleton, cPath,
				C.double(fontSize), 0, 0,
				subpixelShift, cText, C.int(len(ch)),
				&w, &h, &left, &top, 1)
			C.free(unsafe.Pointer(cPath))
			if data != nil {
				break
			}
		}
	}

	// Last resort: render with primary font (tofu) if no
	// fallback has the glyph.
	if data == nil && len(paths) > 0 {
		cPath := C.CString(paths[0])
		data = C.ftRenderGlyph(ftLibSingleton, cPath,
			C.double(fontSize), boldInt, italicInt,
			subpixelShift, cText, C.int(len(ch)),
			&w, &h, &left, &top, 0)
		C.free(unsafe.Pointer(cPath))
	}

	if data == nil || w == 0 || h == 0 {
		return LoadGlyphResult{}, nil
	}
	defer C.free(data)

	width := int(w)
	height := int(h)
	bmpSize, err := checkAllocationSize(width, height, 4)
	if err != nil {
		return LoadGlyphResult{}, err
	}
	goData := C.GoBytes(data, C.int(bmpSize))

	bmp := Bitmap{
		Width:    width,
		Height:   height,
		Channels: 4,
		Data:     goData,
	}

	cached, resetOccurred, resetPage, err := atlas.InsertBitmap(
		bmp, int(left), int(top))
	if err != nil {
		return LoadGlyphResult{}, err
	}

	return LoadGlyphResult{
		Cached:        cached,
		ResetOccurred: resetOccurred,
		ResetPage:     resetPage,
	}, nil
}

// loadStrokedGlyphFT rasterizes a stroked character.
func loadStrokedGlyphFT(atlas *GlyphAtlas, ch string, item Item,
	strokeWidth float32, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	family, fontSize, bold, italic := resolveFTFontParams(
		item.Style, scaleFactor)
	paths := fontFallbackPaths(ftFontPathsSingleton,
		family, bold, italic)

	cText := C.CString(ch)
	defer C.free(unsafe.Pointer(cText))

	subpixelShift := C.double(float64(subpixelBin) / 4.0)
	boldInt := C.int(0)
	if bold {
		boldInt = 1
	}
	italicInt := C.int(0)
	if italic {
		italicInt = 1
	}

	sw := C.double(float64(strokeWidth) * float64(scaleFactor))

	var w, h, left, top C.int
	var data unsafe.Pointer
	for _, fontPath := range paths {
		cPath := C.CString(fontPath)
		data = C.ftRenderStrokedGlyph(ftLibSingleton, cPath,
			C.double(fontSize), boldInt, italicInt, sw,
			subpixelShift, cText, C.int(len(ch)),
			&w, &h, &left, &top, 1)
		C.free(unsafe.Pointer(cPath))
		if data != nil {
			break
		}
	}

	// If .notdef, try script fallback fonts.
	if data == nil && int(w) == -1 {
		for _, fp := range ftScriptFallbacksSingleton {
			cPath := C.CString(fp)
			data = C.ftRenderStrokedGlyph(ftLibSingleton, cPath,
				C.double(fontSize), 0, 0, sw,
				subpixelShift, cText, C.int(len(ch)),
				&w, &h, &left, &top, 1)
			C.free(unsafe.Pointer(cPath))
			if data != nil {
				break
			}
		}
	}

	// Last resort: render with primary font (tofu).
	if data == nil && len(paths) > 0 {
		cPath := C.CString(paths[0])
		data = C.ftRenderStrokedGlyph(ftLibSingleton, cPath,
			C.double(fontSize), boldInt, italicInt, sw,
			subpixelShift, cText, C.int(len(ch)),
			&w, &h, &left, &top, 0)
		C.free(unsafe.Pointer(cPath))
	}

	if data == nil || w == 0 || h == 0 {
		return LoadGlyphResult{}, nil
	}
	defer C.free(data)

	width := int(w)
	height := int(h)
	bmpSize, err := checkAllocationSize(width, height, 4)
	if err != nil {
		return LoadGlyphResult{}, err
	}
	goData := C.GoBytes(data, C.int(bmpSize))

	bmp := Bitmap{
		Width:    width,
		Height:   height,
		Channels: 4,
		Data:     goData,
	}

	cached, resetOccurred, resetPage, err := atlas.InsertBitmap(
		bmp, int(left), int(top))
	if err != nil {
		return LoadGlyphResult{}, err
	}

	return LoadGlyphResult{
		Cached:        cached,
		ResetOccurred: resetOccurred,
		ResetPage:     resetPage,
	}, nil
}
