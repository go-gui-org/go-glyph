//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

/*
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#include <pango/pango.h>
#include <pango/pangoft2.h>
#include <ft2build.h>
#include FT_FREETYPE_H

// CGo cannot access C bitfields. Provide helper accessors.
static inline guint glyph_log_attr_is_cursor_position(PangoLogAttr *a) {
	return a->is_cursor_position;
}
static inline guint glyph_log_attr_is_word_start(PangoLogAttr *a) {
	return a->is_word_start;
}
static inline guint glyph_log_attr_is_word_end(PangoLogAttr *a) {
	return a->is_word_end;
}
static inline guint glyph_log_attr_is_line_break(PangoLogAttr *a) {
	return a->is_line_break;
}
static inline guint glyph_layout_line_is_paragraph_start(PangoLayoutLine *l) {
	return l->is_paragraph_start;
}
*/
import "C"
import (
	"strings"
	"unsafe"
)

const spaceChar = byte(32)

// runMetrics holds underline/strikethrough positioning.
type runMetrics struct {
	UndPos      float64
	UndThick    float64
	StrikePos   float64
	StrikeThick float64
}

// getRunMetrics fetches decoration metrics from a Pango font.
func getRunMetrics(pangoFont *C.PangoFont, lang *C.PangoLanguage, attrs runAttributes) runMetrics {
	var m runMetrics
	if !attrs.HasUnderline && !attrs.HasStrikethrough {
		return m
	}
	metrics := C.pango_font_get_metrics(pangoFont, lang)
	if metrics == nil {
		return m
	}
	defer C.pango_font_metrics_unref(metrics)

	ps := float64(PangoScale)
	if attrs.HasUnderline {
		m.UndPos = float64(C.pango_font_metrics_get_underline_position(metrics)) / ps
		m.UndThick = float64(C.pango_font_metrics_get_underline_thickness(metrics)) / ps
		if m.UndThick < 1.0 {
			m.UndThick = 1.0
		}
		if m.UndPos < m.UndThick {
			m.UndPos = m.UndThick + 2.0
		}
	}
	if attrs.HasStrikethrough {
		m.StrikePos = float64(C.pango_font_metrics_get_strikethrough_position(metrics)) / ps
		m.StrikeThick = float64(C.pango_font_metrics_get_strikethrough_thickness(metrics)) / ps
		if m.StrikeThick < 1.0 {
			m.StrikeThick = 1.0
		}
	}
	return m
}

// processRunConfig holds parameters for processRun.
type processRunConfig struct {
	run                *C.PangoGlyphItem
	iter               *C.PangoLayoutIter
	text               string
	scaleFactor        float32
	pixelScale         float64
	primaryAscent      float64
	primaryDescent     float64
	primaryStrikePos   float64
	primaryStrikeThick float64
	baseColor          Color
	orientation        TextOrientation
	strokeWidth        float32
	strokeColor        Color
}

