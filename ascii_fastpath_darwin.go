//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreText/CoreText.h>
#include <CoreGraphics/CoreGraphics.h>

// ctFontIsMonospace reports whether font carries the monospace symbolic
// trait. Robust across families (name-independent).
static int ctFontIsMonospace(CTFontRef font) {
    if (font == NULL) return 0;
    CTFontSymbolicTraits t = CTFontGetSymbolicTraits(font);
    return (t & kCTFontMonoSpaceTrait) != 0 ? 1 : 0;
}

// ctASCIIGlyphAdvances fills glyphsOut[0..n) with the CGGlyph for each
// UniChar in chars[0..n) and advOut[0..n) with each glyph's horizontal
// advance. Pure cmap lookup + advance query — no attributed string, no
// CTLine, no shaping. The CTFontGetGlyphsForCharacters return value is
// intentionally ignored: control code points (0x00-0x1F, 0x7F) legitimately
// have no glyph (returning false for the batch) but map to 0, and callers
// only consult the printable range [0x20,0x7E]. The Go side validates a
// representative printable glyph instead.
static int ctASCIIGlyphAdvances(CTFontRef font, const unsigned short *chars,
                                int n, unsigned short *glyphsOut,
                                double *advOut) {
    if (font == NULL || n <= 0 || n > 128) return 0;
    CGGlyph glyphs[128];
    CTFontGetGlyphsForCharacters(font, (const UniChar *)chars, glyphs, n);
    CGSize adv[128];
    CTFontGetAdvancesForGlyphs(font, kCTFontOrientationHorizontal,
                               glyphs, adv, n);
    for (int i = 0; i < n; i++) {
        glyphsOut[i] = (unsigned short)glyphs[i];
        advOut[i] = adv[i].width;
    }
    return 1;
}
*/
import "C"

// asciiTableCap bounds the per-(family,size,traits) ASCII glyph/advance
// cache. Terminals touch a handful of font configs; the cap only guards
// pathological churn (mirrors ctMetricsCache policy).
const asciiTableCap = 32

// asciiGlyphTable caches the CGGlyph and horizontal advance for the 128
// ASCII code points of one resolved font config. ok is false when the
// font is not monospace or lacks ASCII glyphs — a cached "decline".
type asciiGlyphTable struct {
	glyphs [128]uint16
	adv    [128]float64
	ok     bool
}

// hasASCIIAlteringFeatures reports whether the FontFeatures enable any
// OpenType feature at all. The ASCII fast-path cache is keyed on
// (family, size, bold, italic) and does NOT include features. Any
// active feature — ligature, stylistic set, character variant, etc. —
// can produce different glyphs for the same code point, so we bail out
// to the full shaper whenever features are present.
func hasASCIIAlteringFeatures(f *FontFeatures) bool {
	if f == nil {
		return false
	}
	for _, feat := range f.OpenTypeFeatures {
		if feat.Value != 0 {
			return true
		}
	}
	return false
}

// asciiMonospaceEligible screens a run cheaply (no font access) for the
// fast path: non-empty, horizontal, non-markup, every byte printable
// ASCII, and no ligature features active. ASCII is unconditionally LTR,
// so no bidi check is needed. The monospace-font gate is applied later
// (and cached) in asciiTable, which needs the CTFontRef.
func asciiMonospaceEligible(cfg TextConfig, text string) bool {
	if len(text) == 0 || cfg.UseMarkup {
		return false
	}
	if cfg.Orientation == OrientationVertical {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 || text[i] > 0x7e {
			return false
		}
	}
	return !hasASCIIAlteringFeatures(cfg.Style.Features)
}

// asciiTable returns the cached ASCII glyph/advance table for style,
// building it on miss from font. Keyed on resolved (family, size,
// traits) like ctMetricsCache, so entries survive CTFontRef churn.
func (ctx *Context) asciiTable(style TextStyle, font ctFont) *asciiGlyphTable {
	family, size, bold, italic := resolveCTFontParams(style, ctx.scaleFactor)
	key := ctMetricsKey{family: family, size: size, bold: bold, italic: italic}
	if t, ok := ctx.asciiTables[key]; ok {
		return t
	}
	t := buildASCIITable(font)
	if ctx.asciiTables == nil {
		ctx.asciiTables = make(map[ctMetricsKey]*asciiGlyphTable, asciiTableCap)
	}
	if len(ctx.asciiTables) >= asciiTableCap {
		for evict := range ctx.asciiTables {
			delete(ctx.asciiTables, evict)
			break
		}
	}
	ctx.asciiTables[key] = t
	return t
}

// buildASCIITable queries font for the 128 ASCII glyphs + advances. Non-
// monospace fonts (or fonts missing ASCII glyphs) yield ok=false.
func buildASCIITable(font ctFont) *asciiGlyphTable {
	t := &asciiGlyphTable{}
	if font.ref == 0 || C.ctFontIsMonospace(font.ref) == 0 {
		return t
	}
	var chars [128]C.ushort
	for i := range chars {
		chars[i] = C.ushort(i)
	}
	var glyphs [128]C.ushort
	var adv [128]C.double
	if C.ctASCIIGlyphAdvances(font.ref, &chars[0], 128,
		&glyphs[0], &adv[0]) == 0 {
		return t
	}
	for i := 0; i < 128; i++ {
		t.glyphs[i] = uint16(glyphs[i])
		t.adv[i] = float64(adv[i])
	}
	// Validate a representative printable glyph: a monospace font that
	// somehow lacks basic ASCII (glyph 0 / zero advance for 'M') is not
	// usable on the fast path. Control code points are expected to be 0.
	if t.glyphs['M'] == 0 || t.adv['M'] <= 0 {
		return t
	}
	t.ok = true
	return t
}

// tryASCIIClusters produces shaped clusters for a pure-ASCII monospace
// run without invoking the CoreText shaper, returning (clusters, true)
// on the fast path. One cluster per byte: glyph and fixed advance come
// from the cached ASCII table. Returns (nil, false) to signal the caller
// should fall back to shapeTextClusters. The advances match the shaper's
// (same CTFont), so the assembled Layout is equivalent.
func (ctx *Context) tryASCIIClusters(font ctFont, cfg TextConfig,
	text string) ([]shapedCluster, bool) {
	if !asciiMonospaceEligible(cfg, text) {
		return nil, false
	}
	t := ctx.asciiTable(cfg.Style, font)
	if t == nil || !t.ok {
		return nil, false
	}
	out := make([]shapedCluster, len(text))
	for i := 0; i < len(text); i++ {
		b := text[i]
		out[i] = shapedCluster{
			byteStart: i,
			byteLen:   1,
			advance:   t.adv[b],
			glyphID:   t.glyphs[b],
		}
	}
	return out, true
}
