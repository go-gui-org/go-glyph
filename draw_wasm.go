//go:build js && wasm

package glyph

import (
	"syscall/js"
	"unicode/utf8"
)

// canvas2DProvider is implemented by backends that expose a
// Canvas2D context for direct fillText rendering.
type canvas2DProvider interface {
	Canvas2DContext() any
}

// getMainContext returns the main canvas 2D context if the backend
// supports direct text rendering.
func (r *Renderer) getMainContext() (js.Value, bool) {
	if p, ok := r.backend.(canvas2DProvider); ok {
		if v, ok := p.Canvas2DContext().(js.Value); ok {
			return v, true
		}
	}
	return js.Value{}, false
}

// drawBoxIfBuiltin draws g as a built-in box-drawing, block-element or
// Powerline glyph at the given baseline pen position, in the color and
// alpha the caller has already set on ctx2d, and reports whether it did.
//
// A false return means the caller should draw the glyph from the font as
// usual: the codepoint is outside the built-in ranges, the style opted out,
// the run is stroked, or the cell is unusable. Powerline is never
// synthesized here — the gate in boxMetricsFor requires an authoritative
// .notdef from the item's own face, which Canvas2D cannot report.
//
// Single-rune clusters only, matching the native path in getOrLoadGlyph: a
// combining mark on a box character is vanishingly rare and the font
// handles it correctly.
// baseX is the pen position the item's cell grid starts at, in the same
// logical units as penX; it anchors the cell-slot snapping that makes a
// coalesced run tile (issue #102).
func (r *Renderer) drawBoxIfBuiltin(ctx2d js.Value, text string, item Item,
	g Glyph, baseX, penX, penY float32, alpha float64) bool {

	ch := glyphText(text, g)
	cp, n := utf8.DecodeRuneInString(ch)
	if n != len(ch) || n == 0 {
		return false
	}
	m, ok := boxMetricsFor(item, g, cp, r.scaleFactor)
	if !ok {
		return false
	}
	ox, oy := boxCellOrigin(penX, penY, r.scaleFactor, m)
	// Same reasoning as the atlas path: rounding each origin on its own is
	// not enough inside a run, where the pen steps by the font's fractional
	// advance while every cell bitmap is a constant m.cellW wide.
	if item.Style.CellWidth > 0 {
		ox = boxSnapOriginX(pxRoundOrigin(baseX*r.scaleFactor), ox, m.cellW)
	}
	r.boxSink.reset(ctx2d, m, ox, oy, r.scaleInv, alpha)
	drawBoxGlyphTo(&r.boxSink, m)
	return true
}