// processRun converts a single Pango glyph run into a Go Item.
// Returns the updated vertical pen position.
func processRun(items *[]Item, allGlyphs *[]Glyph, verticalPenY float64,
	cfg processRunConfig) float64 {

	run := cfg.run
	pixelScale := cfg.pixelScale

	pangoItem := run.item
	pangoFont := pangoItem.analysis.font
	if pangoFont == nil {
		return verticalPenY
	}

	ftFace := C.pango_ft2_font_get_face(pangoFont)
	if ftFace == nil {
		return verticalPenY
	}

	attrs := parseRunAttributes(pangoItem)
	metrics := getRunMetrics(pangoFont, pangoItem.analysis.language, attrs)

	// Logical extents for ascent/descent.
	var logicalRect C.PangoRectangle
	C.pango_layout_iter_get_run_extents(cfg.iter, nil, &logicalRect)

	runX := float64(logicalRect.x) * pixelScale
	baselinePango := C.pango_layout_iter_get_baseline(cfg.iter)
	ascentPango := baselinePango - logicalRect.y
	descentPango := (logicalRect.y + logicalRect.height) - baselinePango

	runAscent := float64(ascentPango) * pixelScale
	runDescent := float64(descentPango) * pixelScale
	runY := float64(baselinePango) * pixelScale

	// Emoji: override with primary font metrics.
	famName := C.GoString(ftFace.family_name)
	if strings.Contains(famName, "Emoji") && cfg.primaryAscent > 0 {
		runAscent = cfg.primaryAscent
		runDescent = cfg.primaryDescent
	}

	// Extract glyphs.
	glyphString := run.glyphs
	numGlyphs := int(glyphString.num_glyphs)
	startGlyphIdx := len(*allGlyphs)
	var width float64

	for i := range numGlyphs {
		info := (*[1 << 20]C.PangoGlyphInfo)(unsafe.Pointer(glyphString.glyphs))[i]
		xOff := float64(info.geometry.x_offset) * pixelScale
		yOff := float64(info.geometry.y_offset) * pixelScale
		xAdv := float64(info.geometry.width) * pixelScale

		lineHeight := cfg.primaryAscent + cfg.primaryDescent
		var fxOff, fyOff, fxAdv, fyAdv float64
		switch cfg.orientation {
		case OrientationVertical:
			centerOffset := (lineHeight - xAdv) / 2.0
			fxOff = centerOffset
			fyOff = yOff
			fxAdv = 0
			fyAdv = -lineHeight
		default:
			fxOff = xOff
			fyOff = yOff
			fxAdv = xAdv
			fyAdv = 0
		}

		*allGlyphs = append(*allGlyphs, Glyph{
			Index:    uint32(info.glyph),
			XOffset:  fxOff,
			YOffset:  fyOff,
			XAdvance: fxAdv,
			YAdvance: fyAdv,
		})
		width += xAdv
	}

	glyphCount := len(*allGlyphs) - startGlyphIdx

	// Run position (horizontal vs vertical).
	lineHeightRun := cfg.primaryAscent + cfg.primaryDescent
	var finalRunX, finalRunY, newVerticalPenY float64
	switch cfg.orientation {
	case OrientationVertical:
		newVerticalPenY = verticalPenY + lineHeightRun*float64(glyphCount)
		finalRunX = runY
		finalRunY = verticalPenY
	default:
		finalRunX = runX
		finalRunY = runY
		newVerticalPenY = verticalPenY
	}

	startIndex := int(pangoItem.offset)
	length := int(pangoItem.length)

	// Color fallback.
	finalColor := attrs.Color
	if finalColor.A == 0 {
		finalColor = cfg.baseColor
	}
	if finalColor.A == 0 && cfg.strokeWidth <= 0 {
		finalColor = Color{0, 0, 0, 255}
	}

	item := Item{
		FTFace:   unsafe.Pointer(ftFace),
		ObjectID: attrs.ObjectID,

		Width:   width,
		X:       finalRunX,
		Y:       finalRunY,
		Ascent:  runAscent,
		Descent: runDescent,

		GlyphStart: startGlyphIdx,
		GlyphCount: glyphCount,
		StartIndex: startIndex,
		Length:     length,

		UnderlineOffset:        metrics.UndPos,
		UnderlineThickness:     metrics.UndThick,
		StrikethroughOffset:    metrics.StrikePos,
		StrikethroughThickness: metrics.StrikeThick,

		Color:   finalColor,
		BgColor: attrs.BgColor,

		StrokeWidth: cfg.strokeWidth,
		StrokeColor: cfg.strokeColor,

		HasUnderline:     attrs.HasUnderline,
		HasStrikethrough: attrs.HasStrikethrough,
		HasBgColor:       attrs.HasBgColor,
		HasStroke:        cfg.strokeWidth > 0,
		UseOriginalColor: (ftFace.face_flags & C.FT_FACE_FLAG_COLOR) != 0,
		IsObject:         attrs.IsObject,
	}
	if item.GlyphCount > 0 || item.IsObject {
		*items = append(*items, item)
	}
	return newVerticalPenY
}

