//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

/*
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_STROKER_H
*/
import "C"
import (
	"math"
	"unsafe"
)

// Renderer rasterizes glyphs, manages the glyph cache and atlas,
// and emits draw calls through the DrawBackend interface.
//
// Not safe for concurrent use. All methods must be called from a
// single goroutine (typically the render/GL thread).
type Renderer struct {
	backend         DrawBackend
	atlas           *GlyphAtlas
	cache           map[uint64]CachedGlyph
	cacheAges       map[uint64]uint64
	pageKeys        map[int][]uint64 // page → cache keys (reverse index)
	maxCacheEntries int
	scaleFactor     float32
	scaleInv        float32
	ftStroker       FTStroker
	hasStroker      bool
}

// RendererConfig configures the Renderer.
type RendererConfig struct {
	MaxGlyphCacheEntries int // Default 4096, minimum 256.
}

// NewRenderer creates a Renderer with default 1024x1024 atlas.
func NewRenderer(backend DrawBackend, scaleFactor float32) (*Renderer, error) {
	return NewRendererWithConfig(backend, scaleFactor, 1024, 1024, RendererConfig{})
}

// NewRendererWithConfig creates a Renderer with custom atlas
// size and configuration.
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

// Free releases renderer resources.
func (r *Renderer) Free() {
	if r.hasStroker {
		r.ftStroker.Close()
		r.hasStroker = false
	}
	r.atlas.Free()
	r.cache = nil
	r.cacheAges = nil
	r.pageKeys = nil
}

// Commit uploads dirty atlas pages to the GPU. Call once per frame.
func (r *Renderer) Commit() {
	r.atlas.FrameCounter++
	r.atlas.SwapAndUpload()
}

// DrawLayout renders a Layout at (x, y) using the identity transform.
func (r *Renderer) DrawLayout(layout Layout, x, y float32) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), nil)
}

// DrawLayoutTransformed renders with an affine transform.
func (r *Renderer) DrawLayoutTransformed(layout Layout, x, y float32,
	transform AffineTransform) {
	r.drawLayoutImpl(layout, x, y, transform, nil)
}

// DrawLayoutRotated renders rotated by angle (radians).
func (r *Renderer) DrawLayoutRotated(layout Layout, x, y, angle float32) {
	r.drawLayoutImpl(layout, x, y, AffineRotation(angle), nil)
}

// DrawLayoutWithGradient renders with gradient colors.
func (r *Renderer) DrawLayoutWithGradient(layout Layout, x, y float32,
	gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), gradient)
}

// DrawLayoutTransformedWithGradient renders with both transform and gradient.
func (r *Renderer) DrawLayoutTransformedWithGradient(layout Layout,
	x, y float32, transform AffineTransform, gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, transform, gradient)
}

// DrawLayoutPlaced renders each glyph at individual placements.
// Decorations are skipped. placements must match layout.Glyphs length.
func (r *Renderer) DrawLayoutPlaced(layout Layout, placements []GlyphPlacement) {
	if len(placements) != len(layout.Glyphs) || len(layout.Glyphs) == 0 {
		return
	}
	r.atlas.Cleanup(r.atlas.FrameCounter)

	// Ensure stroker if needed.
	for _, item := range layout.Items {
		if item.HasStroke && !item.UseOriginalColor {
			r.ensureStroker(item.FTFace)
			break
		}
	}

	// Pass 1: Stroke outlines.
	for _, item := range layout.Items {
		if !item.HasStroke || item.UseOriginalColor {
			continue
		}
		physW := item.StrokeWidth * r.scaleFactor
		sRadius := int64(physW * 0.5 * 64)
		r.configureStroker(sRadius)

		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			if (g.Index & PangoGlyphUnknownFlag) != 0 {
				continue
			}
			placement := placements[i]
			cg := r.getOrLoadGlyph(item, g, 0, sRadius)
			r.touchPage(cg)
			if cg.Width > 0 && cg.Height > 0 && cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				r.emitPlacedQuad(cg, placement, item.StrokeColor,
					float32(item.Ascent), float32(item.Descent),
					item.UseOriginalColor, float32(g.XAdvance))
			}
		}
	}

	// Pass 2: Fill glyphs.
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
			cg := r.getOrLoadGlyph(item, g, bin, 0)
			r.touchPage(cg)
			if cg.Width > 0 && cg.Height > 0 && cg.Page >= 0 && cg.Page < len(r.atlas.Pages) {
				r.emitPlacedQuad(cg, placement, c,
					float32(item.Ascent), float32(item.Descent),
					item.UseOriginalColor, float32(g.XAdvance))
			}
		}
	}
}

