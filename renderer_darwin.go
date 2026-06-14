//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreGraphics/CoreGraphics.h>
#include <CoreText/CoreText.h>
*/
import "C"
import (
	"math"
	"unsafe"
)

// Renderer rasterizes glyphs via Core Graphics, manages the glyph
// cache and atlas, and emits draw calls through DrawBackend.
//
// Not safe for concurrent use. All methods must be called from a
// single goroutine (typically the render/GL thread).
type Renderer struct {
	backend         DrawBackend
	atlas           *GlyphAtlas
	cache           map[uint64]CachedGlyph
	cacheAges       map[uint64]uint64
	pageKeys        map[int][]uint64
	maxCacheEntries int
	scaleFactor     float32
	scaleInv        float32
}

// RendererConfig configures the Renderer.
type RendererConfig struct {
	MaxGlyphCacheEntries int
}

func NewRenderer(backend DrawBackend, scaleFactor float32) (*Renderer, error) {
	return NewRendererWithConfig(backend, scaleFactor, 1024, 1024,
		RendererConfig{})
}

func NewRendererWithConfig(backend DrawBackend, scaleFactor float32,
	atlasW, atlasH int, cfg RendererConfig) (*Renderer, error) {

	atlas, err := NewGlyphAtlas(backend, atlasW, atlasH)
	if err != nil {
		return nil, err
	}
	safeScale := scaleFactor
	if safeScale <= 0 {
		safeScale = 1.0
	}
	maxEntries := cfg.MaxGlyphCacheEntries
	if maxEntries == 0 {
		maxEntries = 4096
	} else if maxEntries < 256 {
		maxEntries = 256
	}

	return &Renderer{
		backend:         backend,
		atlas:           atlas,
		cache:           make(map[uint64]CachedGlyph, 1024),
		cacheAges:       make(map[uint64]uint64, 1024),
		pageKeys:        make(map[int][]uint64),
		maxCacheEntries: maxEntries,
		scaleFactor:     safeScale,
		scaleInv:        1.0 / safeScale,
	}, nil
}

func (r *Renderer) Free() {
	r.atlas.Free()
	r.cache = nil
	r.cacheAges = nil
	r.pageKeys = nil
}

func (r *Renderer) Commit() {
	r.atlas.FrameCounter++
	r.atlas.SwapAndUpload()
}

func (r *Renderer) DrawLayout(layout Layout, x, y float32) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), nil)
}

func (r *Renderer) DrawLayoutTransformed(layout Layout, x, y float32,
	transform AffineTransform) {
	r.drawLayoutImpl(layout, x, y, transform, nil)
}

func (r *Renderer) DrawLayoutRotated(layout Layout,
	x, y, angle float32) {
	r.drawLayoutImpl(layout, x, y, AffineRotation(angle), nil)
}

func (r *Renderer) DrawLayoutWithGradient(layout Layout, x, y float32,
	gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), gradient)
}

func (r *Renderer) DrawLayoutTransformedWithGradient(layout Layout,
	x, y float32, transform AffineTransform,
	gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, transform, gradient)
}

func (r *Renderer) DrawLayoutPlaced(layout Layout,
	placements []GlyphPlacement) {
	if len(placements) != len(layout.Glyphs) ||
		len(layout.Glyphs) == 0 {
		return
	}
	r.atlas.Cleanup(r.atlas.FrameCounter)

	// Fill pass only (no stroke for placed glyphs on iOS).
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

func (r *Renderer) Atlas() *GlyphAtlas { return r.atlas }

// getOrLoadGlyph retrieves from cache or rasterizes via CG.
// strokeWidth > 0 requests a stroked (outline) glyph.
func (r *Renderer) getOrLoadGlyph(text string, item Item, g Glyph,
	bin int, strokeWidth float32) CachedGlyph {

	// g.Index is a byte offset into layout.Text (not a glyph ID).
	// g.GlyphID is the resolved CGGlyph after GSUB shaping; used as
	// a tiebreaker in the cache key for ligatures where multiple
	// glyphs share the same cluster text.
	ch := glyphText(text, g)
	if ch == "" {
		return CachedGlyph{}
	}
	targetH := int(float32(item.Ascent) * r.scaleFactor)

	key := fnvOffsetBasis
	// Always hash the text — GlyphID alone is not unique
	// across fonts or sizes.
	key = fnvHashString(key, ch)
	if item.Style.Features != nil {
		for _, f := range item.Style.Features.OpenTypeFeatures {
			key = fnvHashString(key, f.Tag)
			key = fnvHashU64(key, uint64(f.Value))
		}
	}
	if g.GlyphID != 0 {
		// Include GlyphID as a tiebreaker for ligatures where
		// multiple glyphs share the same cluster text.
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

	// Rasterize via Core Graphics.
	var result LoadGlyphResult
	var err error
	if strokeWidth > 0 {
		result, err = loadStrokedGlyphCG(r.atlas, ch, item,
			g.GlyphID, strokeWidth, bin, r.scaleFactor)
	} else {
		result, err = loadGlyphCG(r.atlas, ch, item,
			g.GlyphID, bin, r.scaleFactor)
	}
	if err != nil {
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

func (r *Renderer) evictOldestGlyph() {
	var oldestKey uint64
	oldestAge := uint64(math.MaxUint64)
	for k, age := range r.cacheAges {
		if age < oldestAge {
			oldestAge = age
			oldestKey = k
		}
	}
	if oldestAge == math.MaxUint64 {
		return
	}
	if cg, ok := r.cache[oldestKey]; ok {
		r.removePageKey(cg.Page, oldestKey)
	}
	delete(r.cache, oldestKey)
	delete(r.cacheAges, oldestKey)
}

func (r *Renderer) removePageKey(page int, key uint64) {
	keys := r.pageKeys[page]
	for i, k := range keys {
		if k == key {
			keys[i] = keys[len(keys)-1]
			r.pageKeys[page] = keys[:len(keys)-1]
			return
		}
	}
}

// ensureStroker is a no-op on iOS (uses CG path stroking).
func (r *Renderer) ensureStroker(_ unsafe.Pointer) {}

// configureStroker is a no-op on iOS.
func (r *Renderer) configureStroker(_ int64) {}

func (r *Renderer) touchPage(cg CachedGlyph) {
	if cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
		r.atlas.Pages[cg.Page].Age = r.atlas.FrameCounter
	}
}

func (r *Renderer) computeSubpixelBin(x float32, isEmoji bool) int {
	if isEmoji {
		return 0
	}
	physX := x * r.scaleFactor
	snapped := float32(math.Round(float64(physX)*4.0)) / 4.0
	frac := snapped - float32(math.Floor(float64(snapped)))
	return int(frac*float32(SubpixelBins)+0.1) & (SubpixelBins - 1)
}

func (r *Renderer) computeDrawOrigin(targetX, targetY float32) (drawOriginX, drawOriginY float32, bin int) {
	scale := r.scaleFactor
	physX := targetX * scale
	snappedX := float32(math.Round(float64(physX)*4.0)) / 4.0
	drawOriginX = float32(math.Floor(float64(snappedX)))
	fracX := snappedX - drawOriginX
	bin = int(fracX*float32(SubpixelBins)+0.1) & (SubpixelBins - 1)
	physY := targetY * scale
	drawOriginY = float32(math.Round(float64(physY)))
	return
}

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
