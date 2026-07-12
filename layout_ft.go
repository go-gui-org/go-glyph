//go:build android || linux || darwin || windows

package glyph

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// shapedGlyph is one HarfBuzz output glyph in device pixels, retained from
// the layout-time shaping pass so the renderer can rasterize it by id without
// re-shaping. xOff/yOff are the glyph's positioning offsets (mark placement);
// xAdv/yAdv are its advances.
type shapedGlyph struct {
	gid        uint32
	xAdv, yAdv float64
	xOff, yOff float64
}

// charFontOverride holds per-character font and position adjustments
// for rich text runs.
type charFontOverride struct {
	font      ftFont
	style     TextStyle
	yShift    float64
	xPad      float64
	isColor   bool // fallback is a color-emoji font (CBDT/CBLC)
	isRichRun bool // override comes from a rich-text run (style is authored)
}

// LayoutText shapes and wraps text using FreeType+HarfBuzz.
func (ctx *Context) LayoutText(text string, cfg TextConfig) (Layout, error) {
	if len(text) == 0 {
		return Layout{}, nil
	}
	if err := ValidateTextInput(text, MaxTextLength, "LayoutText"); err != nil {
		return Layout{}, err
	}

	font := ctx.createFTFont(cfg.Style)
	if font.face == nil {
		return Layout{}, fmt.Errorf("failed to create FT font")
	}
	defer font.close()

	overrides, fbFonts := ctx.buildScriptFallbacks(text, font, cfg)
	layout := ctx.buildLayout(text, font, cfg, overrides)
	for i := range fbFonts {
		fbFonts[i].close()
	}
	return layout, nil
}

// buildScriptFallbacks creates override entries for grapheme
// clusters whose glyphs are missing from the primary font. Returns
// the overrides map and a list of opened fallback fonts to close
// after layout.
func (ctx *Context) buildScriptFallbacks(text string,
	baseFont ftFont, cfg TextConfig) (
	map[int]charFontOverride, []ftFont) {

	if len(ctx.fallbackPaths) == 0 {
		return nil, nil
	}
	clusters := segmentGraphemes(text)
	fontSize := float64(parseSizeFromStyle(cfg.Style)) *
		float64(ctx.scaleFactor)

	var overrides map[int]charFontOverride
	fontByPath := make(map[string]ftFont)
	var fontsToClose []ftFont

	// ensureFont builds (once) the shaping ftFont for a chosen fallback path.
	// Only fonts actually selected for a cluster get a HarfBuzz font; probing
	// uses the cached face's cmap and never allocates a shaper.
	ensureFont := func(path string) ftFont {
		fb, opened := fontByPath[path]
		if !opened {
			fb = newFTFontFromPath(ctx.ftLib, path, fontSize)
			fontByPath[path] = fb
			if fb.face != nil {
				fontsToClose = append(fontsToClose, fb)
			}
		}
		return fb
	}

	for _, cl := range clusters {
		if cl.text == "\n" || cl.text == "\r" {
			continue
		}
		// Emoji clusters prefer a color font even when the base font
		// carries a monochrome glyph for the codepoint (DejaVu covers
		// many emoji as text), so they still render in color.
		wantColor := clusterIsEmoji(cl.text)
		// Coverage is decided by a cheap cmap lookup, not by shaping. The base
		// font covers the vast majority of clusters, and shaping each cluster
		// against dozens of fallback fonts was the dominant re-layout cost on
		// scroll/resize for text the base font lacks.
		if !wantColor && baseFont.covers(cl.text) {
			continue
		}
		// The fallback decision for this cluster is base-independent, so reuse
		// the memoized result. The negative case (no fallback covers it, e.g.
		// Chakma/Javanese with no installed font) is cached too, which is what
		// stops every scroll re-layout from rescanning all fallback fonts.
		res, ok := ctx.fallbackResolve[cl.text]
		if !ok {
			path, isColor := ctx.probeFallback(cl.text, wantColor, ensureFont)
			res = fbResolution{path: path, isColor: isColor}
			ctx.cacheFallback(cl.text, res)
		}
		if res.path == "" {
			continue // no fallback: base font renders (tofu)
		}
		fb := ensureFont(res.path)
		if fb.face == nil {
			continue
		}
		if overrides == nil {
			overrides = make(map[int]charFontOverride)
		}
		overrides[cl.byteI] = charFontOverride{font: fb, isColor: res.isColor}
	}
	return overrides, fontsToClose
}

