//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreText/CoreText.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

// ctMeasureString measures a string's width with the given font.
// @autoreleasepool drains the per-call helpers CoreText autoreleases
// internally (NSURL, NSDictionary, Swift URLComponents on macOS 26);
// without it they pile up forever on the Go-locked OS thread.
static CGFloat ctMeasureString(CTFontRef font, CFStringRef str) {
    @autoreleasepool {
        CFStringRef keys[] = { kCTFontAttributeName };
        CFTypeRef vals[] = { font };
        CFDictionaryRef attrs = CFDictionaryCreate(NULL,
            (const void **)keys, (const void **)vals, 1,
            &kCFTypeDictionaryKeyCallBacks,
            &kCFTypeDictionaryValueCallBacks);
        CFAttributedStringRef astr = CFAttributedStringCreate(
            NULL, str, attrs);
        CTLineRef line = CTLineCreateWithAttributedString(astr);
        CGFloat width = CTLineGetTypographicBounds(line, NULL, NULL, NULL);
        CFRelease(line);
        CFRelease(astr);
        CFRelease(attrs);
        return width;
    }
}

// ctMeasureCString is a convenience wrapper for C strings.
static CGFloat ctMeasureCString(CTFontRef font, const char *text) {
    @autoreleasepool {
        CFStringRef str = CFStringCreateWithCString(NULL, text,
            kCFStringEncodingUTF8);
        if (!str) return 0;
        CGFloat w = ctMeasureString(font, str);
        CFRelease(str);
        return w;
    }
}

// CTGlyphCluster describes one shaped glyph's character cluster span
// (in UTF-16 code-unit indices), typographic advance, and resolved
// glyph ID (after GSUB substitutions like calt/liga).
typedef struct {
    int     utf16Start;  // inclusive
    int     utf16End;    // exclusive
    CGFloat advance;
    CGGlyph glyphID;     // resolved CGGlyph after shaping
    int     isRTL;       // 1 if glyph is from a right-to-left run
} CTGlyphCluster;

