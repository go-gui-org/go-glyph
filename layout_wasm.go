//go:build js && wasm

package glyph

import (
	"strings"
	"unicode/utf8"
)

// charFontOverride holds per-character font and position adjustments
// for rich text runs (e.g. subscript/superscript simulation).
type charFontOverride struct {
	cssFont string
	yShift  float64 // Positive = up (superscript), negative = down.
	xPad    float64 // Leading horizontal padding (pixels).
}

// LayoutText shapes and wraps text using Canvas2D measureText.
func (ctx *Context) LayoutText(text string, cfg TextConfig) (Layout, error) {
	if len(text) == 0 {
		return Layout{}, nil
	}
	if err := ValidateTextInput(text, MaxTextLength, "LayoutText"); err != nil {
		return Layout{}, err
	}

	cssFont := buildCSSFont(cfg.Style)
	ctx.ctx2d.Set("font", cssFont)

	return ctx.buildLayout(text, cssFont, cfg, nil), nil
}

// LayoutRichText shapes multi-styled text.
func (ctx *Context) LayoutRichText(rt RichText, cfg TextConfig) (Layout, error) {
	if len(rt.Runs) == 0 {
		return Layout{}, nil
	}
	for _, run := range rt.Runs {
		if err := ValidateTextInput(run.Text, MaxTextLength,
			"LayoutRichText"); err != nil {
			return Layout{}, err
		}
	}

	// Build full text and per-run ranges.
	var fullText strings.Builder
	type runRange struct {
		start, end int
		style      TextStyle
		cssFont    string
		yShift     float64
		xPad       float64
	}
	runs := make([]runRange, 0, len(rt.Runs))
	idx := 0
	for _, run := range rt.Runs {
		merged := mergeStyles(cfg.Style, run.Style)
		css := buildCSSFont(merged)
		var yShift, xPad float64

		// Simulate OpenType subs/sups via size reduction + shift.
		if merged.Features != nil {
			baseSize := float64(parseSizeFromStyle(merged))
			for _, f := range merged.Features.OpenTypeFeatures {
				if f.Value != 1 {
					continue
				}
				switch f.Tag {
				case "subs":
					small := merged
					small.Size = float32(baseSize * 0.58)
					css = buildCSSFont(small)
					yShift = -baseSize * 0.15
					xPad = baseSize * 0.08
				case "sups":
					small := merged
					small.Size = float32(baseSize * 0.58)
					css = buildCSSFont(small)
					yShift = baseSize * 0.4
					xPad = baseSize * 0.08
				}
			}
		}

		fullText.WriteString(run.Text)
		runs = append(runs, runRange{
			start: idx, end: idx + len(run.Text),
			style: run.Style, cssFont: css,
			yShift: yShift, xPad: xPad,
		})
		idx += len(run.Text)
	}
	text := fullText.String()

	// Build per-byte font override map.
	baseCSS := buildCSSFont(cfg.Style)
	overrides := make(map[int]charFontOverride)
	for _, r := range runs {
		if r.cssFont != baseCSS || r.yShift != 0 {
			for i := r.start; i < r.end; {
				overrides[i] = charFontOverride{
					cssFont: r.cssFont,
					yShift:  r.yShift,
					xPad:    r.xPad,
				}
				_, sz := utf8.DecodeRuneInString(text[i:])
				i += sz
			}
		}
	}

	layout := ctx.buildLayout(text, baseCSS, cfg, overrides)

	// Apply per-run styles to items.
	for i := range layout.Items {
		item := &layout.Items[i]
		for _, r := range runs {
			if item.StartIndex >= r.start && item.StartIndex < r.end {
				if r.style.Color.A > 0 {
					item.Color = r.style.Color
				}
				if r.style.BgColor.A > 0 {
					item.BgColor = r.style.BgColor
					item.HasBgColor = true
				}
				if r.style.Underline {
					item.HasUnderline = true
				}
				if r.style.Strikethrough {
					item.HasStrikethrough = true
				}
				item.CSSFont = r.cssFont
				break
			}
		}
	}

	return layout, nil
}