// probeFallback scans the fallback fonts for one that covers text, returning
// the winning path and whether it is a color font. An empty path means no
// fallback covers the cluster. Coverage uses each cached face's cmap (no
// shaping); only color/emoji candidates are shaped, via ensureFont, to
// confirm a real glyph for multi-codepoint sequences.
func (ctx *Context) probeFallback(text string, wantColor bool,
	ensureFont func(string) ftFont) (string, bool) {

	for _, path := range ctx.fallbackPaths {
		cf := loadCachedFace(path)
		if cf == nil || cf.face == nil {
			continue
		}
		// For emoji hold out for a color font; fallbackPaths lists color-emoji
		// fonts ahead of CJK, so this succeeds when one is installed and
		// otherwise leaves the base text glyph.
		if wantColor && !cf.color {
			continue
		}
		if !wantColor && !faceCovers(cf.face, text) {
			continue
		}
		if wantColor {
			fb := ensureFont(path)
			if fb.face == nil || !fb.hasGlyphs(text) {
				continue
			}
		}
		return path, cf.color
	}
	return "", false
}

// clusterIsEmoji reports whether a grapheme cluster should render with
// a color-emoji font. It matches the common emoji code blocks plus the
// variation selectors, ZWJ, and keycap combiner used in emoji
// sequences.
func clusterIsEmoji(cluster string) bool {
	for _, r := range cluster {
		switch {
		case r >= 0x1F300 && r <= 0x1FAFF: // pictographs, emoticons, symbols
			return true
		case r >= 0x2600 && r <= 0x27BF: // misc symbols + dingbats
			return true
		case r >= 0x1F000 && r <= 0x1F0FF: // mahjong, dominoes, cards
			return true
		case r >= 0x2300 && r <= 0x23FF: // misc technical (⌚⌛⏰)
			return true
		case r == 0x2764 || r == 0x2763: // hearts
			return true
		case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
			return true
		case r == 0x200D || r == 0x20E3: // ZWJ, keycap combiner
			return true
		case r >= 0x1F1E6 && r <= 0x1F1FF: // regional indicators (flags)
			return true
		}
	}
	return false
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
		start, end int
		style      TextStyle
		resolved   TextStyle
		font       ftFont
		yShift     float64
		xPad       float64
	}
	runs := make([]runRange, 0, len(rt.Runs))
	idx := 0
	for _, run := range rt.Runs {
		merged := mergeStyles(cfg.Style, run.Style)
		resolved := merged
		f := ctx.createFTFont(merged)
		var yShift, xPad float64

		if resolved.Features != nil {
			baseSize := float64(parseSizeFromStyle(resolved))
			for _, feat := range resolved.Features.OpenTypeFeatures {
				if feat.Value != 1 {
					continue
				}
				switch feat.Tag {
				case "subs":
					small := resolved
					small.Size = float32(baseSize * 0.58)
					resolved = small
					f.close()
					f = ctx.createFTFont(small)
					yShift = -baseSize * 0.4
					xPad = baseSize * 0.08
				case "sups":
					small := resolved
					small.Size = float32(baseSize * 0.58)
					resolved = small
					f.close()
					f = ctx.createFTFont(small)
					yShift = baseSize * 0.65
					xPad = baseSize * 0.08
				}
			}
		}

		fullText.WriteString(run.Text)
		runs = append(runs, runRange{
			start: idx, end: idx + len(run.Text),
			style: run.Style, resolved: resolved, font: f,
			yShift: yShift, xPad: xPad,
		})
		idx += len(run.Text)
	}
	text := fullText.String()

	baseFont := ctx.createFTFont(cfg.Style)
	defer baseFont.close()

	overrides := make(map[int]charFontOverride)
	for _, r := range runs {
		for i := r.start; i < r.end; {
			overrides[i] = charFontOverride{
				font:      r.font,
				style:     r.resolved,
				yShift:    r.yShift,
				xPad:      r.xPad,
				isRichRun: true,
			}
			_, sz := utf8.DecodeRuneInString(text[i:])
			i += sz
		}
	}

	// Detect emoji clusters and mark them as color so the renderer uses
	// the color-font path instead of monochrome glyphs.
	clusters := segmentGraphemes(text)
	for _, cl := range clusters {
		if clusterIsEmoji(cl.text) {
			if ov, ok := overrides[cl.byteI]; ok {
				ov.isColor = true
				overrides[cl.byteI] = ov
			}
		}
	}

	layout := ctx.buildLayout(text, baseFont, cfg, overrides)

	// Apply per-run styles to items.
	for i := range layout.Items {
		item := &layout.Items[i]
		for _, r := range runs {
			if item.StartIndex >= r.start && item.StartIndex < r.end {
				item.Style = r.resolved
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
				break
			}
		}
	}

	// Clean up run fonts.
	for _, r := range runs {
		r.font.close()
	}

	return layout, nil
}

