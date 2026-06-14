//go:build js && wasm

package glyph

import "syscall/js"

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
				setCanvasTransform(ctx2d, transform, x, y)
				ctx2d.Call("strokeText", ch,
					float64(gx), float64(gy))
				ctx2d.Call("setTransform", 1, 0, 0, 1, 0, 0)
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
			ctx2d.Set("globalAlpha", float64(c.A)/255.0)
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
				ctx2d.Set("globalAlpha", float64(gc.A)/255.0)
				ctx2d.Set("fillStyle", cssColorString(gc))
			}

			gx := cx + float32(g.XOffset)
			gy := cy - float32(g.YOffset)
			ch := glyphText(layout.Text, g)

			if isIdentity {
				ctx2d.Call("fillText", ch,
					float64(x+gx), float64(y+gy))
			} else {
				setCanvasTransform(ctx2d, transform, x, y)
				ctx2d.Call("fillText", ch,
					float64(gx), float64(gy))
				ctx2d.Call("setTransform", 1, 0, 0, 1, 0, 0)
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

func setCanvasTransform(ctx2d js.Value, t AffineTransform,
	ox, oy float32) {
	ctx2d.Call("setTransform",
		float64(t.XX), float64(t.YX),
		float64(t.XY), float64(t.YY),
		float64(ox)+float64(t.X0),
		float64(oy)+float64(t.Y0))
}