// computeHitTestRects generates character bounding boxes.
func computeHitTestRects(layout PangoLayoutW, text string, scaleFactor float32) []CharRect {
	charRects := make([]CharRect, 0, len(text))

	iter := layout.GetIter()
	if iter.ptr == nil {
		return charRects
	}
	defer iter.Close()

	pixelScale := 1.0 / (float32(PangoScale) * scaleFactor)

	// Fallback width for zero-width spaces.
	fontDesc := PangoLayoutGetFontDescription(layout.ptr)
	var fallbackWidth float32
	if fontDesc != nil {
		sizePango := C.pango_font_description_get_size(fontDesc)
		fallbackWidth = float32(sizePango) * pixelScale / 3.0
	}

	for {
		idx := int(C.pango_layout_iter_get_index(iter.ptr))
		if idx >= len(text) {
			break
		}

		var pos C.PangoRectangle
		C.pango_layout_iter_get_char_extents(iter.ptr, &pos)

		fx := float32(pos.x) * pixelScale
		fy := float32(pos.y) * pixelScale
		fw := float32(pos.width) * pixelScale
		fh := float32(pos.height) * pixelScale

		if fw < 0 {
			fx += fw
			fw = -fw
		}
		if fh < 0 {
			fy += fh
			fh = -fh
		}
		if fw == 0 && idx < len(text) && text[idx] == spaceChar {
			fw = fallbackWidth
		}

		charRects = append(charRects, CharRect{
			Rect:  Rect{X: fx, Y: fy, Width: fw, Height: fh},
			Index: idx,
		})

		if !iter.NextChar() {
			break
		}
	}
	return charRects
}

// computeLines extracts line boundaries from the layout.
func computeLines(layout PangoLayoutW, scaleFactor float32) []Line {
	lineCount := PangoLayoutGetLineCount(layout.ptr)
	lines := make([]Line, 0, lineCount)

	lineIter := layout.GetIter()
	if lineIter.ptr == nil {
		return lines
	}
	defer lineIter.Close()

	pixelScale := 1.0 / (float32(PangoScale) * scaleFactor)

	for {
		linePtr := C.pango_layout_iter_get_line_readonly(lineIter.ptr)
		if linePtr != nil {
			var rect C.PangoRectangle
			C.pango_layout_iter_get_line_extents(lineIter.ptr, nil, &rect)

			lines = append(lines, Line{
				StartIndex: int(linePtr.start_index),
				Length:     int(linePtr.length),
				Rect: Rect{
					X:      float32(rect.x) * pixelScale,
					Y:      float32(rect.y) * pixelScale,
					Width:  float32(rect.width) * pixelScale,
					Height: float32(rect.height) * pixelScale,
				},
				IsParagraphStart: C.glyph_layout_line_is_paragraph_start(linePtr) != 0,
			})
		}

		if !lineIter.NextLine() {
			break
		}
	}
	return lines
}

// logAttrResult holds LogAttr array and byte-index mapping.
type logAttrResult struct {
	Attrs   []LogAttr
	ByIndex map[int]int
}

// extractLogAttrs extracts cursor/word boundary info from PangoLayout.
func extractLogAttrs(layout PangoLayoutW, text string) logAttrResult {
	attrsPtr, nAttrs := PangoLayoutGetLogAttrsReadonly(layout.ptr)
	if attrsPtr == nil || nAttrs == 0 {
		return logAttrResult{}
	}

	iter := layout.GetIter()
	if iter.ptr == nil {
		return logAttrResult{}
	}
	defer iter.Close()

	attrs := make([]LogAttr, nAttrs)
	for i := range nAttrs {
		pa := (*C.PangoLogAttr)(unsafe.Pointer(uintptr(unsafe.Pointer(attrsPtr)) + uintptr(i)*unsafe.Sizeof(*attrsPtr)))
		attrs[i] = LogAttr{
			IsCursorPosition: C.glyph_log_attr_is_cursor_position(pa) != 0,
			IsWordStart:      C.glyph_log_attr_is_word_start(pa) != 0,
			IsWordEnd:        C.glyph_log_attr_is_word_end(pa) != 0,
			IsLineBreak:      C.glyph_log_attr_is_line_break(pa) != 0,
		}
	}

	byIndex := make(map[int]int, nAttrs)
	attrIdx := 0
	for {
		byteIdx := int(C.pango_layout_iter_get_index(iter.ptr))
		if attrIdx < len(attrs) {
			byIndex[byteIdx] = attrIdx
		}
		attrIdx++
		if !iter.NextChar() {
			break
		}
	}
	if attrIdx < len(attrs) {
		byIndex[len(text)] = attrIdx
	}

	return logAttrResult{Attrs: attrs, ByIndex: byIndex}
}
