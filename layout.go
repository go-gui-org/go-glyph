//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

/*
#include <pango/pango.h>
#include <pango/pangoft2.h>
*/
import "C"
import (
	"fmt"
	"strings"
	"unsafe"
)

// LayoutText shapes, wraps, and arranges text using Pango.
// Returns a Layout with positioned glyph runs, hit-test rects,
// line boundaries, and cursor attributes.
func (ctx *Context) LayoutText(text string, cfg TextConfig) (Layout, error) {
	if len(text) == 0 {
		return Layout{}, nil
	}
	if err := ValidateTextInput(text, MaxTextLength, "LayoutText"); err != nil {
		return Layout{}, err
	}

	pl, err := setupPangoLayout(ctx, text, cfg)
	if err != nil {
		return Layout{}, err
	}
	defer pl.Close()

	return buildLayoutFromPango(pl, text, ctx.scaleFactor, cfg), nil
}

// LayoutRichText shapes multi-styled text (RichText).
func (ctx *Context) LayoutRichText(rt RichText, cfg TextConfig) (Layout, error) {
	if len(rt.Runs) == 0 {
		return Layout{}, nil
	}
	for _, run := range rt.Runs {
		if err := ValidateTextInput(run.Text, MaxTextLength, "LayoutRichText"); err != nil {
			return Layout{}, err
		}
	}

	// Build full text and byte ranges.
	var fullText strings.Builder
	type runRange struct {
		start int
		end   int
		style TextStyle
	}
	validRuns := make([]runRange, 0, len(rt.Runs))
	currentIdx := 0
	for _, run := range rt.Runs {
		fullText.WriteString(run.Text)
		encodedLen := len(run.Text)
		validRuns = append(validRuns, runRange{
			start: currentIdx,
			end:   currentIdx + encodedLen,
			style: run.Style,
		})
		currentIdx += encodedLen
	}
	text := fullText.String()

	pl, err := setupPangoLayout(ctx, text, cfg)
	if err != nil {
		return Layout{}, err
	}
	defer pl.Close()

	// Copy existing attribute list, apply per-run styles.
	baseList := PangoLayoutGetAttributes(pl.ptr)
	var attrList PangoAttrListW
	if baseList != nil {
		attrList = PangoAttrListCopy(baseList)
	} else {
		attrList = NewPangoAttrList()
	}

	var clonedIDs []string
	for _, r := range validRuns {
		applyRichTextStyle(ctx, attrList, r.style, r.start, r.end, &clonedIDs)
	}
	pl.SetAttributes(attrList)
	attrList.Close()

	result := buildLayoutFromPango(pl, text, ctx.scaleFactor, cfg)
	result.ClonedObjectIDs = clonedIDs
	return result, nil
}

