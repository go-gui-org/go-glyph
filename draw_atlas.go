//go:build android || linux || darwin || windows

package glyph

import "unicode/utf8"

// drawLayoutImpl is the shared implementation for all DrawLayout*
// variants on FreeType/CoreText platforms. Uses atlas-based rendering
// like the native backend.
func (r *Renderer) drawLayoutImpl(layout Layout, x, y float32,
	transform AffineTransform, gradient *GradientConfig) {

	// A transform that is not finite maps every point it
	// touches to garbage, so draw nothing. Same for a draw
	// origin that is not finite.
	if !transform.IsFinite() || !finiteF32(x) || !finiteF32(y) {
		return
	}

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

	isIdentity := transform.IsIdentity()

	// combined folds the draw origin into the matrix once, so
	// per-glyph code must not rebuild it. combined.Apply(p)
	// equals origin + transform.Apply(p): glyphs rotate around
	// the layout origin, then move to (x, y).
	var combined AffineTransform
	if !isIdentity {
		combined = AffineTranslation(x, y).Multiply(transform)
	}

	// Pass 1 — resolve (rasterize) every glyph in the layout before any
	// textured quad is emitted, so a mid-call atlas reset cannot leave
	// earlier quads of this call sampling evicted texels (issue #89).
	// The upload itself is deferred to Commit: hosts call Commit after
	// their draw pass and before the render pass samples the textures,
	// so frame-boundary uploads are visible to the same frame's quads —
	// and batching them costs one full-page upload per frame per page
	// instead of one per draw call (a terminal frame issues hundreds of
	// per-glyph calls; per-call uploads multiply 4 MiB page transfers
	// into GB-scale traffic whenever new glyphs appear).
	fills := scratch(&r.scratchFills, len(layout.Glyphs))
	strokes := scratch(&r.scratchStrokes, len(layout.Glyphs))

	// Resolve stroke glyphs first, then fills: both must complete before
	// the single upload so one upload covers every page touched here.
	// Guards mirror the emit loops below; skipped glyphs keep a zero
	// CachedGlyph, which the emit loops' Width > 0 check discards.
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
			strokes[i] = r.getOrLoadGlyph(layout.Text, item, g, 0,
				item.StrokeWidth)
			r.touchPage(strokes[i])
			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}
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
			_, _, bin := r.computeDrawOrigin(
				cx+float32(g.XOffset), cy-float32(g.YOffset))
			if item.UseOriginalColor {
				bin = 0
			}
			fills[i] = r.getOrLoadGlyph(layout.Text, item, g, bin, 0)
			r.touchPage(fills[i])
			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}

	// Push whatever Pass 1 rasterized to the GPU before any quad below
	// samples it. On a backend that samples textures at draw time (GL)
	// this is what keeps a glyph's first appearance from rendering blank
	// for a frame; the upload is bounded by the new glyphs, not the page.
	// It is a no-op on backends without RectTextureUpdater, which are the
	// ones that do not need it — see UploadDirtyRects. Either way Commit
	// still runs at the frame boundary and covers anything left pending.
	//
	// Known edge (pre-existing): when the atlas exhausts its pages, a
	// resolve can reset the oldest page, evicting glyphs resolved earlier
	// in this pass that lived on it — their quads sample cleared texels
	// and render blank for one frame until the next pass re-rasterizes
	// them (ResetOccurred drops their cache entries). Self-healing, and
	// only reachable under full-atlas thrash (> ~4k distinct glyphs).
	r.atlas.UploadDirtyRects()

	// 1. Backgrounds. They rotate with the glyphs through
	// emitFillRect, so a rotated run keeps its highlight
	// behind the text.
	for _, item := range layout.Items {
		if !item.HasBgColor {
			continue
		}
		r.emitFillRect(Rect{
			X:      float32(item.X),
			Y:      float32(item.Y) - float32(item.Ascent),
			Width:  float32(item.Width),
			Height: float32(item.Ascent + item.Descent),
		}, item.BgColor, x, y, combined, isIdentity)
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

			cg := strokes[i]

			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				gx := cx + float32(g.XOffset)
				gy := cy - float32(g.YOffset)
				r.emitGlyphQuad(cg, gx, gy, x, y,
					combined, isIdentity, item.StrokeColor)
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

		// Snap width for this item's cell grid, or 0 when snapping does not
		// apply. Every box glyph in one item shares the same cell box: it
		// derives from the item, not the glyph. Validate it once here
		// instead of re-deriving it per glyph. The per-glyph kind check
		// below stays, because the codepoint varies per glyph.
		//
		// Ask boxMetricsFor with a representative line codepoint (U+2500)
		// so the gates (opt-out, stroke, cell size limits) live in one
		// place. With CellWidth set, the glyph does not enter the line or
		// block result, so an empty Glyph is enough.
		snapCellW := 0
		cellBaseX := 0
		if item.Style.CellWidth > 0 {
			if m, ok := boxMetricsFor(item, Glyph{}, boxLineLo,
				r.scaleFactor); ok {
				snapCellW = m.cellW
				baseX, _, _ := r.computeDrawOrigin(float32(item.X), 0)
				// Through pxRoundOrigin, the same clamp boxCellOrigin applies
				// to box origins: a NaN or absurd item.X must not fall into
				// the implementation-defined float-to-int conversion.
				cellBaseX = pxRoundOrigin(baseX)
			}
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

			targetX := cx + float32(g.XOffset)
			drawOriginX, drawOriginY, _ := r.computeDrawOrigin(
				targetX, cy-float32(g.YOffset))

			// A built-in box bitmap is exactly one cell wide, so it only
			// tiles if the placement steps by that same integer. Inside a
			// coalesced run the pen steps by the font's fractional advance
			// instead, which opens 1px gaps in a rule (issue #102). Snap
			// this cell onto the item's cell grid. Line and block glyphs
			// take the precomputed width, which equals what boxMetricsFor
			// derives for them. Powerline stays on the full path, because
			// its .notdef gate reads the glyph. Font glyphs in the same
			// run are untouched.
			if snapCellW > 0 {
				ch := glyphText(layout.Text, g)
				if cp, n := utf8.DecodeRuneInString(ch); n == len(ch) && n > 0 {
					switch boxGlyphKind(cp) {
					case boxKindLine, boxKindBlock:
						drawOriginX = float32(boxSnapOriginX(
							cellBaseX, pxRoundOrigin(drawOriginX), snapCellW))
					case boxKindPowerline:
						if m, ok := boxMetricsFor(item, g, cp,
							r.scaleFactor); ok {
							drawOriginX = float32(boxSnapOriginX(
								cellBaseX, pxRoundOrigin(drawOriginX), m.cellW))
						}
					case boxKindNone:
					}
				}
			}

			cg := fills[i]

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
								combined)
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
						combined)
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
				r.emitFillRect(Rect{
					X: runX,
					Y: runY + float32(item.UnderlineOffset) -
						float32(item.UnderlineThickness),
					Width:  float32(item.Width),
					Height: float32(item.UnderlineThickness),
				}, decoColor, x, y, combined, isIdentity)
			}
			if item.HasStrikethrough {
				r.emitFillRect(Rect{
					X: runX,
					Y: runY - float32(item.StrikethroughOffset) +
						float32(item.StrikethroughThickness),
					Width:  float32(item.Width),
					Height: float32(item.StrikethroughThickness),
				}, decoColor, x, y, combined, isIdentity)
			}
		}
	}
}

func (r *Renderer) emitGlyphQuad(cg CachedGlyph, gx, gy, ox, oy float32,
	combined AffineTransform, isIdentity bool, color Color) {

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
		// combined already holds the origin, so dst stays
		// in layout coords here.
		dst := Rect{X: drawX, Y: drawY, Width: w, Height: h}
		r.backend.DrawTexturedQuadTransformed(
			page.TextureID, src, dst, color, combined)
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

func gradientStripCount(glyphH float32) int {
	return max(4, min(16, int(glyphH+0.5)))
}

func clamp01(v float32) float32 {
	if v != v || v > 1e20 || v < -1e20 {
		return 0
	}
	return max(0, min(1, v))
}
