//go:build js && wasm

package glyph

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// charFontOverride holds per-character font and position adjustments
// for rich text runs (e.g. subscript/superscript simulation).
type charFontOverride struct {
	cssFont  string
	yShift   float64 // Positive = up (superscript), negative = down.
	xPad     float64 // Leading horizontal padding (pixels).
	objWidth float64 // >0 when this char is an InlineObject placeholder.
	// run is the rich-text run index plus one (0 = not a rich run). Items
	// split where it changes, so each run keeps its own color and style.
	run int
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

	clusters := segmentGraphemes(text)
	return ctx.buildLayout(clusters, text, cssFont, cfg, nil), nil
}

// LayoutRichText shapes multi-styled text.
//
// Each run is limited to MaxTextLength bytes and all runs together to
// MaxRichTextLength. An empty run is allowed and contributes nothing.
func (ctx *Context) LayoutRichText(rt RichText, cfg TextConfig) (Layout, error) {
	total, err := validateRichRuns(rt)
	if err != nil || total == 0 {
		return Layout{}, err
	}

	// Build full text and per-run ranges.
	var fullText strings.Builder
	fullText.Grow(total)
	type runRange struct {
		start, end int
		style      TextStyle // As given, for the "was it set" checks below.
		merged     TextStyle // Over cfg.Style, which is what the item gets.
		cssFont    string
		yShift     float64
		xPad       float64
	}
	runs := make([]runRange, 0, len(rt.Runs))
	idx := 0
	for _, run := range rt.Runs {
		if run.Text == "" {
			continue // an empty run owns no cluster and no item
		}
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
					yShift = -baseSize * 0.4
					xPad = baseSize * 0.08
				case "sups":
					small := merged
					small.Size = float32(baseSize * 0.58)
					css = buildCSSFont(small)
					yShift = baseSize * 0.65
					xPad = baseSize * 0.08
				}
			}
		}

		fullText.WriteString(run.Text)
		runs = append(runs, runRange{
			start: idx, end: idx + len(run.Text),
			style: run.Style, merged: merged, cssFont: css,
			yShift: yShift, xPad: xPad,
		})
		idx += len(run.Text)
	}
	text := fullText.String()

	// Build the per-byte override map. Every byte of a rich run gets an
	// entry, so items split at run boundaries even where the font does not
	// change (a color-only run).
	baseCSS := buildCSSFont(cfg.Style)
	overrides := make(map[int]charFontOverride, total)
	for ri, r := range runs {
		ov := charFontOverride{
			cssFont: r.cssFont,
			yShift:  r.yShift,
			xPad:    r.xPad,
			run:     ri + 1,
		}
		if r.style.Object != nil {
			ov.objWidth = float64(r.style.Object.Width)
		}
		for i := r.start; i < r.end; {
			overrides[i] = ov
			_, sz := utf8.DecodeRuneInString(text[i:])
			i += sz
		}
	}

	clusters := segmentGraphemes(text)
	layout := ctx.buildLayout(clusters, text, baseCSS, cfg, overrides)

	// Apply per-run styles to items. Runs are sorted and do not overlap, so
	// the run holding an item's first byte is found by binary search.
	for i := range layout.Items {
		item := &layout.Items[i]
		k := sort.Search(len(runs), func(k int) bool {
			return runs[k].end > item.StartIndex
		})
		if k == len(runs) || runs[k].start > item.StartIndex {
			continue
		}
		r := runs[k]
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
		if r.style.Object != nil {
			item.IsObject = true
			item.ObjectID = r.style.Object.ID
		}
		item.CSSFont = r.cssFont
		item.Style = r.merged
	}

	return layout, nil
}

