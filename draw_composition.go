package glyph

// compositionAlpha is the opacity factor (about 70%) for the preedit
// underline and cursor. It scales the caller's alpha, so a see-through
// cursor color stays see-through.
const compositionAlpha = 178

// DrawComposition renders IME preedit visual feedback: clause
// underlines and preedit cursor. Call after DrawLayout when
// composition is active.
func (r *Renderer) DrawComposition(layout Layout, x, y float32,
	cs *CompositionState, cursorColor Color) {

	r.drawCompositionImpl(&layout, x, y, AffineIdentity(), cs, cursorColor)
}

// DrawCompositionTransformed renders IME preedit feedback through
// transform. Use it with DrawLayoutTransformed and the same x, y and
// transform, so the underlines and cursor stay on the glyphs. A
// transform or origin that is not finite draws nothing.
func (r *Renderer) DrawCompositionTransformed(layout Layout, x, y float32,
	transform AffineTransform, cs *CompositionState, cursorColor Color) {

	r.drawCompositionImpl(&layout, x, y, transform, cs, cursorColor)
}

func (r *Renderer) drawCompositionImpl(layout *Layout, x, y float32,
	transform AffineTransform, cs *CompositionState, cursorColor Color) {

	if r.backend == nil || cs == nil || !cs.IsComposing() {
		return
	}
	// Same guard as drawLayoutImpl: NaN or Inf would reach the backend.
	if !transform.IsFinite() || !finiteF32(x) || !finiteF32(y) {
		return
	}
	isIdentity := transform.IsIdentity()
	var combined AffineTransform
	if !isIdentity {
		// Fold the origin into the matrix once, as drawLayoutImpl does.
		combined = AffineTranslation(x, y).Multiply(transform)
	}

	dimmed := cursorColor
	dimmed.A = uint8(uint16(cursorColor.A) * compositionAlpha / 255)

	// Draw clause underlines. forEachClause clips each clause to the
	// preedit; rects go into a reused scratch slice.
	cs.forEachClause(func(_, start, end int, style ClauseStyle) {
		thickness := float32(1.0)
		if style == ClauseSelected {
			thickness = 2.0
		}
		r.scratchRects = layout.appendSelectionRects(
			r.scratchRects[:0], start, end)
		for _, rect := range r.scratchRects {
			r.emitFillRect(Rect{
				X:      rect.X,
				Y:      rect.Y + rect.Height - thickness,
				Width:  rect.Width,
				Height: thickness,
			}, dimmed, x, y, combined, isIdentity)
		}
	})

	// Draw cursor at insertion point within preedit.
	if cp, ok := layout.GetCursorPos(cs.DocumentCursorPos()); ok {
		r.emitFillRect(Rect{
			X:      cp.X,
			Y:      cp.Y,
			Width:  2.0,
			Height: cp.Height,
		}, dimmed, x, y, combined, isIdentity)
	}
}
