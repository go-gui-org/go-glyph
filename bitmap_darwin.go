//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreGraphics/CoreGraphics.h>
#include <CoreText/CoreText.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

// GlyphRenderCtx holds shared state for glyph rasterization.
typedef struct {
    CGContextRef ctx;
    void *data;
    CTLineRef line;
    CFAttributedStringRef astr;
    CFDictionaryRef attrs;
    CFStringRef str;
    CTFontRef font;
    CGFloat minX, minY;
} GlyphRenderCtx;

// cgSetupGlyph creates font, attributed string, measures bounds,
// and allocates a bitmap context. Returns zeroed ctx on failure.
static GlyphRenderCtx cgSetupGlyph(const char *text,
    const char *family, CGFloat fontSize, bool bold, bool italic,
    int pad, int *outW, int *outH, int *outLeft, int *outTop) {

    GlyphRenderCtx r = {0};
    @autoreleasepool {

    CFStringRef fam = CFStringCreateWithCString(NULL, family,
        kCFStringEncodingUTF8);
    CTFontRef baseFont = CTFontCreateWithName(fam, fontSize, NULL);
    CFRelease(fam);

    CTFontRef font = baseFont;
    if (bold || italic) {
        CTFontSymbolicTraits traits = 0;
        if (bold) traits |= kCTFontBoldTrait;
        if (italic) traits |= kCTFontItalicTrait;
        CTFontRef styled = CTFontCreateCopyWithSymbolicTraits(
            baseFont, fontSize, NULL, traits, traits);
        if (styled) {
            CFRelease(baseFont);
            font = styled;
        }
    }

    CFStringRef str = CFStringCreateWithCString(NULL, text,
        kCFStringEncodingUTF8);
    if (!str) {
        CFRelease(font);
        *outW = 0; *outH = 0;
        return r;
    }

    CFStringRef keys[] = { kCTFontAttributeName };
    CFTypeRef vals[] = { font };
    CFDictionaryRef attrs = CFDictionaryCreate(NULL,
        (const void **)keys, (const void **)vals, 1,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    CFAttributedStringRef astr = CFAttributedStringCreate(
        NULL, str, attrs);
    CTLineRef line = CTLineCreateWithAttributedString(astr);

    CGRect bounds = CTLineGetBoundsWithOptions(line,
        kCTLineBoundsUseGlyphPathBounds);
    CGFloat minX = floor(CGRectGetMinX(bounds));
    CGFloat maxX = ceil(CGRectGetMaxX(bounds));
    CGFloat minY = floor(CGRectGetMinY(bounds));
    CGFloat maxY = ceil(CGRectGetMaxY(bounds));
    int w = (int)(maxX - minX) + pad * 2;
    int h = (int)(maxY - minY) + pad * 2;
    if (w < 1) w = 1;
    if (h < 1) h = 1;
    if (w > 256) w = 256;
    if (h > 256) h = 256;

    *outW = w;
    *outH = h;
    *outLeft = (int)minX - pad;
    *outTop = (int)maxY + pad;

    size_t bytesPerRow = w * 4;
    void *data = calloc(h, bytesPerRow);
    if (!data) {
        CFRelease(line);
        CFRelease(astr);
        CFRelease(attrs);
        CFRelease(str);
        CFRelease(font);
        *outW = 0; *outH = 0;
        return r;
    }

    CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate(data, w, h, 8,
        bytesPerRow, cs,
        kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
    CGColorSpaceRelease(cs);

    if (!ctx) {
        free(data);
        CFRelease(line);
        CFRelease(astr);
        CFRelease(attrs);
        CFRelease(str);
        CFRelease(font);
        *outW = 0; *outH = 0;
        return r;
    }

    r.ctx = ctx;
    r.data = data;
    r.line = line;
    r.astr = astr;
    r.attrs = attrs;
    r.str = str;
    r.font = font;
    r.minX = minX;
    r.minY = minY;
    return r;
    } // @autoreleasepool
}

static void cgCleanupGlyph(GlyphRenderCtx *r) {
    CGContextRelease(r->ctx);
    CFRelease(r->line);
    CFRelease(r->astr);
    CFRelease(r->attrs);
    CFRelease(r->str);
    CFRelease(r->font);
}

// cgRenderGlyph rasterizes a text string into an RGBA bitmap.
// Returns bitmap data (caller must free), width and height.
static void* cgRenderGlyph(const char *text, const char *family,
    CGFloat fontSize, bool bold, bool italic, CGFloat subpixelShift,
    int *outW, int *outH, int *outLeft, int *outTop) {
    @autoreleasepool {

    const int pad = 2;
    GlyphRenderCtx r = cgSetupGlyph(text, family, fontSize,
        bold, italic, pad, outW, outH, outLeft, outTop);
    if (!r.ctx) return NULL;

    CGContextSetRGBFillColor(r.ctx, 1, 1, 1, 1);
    CGContextSetTextDrawingMode(r.ctx, kCGTextFill);

    CGFloat baselineY = -r.minY + pad;
    CGFloat baselineX = -r.minX + pad + subpixelShift;
    CGContextSetTextPosition(r.ctx, baselineX, baselineY);
    CTLineDraw(r.line, r.ctx);

    cgCleanupGlyph(&r);
    return r.data;
    } // @autoreleasepool
}

// cgRenderStrokedGlyph rasterizes a stroked text string.
static void* cgRenderStrokedGlyph(const char *text,
    const char *family, CGFloat fontSize,
    bool bold, bool italic, CGFloat strokeWidth, CGFloat subpixelShift,
    int *outW, int *outH, int *outLeft, int *outTop) {
    @autoreleasepool {

    int pad = (int)ceil(strokeWidth) + 4;
    GlyphRenderCtx r = cgSetupGlyph(text, family, fontSize,
        bold, italic, pad, outW, outH, outLeft, outTop);
    if (!r.ctx) return NULL;

    CGContextSetRGBStrokeColor(r.ctx, 1, 1, 1, 1);
    CGContextSetRGBFillColor(r.ctx, 0, 0, 0, 0);
    CGContextSetLineWidth(r.ctx, strokeWidth);
    CGContextSetLineJoin(r.ctx, kCGLineJoinRound);
    CGContextSetLineCap(r.ctx, kCGLineCapRound);
    CGContextSetTextDrawingMode(r.ctx, kCGTextStroke);

    CGFloat baselineY = -r.minY + pad;
    CGFloat baselineX = -r.minX + pad + subpixelShift;
    CGContextSetTextPosition(r.ctx, baselineX, baselineY);
    CTLineDraw(r.line, r.ctx);

    cgCleanupGlyph(&r);
    return r.data;
    } // @autoreleasepool
}
*/
import "C"
import (
	"unsafe"
)

// loadGlyphCG rasterizes a character using Core Graphics.
func loadGlyphCG(atlas *GlyphAtlas, ch string, item Item,
	subpixelBin int, scaleFactor float32) (LoadGlyphResult, error) {

	family, fontSize, bold, italic := resolveCTFontParams(
		item.Style, scaleFactor)

	cText := C.CString(ch)
	defer C.free(unsafe.Pointer(cText))
	cFamily := C.CString(family)
	defer C.free(unsafe.Pointer(cFamily))

	var w, h, left, top C.int
	subpixelShift := C.CGFloat(float64(subpixelBin) / 4.0)

	data := C.cgRenderGlyph(cText, cFamily,
		C.CGFloat(fontSize), C.bool(bold), C.bool(italic),
		subpixelShift,
		&w, &h, &left, &top)

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

	// Color glyphs (e.g. Apple Color Emoji sbix) carry RGB
	// information that must be preserved. Monochrome glyphs are
	// rendered white-on-transparent here so the renderer can tint
	// them with item.Color at draw time. Detect color by scanning
	// for any pixel whose channels disagree; if found, leave the
	// bitmap as the rasterizer wrote it.
	hasColor := false
	for i := 0; i < len(goData); i += 4 {
		r, g, b := goData[i+0], goData[i+1], goData[i+2]
		if r != g || g != b {
			hasColor = true
			break
		}
	}
	if !hasColor {
		for i := 0; i < len(goData); i += 4 {
			a := goData[i+3]
			goData[i+0] = 255
			goData[i+1] = 255
			goData[i+2] = 255
			goData[i+3] = a
		}
	}

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

// loadStrokedGlyphCG rasterizes a stroked character.
func loadStrokedGlyphCG(atlas *GlyphAtlas, ch string, item Item,
	strokeWidth float32, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	family, fontSize, bold, italic := resolveCTFontParams(
		item.Style, scaleFactor)

	cText := C.CString(ch)
	defer C.free(unsafe.Pointer(cText))
	cFamily := C.CString(family)
	defer C.free(unsafe.Pointer(cFamily))

	var w, h, left, top C.int
	subpixelShift := C.CGFloat(float64(subpixelBin) / 4.0)

	data := C.cgRenderStrokedGlyph(cText, cFamily,
		C.CGFloat(fontSize), C.bool(bold), C.bool(italic),
		C.CGFloat(float64(strokeWidth)*float64(scaleFactor)),
		subpixelShift,
		&w, &h, &left, &top)

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

	for i := 0; i < len(goData); i += 4 {
		a := goData[i+3]
		goData[i+0] = 255
		goData[i+1] = 255
		goData[i+2] = 255
		goData[i+3] = a
	}

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
