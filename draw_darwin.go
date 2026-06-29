//go:build darwin && !glyph_pango

package glyph

// drawLayoutImpl is the shared implementation for all DrawLayout*
// variants on iOS. Uses atlas-based rendering like the native
// backend.
func (r *Renderer) drawLayoutImpl(layout Layout, x, y float32,
	transform AffineTransform, gradient *GradientConfig) {

	r.atlas.Cleanup(r.atlas.FrameCounter)

	hasGradient := gradient != nil && len(gradient.Stops) > 0

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

	isIdentity := transform == AffineIdentity()

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

	// 2. Stroke outlines (cached, same as fill path).
	for _, item := range layout.Items {
		if !item.HasStroke || item.UseOriginalColor {
			continue
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

			cg := r.getOrLoadGlyph(layout.Text, item, g, 0,
				item.StrokeWidth)
			r.touchPage(cg)

			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				gx := cx + float32(g.XOffset)
				gy := cy - float32(g.YOffset)
				r.emitGlyphQuad(cg, gx, gy, x, y,
					transform, isIdentity, item.StrokeColor)
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}

	// 3. Fill glyphs.
	for _, item := range layout.Items {
		if item.HasStroke && item.Color.A == 0 {
			continue
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

			targetX := cx + float32(g.XOffset)
			drawOriginX, drawOriginY, bin := r.computeDrawOrigin(
				targetX, cy-float32(g.YOffset))
			if item.UseOriginalColor {
				bin = 0
			}

			cg := r.getOrLoadGlyph(layout.Text, item, g, bin, 0)
			r.touchPage(cg)

			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				c := item.Color
				if item.UseOriginalColor {
					c = Color{255, 255, 255, 255}
				}

				scaleInv := r.scaleInv
				drawX := (drawOriginX + float32(cg.Left)) * scaleInv
				drawY := (drawOriginY - float32(cg.Top)) * scaleInv
				glyphW := float32(cg.Width) * scaleInv
				glyphH := float32(cg.Height) * scaleInv

				// GPU emoji scaling.
				if item.UseOriginalColor && glyphH > 0 {
					targetH := float32(item.Ascent + item.Descent)
					boxW := item.Style.EmojiBoxWidth
					switch {
					case boxW > 0 && glyphW > 0:
						// Grid caller: fill the reserved cell box (boxW × line
						// height), preserving aspect, centered on both axes, so
						// wide glyphs (flags) and square glyphs (people) fill the
						// same width instead of the font's narrower advance.
						emojiScale := min(boxW/glyphW, targetH/glyphH)
						glyphW *= emojiScale
						glyphH *= emojiScale
						drawX = drawOriginX*scaleInv + (boxW-glyphW)*0.5
						drawY = drawOriginY*scaleInv - float32(item.Ascent) +
							(targetH-glyphH)*0.5
					case glyphH != targetH:
						emojiScale := targetH / glyphH
						adv := float32(g.XAdvance)
						if adv > 0 && glyphW*emojiScale > adv {
							emojiScale = adv / glyphW
						}
						glyphW *= emojiScale
						glyphH *= emojiScale
						drawX = (drawOriginX + float32(cg.Left)*emojiScale) * scaleInv
						drawY = drawOriginY*scaleInv - float32(item.Ascent) +
							(targetH-glyphH)*0.5
					}
				}

				if hasGradient {
					c = gradientColorForGlyph(gradient, cx, cy,
						float32(item.Ascent),
						gradXOff, gradYOff, gradW, gradH)
				}

				page := r.atlas.Pages[cg.Page]
				src := Rect{
					X:      float32(cg.X),
					Y:      float32(cg.Y),
					Width:  float32(cg.Width),
					Height: float32(cg.Height),
				}

				if hasGradient &&
					gradient.Direction == GradientVertical &&
					glyphH > 0 {

					numStrips := gradientStripCount(glyphH)
					stripSrcH := src.Height / float32(numStrips)
					stripDstH := glyphH / float32(numStrips)
					glyphTopY := float32(item.Y) - float32(item.Ascent)
					for s := range numStrips {
						sf := float32(s)
						stripSrc := Rect{
							X: src.X, Y: src.Y + sf*stripSrcH,
							Width: src.Width, Height: stripSrcH,
						}
						stripDstY := drawY + sf*stripDstH
						stripMidY := glyphTopY + (sf+0.5)*stripDstH
						t := clamp01((stripMidY - gradYOff) / gradH)
						sc := GradientColorAt(gradient.Stops, t)

						if isIdentity {
							dst := Rect{X: x + drawX, Y: y + stripDstY,
								Width: glyphW, Height: stripDstH}
							r.backend.DrawTexturedQuad(
								page.TextureID, stripSrc, dst, sc)
						} else {
							dst := Rect{X: drawX, Y: stripDstY,
								Width: glyphW, Height: stripDstH}
							r.backend.DrawTexturedQuadTransformed(
								page.TextureID, stripSrc, dst, sc,
								AffineTranslation(x, y).Multiply(transform))
						}
					}
				} else if isIdentity {
					dst := Rect{X: x + drawX, Y: y + drawY,
						Width: glyphW, Height: glyphH}
					r.backend.DrawTexturedQuad(
						page.TextureID, src, dst, c)
				} else {
					dst := Rect{X: drawX, Y: drawY,
						Width: glyphW, Height: glyphH}
					r.backend.DrawTexturedQuadTransformed(
						page.TextureID, src, dst, c,
						AffineTranslation(x, y).Multiply(transform))
				}
			}

			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}

		// 4. Decorations.
		if item.HasUnderline || item.HasStrikethrough {
			runX := float32(item.X)
			runY := float32(item.Y)
			decoColor := item.Color
			if hasGradient {
				decoColor = gradientColorForGlyph(gradient,
					runX, runY, float32(item.Ascent),
					gradXOff, gradYOff, gradW, gradH)
			}

			if item.HasUnderline {
				lineX := runX
				lineY := runY + float32(item.UnderlineOffset) -
					float32(item.UnderlineThickness)
				lineW := float32(item.Width)
				lineH := float32(item.UnderlineThickness)
				r.emitDecorationRect(lineX, lineY, lineW, lineH,
					x, y, transform, isIdentity, decoColor)
			}
			if item.HasStrikethrough {
				lineX := runX
				lineY := runY - float32(item.StrikethroughOffset) +
					float32(item.StrikethroughThickness)
				lineW := float32(item.Width)
				lineH := float32(item.StrikethroughThickness)
				r.emitDecorationRect(lineX, lineY, lineW, lineH,
					x, y, transform, isIdentity, decoColor)
			}
		}
	}
}

