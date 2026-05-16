//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreText/CoreText.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

// ctFontCreateFromStyle creates a CTFont from family name, size,
// and symbolic traits.
static CTFontRef ctCreateFont(const char *family, CGFloat size,
    bool bold, bool italic) {
    @autoreleasepool {
        CFStringRef fam = CFStringCreateWithCString(NULL, family,
            kCFStringEncodingUTF8);
        CTFontRef base = CTFontCreateWithName(fam, size, NULL);
        CFRelease(fam);
        if (!bold && !italic) return base;

        CTFontSymbolicTraits traits = 0;
        if (bold) traits |= kCTFontBoldTrait;
        if (italic) traits |= kCTFontItalicTrait;
        CTFontRef styled = CTFontCreateCopyWithSymbolicTraits(
            base, size, NULL, traits, traits);
        if (styled) {
            CFRelease(base);
            return styled;
        }
        // Trait application failed; return base font as-is.
        return base;
    }
}

// ctFontGetMetrics returns ascent, descent, leading.
static void ctFontGetMetrics(CTFontRef font,
    CGFloat *ascent, CGFloat *descent, CGFloat *leading) {
    *ascent = CTFontGetAscent(font);
    *descent = CTFontGetDescent(font);
    *leading = CTFontGetLeading(font);
}

