//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

/*
#include <pango/pango.h>
#include <pango/pangoft2.h>
#include <pango/pangofc-fontmap.h>
#include <glib-object.h>
#include <ft2build.h>
#include FT_FREETYPE_H
#include <fontconfig/fontconfig.h>
*/
import "C"
import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"
)

// fontHeightKey caches the final FontHeight result, keyed by the
// TextStyle fields that affect the font description. Avoids the
// full createFontDescription → LoadFont → GetMetrics CGo chain
// on repeated calls with the same style.
type fontHeightKey struct {
	FontName string
	Typeface Typeface
	Size     float32
}

// Context holds the Pango and FreeType state needed for text
// shaping. Keep alive for application duration.
//
// Not safe for concurrent use.
type Context struct {
	ftLib       FTLibrary
	fontMap     PangoFontMapW
	pangoCtx    PangoContextW
	scaleFactor float32
	scaleInv    float32
	metrics     metricsCache
	fontHeights map[fontHeightKey]float32
}

// NewContext initializes FreeType, Pango font map, and Pango context.
// scaleFactor is the display DPI scale (1.0 = 72 DPI, 2.0 = Retina).
func NewContext(scaleFactor float32) (*Context, error) {
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}

	ftLib, err := InitFreeType()
	if err != nil {
		return nil, err
	}

	fontMap := NewPangoFT2FontMap()
	if fontMap.ptr == nil {
		ftLib.Close()
		return nil, fmt.Errorf("failed to create Pango font map")
	}
	// 72 DPI * scale => 1 pt == 1 logical px.
	dpi := 72.0 * float64(scaleFactor)
	fontMap.SetResolution(dpi, dpi)

	pangoCtx := fontMap.CreateContext()
	if pangoCtx.ptr == nil {
		fontMap.Close()
		ftLib.Close()
		return nil, fmt.Errorf("failed to create Pango context")
	}

	ctx := &Context{
		ftLib:       ftLib,
		fontMap:     fontMap,
		pangoCtx:    pangoCtx,
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
	}

	// Auto-register system fonts on macOS.
	if runtime.GOOS == "darwin" {
		ctx.registerMacOSFonts()
	}

	return ctx, nil
}

// Free releases all Pango and FreeType resources.
func (ctx *Context) Free() {
	ctx.pangoCtx.Close()
	ctx.fontMap.Close()
	ctx.ftLib.Close()
	ctx.pangoCtx = PangoContextW{}
	ctx.fontMap = PangoFontMapW{}
	ctx.ftLib = FTLibrary{}
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile loads a font file via FontConfig.
func (ctx *Context) AddFontFile(path string) error {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))

	config := C.FcConfigGetCurrent()
	if config == nil {
		config = C.FcInitLoadConfigAndFonts()
		if config == nil {
			return fmt.Errorf("fontconfig initialization failed")
		}
	}
	if C.FcConfigAppFontAddFile(config, (*C.FcChar8)(unsafe.Pointer(cs))) == 0 {
		return fmt.Errorf("FcConfigAppFontAddFile failed for %q", path)
	}
	PangoFCFontMapConfigChanged(ctx.fontMap)
	return nil
}