func (r *Renderer) emitGlyphQuad(cg CachedGlyph, gx, gy, ox, oy float32,
	transform AffineTransform, isIdentity bool, color Color) {

	scaleInv := r.scaleInv
	drawX := gx + float32(cg.Left)*scaleInv
	drawY := gy - float32(cg.Top)*scaleInv
	w := float32(cg.Width) * scaleInv
	h := float32(cg.Height) * scaleInv

	page := r.atlas.Pages[cg.Page]
	src := Rect{
		X:      float32(cg.X),
		Y:      float32(cg.Y),
		Width:  float32(cg.Width),
		Height: float32(cg.Height),
	}

	if isIdentity {
		dst := Rect{X: ox + drawX, Y: oy + drawY, Width: w, Height: h}
		r.backend.DrawTexturedQuad(page.TextureID, src, dst, color)
	} else {
		dst := Rect{X: drawX, Y: drawY, Width: w, Height: h}
		r.backend.DrawTexturedQuadTransformed(
			page.TextureID, src, dst, color,
			AffineTranslation(ox, oy).Multiply(transform))
	}
}

func (r *Renderer) emitPlacedQuad(cg CachedGlyph,
	placement GlyphPlacement, color Color, ascent, descent float32,
	useOriginalColor bool, xAdvance float32) {

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
		combined := AffineTranslation(placement.X, placement.Y).
			Multiply(transform)
		r.backend.DrawTexturedQuadTransformed(
			page.TextureID, src, dst, color, combined)
	} else {
		dst.X += placement.X
		dst.Y += placement.Y
		r.backend.DrawTexturedQuad(page.TextureID, src, dst, color)
	}
}

func (r *Renderer) emitDecorationRect(lx, ly, lw, lh, ox, oy float32,
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

func gradientStripCount(glyphH float32) int {
	return max(4, min(16, int(glyphH+0.5)))
}

func clamp01(v float32) float32 { return max(0, min(1, v)) }
