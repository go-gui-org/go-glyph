//go:build windows

package glyph

// drawLayoutImpl renders a Layout using the atlas-based pipeline.
func (r *Renderer) drawLayoutImpl(layout Layout, x, y float32,
	transform AffineTransform, gradient *GradientConfig) {

	r.atlas.Cleanup(r.atlas.FrameCounter)

	hasGradient := gradient != nil && len(gradient.Stops) > 0
	isIdentity := transform == AffineIdentity()

	// Pre-compute gradient extents from ink bounds.
	var gradXOff, gradYOff float32
	gradW := float32(1.0)
	gradH := float32(1.0)
	if hasGradient {
		if len(layout.Items) > 0 {
			gradXOff = float32(layout.Items[0].X)
			gradYOff = float32(layout.Items[0].Y) - float32(layout.Items[0].Ascent)
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
		gradW = layout.VisualWidth
		gradH = layout.VisualHeight
		if gradW <= 0 {
			gradW = 1
		}
		if gradH <= 0 {
			gradH = 1
		}
	}

	// 1. Draw backgrounds.
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
			tx, ty := transform.Apply(bgX, bgY)
			r.backend.DrawFilledRect(
				Rect{X: x + tx, Y: y + ty, Width: bgW, Height: bgH},
				item.BgColor)
		}
	}

	// 2. Stroke pass: draw dilated outlines behind fill.
	for _, item := range layout.Items {
		if !item.HasStroke || item.UseOriginalColor {
			continue
		}
		physW := item.StrokeWidth * r.scaleFactor

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

			cg := r.getOrLoadStrokedGlyph(layout.Text, item, g, physW)
			r.touchPage(cg)

			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {

				scaleInv := r.scaleInv
				drawX := float32(cg.Left) * scaleInv
				drawY := -float32(cg.Top) * scaleInv
				w := float32(cg.Width) * scaleInv
				h := float32(cg.Height) * scaleInv

				page := r.atlas.Pages[cg.Page]
				src := Rect{
					X:      float32(cg.X),
					Y:      float32(cg.Y),
					Width:  float32(cg.Width),
					Height: float32(cg.Height),
				}

				gx := cx + float32(g.XOffset) + drawX
				gy := cy - float32(g.YOffset) + drawY

				if isIdentity {
					dst := Rect{X: x + gx, Y: y + gy, Width: w, Height: h}
					r.backend.DrawTexturedQuad(
						page.TextureID, src, dst, item.StrokeColor)
				} else {
					dst := Rect{X: gx, Y: gy, Width: w, Height: h}
					r.backend.DrawTexturedQuadTransformed(
						page.TextureID, src, dst, item.StrokeColor,
						AffineTranslation(x, y).Multiply(transform))
				}
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}

	// 3. Draw glyphs (fill pass).
	for _, item := range layout.Items {
		if item.IsObject {
			continue
		}
		if item.HasStroke && item.Color.A == 0 {
			continue // hollow text: stroke only
		}

		c := item.Color
		if item.UseOriginalColor {
			c = Color{255, 255, 255, 255}
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

			bin := r.computeSubpixelBin(cx)
			cg := r.getOrLoadGlyph(layout.Text, item, g, bin)
			r.touchPage(cg)

			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {

				glyphColor := c
				if hasGradient {
					glyphColor = gradientColorForGlyph(gradient, cx, cy,
						float32(item.Ascent),
						gradXOff, gradYOff, gradW, gradH)
				}

				scaleInv := r.scaleInv
				drawX := float32(cg.Left) * scaleInv
				drawY := -float32(cg.Top) * scaleInv
				w := float32(cg.Width) * scaleInv
				h := float32(cg.Height) * scaleInv

				page := r.atlas.Pages[cg.Page]
				src := Rect{
					X:      float32(cg.X),
					Y:      float32(cg.Y),
					Width:  float32(cg.Width),
					Height: float32(cg.Height),
				}

				gx := cx + float32(g.XOffset) + drawX
				gy := cy - float32(g.YOffset) + drawY

				if isIdentity {
					dst := Rect{X: x + gx, Y: y + gy, Width: w, Height: h}
					r.backend.DrawTexturedQuad(
						page.TextureID, src, dst, glyphColor)
				} else {
					dst := Rect{X: gx, Y: gy, Width: w, Height: h}
					r.backend.DrawTexturedQuadTransformed(
						page.TextureID, src, dst, glyphColor,
						AffineTranslation(x, y).Multiply(transform))
				}
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}

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
			winEmitDecoRect(r, lineX, lineY, lineW, lineH,
				x, y, transform, isIdentity, decoColor)
		}
		if item.HasStrikethrough {
			lineX := runX
			lineY := runY - float32(item.StrikethroughOffset) +
				float32(item.StrikethroughThickness)
			lineW := float32(item.Width)
			lineH := float32(item.StrikethroughThickness)
			winEmitDecoRect(r, lineX, lineY, lineW, lineH,
				x, y, transform, isIdentity, decoColor)
		}
	}
}

func winEmitDecoRect(r *Renderer, lx, ly, lw, lh, ox, oy float32,
	transform AffineTransform, isIdentity bool, color Color) {

	if isIdentity {
		r.backend.DrawFilledRect(
			Rect{X: ox + lx, Y: oy + ly, Width: lw, Height: lh},
			color)
	} else {
		tx, ty := transform.Apply(lx, ly)
		r.backend.DrawFilledRect(
			Rect{X: ox + tx, Y: oy + ty, Width: lw, Height: lh}, color)
	}
}

// emitPlacedQuad draws a single glyph at an absolute placement.
func (r *Renderer) emitPlacedQuad(cg CachedGlyph, placement GlyphPlacement,
	color Color, ascent, descent float32, useOriginalColor bool,
	xAdvance float32) {

	scaleInv := r.scaleInv
	dx := float32(cg.Left) * scaleInv
	dy := -float32(cg.Top) * scaleInv
	w := float32(cg.Width) * scaleInv
	h := float32(cg.Height) * scaleInv

	// GPU emoji scaling.
	if useOriginalColor && h > 0 {
		targetH := ascent + descent
		if h != targetH {
			emojiScale := targetH / h
			if xAdvance > 0 && w*emojiScale > xAdvance {
				emojiScale = xAdvance / w
			}
			w *= emojiScale
			h *= emojiScale
			dx = float32(cg.Left) * emojiScale * scaleInv
			dy = -float32(cg.Top)*emojiScale*scaleInv + h - ascent
		}
	}

	page := r.atlas.Pages[cg.Page]
	src := Rect{
		X:      float32(cg.X),
		Y:      float32(cg.Y),
		Width:  float32(cg.Width),
		Height: float32(cg.Height),
	}
	dst := Rect{X: dx, Y: dy, Width: w, Height: h}

	if placement.Angle != 0 {
		transform := AffineRotation(placement.Angle)
		combined := AffineTranslation(placement.X, placement.Y).Multiply(transform)
		r.backend.DrawTexturedQuadTransformed(page.TextureID, src, dst, color, combined)
	} else {
		dst.X += placement.X
		dst.Y += placement.Y
		r.backend.DrawTexturedQuad(page.TextureID, src, dst, color)
	}
}