// FontHeight returns ascent + descent in logical pixels for the
// font described by cfg.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	// Fast path: return cached result when no variation axes are
	// in play (the common case). This skips the entire
	// createFontDescription → LoadFont → GetMetrics CGo chain.
	hasAxes := cfg.Style.Features != nil &&
		len(cfg.Style.Features.VariationAxes) > 0
	if !hasAxes {
		key := fontHeightKey{
			FontName: cfg.Style.FontName,
			Typeface: cfg.Style.Typeface,
			Size:     cfg.Style.Size,
		}
		if h, ok := ctx.fontHeights[key]; ok {
			return h, nil
		}
	}

	desc := ctx.createFontDescription(cfg.Style)
	if desc.ptr == nil {
		return 0, fmt.Errorf("failed to create font description")
	}
	defer desc.Close()

	font := PangoContextLoadFont(ctx.pangoCtx.ptr, desc.ptr)
	if font.ptr == nil {
		return 0, fmt.Errorf("failed to load font")
	}
	defer font.Close()

	face := PangoFT2FontGetFace(font.ptr)
	if face == nil {
		return 0, fmt.Errorf("FreeType face unavailable")
	}
	sizeUnits := PangoFontDescGetSize(desc.ptr)
	cacheKey := uint64(uintptr(unsafe.Pointer(face))) ^ (uint64(sizeUnits) << 32)

	if entry, ok := ctx.metrics.get(cacheKey); ok {
		h := (float32(entry.Ascent+entry.Descent) / float32(PangoScale)) / ctx.scaleFactor
		if !hasAxes {
			ctx.cacheFontHeight(cfg.Style, h)
		}
		return h, nil
	}

	lang := PangoGetDefaultLanguage()
	m := PangoFontGetMetrics(font.ptr, lang)
	if m.ptr == nil {
		return 0, fmt.Errorf("failed to get font metrics")
	}
	defer m.Close()

	ascent := PangoFontMetricsGetAscent(m.ptr)
	descent := PangoFontMetricsGetDescent(m.ptr)

	ctx.metrics.put(cacheKey, FontMetricsEntry{
		Ascent:  ascent,
		Descent: descent,
	})

	h := (float32(ascent+descent) / float32(PangoScale)) / ctx.scaleFactor
	if !hasAxes {
		ctx.cacheFontHeight(cfg.Style, h)
	}
	return h, nil
}

func (ctx *Context) cacheFontHeight(style TextStyle, h float32) {
	if ctx.fontHeights == nil {
		ctx.fontHeights = make(map[fontHeightKey]float32)
	}
	ctx.fontHeights[fontHeightKey{
		FontName: style.FontName,
		Typeface: style.Typeface,
		Size:     style.Size,
	}] = h
}

// FontMetrics returns detailed metrics (ascender, descender, height,
// line gap) in logical pixels.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	desc := ctx.createFontDescription(cfg.Style)
	if desc.ptr == nil {
		return TextMetrics{}, fmt.Errorf("failed to create font description")
	}
	defer desc.Close()

	font := PangoContextLoadFont(ctx.pangoCtx.ptr, desc.ptr)
	if font.ptr == nil {
		return TextMetrics{}, fmt.Errorf("failed to load font")
	}
	defer font.Close()

	face := PangoFT2FontGetFace(font.ptr)
	if face == nil {
		return TextMetrics{}, fmt.Errorf("FreeType face unavailable")
	}
	sizeUnits := PangoFontDescGetSize(desc.ptr)
	cacheKey := uint64(uintptr(unsafe.Pointer(face))) ^ (uint64(sizeUnits) << 32)

	scale := float32(PangoScale) * ctx.scaleFactor

	if entry, ok := ctx.metrics.get(cacheKey); ok {
		asc := float32(entry.Ascent) / scale
		desc := float32(entry.Descent) / scale
		return TextMetrics{
			Ascender:  asc,
			Descender: desc,
			Height:    asc + desc,
			LineGap:   float32(entry.LineGap) / scale,
		}, nil
	}

	lang := PangoGetDefaultLanguage()
	m := PangoFontGetMetrics(font.ptr, lang)
	if m.ptr == nil {
		return TextMetrics{}, fmt.Errorf("failed to get font metrics")
	}
	defer m.Close()

	ascent := PangoFontMetricsGetAscent(m.ptr)
	descent := PangoFontMetricsGetDescent(m.ptr)

	ctx.metrics.put(cacheKey, FontMetricsEntry{
		Ascent:  ascent,
		Descent: descent,
	})

	asc := float32(ascent) / scale
	dsc := float32(descent) / scale
	return TextMetrics{
		Ascender:  asc,
		Descender: dsc,
		Height:    asc + dsc,
	}, nil
}