// buildLayout creates a Layout from measured text with word wrapping.
func (ctx *Context) buildLayout(text string, baseFont ftFont,
	cfg TextConfig,
	overrides map[int]charFontOverride) Layout {

	ascent, descent, _ := baseFont.metrics()
	lineHeight := ascent + descent
	pixelScale := 1.0 / float64(ctx.scaleFactor)

	if cfg.Orientation == OrientationVertical {
		return ctx.buildVerticalLayout(text, baseFont, cfg, overrides,
			ascent, descent, lineHeight, pixelScale)
	}

	// Measure each grapheme cluster.
	type charInfo struct {
		text     string
		width    float64
		byteI    int
		byteL    int
		yShift   float64
		xPad     float64
		isColor  bool
		absorbed bool   // consumed by a ligature; not a caret stop
		styleKey uint64 // per-run appearance identity; splits items
		// The cluster's shaped-glyph stream (HarfBuzz output owned by this
		// cluster), in visual order. nGlyphs == 0 means the cluster was not
		// shaped (newline, color emoji, unloaded font) and falls back to
		// text-based rasterization. Almost every cluster is a single glyph, so
		// the first is stored inline (glyph0) and glyphsEx is allocated only
		// for the rare multi-glyph cluster (ligatures, marks) — avoiding a
		// per-cluster slice allocation on the common path.
		glyph0   shapedGlyph
		glyphsEx []shapedGlyph
		nGlyphs  int
	}
	// glyphAt returns the cluster's j-th shaped glyph (0-based, j < nGlyphs).
	glyphAt := func(c *charInfo, j int) shapedGlyph {
		if j == 0 {
			return c.glyph0
		}
		return c.glyphsEx[j-1]
	}
	clusters := segmentGraphemes(text)
	chars := make([]charInfo, 0, len(clusters))
	// charFonts[i] is the font used to measure chars[i]; parallel slice so
	// the run-shaping pass below can detect font boundaries.
	charFonts := make([]ftFont, 0, len(clusters))
	for _, cl := range clusters {
		var yShift, xPad float64
		var isColor bool
		var styleKey uint64
		measureFont := baseFont
		if overrides != nil {
			if ov, ok := overrides[cl.byteI]; ok {
				if ov.font.face != nil {
					measureFont = ov.font
				}
				yShift = ov.yShift
				xPad = ov.xPad
				isColor = ov.isColor
				// Rich-text runs carry an authored style; glyphs
				// rasterize from item.Style, so items must split wherever
				// a run's rasterized appearance changes. Script fallbacks
				// (LayoutText) are not rich runs and keep key 0.
				if ov.isRichRun {
					styleKey = fnvOffsetBasis
					styleKey = fnvHashString(styleKey, ov.style.FontName)
					styleKey = fnvHashF32(styleKey, ov.style.Size)
					styleKey = fnvHashU64(styleKey,
						uint64(ov.style.Typeface))
					styleKey = fnvHashColor(styleKey, ov.style.Color)
					styleKey = fnvHashColor(styleKey, ov.style.BgColor)
					styleKey = fnvHashF32(styleKey, float32(yShift))
					if ov.style.Underline {
						styleKey = fnvHashU64(styleKey, 1)
					}
					if ov.style.Strikethrough {
						styleKey = fnvHashU64(styleKey, 2)
					}
				}
			}
		}
		chars = append(chars, charInfo{
			text: cl.text, byteI: cl.byteI, byteL: cl.byteL,
			yShift: yShift, xPad: xPad, isColor: isColor,
			styleKey: styleKey,
		})
		charFonts = append(charFonts, measureFont)
	}

	// Fill cluster widths from contextual run shaping. Shaping each cluster
	// in isolation yields isolated-form glyphs and advances; scripts with
	// joining/ligatures (Arabic, Indic) must be shaped as a run so a word's
	// advances are correct. A run is a maximal span of consecutive text
	// clusters sharing one font; newlines and color-emoji cells keep their
	// fixed widths and break runs. Clusters absorbed into a ligature (e.g.
	// the alef of a lam-alef) get zero advance, matching the single glyph
	// the renderer produces for them.
	scale := float64(ctx.scaleFactor)
	for ri := 0; ri < len(chars); {
		ch := chars[ri]
		if ch.text == "\n" || ch.text == "\r" {
			chars[ri].width = ch.xPad * scale
			ri++
			continue
		}
		if ch.isColor {
			// Color-bitmap fonts report advances at their fixed strike size
			// (e.g. 136px), which would blow up the layout. Emoji are square
			// by convention, so reserve one line-height cell; the renderer
			// scales the bitmap to fit.
			chars[ri].width = lineHeight + ch.xPad*scale
			ri++
			continue
		}
		f := charFonts[ri]
		rj := ri + 1
		// A run is one font at one size. Sub/superscripts reuse the base
		// face at a reduced size (same .ttf, so same face pointer); the size
		// check keeps them out of the base run, otherwise they would be shaped
		// with the base font and take a base-size advance while rendering
		// small, leaving a gap after the glyph.
		for rj < len(chars) && !chars[rj].isColor &&
			chars[rj].text != "\n" && chars[rj].text != "\r" &&
			charFonts[rj].face == f.face && charFonts[rj].size == f.size {
			rj++
		}
		runStart := chars[ri].byteI
		runEnd := chars[rj-1].byteI + chars[rj-1].byteL
		buf := f.shape(text[runStart:runEnd])
		// A nil buffer (shaping failed) or an empty one (no output glyphs for
		// non-empty text) both fall back to per-cluster measurement. Without
		// the len==0 guard an empty buffer would leave every cluster with no
		// glyph, marking the whole run absorbed and suppressing its caret stops.
		if buf == nil || len(buf.Info) == 0 {
			for k := ri; k < rj; k++ {
				chars[k].width = f.measureString(chars[k].text) +
					chars[k].xPad*scale
			}
			ri = rj
			continue
		}
		// Starting rune index (within the run) of each cluster.
		startRune := make([]int, rj-ri)
		acc := 0
		for k := ri; k < rj; k++ {
			startRune[k-ri] = acc
			acc += utf8.RuneCountInString(chars[k].text)
			chars[k].width = chars[k].xPad * scale
		}
		// Accumulate each shaped glyph's advance onto its owning cluster and
		// record the glyph in that cluster's stream. HarfBuzz sets a merged
		// glyph's cluster to the smallest rune index it covers, so the owning
		// cluster is the last one starting at or before that rune.
		for gi := range buf.Info {
			c := int(buf.Info[gi].Cluster)
			k := 0
			for k+1 < len(startRune) && startRune[k+1] <= c {
				k++
			}
			pos := buf.Pos[gi]
			ci := &chars[ri+k]
			ci.width += float64(pos.XAdvance) / 64.0
			sg := shapedGlyph{
				gid:  uint32(buf.Info[gi].Glyph),
				xAdv: float64(pos.XAdvance) / 64.0,
				yAdv: float64(pos.YAdvance) / 64.0,
				xOff: float64(pos.XOffset) / 64.0,
				yOff: float64(pos.YOffset) / 64.0,
			}
			if ci.nGlyphs == 0 {
				ci.glyph0 = sg
			} else {
				ci.glyphsEx = append(ci.glyphsEx, sg)
			}
			ci.nGlyphs++
		}
		// A cluster that received no glyph from a run that shaped successfully
		// was absorbed into a neighbouring ligature (e.g. the alef of a
		// lam-alef, or the second half of an "fi"). Its advance is ~0 and the
		// caret must not stop inside it — carets land only at ligature
		// boundaries.
		for k := ri; k < rj; k++ {
			if chars[k].nGlyphs == 0 {
				chars[k].absorbed = true
			}
		}
		ri = rj
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

	// Precompute UAX #14 line-break opportunities for CJK and non-space wrap.
	canBreakBefore := make([]bool, len(chars))
	if len(chars) > 1 {
		byteToChar := make(map[int]int, len(chars))
		for i, ch := range chars {
			byteToChar[ch.byteI] = i
		}
		tb := []byte(text)
		consumed := 0
		state := -1
		for len(tb) > 0 {
			var seg []byte
			seg, tb, _, state = uniseg.FirstLineSegment(tb, state)
			consumed += len(seg)
			if ci, ok := byteToChar[consumed]; ok {
				canBreakBefore[ci] = true
			}
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
	lastCJKBreak := -1
	var lastCJKBreakW float64

	for i, ch := range chars {
		if ch.text == "\n" {
			lines = append(lines, lineInfo{lineStart, i, lineW})
			lineStart = i + 1
			lineW = 0
			lastSpace = -1
			lastCJKBreak = -1
			continue
		}
		if ch.text == " " {
			lastSpace = i
		} else if i > 0 && canBreakBefore[i] {
			lastCJKBreak = i
			lastCJKBreakW = lineW
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
					lastCJKBreak = -1
					continue
				}
				if lastCJKBreak >= lineStart {
					lines = append(lines, lineInfo{
						lineStart, lastCJKBreak, lastCJKBreakW,
					})
					lineStart = lastCJKBreak
					lineW = 0
					for j := lineStart; j <= i; j++ {
						lineW += chars[j].width
					}
					lastSpace = -1
					lastCJKBreak = -1
					continue
				}
			}
			if cfg.Block.Wrap == WrapChar ||
				cfg.Block.Wrap == WrapWordChar {
				lines = append(lines, lineInfo{lineStart, i, lineW})
				lineStart = i
				lineW = ch.width
				lastSpace = -1
				lastCJKBreak = -1
				continue
			}
		}
		lineW = newW
	}
	if lineStart <= len(chars) {
		lines = append(lines, lineInfo{lineStart, len(chars), lineW})
	}

	// Build bidi char info once for all lines.
	bidiChars := make([]charBidiInfo, len(chars))
	for i, ch := range chars {
		bidiChars[i] = charBidiInfo{byteI: ch.byteI, byteL: ch.byteL}
	}

	// Build Layout structures. allGlyphs/charRects/logAttrs run about one
	// entry per cluster, so presize to avoid append regrowth reallocation.
	allGlyphs := make([]Glyph, 0, len(chars))
	var items []Item
	charRects := make([]CharRect, 0, len(chars))
	charRectByIndex := make(map[int]int)
	var layoutLines []Line
	logAttrs := make([]LogAttr, 0, len(chars))
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

		lineLen := 0
		if li.endChar > li.startChar && li.endChar <= len(chars) {
			lastCh := chars[li.endChar-1]
			lineLen = lastCh.byteI + lastCh.byteL - startByteIdx
		}

		cx := alignOffset + indentPx

		itemStart := len(allGlyphs)
		itemX := cx
		itemColor := false // whether the current item is a color-emoji run
		itemStyleKey := uint64(0)
		itemFace := baseFont.face
		itemPath := baseFont.path
		// Logical byte span of the current item. Clusters are visited in
		// visual order, so for an RTL run the first-visited cluster is the
		// logical last; track min/max to keep StartIndex/Length logical.
		itemMinByte, itemMaxByte := startByteIdx, startByteIdx
		itemFirst := true

		flushItem := func(useOrig bool) {
			gc := len(allGlyphs) - itemStart
			if gc <= 0 {
				return
			}
			var w float64
			for _, gl := range allGlyphs[itemStart : itemStart+gc] {
				w += gl.XAdvance
			}
			length := itemMaxByte - itemMinByte
			if length < 0 {
				length = 0
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
				StartIndex:             itemMinByte,
				Length:                 length,
				FontPath:               itemPath,
				Color:                  baseColor,
				UseOriginalColor:       useOrig,
				UnderlineOffset:        2.0,
				UnderlineThickness:     1.0,
				StrikethroughOffset:    ascent * 0.35 * pixelScale,
				StrikethroughThickness: 1.0,
				HasUnderline:           cfg.Style.Underline && !useOrig,
				HasStrikethrough:       cfg.Style.Strikethrough && !useOrig,
				HasBgColor:             cfg.Style.BgColor.A > 0,
				BgColor:                cfg.Style.BgColor,
				StrokeWidth:            cfg.Style.StrokeWidth,
				StrokeColor:            cfg.Style.StrokeColor,
				HasStroke:              cfg.Style.StrokeWidth > 0 && !useOrig,
			})
			itemStart = len(allGlyphs)
		}

		order := visualOrderForLine(text, bidiChars, li.startChar, li.endChar)
		if len(order) == 0 {
			order = make([]int, 0, li.endChar-li.startChar)
			for ci := li.startChar; ci < li.endChar; ci++ {
				order = append(order, ci)
			}
		}

		for _, ci := range order {
			ch := chars[ci]
			if ch.text == "\n" {
				continue
			}

			// Split the item whenever the color-emoji state changes (so
			// emoji glyphs get their own item with UseOriginalColor set,
			// otherwise the color bitmap renders tinted and unscaled), the
			// style-run appearance changes (so each run's glyphs rasterize
			// with its own font size and color), or the measure font
			// changes. Splitting at font boundaries keeps each item's text a
			// single-script run, so the renderer reshapes exactly the run the
			// layout measured — essential for Arabic joining/ligatures, which
			// break when a mixed-script line is shaped as one unit.
			face := charFonts[ci].face
			if len(allGlyphs) > itemStart &&
				(ch.isColor != itemColor || ch.styleKey != itemStyleKey ||
					face != itemFace) {
				flushItem(itemColor)
				itemX = cx
				itemFirst = true
			}
			itemColor = ch.isColor
			itemStyleKey = ch.styleKey
			itemFace = face
			itemPath = charFonts[ci].path
			if itemFirst {
				itemMinByte = ch.byteI
				itemMaxByte = ch.byteI + ch.byteL
				itemFirst = false
			} else {
				itemMinByte = min(itemMinByte, ch.byteI)
				itemMaxByte = max(itemMaxByte, ch.byteI+ch.byteL)
			}

			// Emit the cluster's shaped-glyph stream when present, else a
			// single text-keyed glyph (GlyphID 0) that the renderer rasterizes
			// via re-shaping. The stream's advances must total ch.width so the
			// pen, CharRects and item width stay consistent: the glyphs carry
			// their HarfBuzz advances and the remainder (leading pad plus
			// letter spacing) rides on the last glyph. A single unshifted glyph
			// reproduces the old Glyph exactly, with GlyphID now populated.
			if ch.nGlyphs > 0 {
				var hbSum float64
				for j := 0; j < ch.nGlyphs; j++ {
					hbSum += glyphAt(&ch, j).xAdv
				}
				extra := ch.width - hbSum
				last := ch.nGlyphs - 1
				for j := 0; j < ch.nGlyphs; j++ {
					sg := glyphAt(&ch, j)
					xOff := sg.xOff
					if j == 0 {
						xOff += ch.xPad
					}
					adv := sg.xAdv
					if j == last {
						adv += extra
					}
					allGlyphs = append(allGlyphs, Glyph{
						Index:     uint32(ch.byteI),
						Codepoint: uint32(ch.byteL),
						GlyphID:   sg.gid,
						Shaped:    true,
						XOffset:   xOff * pixelScale,
						YOffset:   (sg.yOff + ch.yShift) * pixelScale,
						XAdvance:  adv * pixelScale,
						YAdvance:  sg.yAdv * pixelScale,
					})
				}
			} else {
				allGlyphs = append(allGlyphs, Glyph{
					Index:     uint32(ch.byteI),
					Codepoint: uint32(ch.byteL),
					XOffset:   ch.xPad * pixelScale,
					XAdvance:  ch.width * pixelScale,
					YOffset:   ch.yShift * pixelScale,
				})
			}

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
				IsCursorPosition: !ch.absorbed,
				IsWordStart:      !isWS && prevWS,
				IsWordEnd: isWS && ci > 0 &&
					chars[ci-1].text != " " &&
					chars[ci-1].text != "\t",
				IsLineBreak: ch.text == "\n",
			})
			logAttrByIndex[ch.byteI] = attrIdx
			cx += ch.width
		}

		flushItem(itemColor)

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
func (ctx *Context) buildVerticalLayout(text string, baseFont ftFont,
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
			if ov, ok := overrides[cl.byteI]; ok && ov.font.face != nil {
				measureFont = ov.font
			}
		}

		charW := measureFont.measureString(cl.text)
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