// buildLayout creates a Layout from measured text with word wrapping.
// overrides maps byte indices to per-character font/position
// adjustments (may be nil for plain text).
func (ctx *Context) buildLayout(clusters []graphemeCluster,
	text, cssFont string,
	cfg TextConfig, overrides map[int]charFontOverride) Layout {

	ctx.ctx2d.Set("font", cssFont)

	// Measure font metrics.
	mRef := ctx.ctx2d.Call("measureText", "Hg")
	fontAscent := mRef.Get("fontBoundingBoxAscent").Float()
	fontDescent := mRef.Get("fontBoundingBoxDescent").Float()
	lineHeight := recommendedLineHeight(
		fontAscent, fontDescent, 0, cssFontSize(cfg.Style))

	// Everything below is in logical units, which is also what measureText
	// and the CSS font size are in: the canvas' base transform carries the
	// device pixel ratio, so the browser rasterizes at device resolution
	// without the layout knowing about it. Metrics used to be divided by
	// the scale factor here, on the assumption that the measurements were
	// physical — they never were, so any scale but 1.0 shrank every advance
	// while leaving the drawn glyphs at full size.

	// Vertical text: stack characters top-to-bottom.
	if cfg.Orientation == OrientationVertical {
		return ctx.buildVerticalLayout(clusters, text, cssFont, cfg,
			overrides, fontAscent, fontDescent, lineHeight)
	}

	// Measure each grapheme cluster with per-char font overrides.
	type charInfo struct {
		text     string
		width    float64
		byteI    int
		byteL    int
		yShift   float64
		xPad     float64
		isObject bool
		objWidth float64
		cssFont  string // font this cluster was measured with; splits runs
		// Byte length of the text this cluster draws. Normally byteL; a
		// cluster that owns a ligature draws the whole merged span, so the
		// browser forms the ligature in one fillText call.
		drawLen int
		// Set on a cluster the browser folded into its predecessor's
		// ligature: zero advance, nothing drawn, and not a caret stop.
		// Mirrors charInfo.absorbed on the FreeType path (layout_ft.go).
		absorbed bool
	}
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
		var isObject bool
		var objWidth float64
		if overrides != nil {
			if ov, ok := overrides[cl.byteI]; ok && ov.objWidth > 0 {
				isObject = true
				objWidth = ov.objWidth
			}
		}
		if isObject {
			w = objWidth
		} else if cl.text == "\n" || cl.text == "\r" {
			w = 0
		} else {
			m := ctx.ctx2d.Call("measureText", cl.text)
			w = m.Get("width").Float()
		}
		chars = append(chars, charInfo{
			text: cl.text, width: w + xPad, byteI: cl.byteI,
			byteL: cl.byteL, yShift: yShift, xPad: xPad,
			isObject: isObject, objWidth: objWidth,
			cssFont: currentFont, drawLen: cl.byteL,
		})
	}

	// Fold mandatory ligatures. Canvas2D shapes each fillText call on its
	// own, and this backend draws one call per cluster, so a lam-alef would
	// otherwise render as two isolated letters and offer a caret stop
	// between them — a stop the FreeType path rejects (layout_ft.go, where
	// HarfBuzz reports the alef as absorbed). measureText is the only
	// shaping signal the canvas exposes, so the run pass below infers the
	// merge from prefix widths, then makes it real: the owning cluster
	// takes the ligature's measured width and its whole draw span, the
	// absorbed clusters take zero width and lose their caret stop.
	shapeable := func(c charInfo) bool {
		return !c.isObject && c.text != "\n" && c.text != "\r"
	}
	for ri := 0; ri < len(chars); {
		if !shapeable(chars[ri]) {
			ri++
			continue
		}
		// A run is a maximal span of consecutive text clusters sharing one
		// font — the unit the browser would shape together.
		rj := ri + 1
		for rj < len(chars) && shapeable(chars[rj]) &&
			chars[rj].cssFont == chars[ri].cssFont {
			rj++
		}
		runStart := chars[ri].byteI
		runEnd := chars[rj-1].byteI + chars[rj-1].byteL
		// Gate on script: Latin text can ligate too ("fi"), but those
		// ligatures still advance and browsers let the caret enter them.
		// Skipping them keeps the common path at zero extra canvas calls.
		if rj-ri < 2 || !hasComplexShaping(text[runStart:runEnd]) {
			ri = rj
			continue
		}
		if chars[ri].cssFont != currentFont {
			ctx.ctx2d.Set("font", chars[ri].cssFont)
			currentFont = chars[ri].cssFont
		}
		measureRun := func(byteEnd int) float64 {
			m := ctx.ctx2d.Call("measureText", text[runStart:byteEnd])
			return m.Get("width").Float()
		}
		absorbed := detectAbsorbed(rj-ri, func(prefixLen int) float64 {
			last := chars[ri+prefixLen-1]
			return measureRun(last.byteI + last.byteL)
		}, ligatureEpsilon)
		if absorbed == nil {
			ri = rj
			continue
		}
		// detectAbsorbed never marks the first cluster of a run, so every
		// absorbed cluster has an owner to its left.
		for k := ri; k < rj; k++ {
			if absorbed[k-ri] {
				continue
			}
			end := k + 1
			for end < rj && absorbed[end-ri] {
				end++
			}
			if end == k+1 {
				continue
			}
			mergedEnd := chars[end-1].byteI + chars[end-1].byteL
			// Measure the merged span directly rather than summing the
			// prefix deltas: this is the exact string the renderer will
			// pass to fillText, so the advance cannot drift from the ink.
			m := ctx.ctx2d.Call("measureText",
				text[chars[k].byteI:mergedEnd])
			chars[k].width = m.Get("width").Float() + chars[k].xPad
			chars[k].drawLen = mergedEnd - chars[k].byteI
			for a := k + 1; a < end; a++ {
				chars[a].width = 0
				chars[a].drawLen = 0
				chars[a].absorbed = true
			}
		}
		ri = rj
	}

	// Restore base font.
	if currentFont != cssFont {
		ctx.ctx2d.Set("font", cssFont)
	}

	// Word-wrap into lines, with the same UAX #14 break opportunities and
	// wrap rules as the FreeType backend.
	charText := func(i int) string { return chars[i].text }
	charByte := func(i int) int { return chars[i].byteI }
	canBreakBefore := lineBreakOpportunities(nil, text, len(chars), charByte)
	wrapWidth := float64(-1)
	if cfg.Block.Width > 0 {
		wrapWidth = float64(cfg.Block.Width)
	}
	lines := wrapLines(nil, len(chars), cfg.Block.Wrap, wrapWidth,
		func(i int) float64 { return chars[i].width }, charText, canBreakBefore)

	// Build Layout structures.
	var allGlyphs []Glyph
	var items []Item
	var charRects []CharRect
	// One entry per processed char (plus the end-of-text attr); presize to
	// allocate the buckets once instead of rehashing while filling.
	charRectByIndex := make(map[int]int, len(chars))
	layoutLines := make([]Line, 0, len(lines))
	// +1: the end-of-text attr appended after the line loop must not force a
	// regrowth copy of an exactly-sized slice. The newline attrs appended by
	// the post-pass below add one entry per '\n' byte.
	newlineAttrs := strings.Count(text, "\n")
	logAttrs := make([]LogAttr, 0, len(chars)+1+newlineAttrs)
	logAttrByIndex := make(map[int]int, len(chars)+1+newlineAttrs)

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
			indentPx = float64(cfg.Block.Indent)
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
		itemRun := 0
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
				// The style rides along because the renderer needs more
				// than the CSS font string: the built-in box glyphs read
				// CellWidth, CellHeight and NoBuiltinBoxGlyphs off it, and
				// without it a grid caller's declared cell is invisible and
				// the opt-out never takes (issue #101). LayoutRichText
				// overwrites it per run below.
				Style:                  cfg.Style,
				CSSFont:                itemCSSFont,
				Width:                  w,
				X:                      itemX,
				Y:                      (lineY + fontAscent),
				Ascent:                 fontAscent,
				Descent:                fontDescent,
				GlyphStart:             itemStart,
				GlyphCount:             gc,
				StartIndex:             itemStartByte,
				Length:                 endByte - itemStartByte,
				Color:                  baseColor,
				UnderlineOffset:        2.0,
				UnderlineThickness:     1.0,
				StrikethroughOffset:    fontAscent * 0.35,
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

			// Split the item at a font or rich-run boundary.
			charFont := cssFont
			charRun := 0
			if overrides != nil {
				if ov, ok := overrides[ch.byteI]; ok {
					charFont = ov.cssFont
					charRun = ov.run
				}
			}
			if charFont != itemCSSFont || charRun != itemRun {
				flushItem(ch.byteI)
				itemStartByte = ch.byteI
				itemCSSFont = charFont
				itemRun = charRun
				itemX = cx
			}

			// drawLen, not byteL: the owner of a ligature draws the
			// whole merged span in one fillText (so the browser actually
			// forms the ligature), and an absorbed cluster draws nothing
			// (drawLen 0 makes glyphText return "") while keeping its
			// place in the glyph stream.
			allGlyphs = append(allGlyphs, Glyph{
				Index:     uint32(ch.byteI),
				Codepoint: uint32(ch.drawLen),
				XOffset:   ch.xPad,
				XAdvance:  ch.width,
				YOffset:   ch.yShift,
			})

			crIdx := len(charRects)
			charRects = append(charRects, CharRect{
				Rect: Rect{
					X:      float32(cx),
					Y:      float32(lineY),
					Width:  float32(ch.width),
					Height: float32(lineHeight),
				},
				Index: ch.byteI,
			})
			charRectByIndex[ch.byteI] = crIdx

			attrIdx := len(logAttrs)
			// Word flags are not set here: this loop runs in visual
			// (bidi) order, while word runs are a property of logical
			// order. applyWordAttrs fills them in once, below.
			logAttrs = append(logAttrs, LogAttr{
				// A cluster the browser folded into a neighbouring
				// ligature is not a caret stop: carets land only on
				// ligature boundaries, same as the FreeType path.
				IsCursorPosition: !ch.absorbed,
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
				X:      float32(alignOffset),
				Y:      float32(lineY),
				Width:  float32(linePixelW),
				Height: float32(lineHeight),
			},
		})

		totalWidth = max(totalWidth, linePixelW)
		lineY += lineHeight
		if cfg.Block.LineSpacing > 0 && lineIdx < len(lines)-1 {
			lineY += float64(cfg.Block.LineSpacing)
		}
	}
	totalHeight = lineY

	logAttrs = appendNewlineAttrs(text, logAttrs, logAttrByIndex)
	logAttrs = appendWrapSpaceAttrs(lines, charText, charByte, logAttrs, logAttrByIndex)

	endAttrIdx := len(logAttrs)
	logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
	logAttrByIndex[len(text)] = endAttrIdx

	applyWordAttrs(text, logAttrs, logAttrByIndex)

	result := Layout{
		Text:            text,
		Items:           items,
		Glyphs:          allGlyphs,
		CharRects:       charRects,
		CharRectByIndex: charRectByIndex,
		Lines:           layoutLines,
		LogAttrs:        logAttrs,
		LogAttrByIndex:  logAttrByIndex,
		Width:           float32(totalWidth),
		Height:          float32(totalHeight),
		VisualWidth:     float32(totalWidth),
		VisualHeight:    float32(totalHeight),
	}
	result.buildPositionCaches()
	return result
}

