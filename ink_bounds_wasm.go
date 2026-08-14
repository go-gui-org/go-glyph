//go:build js && wasm

package glyph

import "math"

// LayoutInkBounds returns the union of every item's painted bounding box
// in the layout, in layout coordinates (origin at the layout's top-left,
// Y downward) and logical pixels.
//
// The Canvas2D path exposes no per-glyph outline, so each item is
// re-measured as a whole run through measureText's actualBoundingBox*
// fields — the ink box of that run drawn at its own origin. For the
// single-glyph case this API exists to serve (centring an icon on its
// ink) that is the same answer the native path gives.
//
// ok is false when the layout is empty or the canvas reports no ink.
func (ctx *Context) LayoutInkBounds(layout *Layout) (Rect, bool) {
	if ctx == nil || layout == nil || len(layout.Items) == 0 ||
		ctx.ctx2d.IsUndefined() || ctx.ctx2d.IsNull() {
		return Rect{}, false
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	painted := false

	for _, item := range layout.Items {
		start := item.StartIndex
		end := start + item.Length
		if start < 0 || end > len(layout.Text) || start >= end {
			continue
		}
		// A vertical layout steps the pen per glyph (YAdvance), so measuring
		// the run as one horizontal string would report a box spanning every
		// line. TextSystem.InkBounds rejects vertical text up front; this
		// covers direct Context calls.
		if item.GlyphStart >= 0 && item.GlyphStart < len(layout.Glyphs) &&
			layout.Glyphs[item.GlyphStart].YAdvance != 0 {
			continue
		}
		ctx.ctx2d.Set("font", buildCSSFont(item.Style))
		m := ctx.ctx2d.Call("measureText", layout.Text[start:end])
		left := m.Get("actualBoundingBoxLeft").Float()
		right := m.Get("actualBoundingBoxRight").Float()
		asc := m.Get("actualBoundingBoxAscent").Float()
		desc := m.Get("actualBoundingBoxDescent").Float()
		// Browsers that predate actualBoundingBox* report undefined, which
		// Float() reads as NaN; NaN must not poison the union (NaN <= 0 is
		// false, so it would slip past the blank guard below).
		if right != right || left != left || asc != asc || desc != desc ||
			right+left <= 0 || asc+desc <= 0 {
			continue
		}
		// Left and Ascent are distances back/up from the drawing origin,
		// which is the item's pen position on its baseline.
		minX = math.Min(minX, item.X-left)
		maxX = math.Max(maxX, item.X+right)
		minY = math.Min(minY, item.Y-asc)
		maxY = math.Max(maxY, item.Y+desc)
		painted = true
	}

	if !painted {
		return Rect{}, false
	}
	return Rect{
		X:      float32(minX),
		Y:      float32(minY),
		Width:  float32(maxX - minX),
		Height: float32(maxY - minY),
	}, true
}