// Atlas returns the glyph atlas for external access (e.g. debug).
func (r *Renderer) Atlas() *GlyphAtlas { return r.atlas }

// --- internal helpers ---

// getOrLoadGlyph retrieves a glyph from cache or loads via FreeType.
func (r *Renderer) getOrLoadGlyph(item Item, g Glyph, bin int,
	strokeRadius int64) CachedGlyph {

	if item.FTFace == nil {
		return CachedGlyph{}
	}
	fontID := uint64(uintptr(item.FTFace))
	targetH := int(float32(item.Ascent) * r.scaleFactor)

	key := fnvOffsetBasis
	key = fnvHashU64(key, fontID)
	key = fnvHashU64(key, (uint64(g.Index)<<2)|uint64(bin))
	key = fnvHashU64(key, uint64(strokeRadius)&0xFFFF)
	key = fnvHashU64(key, uint64(targetH))

	if cached, ok := r.cache[key]; ok {
		r.cacheAges[key] = r.atlas.FrameCounter
		if cached.Page < 0 {
			return CachedGlyph{} // Negative cache hit.
		}
		return cached
	}

	face := (C.FT_Face)(item.FTFace)
	cfg := LoadGlyphConfig{
		Face:         face,
		Index:        g.Index,
		TargetHeight: targetH,
		SubpixelBin:  bin,
	}

	var result LoadGlyphResult
	var err error
	if strokeRadius > 0 {
		result, err = LoadStrokedGlyph(r.atlas, r.ftStroker, cfg, strokeRadius, r.scaleFactor)
	} else {
		result, err = LoadGlyph(r.atlas, cfg, r.scaleFactor)
	}
	if err != nil {
		// Negative cache: prevent repeated C library calls for
		// the same failing glyph.
		failed := CachedGlyph{Page: -1}
		r.cache[key] = failed
		r.cacheAges[key] = r.atlas.FrameCounter
		return CachedGlyph{}
	}

	// Invalidate cache entries on reset page.
	if result.ResetOccurred {
		for _, k := range r.pageKeys[result.ResetPage] {
			delete(r.cache, k)
			delete(r.cacheAges, k)
		}
		delete(r.pageKeys, result.ResetPage)
	}

	// Evict oldest if at capacity.
	if len(r.cache) >= r.maxCacheEntries {
		r.evictOldestGlyph()
	}

	r.cache[key] = result.Cached
	r.cacheAges[key] = r.atlas.FrameCounter
	r.pageKeys[result.Cached.Page] = append(r.pageKeys[result.Cached.Page], key)
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

func (r *Renderer) ensureStroker(facePtr unsafe.Pointer) {
	if r.hasStroker {
		return
	}
	face := (C.FT_Face)(facePtr)
	lib := face.glyph.library
	var s C.FT_Stroker
	if C.FT_Stroker_New(lib, &s) == 0 {
		r.ftStroker = FTStroker{ptr: s}
		r.hasStroker = true
	}
}

func (r *Renderer) configureStroker(radius int64) {
	if !r.hasStroker {
		return
	}
	C.FT_Stroker_Set(r.ftStroker.ptr, C.FT_Fixed(radius),
		C.FT_STROKER_LINECAP_ROUND, C.FT_STROKER_LINEJOIN_ROUND, 0)
}

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
