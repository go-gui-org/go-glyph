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
*/
import "C"
import (
	"cmp"
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"
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

	// Measure each grapheme cluster.
	type charInfo struct {
		text   string
		width  float64
		byteI  int
		byteL  int
		yShift float64
		xPad   float64
	}
	clusters := segmentGraphemes(text)
	chars := make([]charInfo, 0, len(clusters))
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

		for ci := li.startChar; ci < li.endChar; ci++ {
			ch := chars[ci]
			if ch.text == "\n" {
				continue
			}

			allGlyphs = append(allGlyphs, Glyph{
				Index:     uint32(ch.byteI),
				Codepoint: uint32(ch.byteL),
				XOffset:   ch.xPad * pixelScale,
				XAdvance:  ch.width * pixelScale,
				YOffset:   ch.yShift * pixelScale,
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

		allGlyphs = append(allGlyphs, Glyph{
			Index:     uint32(cl.byteI),
			Codepoint: uint32(cl.byteL),
			XOffset:   centerX * pixelScale,
			XAdvance:  0,
			YAdvance:  -lineHeight * pixelScale,
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