// buildVerticalLayout produces a vertical (top-to-bottom) layout.
// Each character occupies one row; XAdvance=0, YAdvance=-lineHeight.
// Items split where the font or the rich run changes, so each rich run
// keeps its own style.
func (ctx *Context) buildVerticalLayout(clusters []graphemeCluster,
	text, cssFont string,
	cfg TextConfig, overrides map[int]charFontOverride,
	fontAscent, fontDescent, lineHeight float64) Layout {

	ctx.ctx2d.Set("font", cssFont)
	currentFont := cssFont

	baseColor := cfg.Style.Color
	if baseColor.A == 0 {
		baseColor = Color{0, 0, 0, 255}
	}

	var allGlyphs []Glyph
	var charRects []CharRect
	var items []Item
	charRectByIndex := make(map[int]int, len(clusters))
	newlineAttrs := strings.Count(text, "\n")
	logAttrs := make([]LogAttr, 0, len(clusters)+1+newlineAttrs)
	logAttrByIndex := make(map[int]int, len(clusters)+1+newlineAttrs)

	penY := fontAscent // start at first baseline

	// The open item: its first glyph, first byte, pen y, font and run.
	itemStart, itemByte := 0, 0
	itemY := penY
	itemFont := cssFont
	itemRun := 0
	flush := func(endByte int) {
		if len(allGlyphs) == itemStart {
			return
		}
		items = append(items, Item{
			Style:      cfg.Style,
			CSSFont:    itemFont,
			Width:      lineHeight,
			X:          fontAscent,
			Y:          itemY,
			Ascent:     fontAscent,
			Descent:    fontDescent,
			GlyphStart: itemStart,
			GlyphCount: len(allGlyphs) - itemStart,
			StartIndex: itemByte,
			Length:     endByte - itemByte,
			Color:      baseColor,
		})
		itemStart = len(allGlyphs)
	}

	for _, cl := range clusters {
		if cl.text == "\n" || cl.text == "\r" {
			continue
		}

		charFont := cssFont
		charRun := 0
		if ov, ok := overrides[cl.byteI]; ok {
			charFont = ov.cssFont
			charRun = ov.run
		}
		if len(allGlyphs) > itemStart && (charFont != itemFont || charRun != itemRun) {
			flush(cl.byteI)
		}
		if len(allGlyphs) == itemStart {
			itemByte = cl.byteI
			itemY = penY
			itemFont, itemRun = charFont, charRun
		}
		if charFont != currentFont {
			ctx.ctx2d.Set("font", charFont)
			currentFont = charFont
		}

		// Measure char width for centering.
		m := ctx.ctx2d.Call("measureText", cl.text)
		charW := m.Get("width").Float()
		centerX := (lineHeight - charW) / 2.0

		allGlyphs = append(allGlyphs, Glyph{
			Index:     uint32(cl.byteI),
			Codepoint: uint32(cl.byteL),
			XOffset:   centerX,
			XAdvance:  0,
			YAdvance:  -lineHeight,
		})

		crIdx := len(charRects)
		charRects = append(charRects, CharRect{
			Rect: Rect{
				X:      0,
				Y:      float32((penY - fontAscent)),
				Width:  float32(lineHeight),
				Height: float32(lineHeight),
			},
			Index: cl.byteI,
		})
		charRectByIndex[cl.byteI] = crIdx

		attrIdx := len(logAttrs)
		logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
		logAttrByIndex[cl.byteI] = attrIdx

		penY += lineHeight
	}
	flush(len(text))
	if currentFont != cssFont {
		ctx.ctx2d.Set("font", cssFont)
	}

	logAttrs = appendNewlineAttrs(text, logAttrs, logAttrByIndex)
	// End-of-text attr.
	endIdx := len(logAttrs)
	logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
	logAttrByIndex[len(text)] = endIdx

	applyWordAttrs(text, logAttrs, logAttrByIndex)

	totalH := penY

	lines := []Line{{
		StartIndex: 0,
		Length:     len(text),
		Rect: Rect{
			X: 0, Y: 0,
			Width:  float32(lineHeight),
			Height: float32(totalH),
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
		Width:           float32(lineHeight),
		Height:          float32(totalH),
		VisualWidth:     float32(lineHeight),
		VisualHeight:    float32(totalH),
	}
	result.buildPositionCaches()
	return result
}