// ctApplyOpenTypeFeatures returns a copy of base with the supplied
// OpenType feature tags applied. tagsBlob is count*4 bytes of ASCII
// tag chars; values is count int32 enable values. Releases base on
// success and returns the styled copy. Returns base unchanged on
// failure.
static CTFontRef ctApplyOpenTypeFeatures(CTFontRef base,
    const char *tagsBlob, const int *values, int count) {
    if (!base || count <= 0) {
        return base;
    }
    @autoreleasepool {
    CFMutableArrayRef arr = CFArrayCreateMutable(NULL, count,
        &kCFTypeArrayCallBacks);
    for (int i = 0; i < count; i++) {
        char tag[5];
        memcpy(tag, tagsBlob + i*4, 4);
        tag[4] = 0;
        CFStringRef cTag = CFStringCreateWithCString(NULL, tag,
            kCFStringEncodingASCII);
        int v = values[i];
        CFNumberRef cVal = CFNumberCreate(NULL, kCFNumberIntType, &v);
        const void *keys[2] = {
            (const void *)kCTFontOpenTypeFeatureTag,
            (const void *)kCTFontOpenTypeFeatureValue,
        };
        const void *vals[2] = { cTag, cVal };
        CFDictionaryRef d = CFDictionaryCreate(NULL, keys, vals, 2,
            &kCFTypeDictionaryKeyCallBacks,
            &kCFTypeDictionaryValueCallBacks);
        CFArrayAppendValue(arr, d);
        CFRelease(cTag);
        CFRelease(cVal);
        CFRelease(d);
    }
    const void *attrKeys[1] = {
        (const void *)kCTFontFeatureSettingsAttribute,
    };
    const void *attrVals[1] = { arr };
    CFDictionaryRef attrs = CFDictionaryCreate(NULL,
        attrKeys, attrVals, 1,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    CTFontDescriptorRef desc = CTFontDescriptorCreateWithAttributes(attrs);
    CFRelease(arr);
    CFRelease(attrs);
    CTFontRef styled = CTFontCreateCopyWithAttributes(base,
        CTFontGetSize(base), NULL, desc);
    CFRelease(desc);
    if (styled) {
        CFRelease(base);
        return styled;
    }
    return base;
    } // @autoreleasepool
}

// ctApplyVariationAxes returns a copy of base with variable-font axis
// values applied via kCTFontVariationAttribute. tagsBlob is count*4
// bytes of ASCII tag chars; values is count floats. Releases base on
// success and returns the styled copy. Returns base unchanged on
// failure.
static CTFontRef ctApplyVariationAxes(CTFontRef base,
    const char *tagsBlob, const float *values, int count) {
    if (!base || count <= 0) {
        return base;
    }
    @autoreleasepool {
    CFMutableDictionaryRef dict = CFDictionaryCreateMutable(NULL, count,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    for (int i = 0; i < count; i++) {
        uint32_t tagBE = ((uint32_t)(unsigned char)tagsBlob[i*4+0])<<24
                       | ((uint32_t)(unsigned char)tagsBlob[i*4+1])<<16
                       | ((uint32_t)(unsigned char)tagsBlob[i*4+2])<<8
                       | ((uint32_t)(unsigned char)tagsBlob[i*4+3]);
        int tagInt = (int)tagBE;
        CFNumberRef cKey = CFNumberCreate(NULL, kCFNumberIntType, &tagInt);
        float v = values[i];
        CFNumberRef cVal = CFNumberCreate(NULL, kCFNumberFloatType, &v);
        CFDictionarySetValue(dict, cKey, cVal);
        CFRelease(cKey);
        CFRelease(cVal);
    }
    const void *attrKeys[1] = {
        (const void *)kCTFontVariationAttribute,
    };
    const void *attrVals[1] = { dict };
    CFDictionaryRef attrs = CFDictionaryCreate(NULL,
        attrKeys, attrVals, 1,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    CTFontDescriptorRef desc = CTFontDescriptorCreateWithAttributes(attrs);
    CFRelease(dict);
    CFRelease(attrs);
    CTFontRef styled = CTFontCreateCopyWithAttributes(base,
        CTFontGetSize(base), NULL, desc);
    CFRelease(desc);
    if (styled) {
        CFRelease(base);
        return styled;
    }
    return base;
    } // @autoreleasepool
}
*/
import "C"
import (
	"strings"
	"sync"
	"unsafe"
)

// parsedFontName memoizes one Pango-style font-name parse.
// resolvedFamily is post-alias-mapping, ready for CTFontCreateWithName.
// hasBold / hasItalic capture the embedded " bold" / " italic" hints
// that drive the trait override at the bottom of resolveCTFontParams.
type parsedFontName struct {
	resolvedFamily string
	size           float32
	hasBold        bool
	hasItalic      bool
}

// fontNameParseCacheMax caps the number of entries in fontNameParseCache.
// Prevents unbounded growth when callers supply many unique dynamic font names.
const fontNameParseCacheMax = 512

// fontNameParseCache memoizes parsedFontName by raw TextStyle.FontName.
// Terminal-style workloads churn one or two unique font names per
// session; reads use RLock and stay allocation-free.
var (
	fontNameParseMu    sync.RWMutex
	fontNameParseCache = map[string]parsedFontName{}
)

// lookupParsedFontName returns the cached parse for name, computing
// and storing it on first miss. Hot path is the RLock'd map read —
// zero allocations.
func lookupParsedFontName(name string) parsedFontName {
	fontNameParseMu.RLock()
	p, ok := fontNameParseCache[name]
	fontNameParseMu.RUnlock()
	if ok {
		return p
	}
	p = computeParsedFontName(name)
	fontNameParseMu.Lock()
	if len(fontNameParseCache) < fontNameParseCacheMax {
		fontNameParseCache[name] = p
	}
	fontNameParseMu.Unlock()
	return p
}

func computeParsedFontName(name string) parsedFontName {
	family := parseFamilyFromFontName(name)
	p := parsedFontName{
		resolvedFamily: resolveDarwinSystemFamily(family),
		size:           parseSizeFromFontName(name),
	}
	lower := strings.ToLower(name)
	p.hasBold = strings.Contains(lower, " bold")
	p.hasItalic = strings.Contains(lower, " italic")
	return p
}

// resolveDarwinSystemFamily maps generic Pango family names (sans,
// serif, monospace, …) to their macOS/iOS counterparts. Called once
// per unique fontName via the parse cache.
func resolveDarwinSystemFamily(family string) string {
	if family == "" {
		return ".AppleSystemUIFont"
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return ".AppleSystemUIFont"
	case "serif":
		return "New York"
	case "monospace", "mono":
		return "SF Mono"
	default:
		return family
	}
}

// ctFont wraps a CTFontRef with a Go-friendly interface.
type ctFont struct {
	ref C.CTFontRef
}

func resolveCTFontParams(style TextStyle, scaleFactor float32) (
	family string, size float64, bold, italic bool,
) {
	parsed := lookupParsedFontName(style.FontName)
	family = parsed.resolvedFamily

	rawSize := style.Size
	if rawSize <= 0 {
		rawSize = parsed.size
	}
	if rawSize <= 0 {
		rawSize = 16
	}
	size = float64(rawSize) * float64(scaleFactor)

	bold = style.Typeface == TypefaceBold ||
		style.Typeface == TypefaceBoldItalic
	italic = style.Typeface == TypefaceItalic ||
		style.Typeface == TypefaceBoldItalic

	if !bold && parsed.hasBold {
		bold = true
	}
	if !italic && parsed.hasItalic {
		italic = true
	}

	return family, size, bold, italic
}

// newCTFont creates a Core Text font from a TextStyle.
func newCTFont(style TextStyle, scaleFactor float32) ctFont {
	family, size, bold, italic := resolveCTFontParams(style, scaleFactor)

	cFamily := C.CString(family)
	defer C.free(unsafe.Pointer(cFamily))

	ref := C.ctCreateFont(cFamily,
		C.CGFloat(size),
		C.bool(bold), C.bool(italic))
	if ref != 0 && style.Features != nil {
		ref = applyOpenTypeFeatures(ref, style.Features.OpenTypeFeatures)
		ref = applyVariationAxes(ref, style.Features.VariationAxes)
	}
	return ctFont{ref: ref}
}

// maxFontFeatures caps the number of OpenType features or variation axes
// accepted per font to bound CGo allocation and processing time.
const maxFontFeatures = 64

// applyOpenTypeFeatures applies user-supplied OpenType feature tags
// (e.g. liga, tnum, calt) to the font via
// kCTFontFeatureSettingsAttribute. Returns the styled CTFontRef
// (may equal base on failure or empty input). The "subs" / "sups"
// tags are skipped because most system fonts (.AppleSystemUIFont,
// SF Pro, Helvetica) lack OT subs/sups glyph substitution; the
// LayoutRichText size-scaling fallback provides visible sub/super
// rendering on any font.
func applyOpenTypeFeatures(base C.CTFontRef, feats []FontFeature) C.CTFontRef {
	if len(feats) == 0 {
		return base
	}
	if len(feats) > maxFontFeatures {
		feats = feats[:maxFontFeatures]
	}
	tags := make([]byte, 0, len(feats)*4)
	vals := make([]C.int, 0, len(feats))
	for _, f := range feats {
		if f.Tag == "subs" || f.Tag == "sups" {
			continue
		}
		t := f.Tag
		switch len(t) {
		case 4:
			tags = append(tags, t[0], t[1], t[2], t[3])
		case 0, 1, 2, 3:
			padded := []byte(t + "    ")[:4]
			tags = append(tags, padded...)
		default:
			tags = append(tags, t[0], t[1], t[2], t[3])
		}
		vals = append(vals, C.int(f.Value))
	}
	count := len(vals)
	if count == 0 {
		return base
	}
	return C.ctApplyOpenTypeFeatures(base,
		(*C.char)(unsafe.Pointer(&tags[0])),
		(*C.int)(unsafe.Pointer(&vals[0])),
		C.int(count))
}

// applyVariationAxes applies variable-font axis settings (e.g.
// wght=600, wdth=110, opsz=12) via kCTFontVariationAttribute.
// Returns the styled CTFontRef (may equal base on failure or empty
// input). Tags shorter than 4 chars are space-padded; longer tags
// are truncated to the first 4 chars.
func applyVariationAxes(base C.CTFontRef, axes []FontAxis) C.CTFontRef {
	if len(axes) == 0 {
		return base
	}
	if len(axes) > maxFontFeatures {
		axes = axes[:maxFontFeatures]
	}
	tags := make([]byte, 0, len(axes)*4)
	vals := make([]C.float, 0, len(axes))
	for _, a := range axes {
		t := a.Tag
		switch len(t) {
		case 4:
			tags = append(tags, t[0], t[1], t[2], t[3])
		case 0, 1, 2, 3:
			padded := []byte(t + "    ")[:4]
			tags = append(tags, padded...)
		default:
			tags = append(tags, t[0], t[1], t[2], t[3])
		}
		vals = append(vals, C.float(a.Value))
	}
	count := len(vals)
	if count == 0 {
		return base
	}
	return C.ctApplyVariationAxes(base,
		(*C.char)(unsafe.Pointer(&tags[0])),
		(*C.float)(unsafe.Pointer(&vals[0])),
		C.int(count))
}

// close releases the CTFont.
func (f *ctFont) close() {
	if f.ref != 0 {
		C.CFRelease(C.CFTypeRef(f.ref))
		f.ref = 0
	}
}

// metrics returns ascent, descent, leading in Core Text units.
// Returns zeros for a null ref rather than passing nil to CGo.
func (f ctFont) metrics() (ascent, descent, leading float64) {
	if f.ref == 0 {
		return 0, 0, 0
	}
	var a, d, l C.CGFloat
	C.ctFontGetMetrics(f.ref, &a, &d, &l)
	return float64(a), float64(d), float64(l)
}

// resolveFontFamilyDarwin maps generic Pango-style font names to
// macOS / iOS system font families. SF Mono ships in 10.15+; older
// macOS targets fall back to Menlo via CoreText's font matcher when
// "SF Mono" is unavailable. Result is memoized on fontName via
// the package-level parse cache.
func resolveFontFamilyDarwin(fontName string) string {
	return lookupParsedFontName(fontName).resolvedFamily
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
			// Skip fractional parsing for simplicity.
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
	// Strip trailing number (size).
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
