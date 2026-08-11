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

	hasGradient := gradient != nil && len(gradient.Stops) > 0
	isIdentity := transform == AffineIdentity()

	// Pre-compute gradient extents.
	var gradXOff, gradYOff float32
	gradW := float32(1.0)
	gradH := float32(1.0)
	if hasGradient {
		if layout.VisualWidth > 0 {
			gradW = layout.VisualWidth
		}
		if layout.VisualHeight > 0 {
			gradH = layout.VisualHeight
		}
		if len(layout.Items) > 0 {
			gradXOff = float32(layout.Items[0].X)
			gradYOff = float32(layout.Items[0].Y) -
				float32(layout.Items[0].Ascent)
			for _, item := range layout.Items {
				ix := float32(item.X)
				iy := float32(item.Y) - float32(item.Ascent)
				if ix < gradXOff {
					gradXOff = ix
				}
				if iy < gradYOff {
					gradYOff = iy
				}
			}
		}
	}

	// 1. Backgrounds.
	for _, item := range layout.Items {
		if !item.HasBgColor {
			continue
		}
		bgX := float32(item.X)
		bgY := float32(item.Y) - float32(item.Ascent)
		bgW := float32(item.Width)
		bgH := float32(item.Ascent + item.Descent)

		if isIdentity {
			r.backend.DrawFilledRect(
				Rect{X: x + bgX, Y: y + bgY, Width: bgW, Height: bgH},
				item.BgColor)
		} else {
			tx, ty := transformLayoutPoint(transform, x, y, bgX, bgY)
			r.backend.DrawFilledRect(
				Rect{X: tx, Y: ty, Width: bgW, Height: bgH},
				item.BgColor)
		}
	}

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
		ctx2d.Set("strokeStyle", cssColorString(sc))
		ctx2d.Set("lineWidth", float64(item.StrokeWidth))
		ctx2d.Set("textBaseline", "alphabetic")
		ctx2d.Set("globalAlpha", float64(sc.A)/255.0)

		cx := float32(item.X)
		cy := float32(item.Y)

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
				setCanvasTransform(ctx2d, transform, x, y, r.scaleFactor)
				ctx2d.Call("strokeText", ch,
					float64(gx), float64(gy))
				resetCanvasTransform(ctx2d, r.scaleFactor)
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}
	ctx2d.Set("globalAlpha", 1.0)

	// 3. Fill text via fillText.
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

		// For vertical gradient, create a Canvas2D linear gradient
		// that the browser composites per-pixel.
		if hasGradient && gradient.Direction == GradientVertical {
			canvasGrad := ctx2d.Call("createLinearGradient",
				0, float64(y+gradYOff),
				0, float64(y+gradYOff+gradH))
			for _, stop := range gradient.Stops {
				canvasGrad.Call("addColorStop",
					float64(stop.Position),
					cssColorString(stop.Color))
			}
			ctx2d.Set("fillStyle", canvasGrad)
			ctx2d.Set("globalAlpha", 1.0)
		} else if !hasGradient {
			alpha = float64(c.A) / 255.0
			ctx2d.Set("globalAlpha", alpha)
			ctx2d.Set("fillStyle", cssColorString(c))
		}

		cx := float32(item.X)
		cy := float32(item.Y)

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

			// Per-glyph color for horizontal gradient only.
			if hasGradient &&
				gradient.Direction == GradientHorizontal {
				gc := gradientColorForGlyph(gradient, cx, cy,
					float32(item.Ascent),
					gradXOff, gradYOff, gradW, gradH)
				alpha = float64(gc.A) / 255.0
				ctx2d.Set("globalAlpha", alpha)
				ctx2d.Set("fillStyle", cssColorString(gc))
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
				setCanvasTransform(ctx2d, transform, x, y, r.scaleFactor)
				ctx2d.Call("fillText", ch,
					float64(gx), float64(gy))
				resetCanvasTransform(ctx2d, r.scaleFactor)
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}
	ctx2d.Set("globalAlpha", 1.0)

	// 4. Decorations (underline / strikethrough).
	for _, item := range layout.Items {
		if !item.HasUnderline && !item.HasStrikethrough {
			continue
		}
		runX := float32(item.X)
		runY := float32(item.Y)
		decoColor := item.Color
		if hasGradient {
			decoColor = gradientColorForGlyph(gradient, runX, runY,
				float32(item.Ascent),
				gradXOff, gradYOff, gradW, gradH)
		}

		if item.HasUnderline {
			lineX := runX
			lineY := runY + float32(item.UnderlineOffset) -
				float32(item.UnderlineThickness)
			lineW := float32(item.Width)
			lineH := float32(item.UnderlineThickness)
			emitDecorationRect(r, lineX, lineY, lineW, lineH,
				x, y, transform, isIdentity, decoColor)
		}
		if item.HasStrikethrough {
			lineX := runX
			lineY := runY - float32(item.StrikethroughOffset) +
				float32(item.StrikethroughThickness)
			lineW := float32(item.Width)
			lineH := float32(item.StrikethroughThickness)
			emitDecorationRect(r, lineX, lineY, lineW, lineH,
				x, y, transform, isIdentity, decoColor)
		}
	}
}

// emitDecorationRect draws an underline or strikethrough line.
func emitDecorationRect(r *Renderer, lx, ly, lw, lh, ox, oy float32,
	transform AffineTransform, isIdentity bool, color Color) {

	if isIdentity {
		r.backend.DrawFilledRect(
			Rect{X: ox + lx, Y: oy + ly, Width: lw, Height: lh},
			color)
	} else {
		tx, ty := transformLayoutPoint(transform, ox, oy, lx, ly)
		r.backend.DrawFilledRect(
			Rect{X: tx, Y: ty, Width: lw, Height: lh}, color)
	}
}

func transformLayoutPoint(transform AffineTransform,
	originX, originY, x, y float32) (float32, float32) {
	tx, ty := transform.Apply(x, y)
	return originX + tx, originY + ty
}

var (
	lastColor Color
	lastCSS   string
)

func cssColorString(c Color) string {
	if c == lastColor && lastCSS != "" {
		return lastCSS
	}
	s := "rgba(" +
		jsItoa(int(c.R)) + "," +
		jsItoa(int(c.G)) + "," +
		jsItoa(int(c.B)) + "," +
		jsAlpha(c.A) + ")"
	lastColor = c
	lastCSS = s
	return s
}

func jsAlpha(a uint8) string {
	if a == 255 {
		return "1"
	}
	if a == 0 {
		return "0"
	}
	v := int(a) * 100 / 255
	return "0." + jsItoa(v/10) + jsItoa(v%10)
}

func jsItoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + jsItoa(-i)
	}
	var buf [10]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
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
