//go:build android

package glyph

/*
#include <ft2build.h>
#include FT_FREETYPE_H
#include <hb.h>
#include <hb-ft.h>
#include <stdlib.h>

// ftCreateFont opens a font face from a file path and creates an
// HarfBuzz font for shaping. Handles bitmap-only fonts (CBDT)
// via FT_Select_Size.
static int ftCreateFont(FT_Library lib, const char *path,
    FT_F26Dot6 charSize, FT_Face *outFace, hb_font_t **outHB) {
    FT_Error err = FT_New_Face(lib, path, 0, outFace);
    if (err) return err;

    if ((*outFace)->num_fixed_sizes > 0) {
        int bestIdx = 0;
        int target = (int)(charSize / 64);
        int bestDiff = abs((*outFace)->available_sizes[0].height
                          - target);
        for (int i = 1; i < (*outFace)->num_fixed_sizes; i++) {
            int diff = abs(
                (*outFace)->available_sizes[i].height - target);
            if (diff < bestDiff) {
                bestDiff = diff;
                bestIdx = i;
            }
        }
        FT_Select_Size(*outFace, bestIdx);
    } else {
        err = FT_Set_Char_Size(*outFace, 0, charSize, 72, 72);
        if (err) {
            FT_Done_Face(*outFace);
            *outFace = NULL;
            return err;
        }
    }
    *outHB = hb_ft_font_create_referenced(*outFace);
    return 0;
}

// ftFontGetMetrics returns ascent, descent, leading in font units
// scaled to 26.6 fixed point.
static void ftFontGetMetrics(FT_Face face,
    double *ascent, double *descent, double *leading) {
    *ascent = (double)face->size->metrics.ascender / 64.0;
    *descent = (double)(-face->size->metrics.descender) / 64.0;
    double height = (double)face->size->metrics.height / 64.0;
    *leading = height - *ascent - *descent;
    if (*leading < 0) *leading = 0;
}

// ftMeasureString measures a UTF-8 string width using HarfBuzz.
static double ftMeasureString(hb_font_t *hbFont, const char *text,
    int textLen) {
    hb_buffer_t *buf = hb_buffer_create();
    hb_buffer_add_utf8(buf, text, textLen, 0, textLen);
    hb_buffer_guess_segment_properties(buf);
    hb_shape(hbFont, buf, NULL, 0);

    unsigned int glyphCount;
    hb_glyph_position_t *positions =
        hb_buffer_get_glyph_positions(buf, &glyphCount);
    double width = 0;
    for (unsigned int i = 0; i < glyphCount; i++) {
        width += (double)positions[i].x_advance / 64.0;
    }
    hb_buffer_destroy(buf);
    return width;
}

// ftHasGlyphs returns 1 if the font can shape all glyphs in text
// (no .notdef), 0 otherwise.
static int ftHasGlyphs(hb_font_t *hbFont, const char *text,
    int textLen) {
    hb_buffer_t *buf = hb_buffer_create();
    hb_buffer_add_utf8(buf, text, textLen, 0, textLen);
    hb_buffer_guess_segment_properties(buf);
    hb_shape(hbFont, buf, NULL, 0);

    unsigned int glyphCount;
    hb_glyph_info_t *infos =
        hb_buffer_get_glyph_infos(buf, &glyphCount);
    int ok = (glyphCount > 0) ? 1 : 0;
    for (unsigned int i = 0; i < glyphCount; i++) {
        if (infos[i].codepoint == 0) { ok = 0; break; }
    }
    hb_buffer_destroy(buf);
    return ok;
}
*/
import "C"
import (
	"strings"
	"unsafe"
)

// ftFont wraps an FT_Face + hb_font_t with a Go-friendly interface.
type ftFont struct {
	face  C.FT_Face
	hb    *C.hb_font_t
	ftLib C.FT_Library
}

func resolveFTFontParams(style TextStyle, scaleFactor float32) (
	family string, size float64, bold, italic bool,
) {
	family = resolveFontFamilyAndroid(style.FontName)

	rawSize := style.Size
	if rawSize <= 0 {
		rawSize = parseSizeFromFontName(style.FontName)
	}
	if rawSize <= 0 {
		rawSize = 16
	}
	size = float64(rawSize) * float64(scaleFactor)

	bold = style.Typeface == TypefaceBold ||
		style.Typeface == TypefaceBoldItalic
	italic = style.Typeface == TypefaceItalic ||
		style.Typeface == TypefaceBoldItalic

	lower := strings.ToLower(style.FontName)
	if !bold && strings.Contains(lower, " bold") {
		bold = true
	}
	if !italic && strings.Contains(lower, " italic") {
		italic = true
	}

	return family, size, bold, italic
}

// newFTFont creates a FreeType+HarfBuzz font from a TextStyle.
// Falls back to the regular variant, then to Roboto-Regular if
// the requested style cannot be opened.
func newFTFont(lib C.FT_Library, fontPaths map[string]string,
	style TextStyle, scaleFactor float32) ftFont {

	family, size, bold, italic := resolveFTFontParams(style, scaleFactor)
	charSize := C.FT_F26Dot6(size * 64.0)

	// Try the requested style first, then fall back.
	paths := fontFallbackPaths(fontPaths, family, bold, italic)
	for _, path := range paths {
		cPath := C.CString(path)
		var face C.FT_Face
		var hb *C.hb_font_t
		rc := C.ftCreateFont(lib, cPath, charSize, &face, &hb)
		C.free(unsafe.Pointer(cPath))
		if rc == 0 {
			return ftFont{face: face, hb: hb, ftLib: lib}
		}
	}
	return ftFont{}
}

