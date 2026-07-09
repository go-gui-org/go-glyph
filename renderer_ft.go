//go:build android || linux

package glyph

func (r *Renderer) DrawLayoutPlaced(layout Layout,
	placements []GlyphPlacement) {
	if len(placements) != len(layout.Glyphs) ||
		len(layout.Glyphs) == 0 {
		return
	}
	r.atlas.Cleanup(r.atlas.FrameCounter)

	for _, item := range layout.Items {
		if item.HasStroke && item.Color.A == 0 {
			continue
		}
		c := item.Color
		if item.UseOriginalColor {
			c = Color{255, 255, 255, 255}
		}

		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			if (g.Index & PangoGlyphUnknownFlag) != 0 {
				continue
			}
			placement := placements[i]
			bin := r.computeSubpixelBin(placement.X, item.UseOriginalColor)
			cg := r.getOrLoadGlyph(layout.Text, item, g, bin, 0)
			r.touchPage(cg)
			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				r.emitPlacedQuad(cg, placement, c,
					float32(item.Ascent), float32(item.Descent),
					item.UseOriginalColor, float32(g.XAdvance))
			}
		}
	}
}

// getOrLoadGlyph retrieves from cache or rasterizes via FreeType.
// strokeWidth > 0 requests a stroked (outline) glyph.
func (r *Renderer) getOrLoadGlyph(text string, item Item, g Glyph,
	bin int, strokeWidth float32) CachedGlyph {

	ch := glyphText(text, g)
	if ch == "" {
		return CachedGlyph{}
	}
	targetH := max(1, int(float32(item.Ascent)*r.scaleFactor))

	key := fnvOffsetBasis
	key = fnvHashString(key, ch)
	key = fnvHashU64(key, uint64(bin))
	key = fnvHashU64(key, uint64(targetH))
	key = fnvHashF32(key, strokeWidth)
	key = fnvHashString(key, item.Style.FontName)
	key = fnvHashF32(key, item.Style.Size)
	key = fnvHashU64(key, uint64(item.Style.Typeface))

	if cached, ok := r.cache[key]; ok {
		r.cacheAges[key] = r.atlas.FrameCounter
		if cached.Page < 0 {
			return CachedGlyph{}
		}
		return cached
	}

	var result LoadGlyphResult
	var loadErr error
	if strokeWidth > 0 {
		result, loadErr = loadStrokedGlyphFT(r.atlas, ch, item,
			strokeWidth, bin, r.scaleFactor)
	} else {
		result, loadErr = loadGlyphFT(r.atlas, ch, item, bin, r.scaleFactor)
	}
	if loadErr != nil {
		failed := CachedGlyph{Page: -1}
		r.cache[key] = failed
		r.cacheAges[key] = r.atlas.FrameCounter
		return CachedGlyph{}
	}

	if result.ResetOccurred {
		for _, k := range r.pageKeys[result.ResetPage] {
			delete(r.cache, k)
			delete(r.cacheAges, k)
		}
		delete(r.pageKeys, result.ResetPage)
	}

	if len(r.cache) >= r.maxCacheEntries {
		r.evictOldestGlyph()
	}

	r.cache[key] = result.Cached
	r.cacheAges[key] = r.atlas.FrameCounter
	r.pageKeys[result.Cached.Page] = append(
		r.pageKeys[result.Cached.Page], key)
	return result.Cached
}