// ctShapeGlyphClusters shapes utf8Text with font and fills out[] with
// per-glyph cluster info. Returns the number of shaped glyphs, which may
// be less than utf16Len when ligatures collapse multiple code units into
// one glyph. out must have capacity >= utf16Len.
static int ctShapeGlyphClusters(CTFontRef font, const char *utf8Text,
    int utf16Len, CTGlyphCluster *out) {
    @autoreleasepool {
    if (!font || !utf8Text || utf16Len <= 0 || !out) return 0;

    CFStringRef str = CFStringCreateWithCString(NULL, utf8Text,
        kCFStringEncodingUTF8);
    if (!str) return 0;

    CFStringRef attrKeys[] = { kCTFontAttributeName };
    CFTypeRef   attrVals[] = { font };
    CFDictionaryRef attrs = CFDictionaryCreate(NULL,
        (const void **)attrKeys, (const void **)attrVals, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    if (!attrs) { CFRelease(str); return 0; }
    CFAttributedStringRef astr = CFAttributedStringCreate(NULL, str, attrs);
    CFRelease(str);
    CFRelease(attrs);
    if (!astr) return 0;
    CTLineRef line = CTLineCreateWithAttributedString(astr);
    CFRelease(astr);
    if (!line) return 0;

    CFArrayRef runs = CTLineGetGlyphRuns(line);
    int runCount = (int)CFArrayGetCount(runs);
    int total = 0;

    // Primary font family name for fallback detection.
    CFStringRef primaryFamily = CTFontCopyFamilyName(font);

    for (int ri = 0; ri < runCount && total < utf16Len; ri++) {
        CTRunRef run = (CTRunRef)CFArrayGetValueAtIndex(runs, ri);
        int gc = (int)CTRunGetGlyphCount(run);
        if (gc == 0) continue;

        // Detect fallback font runs. When CoreText substitutes a glyph
        // from a different font, the run's font attribute differs from the
        // requested font. Glyph IDs from fallback fonts are meaningless in
        // the primary font (wrong glyph-ID space), so we clear them to
        // force the text-based rendering path and correct cache keys.
        bool isFallback = false;
        CFDictionaryRef runAttrs = CTRunGetAttributes(run);
        if (runAttrs && primaryFamily) {
            CTFontRef runFont = (CTFontRef)CFDictionaryGetValue(
                runAttrs, kCTFontAttributeName);
            if (runFont) {
                CFStringRef runFamily = CTFontCopyFamilyName(runFont);
                if (runFamily) {
                    isFallback = !CFEqual(primaryFamily, runFamily);
                    CFRelease(runFamily);
                }
            }
        }

        CFIndex  *idx    = (CFIndex  *)malloc(gc * sizeof(CFIndex));
        CGSize   *adv    = (CGSize   *)malloc(gc * sizeof(CGSize));
        CGGlyph  *glyphs = (CGGlyph  *)malloc(gc * sizeof(CGGlyph));
        if (!idx || !adv || !glyphs) {
            free(idx); free(adv); free(glyphs); continue;
        }

        CTRunGetStringIndices(run, CFRangeMake(0, 0), idx);
        CTRunGetAdvances(run,      CFRangeMake(0, 0), adv);
        CTRunGetGlyphs(run,        CFRangeMake(0, 0), glyphs);

        // Use the run's own string range for cluster boundaries.
        // This is more reliable than the next-run's first-glyph index,
        // especially for RTL runs where the first visual glyph has the
        // highest string index (making the old nextRunStart formula
        // produce inverted spans for every glyph in the run).
        CFRange runRange = CTRunGetStringRange(run);
        int runStart = (int)runRange.location;
        int runEnd   = (int)(runRange.location + runRange.length);
        if (runStart < 0)      runStart = 0;
        if (runEnd > utf16Len) runEnd   = utf16Len;

        // Sort glyph positions by ascending string index so the
        // cluster-end formula (end = next cluster's start) is correct
        // for both LTR and RTL.  For LTR indices are already ascending;
        // for RTL they are in descending visual order.
        // Insertion sort — gc is typically < 20.
        int *sortOrder = (int *)malloc(gc * sizeof(int));
        if (!sortOrder) { free(idx); free(adv); free(glyphs); continue; }
        for (int k = 0; k < gc; k++) sortOrder[k] = k;
        for (int k = 1; k < gc; k++) {
            int key = sortOrder[k];
            int j = k - 1;
            while (j >= 0 && idx[sortOrder[j]] > idx[key]) {
                sortOrder[j + 1] = sortOrder[j];
                j--;
            }
            sortOrder[j + 1] = key;
        }

        int isRTL = (CTRunGetStatus(run) & kCTRunStatusRightToLeft) ? 1 : 0;
        for (int k = 0; k < gc && total < utf16Len; k++) {
            int gi    = sortOrder[k];
            // Anchor the first sorted cluster to runStart so that RTL
            // ligatures (where idx[gi] > runStart) get the full span.
            int start = (k == 0) ? runStart : (int)idx[gi];
            int end   = (k + 1 < gc) ? (int)idx[sortOrder[k + 1]] : runEnd;
            if (end <= start) end = start + 1;
            if (end > utf16Len) end = utf16Len;
            out[total].utf16Start = start;
            out[total].utf16End   = end;
            out[total].advance    = adv[gi].width;
            out[total].glyphID    = isFallback ? 0 : glyphs[gi];
            out[total].isRTL      = isRTL;
            total++;
        }
        free(sortOrder);
        free(idx);
        free(adv);
        free(glyphs);
    }

    if (primaryFamily) CFRelease(primaryFamily);

    CFRelease(line);
    return total;
    } // @autoreleasepool
}
*/
import "C"
import (
	"cmp"
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"

	xbidi "golang.org/x/text/unicode/bidi"
)

// charFontOverride holds per-character font and position adjustments
// for rich text runs.
type charFontOverride struct {
	font        ctFont
	style       TextStyle
	yShift      float64
	xPad        float64
	objectWidth float64 // Inline-object reserved width (scaled px). 0 = not an object.
	objectID    string
}

// LayoutText shapes and wraps text using Core Text.
func (ctx *Context) LayoutText(text string, cfg TextConfig) (Layout, error) {
	if len(text) == 0 {
		return Layout{}, nil
	}
	if err := ValidateTextInput(text, MaxTextLength, "LayoutText"); err != nil {
		return Layout{}, err
	}

	font := ctx.createCTFont(cfg.Style)
	if font.ref == 0 {
		return Layout{}, fmt.Errorf("failed to create CTFont")
	}
	defer font.close()

	return ctx.buildLayout(text, font, cfg, nil), nil
}

// LayoutRichText shapes multi-styled text.
func (ctx *Context) LayoutRichText(rt RichText,
	cfg TextConfig) (Layout, error) {
	if len(rt.Runs) == 0 {
		return Layout{}, nil
	}
	for _, run := range rt.Runs {
		if err := ValidateTextInput(run.Text, MaxTextLength,
			"LayoutRichText"); err != nil {
			return Layout{}, err
		}
	}

	var fullText strings.Builder
	type runRange struct {
		start, end  int
		style       TextStyle
		resolved    TextStyle
		font        ctFont
		yShift      float64
		xPad        float64
		objectWidth float64
		objectID    string
	}
	runs := make([]runRange, 0, len(rt.Runs))
	idx := 0
	const objectReplacement = "￼" // 3 UTF-8 bytes.
	for _, run := range rt.Runs {
		merged := mergeStyles(cfg.Style, run.Style)
		resolved := merged
		f := ctx.createCTFont(merged)
		var yShift, xPad float64

		// Sub/superscript fallback: most system fonts lack OT
		// subs/sups glyph substitution, so scale the font down and
		// shift the baseline to render visibly. Callers that set
		// Size *= 1.2 before requesting subs/sups (e.g. markdown
		// styling) end up at ~0.7x base size, matching the
		// expected typographic ratio.
		if resolved.Features != nil {
			baseSize := float64(parseSizeFromStyle(resolved))
			for _, feat := range resolved.Features.OpenTypeFeatures {
				if feat.Value != 1 {
					continue
				}
				switch feat.Tag {
				case "subs":
					small := resolved
					small.Size = float32(baseSize * 0.5)
					resolved = small
					f.close()
					f = ctx.createCTFont(small)
					yShift = -baseSize * 0.15
					xPad = baseSize * 0.15
				case "sups":
					small := resolved
					small.Size = float32(baseSize * 0.5)
					resolved = small
					f.close()
					f = ctx.createCTFont(small)
					yShift = baseSize * 0.4
					xPad = baseSize * 0.15
				}
			}
		}

		runText := run.Text
		var objectWidth float64
		var objectID string
		if run.Style.Object != nil {
			// Replace run text with a single OBJECT REPLACEMENT
			// CHARACTER (U+FFFC). Width is taken from the
			// InlineObject; baseline offset becomes yShift.
			runText = objectReplacement
			objectWidth = float64(run.Style.Object.Width) *
				float64(ctx.scaleFactor)
			objectID = run.Style.Object.ID
			yShift = float64(run.Style.Object.Offset) *
				float64(ctx.scaleFactor)
			xPad = 0
		}

		fullText.WriteString(runText)
		runs = append(runs, runRange{
			start: idx, end: idx + len(runText),
			style: run.Style, resolved: resolved, font: f,
			yShift: yShift, xPad: xPad,
			objectWidth: objectWidth, objectID: objectID,
		})
		idx += len(runText)
	}
	text := fullText.String()

	baseFont := ctx.createCTFont(cfg.Style)
	defer baseFont.close()

	overrides := make(map[int]charFontOverride)
	for _, r := range runs {
		for i := r.start; i < r.end; {
			overrides[i] = charFontOverride{
				font:        r.font,
				style:       r.resolved,
				yShift:      r.yShift,
				xPad:        r.xPad,
				objectWidth: r.objectWidth,
				objectID:    r.objectID,
			}
			_, sz := utf8.DecodeRuneInString(text[i:])
			i += sz
		}
	}

	layout := ctx.buildLayout(text, baseFont, cfg, overrides)

	// buildLayout produces one Item per line. To honor per-run
	// styles (sub/sup font scaling, run colors, underline,
	// strikethrough, background) the per-line Items must be split
	// at run boundaries so each sub-Item carries the correct
	// resolved Style. Without this split, glyph rasterization —
	// which keys off Item.Style — would render every run at the
	// first run's font size.
	findRun := func(byteIdx int) *runRange {
		for i := range runs {
			if byteIdx >= runs[i].start && byteIdx < runs[i].end {
				return &runs[i]
			}
		}
		return nil
	}

	newItems := make([]Item, 0, len(layout.Items))
	for _, item := range layout.Items {
		if item.GlyphCount == 0 {
			newItems = append(newItems, item)
			continue
		}
		glyphs := layout.Glyphs[item.GlyphStart : item.GlyphStart+item.GlyphCount]

		flush := func(chunkStart, chunkEnd int, x float64, r *runRange) {
			if chunkEnd <= chunkStart {
				return
			}
			sub := item
			sub.GlyphStart = item.GlyphStart + chunkStart
			sub.GlyphCount = chunkEnd - chunkStart
			var w float64
			for _, g := range glyphs[chunkStart:chunkEnd] {
				w += g.XAdvance
			}
			sub.Width = w
			sub.X = x
			firstByte := int(glyphs[chunkStart].Index)
			lastByte := int(glyphs[chunkEnd-1].Index)
			sub.StartIndex = firstByte
			sub.Length = lastByte - firstByte + 1
			if r != nil {
				sub.Style = r.resolved
				if r.style.Color.A > 0 {
					sub.Color = r.style.Color
				}
				if r.style.BgColor.A > 0 {
					sub.BgColor = r.style.BgColor
					sub.HasBgColor = true
				}
				if r.style.Underline {
					sub.HasUnderline = true
				}
				if r.style.Strikethrough {
					sub.HasStrikethrough = true
				}
				if r.style.Object != nil {
					sub.IsObject = true
					sub.ObjectID = r.objectID
				}
			}
			newItems = append(newItems, sub)
		}

		curRun := findRun(int(glyphs[0].Index))
		chunkStart := 0
		chunkX := item.X
		chunkW := 0.0
		for gi, g := range glyphs {
			r := findRun(int(g.Index))
			if r != curRun && gi > chunkStart {
				flush(chunkStart, gi, chunkX, curRun)
				chunkX += chunkW
				chunkStart = gi
				chunkW = 0
				curRun = r
			}
			chunkW += g.XAdvance
		}
		flush(chunkStart, len(glyphs), chunkX, curRun)
	}
	layout.Items = newItems

	// Clean up run fonts.
	for _, r := range runs {
		r.font.close()
	}

	return layout, nil
}

// parseSizeFromStyle returns the effective font size.
func parseSizeFromStyle(s TextStyle) float32 {
	if s.Size > 0 {
		return s.Size
	}
	sz := parseSizeFromFontName(s.FontName)
	if sz > 0 {
		return sz
	}
	return 16
}

// mergeStyles merges run style on top of base style.
func mergeStyles(base, run TextStyle) TextStyle {
	result := run
	result.FontName = cmp.Or(result.FontName, base.FontName)
	if result.Size <= 0 {
		result.Size = base.Size
	}
	if result.Color.A == 0 {
		result.Color = base.Color
	}
	return result
}

// shapedCluster holds one shaped glyph's byte range, advance, and
// resolved CGGlyph ID from a full-text CTLine shaping pass.
type shapedCluster struct {
	byteStart int
	byteLen   int
	advance   float64
	glyphID   uint16 // CGGlyph after GSUB (calt/liga) substitution
	isRTL     bool
}

type charInfo struct {
	text    string
	width   float64
	byteI   int
	byteL   int
	yShift  float64
	xPad    float64
	glyphID uint16 // resolved CGGlyph after shaping (0 = use text)
}

// buildUTF16ToByteSlice builds a mapping where result[utf16Pos] = UTF-8 byte
// offset for that UTF-16 code unit. The final element is len(text) (sentinel).
// Supplementary-plane runes (U+10000..U+10FFFF) use two UTF-16 code units
// (surrogate pair) and both entries point to the same byte offset.
func buildUTF16ToByteSlice(text string) []int {
	m := make([]int, 0, len(text)+1)
	for i := 0; i < len(text); {
		r, sz := utf8.DecodeRuneInString(text[i:])
		m = append(m, i)
		if r > 0xFFFF { // surrogate pair: two UTF-16 code units
			m = append(m, i)
		}
		i += sz
	}
	m = append(m, len(text)) // sentinel: byte offset of string end
	return m
}

// buildByteToRuneIndexSlice maps each byte offset to its rune index.
// Non-rune-start positions are -1. Index len(text) holds the total rune count.
func buildByteToRuneIndexSlice(text string) []int {
	m := make([]int, len(text)+1)
	for i := range m {
		m[i] = -1
	}
	runeIdx := 0
	for i := 0; i < len(text); {
		m[i] = runeIdx
		_, sz := utf8.DecodeRuneInString(text[i:])
		i += sz
		runeIdx++
	}
	m[len(text)] = runeIdx
	return m
}

// visualOrderForLine returns the char indices for one line in visual order.
// Within an RTL bidi run, char clusters are emitted in reverse logical order.
func visualOrderForLine(text string, chars []charInfo, startChar, endChar int) []int {
	if startChar < 0 || endChar > len(chars) || startChar >= endChar {
		return nil
	}

	startByte := chars[startChar].byteI
	endByte := chars[endChar-1].byteI + chars[endChar-1].byteL
	if startByte < 0 || endByte < startByte || endByte > len(text) {
		return nil
	}
	lineText := text[startByte:endByte]
	runeMap := buildByteToRuneIndexSlice(lineText)

	type span struct {
		charIndex          int
		runeStart, runeEnd int
	}
	spans := make([]span, 0, endChar-startChar)
	for i := startChar; i < endChar; i++ {
		relStart := chars[i].byteI - startByte
		relEnd := relStart + chars[i].byteL
		if relStart < 0 || relEnd < relStart || relStart >= len(runeMap) || relEnd >= len(runeMap) {
			return nil
		}
		rs, re := runeMap[relStart], runeMap[relEnd]
		if rs < 0 || re < 0 {
			return nil
		}
		spans = append(spans, span{
			charIndex: i,
			runeStart: rs,
			runeEnd:   re,
		})
	}

	var para xbidi.Paragraph
	if _, err := para.SetString(lineText); err != nil {
		return nil
	}
	ordering, err := para.Order()
	if err != nil || ordering.NumRuns() == 0 {
		return nil
	}

	order := make([]int, 0, len(spans))
	used := make([]bool, len(spans))
	runChars := make([]int, 0, len(spans))
	runSpanIdx := make([]int, 0, len(spans))
	for ri := 0; ri < ordering.NumRuns(); ri++ {
		run := ordering.Run(ri)
		runStart, runEndInclusive := run.Pos()
		runEnd := runEndInclusive + 1

		runChars = runChars[:0]
		runSpanIdx = runSpanIdx[:0]
		for si, sp := range spans {
			if used[si] {
				continue
			}
			if sp.runeStart >= runStart && sp.runeEnd <= runEnd {
				runChars = append(runChars, sp.charIndex)
				runSpanIdx = append(runSpanIdx, si)
			}
		}
		if len(runChars) == 0 {
			continue
		}

		if run.Direction() == xbidi.RightToLeft {
			for i := len(runChars) - 1; i >= 0; i-- {
				order = append(order, runChars[i])
				used[runSpanIdx[i]] = true
			}
		} else {
			order = append(order, runChars...)
			for _, si := range runSpanIdx {
				used[si] = true
			}
		}
	}

	for si, sp := range spans {
		if !used[si] {
			order = append(order, sp.charIndex)
		}
	}
	return order
}

// shapeTextClusters shapes text with CoreText (via CTLine) and returns
// per-glyph clusters with UTF-8 byte ranges and physical-pixel advances,
// in logical (ascending byte) order. Returns nil on failure; callers fall
// back to per-grapheme measurement.
//
// CTLineGetGlyphRuns returns runs in visual order, not byte order. For RTL
// paragraphs visual order is the reverse of byte order, so a sequential
// gap-fill across run boundaries produces duplicate entries. Instead we
// build a byteStart→cluster map and walk the text in byte order, which is
// correct for both LTR and RTL scripts and correctly handles ligatures (the
// walker jumps past all bytes consumed by a multi-char cluster).
func shapeTextClusters(font ctFont, text string) []shapedCluster {
	if len(text) == 0 || font.ref == 0 {
		return nil
	}
	utf16Map := buildUTF16ToByteSlice(text)
	utf16Len := len(utf16Map) - 1
	if utf16Len <= 0 {
		return nil
	}

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	out := make([]C.CTGlyphCluster, utf16Len)
	n := int(C.ctShapeGlyphClusters(font.ref, cText, C.int(utf16Len), &out[0]))
	if n <= 0 {
		return nil
	}

	// Build map: UTF-8 byte start → shaped cluster.
	clusterMap := make(map[int]shapedCluster, n)
	for i := range n {
		us := int(out[i].utf16Start)
		ue := int(out[i].utf16End)
		if us < 0 {
			us = 0
		} else if us > utf16Len {
			us = utf16Len
		}
		if ue > utf16Len {
			ue = utf16Len
		}
		if ue < us {
			ue = us
		}
		sc := shapedCluster{
			byteStart: utf16Map[us],
			byteLen:   utf16Map[ue] - utf16Map[us],
			advance:   float64(out[i].advance),
			glyphID:   uint16(out[i].glyphID),
			isRTL:     out[i].isRTL != 0,
		}
		if sc.byteLen > 0 {
			clusterMap[sc.byteStart] = sc
		}
	}

	// Walk text in logical byte order. For each position:
	//   - shaped cluster found → use it; jump past the full cluster span
	//     (skipping any subsequent graphemes consumed by a ligature)
	//   - not found (newline, control char CoreText skips) → zero-advance
	//     placeholder so every byte is represented in the result
	result := make([]shapedCluster, 0, n+4)
	pos := 0
	for pos < len(text) {
		_, sz := utf8.DecodeRuneInString(text[pos:])
		if sc, ok := clusterMap[pos]; ok {
			result = append(result, sc)
			pos = sc.byteStart + sc.byteLen
		} else {
			result = append(result, shapedCluster{byteStart: pos, byteLen: sz})
			pos += sz
		}
	}

	// Merge consecutive non-space RTL clusters into word-level clusters.
	// Arabic and other RTL scripts require full-word context for correct
	// contextual shaping; per-glyph CTLine rendering produces isolated
	// letter forms instead of the correct initial/medial/final forms.
	merged := make([]shapedCluster, 0, len(result))
	for i := 0; i < len(result); {
		sc := result[i]
		clText := text[sc.byteStart : sc.byteStart+sc.byteLen]
		if !sc.isRTL || clText == " " || clText == "\t" {
			merged = append(merged, sc)
			i++
			continue
		}
		j := i + 1
		totalAdv := sc.advance
		for j < len(result) {
			nx := result[j]
			nt := text[nx.byteStart : nx.byteStart+nx.byteLen]
			if !nx.isRTL || nt == " " || nt == "\t" {
				break
			}
			totalAdv += nx.advance
			j++
		}
		if j > i+1 {
			last := result[j-1]
			// glyphID intentionally zero: the whole merged word is rendered as
			// a text run by CTLine so per-glyph IDs are irrelevant here.
			merged = append(merged, shapedCluster{
				byteStart: sc.byteStart,
				byteLen:   last.byteStart + last.byteLen - sc.byteStart,
				advance:   totalAdv,
				isRTL:     true,
			})
		} else {
			merged = append(merged, sc)
		}
		i = j
	}
	return merged
}

// buildLayout creates a Layout from measured text with word wrapping.
func (ctx *Context) buildLayout(text string, baseFont ctFont,
	cfg TextConfig,
	overrides map[int]charFontOverride) Layout {

	bm := ctx.fontMetrics(cfg.Style, baseFont)
	ascent, descent := bm.ascent, bm.descent
	lineHeight := ascent + descent
	pixelScale := 1.0 / float64(ctx.scaleFactor)

	if cfg.Orientation == OrientationVertical {
		return ctx.buildVerticalLayout(text, baseFont, cfg, overrides,
			ascent, descent, lineHeight, pixelScale)
	}

	// Measure each grapheme cluster, producing a charInfo per visual unit.
	// When no per-character overrides are active, use full-text CTLine
	// shaping so CoreText can apply ligature substitutions (liga, calt)
	// across adjacent characters. The shaped clusters may span multiple
	// code points when a ligature was formed.
	var chars []charInfo
	if overrides == nil {
		if sc := shapeTextClusters(baseFont, text); len(sc) > 0 {
			chars = make([]charInfo, 0, len(sc))
			for _, cl := range sc {
				clText := text[cl.byteStart : cl.byteStart+cl.byteLen]
				var w float64
				if clText != "\n" && clText != "\r" {
					w = cl.advance
				}
				chars = append(chars, charInfo{
					text:    clText,
					width:   w,
					byteI:   cl.byteStart,
					byteL:   cl.byteLen,
					glyphID: cl.glyphID,
				})
			}
		}
	}
	if chars == nil {
		// Fall back: per-grapheme measurement (overrides path, or shaping failure).
		clusters := segmentGraphemes(text)
		chars = make([]charInfo, 0, len(clusters))
		for _, cl := range clusters {
			var yShift, xPad, objectWidth float64
			measureFont := baseFont
			if overrides != nil {
				if ov, ok := overrides[cl.byteI]; ok {
					if ov.font.ref != 0 {
						measureFont = ov.font
					}
					yShift = ov.yShift
					xPad = ov.xPad
					objectWidth = ov.objectWidth
				}
			}

			var w float64
			switch {
			case cl.text == "\n" || cl.text == "\r":
				w = 0
			case objectWidth > 0:
				// Inline object: skip CT measurement, use the
				// caller-supplied reservation width directly.
				w = objectWidth
			default:
				cs := C.CString(cl.text)
				w = float64(C.ctMeasureCString(measureFont.ref, cs))
				C.free(unsafe.Pointer(cs))
			}
			totalW := w
			if objectWidth == 0 {
				totalW = w + xPad*float64(ctx.scaleFactor)
			}
			chars = append(chars, charInfo{
				text: cl.text, width: totalW,
				byteI: cl.byteI, byteL: cl.byteL,
				yShift: yShift, xPad: xPad,
			})
		}
	}

	if cfg.Style.LetterSpacing != 0 {
		spacing := float64(cfg.Style.LetterSpacing) *
			float64(ctx.scaleFactor)
		for i := range len(chars) - 1 {
			if chars[i].text == "\n" || chars[i].text == "\r" {
				continue
			}
			if chars[i+1].text == "\n" || chars[i+1].text == "\r" {
				continue
			}
			chars[i].width += spacing
		}
	}

	// Word-wrap into lines.
	wrapWidth := float64(-1)
	if cfg.Block.Width > 0 {
		wrapWidth = float64(cfg.Block.Width) * float64(ctx.scaleFactor)
	}

	type lineInfo struct {
		startChar, endChar int
		width              float64
	}
	var lines []lineInfo
	lineStart := 0
	lineW := float64(0)
	lastSpace := -1

	for i, ch := range chars {
		if ch.text == "\n" {
			lines = append(lines, lineInfo{lineStart, i, lineW})
			lineStart = i + 1
			lineW = 0
			lastSpace = -1
			continue
		}
		if ch.text == " " {
			lastSpace = i
		}

		newW := lineW + ch.width
		if wrapWidth > 0 && newW > wrapWidth && i > lineStart {
			if cfg.Block.Wrap == WrapNone {
				lineW = newW
				continue
			}
			if cfg.Block.Wrap == WrapWord ||
				cfg.Block.Wrap == WrapWordChar {
				if lastSpace >= lineStart {
					lines = append(lines, lineInfo{
						lineStart, lastSpace, lineW - ch.width,
					})
					lineStart = lastSpace + 1
					lineW = 0
					for j := lineStart; j <= i; j++ {
						lineW += chars[j].width
					}
					lastSpace = -1
					continue
				}
			}
			if cfg.Block.Wrap == WrapChar ||
				cfg.Block.Wrap == WrapWordChar {
				lines = append(lines, lineInfo{lineStart, i, lineW})
				lineStart = i
				lineW = ch.width
				lastSpace = -1
				continue
			}
		}
		lineW = newW
	}
	if lineStart <= len(chars) {
		lines = append(lines, lineInfo{lineStart, len(chars), lineW})
	}

	// Build Layout structures.
	var allGlyphs []Glyph
	var items []Item
	var charRects []CharRect
	charRectByIndex := make(map[int]int)
	var layoutLines []Line
	var logAttrs []LogAttr
	logAttrByIndex := make(map[int]int)

	var totalWidth, totalHeight float64
	lineY := float64(0)

	baseColor := cfg.Style.Color
	if baseColor.A == 0 {
		baseColor = Color{0, 0, 0, 255}
	}

	for lineIdx, li := range lines {
		if li.endChar < li.startChar {
			li.endChar = li.startChar
		}

		linePixelW := li.width
		var alignOffset float64
		if wrapWidth > 0 {
			switch cfg.Block.Align {
			case AlignCenter:
				alignOffset = (wrapWidth - linePixelW) / 2
			case AlignRight:
				alignOffset = wrapWidth - linePixelW
			}
		}

		indentPx := float64(0)
		if lineIdx == 0 && cfg.Block.Indent != 0 {
			indentPx = float64(cfg.Block.Indent) *
				float64(ctx.scaleFactor)
		}

		startByteIdx := 0
		if li.startChar < len(chars) {
			startByteIdx = chars[li.startChar].byteI
		} else if len(chars) > 0 {
			last := chars[len(chars)-1]
			startByteIdx = last.byteI + last.byteL
		}

		endByteIdx := startByteIdx
		lineLen := 0
		if li.endChar > li.startChar && li.endChar <= len(chars) {
			lastCh := chars[li.endChar-1]
			endByteIdx = lastCh.byteI + lastCh.byteL
			lineLen = endByteIdx - startByteIdx
		}

		cx := alignOffset + indentPx

		itemStart := len(allGlyphs)
		itemStartByte := startByteIdx
		itemX := cx

		flushItem := func(endByte int) {
			gc := len(allGlyphs) - itemStart
			if gc <= 0 {
				return
			}
			var w float64
			for _, gl := range allGlyphs[itemStart : itemStart+gc] {
				w += gl.XAdvance
			}
			items = append(items, Item{
				Style:                  cfg.Style,
				Width:                  w,
				X:                      itemX * pixelScale,
				Y:                      (lineY + ascent) * pixelScale,
				Ascent:                 ascent * pixelScale,
				Descent:                descent * pixelScale,
				GlyphStart:             itemStart,
				GlyphCount:             gc,
				StartIndex:             itemStartByte,
				Length:                 endByte - itemStartByte,
				Color:                  baseColor,
				UnderlineOffset:        2.0,
				UnderlineThickness:     1.0,
				StrikethroughOffset:    ascent * 0.35 * pixelScale,
				StrikethroughThickness: 1.0,
				HasUnderline:           cfg.Style.Underline,
				HasStrikethrough:       cfg.Style.Strikethrough,
				HasBgColor:             cfg.Style.BgColor.A > 0,
				BgColor:                cfg.Style.BgColor,
				StrokeWidth:            cfg.Style.StrokeWidth,
				StrokeColor:            cfg.Style.StrokeColor,
				HasStroke:              cfg.Style.StrokeWidth > 0,
			})
			itemStart = len(allGlyphs)
		}

		emitChar := func(ch charInfo, ci int) {
			allGlyphs = append(allGlyphs, Glyph{
				Index:     uint32(ch.byteI),
				Codepoint: uint32(ch.byteL),
				XOffset:   ch.xPad * pixelScale,
				XAdvance:  ch.width * pixelScale,
				YOffset:   ch.yShift * pixelScale,
				GlyphID:   ch.glyphID,
			})
			crIdx := len(charRects)
			charRects = append(charRects, CharRect{
				Rect: Rect{
					X:      float32(cx * pixelScale),
					Y:      float32(lineY * pixelScale),
					Width:  float32(ch.width * pixelScale),
					Height: float32(lineHeight * pixelScale),
				},
				Index: ch.byteI,
			})
			charRectByIndex[ch.byteI] = crIdx
			attrIdx := len(logAttrs)
			isWS := ch.text == " " || ch.text == "\t"
			prevWS := ci > 0 && (chars[ci-1].text == " " ||
				chars[ci-1].text == "\t" ||
				chars[ci-1].text == "\n")
			logAttrs = append(logAttrs, LogAttr{
				IsCursorPosition: true,
				IsWordStart:      !isWS && prevWS,
				IsWordEnd: isWS && ci > 0 &&
					chars[ci-1].text != " " &&
					chars[ci-1].text != "\t",
				IsLineBreak: ch.text == "\n",
			})
			logAttrByIndex[ch.byteI] = attrIdx
			cx += ch.width
		}

		order := visualOrderForLine(text, chars, li.startChar, li.endChar)
		if len(order) == 0 {
			order = make([]int, 0, li.endChar-li.startChar)
			for ci := li.startChar; ci < li.endChar; ci++ {
				order = append(order, ci)
			}
		}
		for _, ci := range order {
			ch := chars[ci]
			if ch.text == "\n" || ch.text == "\r" {
				continue
			}
			emitChar(ch, ci)
		}

		flushItem(endByteIdx)

		layoutLines = append(layoutLines, Line{
			StartIndex: startByteIdx,
			Length:     lineLen,
			IsParagraphStart: lineIdx == 0 ||
				(li.startChar > 0 &&
					chars[li.startChar-1].text == "\n"),
			Rect: Rect{
				X:      float32(alignOffset * pixelScale),
				Y:      float32(lineY * pixelScale),
				Width:  float32(linePixelW * pixelScale),
				Height: float32(lineHeight * pixelScale),
			},
		})

		totalWidth = max(totalWidth, linePixelW)
		lineY += lineHeight
		if cfg.Block.LineSpacing > 0 && lineIdx < len(lines)-1 {
			lineY += float64(cfg.Block.LineSpacing) *
				float64(ctx.scaleFactor)
		}
	}
	totalHeight = lineY

	endAttrIdx := len(logAttrs)
	logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
	logAttrByIndex[len(text)] = endAttrIdx

	result := Layout{
		Text:            text,
		Items:           items,
		Glyphs:          allGlyphs,
		CharRects:       charRects,
		CharRectByIndex: charRectByIndex,
		Lines:           layoutLines,
		LogAttrs:        logAttrs,
		LogAttrByIndex:  logAttrByIndex,
		Width:           float32(totalWidth * pixelScale),
		Height:          float32(totalHeight * pixelScale),
		VisualWidth:     float32(totalWidth * pixelScale),
		VisualHeight:    float32(totalHeight * pixelScale),
	}
	result.buildPositionCaches()
	return result
}

// buildVerticalLayout produces a vertical (top-to-bottom) layout.
func (ctx *Context) buildVerticalLayout(text string, baseFont ctFont,
	cfg TextConfig, overrides map[int]charFontOverride,
	fontAscent, fontDescent, lineHeight, pixelScale float64) Layout {

	baseColor := cfg.Style.Color
	if baseColor.A == 0 {
		baseColor = Color{0, 0, 0, 255}
	}

	var allGlyphs []Glyph
	var charRects []CharRect
	charRectByIndex := make(map[int]int)
	var logAttrs []LogAttr
	logAttrByIndex := make(map[int]int)

	penY := fontAscent
	clusters := segmentGraphemes(text)

	// Shape the full text to obtain post-GSUB glyph IDs, matching what
	// buildLayout does for horizontal text. Advances are discarded because
	// vertical spacing is lineHeight-derived. Only single-grapheme clusters
	// receive a shaped ID; ligatures that collapse multiple graphemes into
	// one glyph are skipped — vertical layout emits one Glyph per grapheme
	// and cannot correctly render a combined form across two glyph slots.
	var shapedMap map[int]shapedCluster
	if overrides == nil {
		if sc := shapeTextClusters(baseFont, text); len(sc) > 0 {
			shapedMap = make(map[int]shapedCluster, len(sc))
			for _, cl := range sc {
				if cl.glyphID != 0 {
					shapedMap[cl.byteStart] = cl
				}
			}
		}
	}

	for _, cl := range clusters {
		if cl.text == "\n" || cl.text == "\r" {
			continue
		}

		measureFont := baseFont
		if overrides != nil {
			if ov, ok := overrides[cl.byteI]; ok && ov.font.ref != 0 {
				measureFont = ov.font
			}
		}

		cs := C.CString(cl.text)
		charW := float64(C.ctMeasureCString(measureFont.ref, cs))
		C.free(unsafe.Pointer(cs))
		centerX := (lineHeight - charW) / 2.0

		var gid uint16
		if s, ok := shapedMap[cl.byteI]; ok && s.byteLen == cl.byteL {
			gid = s.glyphID
		}
		allGlyphs = append(allGlyphs, Glyph{
			Index:     uint32(cl.byteI),
			Codepoint: uint32(cl.byteL),
			XOffset:   centerX * pixelScale,
			XAdvance:  0,
			YAdvance:  -lineHeight * pixelScale,
			GlyphID:   gid,
		})

		crIdx := len(charRects)
		charRects = append(charRects, CharRect{
			Rect: Rect{
				X:      0,
				Y:      float32((penY - fontAscent) * pixelScale),
				Width:  float32(lineHeight * pixelScale),
				Height: float32(lineHeight * pixelScale),
			},
			Index: cl.byteI,
		})
		charRectByIndex[cl.byteI] = crIdx

		attrIdx := len(logAttrs)
		logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
		logAttrByIndex[cl.byteI] = attrIdx

		penY += lineHeight
	}

	endIdx := len(logAttrs)
	logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
	logAttrByIndex[len(text)] = endIdx

	glyphCount := len(allGlyphs)
	totalH := penY

	var items []Item
	if glyphCount > 0 {
		items = append(items, Item{
			Style:      cfg.Style,
			Width:      lineHeight * pixelScale,
			X:          fontAscent * pixelScale,
			Y:          fontAscent * pixelScale,
			Ascent:     fontAscent * pixelScale,
			Descent:    fontDescent * pixelScale,
			GlyphStart: 0,
			GlyphCount: glyphCount,
			StartIndex: 0,
			Length:     len(text),
			Color:      baseColor,
		})
	}

	lines := []Line{{
		StartIndex: 0,
		Length:     len(text),
		Rect: Rect{
			X: 0, Y: 0,
			Width:  float32(lineHeight * pixelScale),
			Height: float32(totalH * pixelScale),
		},
		IsParagraphStart: true,
	}}

	result := Layout{
		Text:            text,
		Items:           items,
		Glyphs:          allGlyphs,
		CharRects:       charRects,
		CharRectByIndex: charRectByIndex,
		Lines:           lines,
		LogAttrs:        logAttrs,
		LogAttrByIndex:  logAttrByIndex,
		Width:           float32(lineHeight * pixelScale),
		Height:          float32(totalH * pixelScale),
		VisualWidth:     float32(lineHeight * pixelScale),
		VisualHeight:    float32(totalH * pixelScale),
	}
	result.buildPositionCaches()
	return result
}