// setupPangoLayout creates and configures a PangoLayout.
func setupPangoLayout(ctx *Context, text string, cfg TextConfig) (PangoLayoutW, error) {
	// Reset context gravity.
	PangoContextSetBaseGravity(ctx.pangoCtx.ptr)

	pl := NewPangoLayout(ctx.pangoCtx)
	if pl.ptr == nil {
		return PangoLayoutW{}, fmt.Errorf("failed to create Pango layout")
	}

	if cfg.UseMarkup {
		pl.SetMarkup(text)
	} else {
		pl.SetText(text)
	}

	// Width and wrapping.
	if cfg.Block.Width > 0 {
		pl.SetWidth(int(cfg.Block.Width * ctx.scaleFactor * float32(PangoScale)))
		if cfg.Block.Wrap != WrapNone {
			pl.SetWrap(cfg.Block.Wrap)
		}
	}
	pl.SetAlignment(cfg.Block.Align)
	if cfg.Block.Indent != 0 {
		pl.SetIndent(int(cfg.Block.Indent * ctx.scaleFactor * float32(PangoScale)))
	}

	// Font description.
	desc := ctx.createFontDescription(cfg.Style)
	if desc.ptr != nil {
		pl.SetFontDescription(desc)
		desc.Close()
	}

	// Style attributes.
	baseList := PangoLayoutGetAttributes(pl.ptr)
	var attrList PangoAttrListW
	if baseList != nil {
		attrList = PangoAttrListCopy(baseList)
	} else {
		attrList = NewPangoAttrList()
	}

	if attrList.ptr != nil {
		maxIdx := C.guint(C.G_MAXUINT)

		// Background color.
		if cfg.Style.BgColor.A > 0 {
			attr := C.pango_attr_background_new(
				C.guint16(uint16(cfg.Style.BgColor.R)<<8),
				C.guint16(uint16(cfg.Style.BgColor.G)<<8),
				C.guint16(uint16(cfg.Style.BgColor.B)<<8))
			attr.start_index = 0
			attr.end_index = maxIdx
			C.pango_attr_list_insert(attrList.ptr, attr)
		}

		// Underline.
		if cfg.Style.Underline {
			attr := C.pango_attr_underline_new(C.PANGO_UNDERLINE_SINGLE)
			attr.start_index = 0
			attr.end_index = maxIdx
			C.pango_attr_list_insert(attrList.ptr, attr)
		}

		// Strikethrough.
		if cfg.Style.Strikethrough {
			attr := C.pango_attr_strikethrough_new(C.TRUE)
			attr.start_index = 0
			attr.end_index = maxIdx
			C.pango_attr_list_insert(attrList.ptr, attr)
		}

		// Letter spacing.
		if cfg.Style.LetterSpacing != 0 {
			spacing := int(cfg.Style.LetterSpacing * ctx.scaleFactor * float32(PangoScale))
			attr := C.pango_attr_letter_spacing_new(C.int(spacing))
			attr.start_index = 0
			attr.end_index = maxIdx
			C.pango_attr_list_insert(attrList.ptr, attr)
		}

		// OpenType features.
		if cfg.Style.Features != nil && len(cfg.Style.Features.OpenTypeFeatures) > 0 {
			var sb strings.Builder
			for i, f := range cfg.Style.Features.OpenTypeFeatures {
				if i > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "%s=%d", f.Tag, f.Value)
			}
			cs := C.CString(sb.String())
			attr := C.pango_attr_font_features_new(cs)
			C.free(unsafe.Pointer(cs))
			attr.start_index = 0
			attr.end_index = maxIdx
			C.pango_attr_list_insert(attrList.ptr, attr)
		}

		pl.SetAttributes(attrList)
		attrList.Close()
	}

	// Tabs.
	if len(cfg.Block.Tabs) > 0 {
		tabs := NewPangoTabArray(len(cfg.Block.Tabs))
		for i, posPx := range cfg.Block.Tabs {
			posPango := int(float32(posPx) * ctx.scaleFactor * float32(PangoScale))
			tabs.SetTab(i, posPango)
		}
		pl.SetTabs(tabs)
		tabs.Close()
	}

	return pl, nil
}

