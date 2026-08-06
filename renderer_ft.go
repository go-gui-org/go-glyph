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

	// Resolve (rasterize) every glyph before emitting any quad, so a
	// mid-call atlas reset cannot leave this call's quads sampling
	// evicted texels (issue #89). Uploads are deferred to Commit — hosts
	// call it after their draw pass and before the render pass samples
	// the textures, and batching the dirty pages there costs one
	// full-page upload per frame per page instead of one per draw call.
	// Skipped glyphs keep a zero CachedGlyph, discarded by the Width > 0
	// check in the emit pass below.
	cgs := make([]CachedGlyph, len(layout.Glyphs))
	for _, item := range layout.Items {
		if item.HasStroke && item.Color.A == 0 {
			continue
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
			cgs[i] = r.getOrLoadGlyph(layout.Text, item, g, bin, 0)
			r.touchPage(cgs[i])
		}
	}
	// Dirty pages are uploaded once per frame by Commit, never here: a
	// per-call upload would transfer the whole page for every draw call
	// that inserted a glyph (see the resolve-pass comment).

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
			cg := cgs[i]
			if cg.Width > 0 && cg.Height > 0 &&
				cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				r.emitPlacedQuad(cg, placement, c,
					float32(item.Ascent), float32(item.Descent),
					item.UseOriginalColor, float32(g.XAdvance))
			}
		}
	}
}

// hashGlyphStyle folds the appearance inputs shared by both cache-key paths
// (subpixel bin, target height, stroke width, and the Style triple that
// resolveFTFontParams reads) into key. Typeface distinguishes synthetic
// bold/italic.
func hashGlyphStyle(key uint64, item Item, bin, targetH int,
	strokeWidth float32) uint64 {

	key = fnvHashU64(key, uint64(bin))
	key = fnvHashU64(key, uint64(targetH))
	key = fnvHashF32(key, strokeWidth)
	key = fnvHashString(key, item.Style.FontName)
	key = fnvHashF32(key, item.Style.Size)
	key = fnvHashU64(key, uint64(item.Style.Typeface))
	return key
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
	// clusters. A shaped .notdef (GlyphID 0, Shaped set) also renders by id —
	// the font's box glyph — so unsupported scripts (tofu) share one atlas
	// entry per font instead of re-shaping the whole run per distinct
	// codepoint on every scroll re-render.
	byID := item.FontPath != "" && (g.GlyphID != 0 || g.Shaped)

	var runText string
	var targetRuneIdx int

	key := fnvOffsetBasis
	if byID {
		// The render size comes from resolveFTFontParams(item.Style), which
		// reads FontName when Style.Size is 0 (size encoded as "Sans 16").
		// FontName and Size share one .ttf FontPath, so both are needed (via
		// hashGlyphStyle) to key distinct sizes apart.
		key = fnvHashU64(key, uint64(g.GlyphID))
		key = fnvHashString(key, item.FontPath)
		key = hashGlyphStyle(key, item, bin, targetH, strokeWidth)
	} else {
		runText, targetRuneIdx = computeRunText(text, item, g)
		key = fnvHashString(key, ch)
		key = hashGlyphStyle(key, item, bin, targetH, strokeWidth)
		if runText != "" && runText != ch {
			key = fnvHashString(key, runText)
			key = fnvHashU64(key, uint64(targetRuneIdx))
		}
	}

	if e, ok := r.cache[key]; ok {
		// Refresh the LRU age only once it is glyphAgeRefreshFrames stale.
		// Eviction is sampled ("old", not "oldest"), so quantized ages are
		// fine, and steady-state hits stay a single map lookup instead of
		// lookup + full-entry store-back every frame.
		if r.atlas.FrameCounter-e.age >= glyphAgeRefreshFrames {
			e.age = r.atlas.FrameCounter
			r.cache[key] = e
		}
		if e.Page < 0 {
			return CachedGlyph{}
		}
		return e.CachedGlyph
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
		// Negative-cache the failure (Page -1) so a broken glyph is not
		// re-rasterized every frame. The cap applies here too: without it a
		// stream of distinct un-rasterizable glyphs (hostile or corrupt
		// text) would grow the cache without bound.
		if len(r.cache) >= r.maxCacheEntries {
			r.evictOldestGlyph()
		}
		r.cache[key] = cacheEntry{
			CachedGlyph: CachedGlyph{Page: -1},
			age:         r.atlas.FrameCounter,
		}
		return CachedGlyph{}
	}

	if result.ResetOccurred {
		for _, k := range r.pageKeys[result.ResetPage] {
			delete(r.cache, k)
		}
		delete(r.pageKeys, result.ResetPage)
	}

	if len(r.cache) >= r.maxCacheEntries {
		r.evictOldestGlyph()
	}

	r.cache[key] = cacheEntry{
		CachedGlyph: result.Cached,
		age:         r.atlas.FrameCounter,
	}
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
