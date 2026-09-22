//go:build android || linux || darwin || windows || (js && wasm)

package glyph

// emitFillRect draws dst (layout coords, origin not yet applied)
// in color c. One shared helper for backgrounds and decorations,
// so the atlas path and the WASM path cannot drift apart.
//
// Under identity it offsets dst by (ox, oy) and calls
// DrawFilledRect. Under a transform it calls
// DrawFilledRectTransformed with combined, which already holds
// the origin (Translation(ox, oy) after transform), so the fill
// rotates with the glyphs. Backends without
// TransformedFillBackend fall back to an origin-shifted
// axis-aligned rect through combined.
func (r *Renderer) emitFillRect(dst Rect, c Color, ox, oy float32,
	combined AffineTransform, isIdentity bool) {

	if isIdentity {
		dst.X += ox
		dst.Y += oy
		r.backend.DrawFilledRect(dst, c)
		return
	}
	if tf, ok := r.backend.(TransformedFillBackend); ok {
		tf.DrawFilledRectTransformed(dst, c, combined)
		return
	}
	tx, ty := combined.Apply(dst.X, dst.Y)
	dst.X, dst.Y = tx, ty
	r.backend.DrawFilledRect(dst, c)
}

// emitBackgrounds draws every item's background rect. Backgrounds rotate
// with the glyphs through emitFillRect, so a rotated run keeps its
// highlight behind the text.
func (r *Renderer) emitBackgrounds(layout *Layout, ox, oy float32,
	combined AffineTransform, isIdentity bool) {

	for i := range layout.Items {
		item := &layout.Items[i]
		if !item.HasBgColor {
			continue
		}
		r.emitFillRect(Rect{
			X:      float32(item.X),
			Y:      float32(item.Y) - float32(item.Ascent),
			Width:  float32(item.Width),
			Height: float32(item.Ascent + item.Descent),
		}, item.BgColor, ox, oy, combined, isIdentity)
	}
}

// emitDecorations draws every item's underline and strikethrough after
// all glyphs, so a line sits on top of the text in every backend. Under a
// gradient the line takes the gradient color at the run start.
func (r *Renderer) emitDecorations(layout *Layout, gradient *GradientConfig,
	ext gradientExtents, ox, oy float32, combined AffineTransform,
	isIdentity bool) {

	hasGradient := gradient != nil && len(gradient.Stops) > 0
	for i := range layout.Items {
		item := &layout.Items[i]
		if !item.HasUnderline && !item.HasStrikethrough {
			continue
		}
		runX := float32(item.X)
		runY := float32(item.Y)
		decoColor := item.Color
		if hasGradient {
			decoColor = gradientColorForGlyph(gradient, runX, runY,
				float32(item.Ascent), ext.xOff, ext.yOff, ext.w, ext.h)
		}
		// A transparent line draws nothing; skip the backend call. This
		// is the stroke-only case (HasStroke with a clear fill color).
		if decoColor.A == 0 {
			continue
		}
		if item.HasUnderline {
			r.emitFillRect(Rect{
				X: runX,
				Y: runY + float32(item.UnderlineOffset) -
					float32(item.UnderlineThickness),
				Width:  float32(item.Width),
				Height: float32(item.UnderlineThickness),
			}, decoColor, ox, oy, combined, isIdentity)
		}
		if item.HasStrikethrough {
			r.emitFillRect(Rect{
				X: runX,
				Y: runY - float32(item.StrikethroughOffset) +
					float32(item.StrikethroughThickness),
				Width:  float32(item.Width),
				Height: float32(item.StrikethroughThickness),
			}, decoColor, ox, oy, combined, isIdentity)
		}
	}
}