// buildLayoutFromPango extracts Items, Lines, CharRects from a
// configured PangoLayout.
func buildLayoutFromPango(pl PangoLayoutW, text string,
	scaleFactor float32, cfg TextConfig) Layout {

	iter := pl.GetIter()
	if iter.ptr == nil {
		return Layout{}
	}
	defer iter.Close()

	pixelScale := 1.0 / (float64(PangoScale) * float64(scaleFactor))

	// Primary font metrics for emoji alignment.
	var primaryAscent, primaryDescent float64
	var primaryStrikePos, primaryStrikeThick float64

	fontDesc := PangoLayoutGetFontDescription(pl.ptr)
	if fontDesc != nil {
		pangoCtx := PangoLayoutGetContext(pl.ptr)
		lang := PangoGetDefaultLanguage()
		m := PangoContextGetMetrics(pangoCtx, fontDesc, lang)
		if m != nil {
			primaryAscent = float64(C.pango_font_metrics_get_ascent(m)) * pixelScale
			primaryDescent = float64(C.pango_font_metrics_get_descent(m)) * pixelScale
			primaryStrikePos = float64(C.pango_font_metrics_get_strikethrough_position(m)) * pixelScale
			primaryStrikeThick = float64(C.pango_font_metrics_get_strikethrough_thickness(m)) * pixelScale
			C.pango_font_metrics_unref(m)
		}
	}

	// Fallback: derive from first run's font.
	if primaryAscent == 0 {
		runPtr := C.pango_layout_iter_get_run_readonly(iter.ptr)
		if runPtr != nil {
			font := runPtr.item.analysis.font
			if font != nil {
				lang := runPtr.item.analysis.language
				m := C.pango_font_get_metrics(font, lang)
				if m != nil {
					primaryAscent = float64(C.pango_font_metrics_get_ascent(m)) * pixelScale
					primaryDescent = float64(C.pango_font_metrics_get_descent(m)) * pixelScale
					primaryStrikePos = float64(C.pango_font_metrics_get_strikethrough_position(m)) * pixelScale
					primaryStrikeThick = float64(C.pango_font_metrics_get_strikethrough_thickness(m)) * pixelScale
					C.pango_font_metrics_unref(m)
				}
			}
		}
	}

	var allGlyphs []Glyph
	var items []Item

	var verticalPenY float64
	if cfg.Orientation == OrientationVertical {
		verticalPenY = primaryAscent
	}

	for {
		runPtr := C.pango_layout_iter_get_run_readonly(iter.ptr)
		if runPtr != nil {
			verticalPenY = processRun(&items, &allGlyphs, verticalPenY, processRunConfig{
				run:                runPtr,
				iter:               iter.ptr,
				text:               text,
				scaleFactor:        scaleFactor,
				pixelScale:         pixelScale,
				primaryAscent:      primaryAscent,
				primaryDescent:     primaryDescent,
				primaryStrikePos:   primaryStrikePos,
				primaryStrikeThick: primaryStrikeThick,
				baseColor:          cfg.Style.Color,
				orientation:        cfg.Orientation,
				strokeWidth:        cfg.Style.StrokeWidth,
				strokeColor:        cfg.Style.StrokeColor,
			})
		}
		if !iter.NextRun() {
			break
		}
	}

	var charRects []CharRect
	var charRectByIndex map[int]int
	if !cfg.NoHitTesting {
		charRects = computeHitTestRects(pl, text, scaleFactor)
		charRectByIndex = make(map[int]int, len(charRects))
		for i, cr := range charRects {
			charRectByIndex[cr.Index] = i
		}
	}
	lines := computeLines(pl, scaleFactor)

	var inkRect, logicalRect C.PangoRectangle
	C.pango_layout_get_extents(pl.ptr, &inkRect, &logicalRect)

	ps := float32(PangoScale)
	lWidth := (float32(logicalRect.width) / ps) / scaleFactor
	lHeight := (float32(logicalRect.height) / ps) / scaleFactor

	var vWidth, vHeight float32
	switch cfg.Orientation {
	case OrientationVertical:
		vWidth = lHeight
		vHeight = float32(verticalPenY)
	default:
		vWidth = (float32(inkRect.width) / ps) / scaleFactor
		vHeight = (float32(inkRect.height) / ps) / scaleFactor
	}

	logAttrResult := extractLogAttrs(pl, text)

	result := Layout{
		Text:            text,
		Items:           items,
		Glyphs:          allGlyphs,
		CharRects:       charRects,
		CharRectByIndex: charRectByIndex,
		Lines:           lines,
		LogAttrs:        logAttrResult.Attrs,
		LogAttrByIndex:  logAttrResult.ByIndex,
		Width:           lWidth,
		Height:          lHeight,
		VisualWidth:     vWidth,
		VisualHeight:    vHeight,
	}
	applyLineSpacing(&result, cfg.Block.LineSpacing)
	result.buildPositionCaches()
	return result
}

func applyLineSpacing(layout *Layout, spacing float32) {
	if layout == nil || spacing <= 0 || len(layout.Lines) < 2 {
		return
	}

	lines := append([]Line(nil), layout.Lines...)
	offsets := make([]float32, len(lines))
	var extraHeight float32
	for i := range layout.Lines {
		offsets[i] = extraHeight
		layout.Lines[i].Rect.Y += extraHeight
		if i < len(layout.Lines)-1 {
			layout.Lines[i].Rect.Height += spacing
			extraHeight += spacing
		}
	}

	for i := range layout.Items {
		lineIdx := lineIndexForBaseline(lines, float32(layout.Items[i].Y))
		if lineIdx < 0 {
			continue
		}
		layout.Items[i].Y += float64(offsets[lineIdx])
	}

	for i := range layout.CharRects {
		lineIdx := lineIndexForRect(lines, layout.CharRects[i].Rect)
		if lineIdx < 0 {
			continue
		}
		layout.CharRects[i].Rect.Y += offsets[lineIdx]
	}

	layout.Height += extraHeight
	layout.VisualHeight += extraHeight
}

func lineIndexForBaseline(lines []Line, baselineY float32) int {
	const eps = 0.001
	for i := range lines {
		top := lines[i].Rect.Y - eps
		bottom := lines[i].Rect.Y + lines[i].Rect.Height + eps
		if baselineY >= top && baselineY <= bottom {
			return i
		}
	}
	return -1
}

func lineIndexForRect(lines []Line, rect Rect) int {
	centerY := rect.Y + rect.Height/2
	const eps = 0.001
	for i := range lines {
		top := lines[i].Rect.Y - eps
		bottom := lines[i].Rect.Y + lines[i].Rect.Height + eps
		if centerY >= top && centerY <= bottom {
			return i
		}
	}
	return -1
}
