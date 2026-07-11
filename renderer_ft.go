//go:build android || linux || darwin || windows

package glyph

import "unicode/utf8"

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

	// When layout resolved a concrete glyph id and its font, rasterize by id
	// directly (no re-shaping). Otherwise fall back to text-based shaping,
	// which handles color emoji, unloaded fonts, and ligature-absorbed
	// clusters (GlyphID 0).
	byID := g.GlyphID != 0 && item.FontPath != ""

	var runText string
	var targetRuneIdx int

	key := fnvOffsetBasis
	if byID {
		key = fnvHashU64(key, uint64(g.GlyphID))
		key = fnvHashString(key, item.FontPath)
		key = fnvHashU64(key, uint64(bin))
		key = fnvHashU64(key, uint64(targetH))
		key = fnvHashF32(key, strokeWidth)
		// The render size comes from resolveFTFontParams(item.Style), which
		// reads FontName when Style.Size is 0 (size encoded as "Sans 16").
		// FontName and Size share one .ttf FontPath, so both are needed to key
		// distinct sizes apart; Typeface distinguishes synthetic bold/italic.
		key = fnvHashString(key, item.Style.FontName)
		key = fnvHashF32(key, item.Style.Size)
		key = fnvHashU64(key, uint64(item.Style.Typeface))
	} else {
		runText, targetRuneIdx = computeRunText(text, item, g)
		key = fnvHashString(key, ch)
		key = fnvHashU64(key, uint64(bin))
		key = fnvHashU64(key, uint64(targetH))
		key = fnvHashF32(key, strokeWidth)
		key = fnvHashString(key, item.Style.FontName)
		key = fnvHashF32(key, item.Style.Size)
		key = fnvHashU64(key, uint64(item.Style.Typeface))
		if runText != "" && runText != ch {
			key = fnvHashString(key, runText)
			key = fnvHashU64(key, uint64(targetRuneIdx))
		}
	}

	if cached, ok := r.cache[key]; ok {
		r.cacheAges[key] = r.atlas.FrameCounter
		if cached.Page < 0 {
			return CachedGlyph{}
		}
		return cached
	}

	var result LoadGlyphResult
	var loadErr error
	switch {
	case byID:
		result, loadErr = loadGlyphByIDFT(r.atlas, item.FontPath,
			g.GlyphID, item, strokeWidth, bin, r.scaleFactor)
	case strokeWidth > 0:
		result, loadErr = loadStrokedGlyphFT(r.atlas, ch, runText,
			targetRuneIdx, item, strokeWidth, bin, r.scaleFactor)
	default:
		result, loadErr = loadGlyphFT(r.atlas, ch, runText,
			targetRuneIdx, item, bin, r.scaleFactor)
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

// computeRunText extracts the full run of text spanning the item that
// contains the given glyph, plus the target cluster's rune index within
// that run. When the item has only one glyph the run text is empty (no
// context needed).
func computeRunText(text string, item Item, g Glyph) (runText string, targetRuneIdx int) {
	if item.Length <= 0 {
		return "", 0
	}
	start := item.StartIndex
	end := start + item.Length
	if start < 0 || end > len(text) {
		return "", 0
	}
	runText = text[start:end]
	// Single-glyph items need no contextual shaping.
	if item.GlyphCount <= 1 {
		return "", 0
	}
	byteOff := int(g.Index) - start
	if byteOff < 0 || byteOff > len(runText) {
		// Malformed index: fall back to isolated shaping of ch rather than
		// rasterizing the run's first cluster for the wrong glyph slot.
		return "", 0
	}
	targetRuneIdx = utf8.RuneCountInString(runText[:byteOff])
	return runText, targetRuneIdx
}
