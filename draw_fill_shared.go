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