// drawLayoutImpl renders text using Canvas2D fillText directly,
// bypassing the atlas pipeline for dramatically faster rendering.
func (r *Renderer) drawLayoutImpl(layout Layout, x, y float32,
	transform AffineTransform, gradient *GradientConfig) {

	ctx2d, ok := r.getMainContext()
	if !ok {
		return
	}

	// A transform that is not finite maps every point it
	// touches to garbage, so draw nothing. Same for a draw
	// origin that is not finite.
	if !transform.IsFinite() || !finiteF32(x) || !finiteF32(y) {
		return
	}

	isIdentity := transform.IsIdentity()
	fillMode := canvasFillModeFor(gradient)
	var ext gradientExtents
	if fillMode != canvasFillFlat {
		ext = layoutGradientExtents(&layout)
	}

	// combined folds the draw origin into the matrix for the
	// fill-rect path (backgrounds, decorations). The fillText
	// path instead installs transform + origin directly with
	// setCanvasTransform, once per item, not once per glyph.
	var combined AffineTransform
	if !isIdentity {
		combined = AffineTranslation(x, y).Multiply(transform)
	}

	// 1. Backgrounds.
	r.emitBackgrounds(&layout, x, y, combined, isIdentity)

	// 2. Stroke outlines via strokeText.
	for _, item := range layout.Items {
		if !item.HasStroke || item.UseOriginalColor {
			continue
		}
		sc := item.StrokeColor
		cssFont := item.CSSFont
		if cssFont == "" {
			continue
		}

		ctx2d.Set("font", cssFont)
		ctx2d.Set("strokeStyle", cssColorRGB(sc))
		ctx2d.Set("lineWidth", float64(item.StrokeWidth))
		ctx2d.Set("textBaseline", "alphabetic")
		ctx2d.Set("globalAlpha", float64(sc.A)/255.0)

		cx := float32(item.X)
		cy := float32(item.Y)

		// One canvas transform per item, not per glyph: each
		// setTransform/reset pair crosses the JS bridge.
		if !isIdentity {
			setCanvasTransform(ctx2d, transform, x, y, r.scaleFactor)
		}
		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			if (g.Index & PangoGlyphUnknownFlag) != 0 {
				cx += float32(g.XAdvance)
				cy -= float32(g.YAdvance)
				continue
			}

			gx := cx + float32(g.XOffset)
			gy := cy - float32(g.YOffset)

			ch := glyphText(layout.Text, g)
			if isIdentity {
				ctx2d.Call("strokeText", ch,
					float64(x+gx), float64(y+gy))
			} else {
				ctx2d.Call("strokeText", ch,
					float64(gx), float64(gy))
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
		if !isIdentity {
			resetCanvasTransform(ctx2d, r.scaleFactor)
		}
	}
	ctx2d.Set("globalAlpha", 1.0)

	// 3. Fill text via fillText. A vertical gradient is one Canvas2D
	// gradient for the whole call, built on first use: every item shares
	// the same span and matrix, and each build crosses the JS bridge once
	// per stop.
	var canvasGrad js.Value
	for _, item := range layout.Items {
		if item.HasStroke && item.Color.A == 0 {
			continue
		}
		c := item.Color
		if item.UseOriginalColor {
			c = Color{255, 255, 255, 255}
		}

		cssFont := item.CSSFont
		if cssFont == "" {
			continue
		}

		ctx2d.Set("font", cssFont)
		ctx2d.Set("textBaseline", "alphabetic")

		// Tracks globalAlpha alongside every Set of it, so a built-in box
		// glyph can modulate it for the shade blocks and put it back.
		alpha := 1.0

		// Every mode sets fillStyle here or per glyph below, so no item
		// inherits the style the previous item or pass left.
		switch fillMode {
		case canvasFillGradient:
			if canvasGrad.IsUndefined() {
				canvasGrad = newCanvasGradient(ctx2d, gradient,
					ext, y, isIdentity)
			}
			ctx2d.Set("fillStyle", canvasGrad)
			ctx2d.Set("globalAlpha", 1.0)
		case canvasFillFlat:
			alpha = float64(c.A) / 255.0
			ctx2d.Set("globalAlpha", alpha)
			ctx2d.Set("fillStyle", cssColorRGB(c))
		case canvasFillPerGlyph:
		}

		cx := float32(item.X)
		cy := float32(item.Y)

		// One canvas transform per item, not per glyph: each
		// setTransform/reset pair crosses the JS bridge.
		if !isIdentity {
			setCanvasTransform(ctx2d, transform, x, y, r.scaleFactor)
		}
		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			if (g.Index & PangoGlyphUnknownFlag) != 0 {
				cx += float32(g.XAdvance)
				cy -= float32(g.YAdvance)
				continue
			}

			// Per-glyph color for horizontal and diagonal gradients.
			if fillMode == canvasFillPerGlyph {
				gc := gradientColorForGlyph(gradient, cx, cy,
					float32(item.Ascent),
					ext.xOff, ext.yOff, ext.w, ext.h)
				alpha = float64(gc.A) / 255.0
				ctx2d.Set("globalAlpha", alpha)
				ctx2d.Set("fillStyle", cssColorRGB(gc))
			}

			gx := cx + float32(g.XOffset)
			gy := cy - float32(g.YOffset)
			ch := glyphText(layout.Text, g)

			// Box-drawing and block codepoints are drawn from the built-in
			// cell geometry instead of the font, so a frame's stroke weight
			// is uniform and neighbouring cells abut exactly (issue #101).
			// Only under the identity transform: the snapping that buys
			// those properties assumes the cell sits square on the pixel
			// grid, which a rotation or skew breaks.
			if isIdentity && r.drawBoxIfBuiltin(ctx2d, layout.Text, item, g,
				x+float32(item.X), x+gx, y+gy, alpha) {
				cx += float32(g.XAdvance)
				cy -= float32(g.YAdvance)
				continue
			}

			if isIdentity {
				ctx2d.Call("fillText", ch,
					float64(x+gx), float64(y+gy))
			} else {
				ctx2d.Call("fillText", ch,
					float64(gx), float64(gy))
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
		if !isIdentity {
			resetCanvasTransform(ctx2d, r.scaleFactor)
		}
	}
	ctx2d.Set("globalAlpha", 1.0)

	// 4. Decorations (underline / strikethrough).
	r.emitDecorations(&layout, gradient, ext, x, y, combined, isIdentity)
}

// newCanvasGradient builds the Canvas2D linear gradient for a vertical
// text gradient. The stops keep their own alpha, because globalAlpha is 1
// while it fills.
func newCanvasGradient(ctx2d js.Value, gradient *GradientConfig,
	ext gradientExtents, oy float32, isIdentity bool) js.Value {

	y0, y1 := ext.canvasSpanY(oy, isIdentity)
	g := ctx2d.Call("createLinearGradient", 0, float64(y0), 0, float64(y1))
	for _, stop := range gradient.Stops {
		g.Call("addColorStop", float64(stop.Position),
			cssColorRGBA(stop.Color))
	}
	return g
}

// setCanvasTransform installs the layout transform. setTransform replaces
// the whole matrix, so the backend's device-pixel-ratio scale has to be
// folded in here rather than left standing: on a HiDPI canvas the base
// transform is a uniform scale by the ratio, and dropping it would draw a
// transformed run at half size in the top-left corner.
func setCanvasTransform(ctx2d js.Value, t AffineTransform,
	ox, oy, scale float32) {

	s := float64(scale)
	ctx2d.Call("setTransform",
		float64(t.XX)*s, float64(t.YX)*s,
		float64(t.XY)*s, float64(t.YY)*s,
		(float64(ox)+float64(t.X0))*s,
		(float64(oy)+float64(t.Y0))*s)
}

// resetCanvasTransform restores the backend's base transform: the
// device-pixel-ratio scale, which leaves callers drawing in logical units.
func resetCanvasTransform(ctx2d js.Value, scale float32) {
	s := float64(scale)
	ctx2d.Call("setTransform", s, 0, 0, s, 0, 0)
}
