//go:build darwin

package glyph

/*
#include <CoreGraphics/CoreGraphics.h>
#include <CoreText/CoreText.h>
*/
import "C"
import (
	"unsafe"
)

func (r *Renderer) DrawLayoutPlaced(layout Layout,
	placements []GlyphPlacement) {
	if len(placements) != len(layout.Glyphs) ||
		len(layout.Glyphs) == 0 {
		return
	}
	r.atlas.Cleanup(r.atlas.FrameCounter)

	// Fill pass only — no stroke for placed glyphs on iOS (Core
	// Graphics handles stroke at rasterization time).
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

// getOrLoadGlyph retrieves from cache or rasterizes via Core Graphics.
// strokeWidth > 0 requests a stroked (outline) glyph.
//
// g.Index is a byte offset into layout.Text (not a glyph ID).
// g.GlyphID is the resolved CGGlyph after GSUB shaping, used as
// a tiebreaker in the cache key for ligatures where multiple
// glyphs share the same cluster text.
func (r *Renderer) getOrLoadGlyph(text string, item Item, g Glyph,
	bin int, strokeWidth float32) CachedGlyph {

	ch := glyphText(text, g)
	if ch == "" {
		return CachedGlyph{}
	}
	targetH := max(1, int(float32(item.Ascent)*r.scaleFactor))

	key := fnvOffsetBasis
	key = fnvHashString(key, ch)
	if item.Style.Features != nil {
		for _, f := range item.Style.Features.OpenTypeFeatures {
			key = fnvHashString(key, f.Tag)
			key = fnvHashU64(key, uint64(f.Value))
		}
	}
	if g.GlyphID != 0 {
		key = fnvHashU64(key, uint64(g.GlyphID))
	}
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
		result, loadErr = loadStrokedGlyphCG(r.atlas, ch, item,
			g.GlyphID, strokeWidth, bin, r.scaleFactor)
	} else {
		result, loadErr = loadGlyphCG(r.atlas, ch, item,
			g.GlyphID, bin, r.scaleFactor)
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

func (r *Renderer) ensureStroker(_ unsafe.Pointer) {}

func (r *Renderer) configureStroker(_ int64) {}

// glyphText extracts the original cluster text for a glyph.
// Index stores byte offset, Codepoint stores byte length.
func glyphText(text string, g Glyph) string {
	start := int(g.Index)
	end := start + int(g.Codepoint)
	if start >= 0 && end <= len(text) {
		return text[start:end]
	}
	return ""
}