// ResolveFontName returns the FreeType family name that Pango
// resolves for the given font description string.
func (ctx *Context) ResolveFontName(fontDescStr string) (string, error) {
	desc := NewPangoFontDescFromString(fontDescStr)
	if desc.ptr == nil {
		return "", fmt.Errorf("invalid font description %q", fontDescStr)
	}
	defer desc.Close()

	fam := PangoFontDescGetFamily(desc.ptr)
	resolved := resolveFamilyAlias(fam, isMonospaceName(fam))
	PangoFontDescSetFamily(desc.ptr, resolved)

	font := PangoContextLoadFont(ctx.pangoCtx.ptr, desc.ptr)
	if font.ptr == nil {
		return "", fmt.Errorf("could not load font %q", fontDescStr)
	}
	defer font.Close()

	face := PangoFT2FontGetFace(font.ptr)
	if face == nil {
		return "", fmt.Errorf("could not get FT_Face for %q", fontDescStr)
	}
	return C.GoString(face.family_name), nil
}

// createFontDescription builds a PangoFontDescription from TextStyle.
// Caller must Close the result.
func (ctx *Context) createFontDescription(style TextStyle) PangoFontDescW {
	desc := NewPangoFontDescFromString(style.FontName)
	if desc.ptr == nil {
		return desc
	}

	// Resolve family aliases.
	fam := PangoFontDescGetFamily(desc.ptr)
	resolved := resolveFamilyAlias(fam, isMonospaceName(fam))
	PangoFontDescSetFamily(desc.ptr, resolved)

	// Typeface override.
	applyTypeface(desc.ptr, style.Typeface)

	// Variable font axes.
	if style.Features != nil && len(style.Features.VariationAxes) > 0 {
		var sb strings.Builder
		for i, a := range style.Features.VariationAxes {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, "%s=%g", a.Tag, a.Value)
		}
		desc.SetVariations(sb.String())
	}

	// Explicit size override.
	if style.Size > 0 {
		desc.SetSize(int(style.Size * float32(PangoScale)))
	}

	return desc
}

// applyTypeface sets weight/style on a font description.
func applyTypeface(desc *C.PangoFontDescription, tf Typeface) {
	switch tf {
	case TypefaceRegular:
		// no-op
	case TypefaceBold:
		C.pango_font_description_set_weight(desc, C.PANGO_WEIGHT_BOLD)
	case TypefaceItalic:
		C.pango_font_description_set_style(desc, C.PANGO_STYLE_ITALIC)
	case TypefaceBoldItalic:
		C.pango_font_description_set_weight(desc, C.PANGO_WEIGHT_BOLD)
		C.pango_font_description_set_style(desc, C.PANGO_STYLE_ITALIC)
	}
}

// resolveFamilyAlias appends platform fallback families. When monospace
// is true, only monospace fallbacks are appended so mixed-pitch rendering
// is avoided for code/terminal fonts.
func resolveFamilyAlias(fam string, monospace bool) string {
	var aliases []string
	switch runtime.GOOS {
	case "darwin":
		if monospace {
			aliases = []string{"Menlo", "Courier New"}
		} else {
			aliases = []string{"SF Pro", "System Font"}
		}
	case "windows":
		if monospace {
			aliases = []string{"Consolas", "Courier New"}
		} else {
			aliases = []string{"Segoe UI"}
		}
	default:
		if monospace {
			aliases = []string{"monospace"}
		} else {
			aliases = []string{"Sans"}
		}
	}
	result := fam
	for _, a := range aliases {
		if strings.Contains(result, a) {
			continue
		}
		if len(result) > 0 {
			result += ", "
		}
		result += a
	}
	return result
}

// isMonospaceName reports whether the font family name is a known
// monospace font based on common naming conventions.
func isMonospaceName(family string) bool {
	lower := strings.ToLower(family)
	for _, kw := range []string{
		"mono", "courier", "consol", "code", "terminal",
		"typewriter", "fixed", "menlo", "monaco",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (ctx *Context) registerMacOSFonts() {
	config := C.FcConfigGetCurrent()
	if config == nil {
		config = C.FcInitLoadConfigAndFonts()
	}
	if config == nil {
		return
	}

	dirs := []string{
		"/System/Library/Fonts",
		"/Library/Fonts",
	}
	if home := os.Getenv("HOME"); home != "" {
		dirs = append(dirs, home+"/Library/Fonts")
	}

	for _, d := range dirs {
		cs := C.CString(d)
		C.FcConfigAppFontAddDir(config, (*C.FcChar8)(unsafe.Pointer(cs)))
		C.free(unsafe.Pointer(cs))
	}
	PangoFCFontMapConfigChanged(ctx.fontMap)
}
