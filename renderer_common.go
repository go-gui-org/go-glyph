//go:build android || linux || darwin || windows

package glyph

import "math"

// Renderer rasterizes glyphs via the platform rasterizer, manages the
// glyph cache and atlas, and emits draw calls through DrawBackend.
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

func (r *Renderer) Atlas() *GlyphAtlas { return r.atlas }

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
