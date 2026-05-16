//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreText/CoreText.h>
#include <CoreFoundation/CoreFoundation.h>

// ctRegisterFontURL registers a font file with Core Text.
static bool ctRegisterFontFile(const char *path) {
    CFStringRef pathStr = CFStringCreateWithCString(NULL, path,
        kCFStringEncodingUTF8);
    CFURLRef url = CFURLCreateWithFileSystemPath(NULL, pathStr,
        kCFURLPOSIXPathStyle, false);
    CFRelease(pathStr);
    if (!url) return false;
    bool ok = CTFontManagerRegisterFontsForURL(
        url, kCTFontManagerScopeProcess, NULL);
    CFRelease(url);
    return ok;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Context holds Core Text state for text shaping on iOS.
//
// Not safe for concurrent use.
type Context struct {
	scaleFactor float32
	scaleInv    float32
	metrics     ctMetricsCache
}

// NewContext creates an iOS text context.
func NewContext(scaleFactor float32) (*Context, error) {
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}
	return &Context{
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newCTMetricsCache(32),
	}, nil
}

// Free releases resources.
func (ctx *Context) Free() {
	ctx.metrics = ctMetricsCache{}
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile registers a font file with Core Text.
func (ctx *Context) AddFontFile(path string) error {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	if !C.ctRegisterFontFile(cs) {
		return fmt.Errorf("CTFontManagerRegisterFontsForURL failed for %q", path)
	}
	return nil
}

// fontMetrics returns cached ascent/descent/leading for the given
// style+font in raw Core Text units. On cache miss, queries the
// CTFont and stores the result. Keying on resolved style params (not
// the CTFontRef pointer) keeps the cache correct across font
// create/close cycles that may reuse pointer addresses.
// Use metricsForStyle when no CTFont has been created yet.
func (ctx *Context) fontMetrics(style TextStyle, font ctFont) ctFontMetrics {
	family, size, bold, italic := resolveCTFontParams(style, ctx.scaleFactor)
	key := ctMetricsKey{family: family, size: size, bold: bold, italic: italic}
	if m, ok := ctx.metrics.get(key); ok {
		return m
	}
	a, d, l := font.metrics()
	m := ctFontMetrics{ascent: a, descent: d, leading: l}
	ctx.metrics.put(key, m)
	return m
}

// metricsForStyle returns cached font metrics, creating a temporary CTFont
// only on cache miss. Avoids the CGo newCTFont allocation on cache hits.
func (ctx *Context) metricsForStyle(style TextStyle) (ctFontMetrics, error) {
	family, size, bold, italic := resolveCTFontParams(style, ctx.scaleFactor)
	key := ctMetricsKey{family: family, size: size, bold: bold, italic: italic}
	if m, ok := ctx.metrics.get(key); ok {
		return m, nil
	}
	font := newCTFont(style, ctx.scaleFactor)
	if font.ref == 0 {
		return ctFontMetrics{}, fmt.Errorf("failed to create CTFont")
	}
	defer font.close()
	a, d, l := font.metrics()
	m := ctFontMetrics{ascent: a, descent: d, leading: l}
	ctx.metrics.put(key, m)
	return m, nil
}

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	m, err := ctx.metricsForStyle(cfg.Style)
	if err != nil {
		return 0, err
	}
	return float32(m.ascent+m.descent) / ctx.scaleFactor, nil
}

// FontMetrics returns detailed metrics in logical pixels.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	m, err := ctx.metricsForStyle(cfg.Style)
	if err != nil {
		return TextMetrics{}, err
	}
	sf := float64(ctx.scaleFactor)
	asc := float32(m.ascent / sf)
	dsc := float32(m.descent / sf)
	return TextMetrics{
		Ascender:  asc,
		Descender: dsc,
		Height:    asc + dsc,
		LineGap:   float32(m.leading / sf),
	}, nil
}

// ResolveFontName returns the resolved Darwin (macOS / iOS) font
// family name.
func (ctx *Context) ResolveFontName(fontDescStr string) (string, error) {
	family := resolveFontFamilyDarwin(fontDescStr)
	return family, nil
}

// createFontDescription builds a ctFont from TextStyle. Caller
// must call close().
func (ctx *Context) createCTFont(style TextStyle) ctFont {
	return newCTFont(style, ctx.scaleFactor)
}
