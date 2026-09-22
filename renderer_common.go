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
	cache           map[uint64]cacheEntry
	pageKeys        map[int][]uint64
	maxCacheEntries int
	scaleFactor     float32
	scaleInv        float32
	// Scratch slices reused across draw calls so the resolve-then-emit
	// passes (issue #89) don't allocate per call (issue #92). Each is
	// consumed within a single draw call and never escapes; the Renderer
	// is single-goroutine, so reuse is safe. They grow only when a
	// layout's glyph count exceeds the previous high-water mark.
	scratchFills   []CachedGlyph
	scratchStrokes []CachedGlyph
	// scratchRects holds per-clause selection rects in DrawComposition,
	// reused so drawing the IME preedit each frame does not allocate.
	scratchRects []Rect
	// fontPaths is the font map of the Context this Renderer draws for, set
	// by useContextFonts. nil (a Renderer made without a TextSystem) falls
	// back to the process-wide map; see renderFontPaths.
	fontPaths map[string]string
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
	safeScale := sanitizeScale(scaleFactor)
	maxEntries := cfg.MaxGlyphCacheEntries
	if maxEntries == 0 {
		maxEntries = 4096
	} else if maxEntries < 256 {
		maxEntries = 256
	}

	return &Renderer{
		backend:         backend,
		atlas:           atlas,
		cache:           make(map[uint64]cacheEntry, 1024),
		pageKeys:        make(map[int][]uint64),
		maxCacheEntries: maxEntries,
		scaleFactor:     safeScale,
		scaleInv:        1.0 / safeScale,
	}, nil
}

// useContextFonts makes the Renderer resolve font names in ctx's font
// map, the same map ctx uses for layout, so fonts added to ctx with
// AddFontFile render too. The map is shared, not copied: later additions
// are seen without another call.
func (r *Renderer) useContextFonts(ctx *Context) {
	r.fontPaths = ctx.fontPaths
}

func (r *Renderer) Free() {
	r.atlas.Free()
	r.cache = nil
	r.pageKeys = nil
}

func (r *Renderer) Commit() {
	r.atlas.FrameCounter++
	r.atlas.SwapAndUpload()
}

// PurgeGlyphCache clears the glyph cache and resets atlas pages,
// reclaiming GPU textures and Go heap memory. Call after a full
// terminal clear (e.g. CSI 3 J) to drop cached glyphs no longer
// on screen while keeping the TextSystem alive.
func (r *Renderer) PurgeGlyphCache() {
	clear(r.cache)
	clear(r.pageKeys)
	r.atlas.Reset()
}

func (r *Renderer) DrawLayout(layout Layout, x, y float32) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), nil)
}

// DrawLayoutTransformed draws layout at (x, y) through transform.
// The transform runs first (rotation around the layout origin),
// then the result moves to (x, y). To rotate around another
// point, build the transform with AffineRotationAround.
// Backgrounds and decorations rotate with the glyphs. A
// transform that is not finite draws nothing.
func (r *Renderer) DrawLayoutTransformed(layout Layout, x, y float32,
	transform AffineTransform) {
	r.drawLayoutImpl(layout, x, y, transform, nil)
}

// DrawLayoutRotated draws layout at (x, y) rotated by angle in
// radians around the layout origin. Backgrounds and decorations
// rotate with the glyphs.
func (r *Renderer) DrawLayoutRotated(layout Layout,
	x, y, angle float32) {
	r.drawLayoutImpl(layout, x, y, AffineRotation(angle), nil)
}

func (r *Renderer) DrawLayoutWithGradient(layout Layout, x, y float32,
	gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), gradient)
}

// DrawLayoutTransformedWithGradient combines DrawLayoutTransformed
// and DrawLayoutWithGradient. The pivot rule from
// DrawLayoutTransformed applies.
func (r *Renderer) DrawLayoutTransformedWithGradient(layout Layout,
	x, y float32, transform AffineTransform,
	gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, transform, gradient)
}

func (r *Renderer) Atlas() *GlyphAtlas { return r.atlas }

// maxScratchGlyphs bounds how much scratch memory draw calls may retain.
// Layouts produced through the public API are capped by MaxTextLength
// (~10k runes, one glyph per cluster), so this leaves headroom for
// legitimate layouts. Larger layouts can only come from a host-built
// Layout struct; retaining their scratch forever would turn one bad draw
// call into a permanent multi-MB hold, so they fall back to a transient
// allocation (the pre-issue-#92 behavior).
const maxScratchGlyphs = 16384

// scratch returns a slice of n zeroed CachedGlyphs backed by *buf,
// growing *buf only when its capacity is insufficient. Callers must not
// retain the returned slice past the current draw call.
func scratch(buf *[]CachedGlyph, n int) []CachedGlyph {
	if n > maxScratchGlyphs {
		return make([]CachedGlyph, n)
	}
	if cap(*buf) < n {
		*buf = make([]CachedGlyph, n)
	}
	out := (*buf)[:n]
	// Zero every slot: resolve passes leave skipped glyphs (unknown-flag,
	// out-of-range) untouched, and the emit passes discard slots whose
	// Width is 0. A stale entry from a previous call would otherwise emit
	// a phantom quad.
	clear(out)
	return out
}

// evictSampleSize bounds the eviction probe: examine at most this many
// entries and evict the oldest of the sample (approximate LRU, memcached
// style). Go's randomized map iteration order supplies the sampling, so a
// full O(cache-size) age scan per insert is avoided; eviction only needs
// "old", not "oldest".
const evictSampleSize = 8

// glyphAgeRefreshFrames is how stale a cache entry's age may get before a
// hit stores back a refreshed age. Ages are only consumed by sampled
// eviction, which tolerates quantization: an entry drawn every frame looks
// at most this many frames old, still far younger than genuinely idle
// entries. In exchange, steady-state hits are a single map lookup.
const glyphAgeRefreshFrames = 64

func (r *Renderer) evictOldestGlyph() {
	var oldestKey uint64
	oldestAge := uint64(math.MaxUint64)
	sampled := 0
	for k, e := range r.cache {
		if e.age < oldestAge {
			oldestAge = e.age
			oldestKey = k
		}
		sampled++
		if sampled >= evictSampleSize {
			break
		}
	}
	if sampled == 0 {
		return
	}
	r.dropGlyph(oldestKey)
}

// storeGlyph inserts e under key and, when e lives on an atlas page,
// appends key to that page's list and records its slot there.
func (r *Renderer) storeGlyph(key uint64, e cacheEntry) {
	if e.Page >= 0 {
		keys := r.pageKeys[e.Page]
		e.slot = len(keys)
		r.pageKeys[e.Page] = append(keys, key)
	}
	r.cache[key] = e
}

// dropGlyph removes key from the cache and from its page's list in O(1):
// the page's last key moves into the freed slot and its entry's slot is
// updated to match.
func (r *Renderer) dropGlyph(key uint64) {
	e, ok := r.cache[key]
	if !ok {
		return
	}
	delete(r.cache, key)
	if e.Page < 0 {
		return
	}
	keys := r.pageKeys[e.Page]
	i := e.slot
	if i < 0 || i >= len(keys) || keys[i] != key {
		// The slot does not match; the page list was changed outside
		// storeGlyph/dropGlyph. Leave it: a stale key in a page list
		// only costs a no-op delete when that page resets.
		return
	}
	last := len(keys) - 1
	if i != last {
		moved := keys[last]
		keys[i] = moved
		if m, ok := r.cache[moved]; ok {
			m.slot = i
			r.cache[moved] = m
		}
	}
	r.pageKeys[e.Page] = keys[:last]
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