// fontFallbackPaths returns a list of font paths to try in order:
// requested style → regular variant → Roboto-Regular fallback.
func fontFallbackPaths(fontPaths map[string]string,
	family string, bold, italic bool) []string {

	primary := resolveFontPath(fontPaths, family, bold, italic)
	paths := []string{primary}

	if bold || italic {
		regular := resolveFontPath(fontPaths, family, false, false)
		if regular != primary {
			paths = append(paths, regular)
		}
	}

	const fallback = "/system/fonts/Roboto-Regular.ttf"
	if paths[len(paths)-1] != fallback {
		paths = append(paths, fallback)
	}
	return paths
}

// newFTFontFromPath creates a font from a file path and size
// (in physical pixels). Caller must call close().
func newFTFontFromPath(lib C.FT_Library, path string,
	fontSize float64) ftFont {

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	charSize := C.FT_F26Dot6(fontSize * 64.0)
	var face C.FT_Face
	var hb *C.hb_font_t
	if C.ftCreateFont(lib, cPath, charSize, &face, &hb) != 0 {
		return ftFont{}
	}
	return ftFont{face: face, hb: hb, ftLib: lib}
}

// hasGlyphs returns true if the font can render all glyphs in
// text (no .notdef).
func (f ftFont) hasGlyphs(text string) bool {
	if f.hb == nil || len(text) == 0 {
		return false
	}
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	return C.ftHasGlyphs(f.hb, cs, C.int(len(text))) != 0
}

// isColorFont reports whether the face carries color bitmap glyphs
// (CBDT/CBLC emoji fonts such as Noto Color Emoji).
func (f ftFont) isColorFont() bool {
	if f.face == nil {
		return false
	}
	return int64(f.face.face_flags)&int64(C.FT_FACE_FLAG_COLOR) != 0
}

// close releases the FreeType face and HarfBuzz font.
func (f *ftFont) close() {
	if f.hb != nil {
		C.hb_font_destroy(f.hb)
		f.hb = nil
	}
	if f.face != nil {
		C.FT_Done_Face(f.face)
		f.face = nil
	}
}

// metrics returns ascent, descent, leading in font units.
func (f ftFont) metrics() (ascent, descent, leading float64) {
	if f.face == nil {
		return 0, 0, 0
	}
	var a, d, l C.double
	C.ftFontGetMetrics(f.face, &a, &d, &l)
	return float64(a), float64(d), float64(l)
}

// measureString measures a UTF-8 string width using HarfBuzz.
func (f ftFont) measureString(text string) float64 {
	if f.hb == nil || len(text) == 0 {
		return 0
	}
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	return float64(C.ftMeasureString(f.hb, cs, C.int(len(text))))
}

// resolveFontFamilyAndroid maps generic Pango-style font names to
// Android system font families.
func resolveFontFamilyAndroid(fontName string) string {
	family := parseFamilyFromFontName(fontName)
	if family == "" {
		return "Roboto"
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return "Roboto"
	case "serif":
		return "NotoSerif"
	case "monospace", "mono":
		return "DroidSansMono"
	default:
		return family
	}
}

// resolveFontPath finds the .ttf/.otf path for a family+style combo.
func resolveFontPath(fontPaths map[string]string,
	family string, bold, italic bool) string {

	// Try style-specific keys first.
	suffix := ""
	if bold && italic {
		suffix = "-BoldItalic"
	} else if bold {
		suffix = "-Bold"
	} else if italic {
		suffix = "-Italic"
	} else {
		suffix = "-Regular"
	}

	if p, ok := fontPaths[family+suffix]; ok {
		return p
	}
	// Fall back to regular variant.
	if p, ok := fontPaths[family+"-Regular"]; ok {
		return p
	}
	// Fall back to bare family name.
	if p, ok := fontPaths[family]; ok {
		return p
	}
	// Last resort: Roboto-Regular.
	if p, ok := fontPaths["Roboto-Regular"]; ok {
		return p
	}
	return "/system/fonts/Roboto-Regular.ttf"
}

// parseSizeFromFontName extracts trailing numeric size from Pango
// font name like "Sans Bold 18".
func parseSizeFromFontName(name string) float32 {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return 0
	}
	last := parts[len(parts)-1]
	var sz float32
	for _, c := range last {
		if c >= '0' && c <= '9' {
			sz = sz*10 + float32(c-'0')
		} else if c == '.' {
			break
		} else {
			return 0
		}
	}
	return sz
}

// parseFamilyFromFontName extracts the family portion from a Pango
// font name, stripping trailing size and style keywords.
func parseFamilyFromFontName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}

	end := len(parts)
	if sz := parseSizeFromFontName(name); sz > 0 {
		end--
	}

	styleWords := map[string]bool{
		"bold": true, "italic": true, "oblique": true,
		"light": true, "medium": true, "semibold": true,
		"heavy": true, "ultrabold": true, "ultralight": true,
		"condensed": true, "expanded": true, "regular": true,
	}
	for end > 0 && styleWords[strings.ToLower(parts[end-1])] {
		end--
	}
	if end == 0 {
		end = 1
	}
	return strings.Join(parts[:end], " ")
}