// buildLayout creates a Layout from measured text with word wrapping.
// overrides maps byte indices to per-character font/position
// adjustments (may be nil for plain text).
func (ctx *Context) buildLayout(text, cssFont string,
	cfg TextConfig, overrides map[int]charFontOverride) Layout {

	ctx.ctx2d.Set("font", cssFont)

	// Measure font metrics.
	mRef := ctx.ctx2d.Call("measureText", "Hg")
	fontAscent := mRef.Get("fontBoundingBoxAscent").Float()
	fontDescent := mRef.Get("fontBoundingBoxDescent").Float()
	lineHeight := fontAscent + fontDescent

	pixelScale := 1.0 / float64(ctx.scaleFactor)

	// Vertical text: stack characters top-to-bottom.
	if cfg.Orientation == OrientationVertical {
		return ctx.buildVerticalLayout(text, cssFont, cfg, overrides,
			fontAscent, fontDescent, lineHeight, pixelScale)
	}

	// Measure each grapheme cluster with per-char font overrides.
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
	currentFont := cssFont
	for _, cl := range clusters {
		var yShift, xPad float64
		if overrides != nil {
			if ov, ok := overrides[cl.byteI]; ok {
				if ov.cssFont != currentFont {
					ctx.ctx2d.Set("font", ov.cssFont)
					currentFont = ov.cssFont
				}
				yShift = ov.yShift
				xPad = ov.xPad
			} else if currentFont != cssFont {
				ctx.ctx2d.Set("font", cssFont)
				currentFont = cssFont
			}
		}

		var w float64
		if cl.text == "\n" || cl.text == "\r" {
			w = 0
		} else {
			m := ctx.ctx2d.Call("measureText", cl.text)
			w = m.Get("width").Float()
		}
		chars = append(chars, charInfo{
			text: cl.text, width: w + xPad, byteI: cl.byteI,
			byteL: cl.byteL, yShift: yShift, xPad: xPad,
		})
	}

	// Restore base font.
	if currentFont != cssFont {
		ctx.ctx2d.Set("font", cssFont)
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
			if cfg.Block.Wrap == WrapWord || cfg.Block.Wrap == WrapWordChar {
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
			if cfg.Block.Wrap == WrapChar || cfg.Block.Wrap == WrapWordChar {
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
			indentPx = float64(cfg.Block.Indent) * float64(ctx.scaleFactor)
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

		// Helper to flush the current item span.
		itemStart := len(allGlyphs)
		itemStartByte := startByteIdx
		itemCSSFont := cssFont
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
				CSSFont:                itemCSSFont,
				Width:                  w,
				X:                      itemX * pixelScale,
				Y:                      (lineY + fontAscent) * pixelScale,
				Ascent:                 fontAscent * pixelScale,
				Descent:                fontDescent * pixelScale,
				GlyphStart:             itemStart,
				GlyphCount:             gc,
				StartIndex:             itemStartByte,
				Length:                 endByte - itemStartByte,
				Color:                  baseColor,
				UnderlineOffset:        2.0,
				UnderlineThickness:     1.0,
				StrikethroughOffset:    fontAscent * 0.35 * pixelScale,
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

			// Split item at font boundary.
			charFont := cssFont
			if overrides != nil {
				if ov, ok := overrides[ch.byteI]; ok {
					charFont = ov.cssFont
				}
			}
			if charFont != itemCSSFont {
				flushItem(ch.byteI)
				itemStartByte = ch.byteI
				itemCSSFont = charFont
				itemX = cx
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
			prevWS := ci > 0 && (chars[ci-1].text == " " || chars[ci-1].text == "\t" || chars[ci-1].text == "\n")
			logAttrs = append(logAttrs, LogAttr{
				IsCursorPosition: true,
				IsWordStart:      !isWS && prevWS,
				IsWordEnd:        isWS && ci > 0 && chars[ci-1].text != " " && chars[ci-1].text != "\t",
				IsLineBreak:      ch.text == "\n",
			})
			logAttrByIndex[ch.byteI] = attrIdx
			cx += ch.width
		}

		// Flush final item for this line.
		flushItem(endByteIdx)

		layoutLines = append(layoutLines, Line{
			StartIndex:       startByteIdx,
			Length:           lineLen,
			IsParagraphStart: lineIdx == 0 || (li.startChar > 0 && chars[li.startChar-1].text == "\n"),
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
			lineY += float64(cfg.Block.LineSpacing) * float64(ctx.scaleFactor)
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
// Each character occupies one row; XAdvance=0, YAdvance=-lineHeight.
func (ctx *Context) buildVerticalLayout(text, cssFont string,
	cfg TextConfig, overrides map[int]charFontOverride,
	fontAscent, fontDescent, lineHeight, pixelScale float64) Layout {

	ctx.ctx2d.Set("font", cssFont)

	baseColor := cfg.Style.Color
	if baseColor.A == 0 {
		baseColor = Color{0, 0, 0, 255}
	}

	var allGlyphs []Glyph
	var charRects []CharRect
	charRectByIndex := make(map[int]int)
	var logAttrs []LogAttr
	logAttrByIndex := make(map[int]int)

	penY := fontAscent // start at first baseline
	clusters := segmentGraphemes(text)

	for _, cl := range clusters {
		if cl.text == "\n" || cl.text == "\r" {
			continue
		}

		// Measure char width for centering.
		m := ctx.ctx2d.Call("measureText", cl.text)
		charW := m.Get("width").Float()
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

	// End-of-text attr.
	endIdx := len(logAttrs)
	logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
	logAttrByIndex[len(text)] = endIdx

	glyphCount := len(allGlyphs)
	totalH := penY

	var items []Item
	if glyphCount > 0 {
		items = append(items, Item{
			CSSFont:    cssFont,
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
