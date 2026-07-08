//go:build linux && !android

package glyph

/*
#include <ft2build.h>
#include FT_FREETYPE_H
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// Context holds FreeType state for text shaping on Linux.
//
// Not safe for concurrent use.
type Context struct {
	ftLib         C.FT_Library
	scaleFactor   float32
	scaleInv      float32
	metrics       metricsCache
	fontPaths     map[string]string
	fallbackPaths []string // script fallback fonts (CJK, Arabic, etc.)
	colorPaths    []string // color-emoji fonts (CBDT/CBLC), render side
}

// NewContext creates a Linux text context backed by FreeType+HarfBuzz.
func NewContext(scaleFactor float32) (*Context, error) {
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}

	var lib C.FT_Library
	if rc := C.FT_Init_FreeType(&lib); rc != 0 {
		return nil, fmt.Errorf("FT_Init_FreeType failed: %d", rc)
	}

	ctx := &Context{
		ftLib:       lib,
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
		fontPaths:   make(map[string]string),
	}
	setFTLib(lib)
	ctx.discoverSystemFonts()
	setFTFontPaths(ctx.fontPaths)
	setFTScriptFallbacks(ctx.fallbackPaths)
	setFTColorFallbacks(ctx.colorPaths)
	return ctx, nil
}

// Free releases resources.
func (ctx *Context) Free() {
	if ctx.ftLib != nil {
		C.FT_Done_FreeType(ctx.ftLib)
		ctx.ftLib = nil
	}
	ctx.metrics = metricsCache{}
	ctx.fontPaths = nil
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile registers a font file by loading it temporarily to
// extract the family name.
func (ctx *Context) AddFontFile(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var face C.FT_Face
	if rc := C.FT_New_Face(ctx.ftLib, cPath, 0, &face); rc != 0 {
		return fmt.Errorf("FT_New_Face failed for %q: %d", path, rc)
	}
	family := C.GoString(face.family_name)
	style := C.GoString(face.style_name)
	C.FT_Done_Face(face)

	key := family + "-" + style
	ctx.fontPaths[key] = path
	ctx.fontPaths[family] = path
	return nil
}

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	font := newFTFont(ctx.ftLib, ctx.fontPaths, cfg.Style, ctx.scaleFactor)
	if font.face == nil {
		return 0, fmt.Errorf("failed to create FT font")
	}
	defer font.close()

	ascent, descent, _ := font.metrics()
	return float32(ascent+descent) / ctx.scaleFactor, nil
}

// FontMetrics returns detailed metrics in logical pixels.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	font := newFTFont(ctx.ftLib, ctx.fontPaths, cfg.Style, ctx.scaleFactor)
	if font.face == nil {
		return TextMetrics{}, fmt.Errorf("failed to create FT font")
	}
	defer font.close()

	ascent, descent, leading := font.metrics()
	sf := float64(ctx.scaleFactor)
	asc := float32(ascent / sf)
	dsc := float32(descent / sf)
	return TextMetrics{
		Ascender:  asc,
		Descender: dsc,
		Height:    asc + dsc,
		LineGap:   float32(leading / sf),
	}, nil
}

// ResolveFontName returns the resolved Linux font family name.
func (ctx *Context) ResolveFontName(fontDescStr string) (string, error) {
	family := resolveFontFamilyLinux(fontDescStr)
	return family, nil
}

// createFTFont builds an ftFont from TextStyle. Caller must call
// close().
func (ctx *Context) createFTFont(style TextStyle) ftFont {
	return newFTFont(ctx.ftLib, ctx.fontPaths, style, ctx.scaleFactor)
}

// discoverSystemFonts walks standard Linux font directories and
// populates ctx.fontPaths using FreeType to read family names.
// No fontconfig dependency.
func (ctx *Context) discoverSystemFonts() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".local", "share", "fonts"),
		filepath.Join(home, ".fonts"),
		"/usr/local/share/fonts",
		"/usr/share/fonts",
	}

	// Script-fallback candidates, collected during the walk and
	// assembled in priority order afterwards. Color emoji is tried
	// before CJK so colored glyphs win over monochrome coverage.
	var emojiPaths, cjkPaths, colorPaths []string
	seenFallback := map[string]bool{}
	seenColor := map[string]bool{}

	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			lower := strings.ToLower(path)
			// .ttc covers OpenType collections (Noto CJK ships this way).
			if !strings.HasSuffix(lower, ".ttf") &&
				!strings.HasSuffix(lower, ".otf") &&
				!strings.HasSuffix(lower, ".ttc") {
				return nil
			}

			cPath := C.CString(path)
			var face C.FT_Face
			rc := C.FT_New_Face(ctx.ftLib, cPath, 0, &face)
			C.free(unsafe.Pointer(cPath))
			if rc != 0 {
				return nil
			}

			family := C.GoString(face.family_name)
			style := C.GoString(face.style_name)
			isColorFace := int64(face.face_flags)&
				int64(C.FT_FACE_FLAG_COLOR) != 0
			C.FT_Done_Face(face)

			if family == "" {
				return nil
			}

			key := family + "-" + style
			// Earlier dirs (user fonts) take priority.
			if _, exists := ctx.fontPaths[key]; !exists {
				ctx.fontPaths[key] = path
			}
			if _, exists := ctx.fontPaths[family]; !exists {
				ctx.fontPaths[family] = path
			}

			// Register generic aliases on first match.
			lower = strings.ToLower(family)
			switch {
			case strings.Contains(lower, "dejavu sans mono") ||
				strings.Contains(lower, "liberation mono") ||
				strings.Contains(lower, "noto mono"):
				if _, exists := ctx.fontPaths["monospace"]; !exists {
					ctx.fontPaths["monospace"] = path
				}
			case strings.Contains(lower, "dejavu serif") ||
				strings.Contains(lower, "liberation serif") ||
				strings.Contains(lower, "noto serif"):
				if _, exists := ctx.fontPaths["serif"]; !exists {
					ctx.fontPaths["serif"] = path
				}
			case strings.Contains(lower, "dejavu sans") ||
				strings.Contains(lower, "liberation sans") ||
				strings.Contains(lower, "noto sans"):
				if _, exists := ctx.fontPaths["sans-serif"]; !exists {
					ctx.fontPaths["sans-serif"] = path
				}
			}

			// Color-emoji fonts (CBDT/CBLC) are tracked separately so
			// the renderer can pick a true color font over a monochrome
			// emoji font (e.g. Noto Emoji) that also covers the glyph.
			if isColorFace && !seenColor[family] {
				colorPaths = append(colorPaths, path)
				seenColor[family] = true
				// Already leads the general fallback list; don't re-add.
				seenFallback[family] = true
			}

			// Collect script-fallback fonts (one path per family).
			if !seenFallback[family] {
				switch {
				case isEmojiFamily(lower):
					emojiPaths = append(emojiPaths, path)
					seenFallback[family] = true
				case isCJKFamily(lower):
					cjkPaths = append(cjkPaths, path)
					seenFallback[family] = true
				}
			}
			return nil
		})
	}

	// Color emoji leads the general fallback list so colored glyphs win
	// over monochrome coverage; colorPaths drives the render color path.
	ctx.colorPaths = colorPaths
	ctx.fallbackPaths = append(ctx.fallbackPaths, colorPaths...)
	ctx.fallbackPaths = append(ctx.fallbackPaths, emojiPaths...)
	ctx.fallbackPaths = append(ctx.fallbackPaths, cjkPaths...)
}

// isEmojiFamily reports whether a lower-cased family name is a color
// emoji font (e.g. "Noto Color Emoji").
func isEmojiFamily(lowerFamily string) bool {
	return strings.Contains(lowerFamily, "emoji")
}

// isCJKFamily reports whether a lower-cased family name covers CJK
// scripts, matching the common Linux CJK font packages.
func isCJKFamily(lowerFamily string) bool {
	needles := []string{
		"cjk", "han",
		"wenquanyi", "wqy", "uming", "ukai", "ar pl",
		"noto sans jp", "noto serif jp",
		"noto sans sc", "noto sans tc", "noto sans kr",
		"droid sans fallback", "nanum", "ipa", "vl gothic", "takao",
	}
	for _, n := range needles {
		if strings.Contains(lowerFamily, n) {
			return true
		}
	}
	return false
}
