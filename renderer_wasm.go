//go:build js && wasm

package glyph

// Renderer rasterizes glyphs via Canvas2D fillText, manages the
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
	// boxSink is re-aimed per built-in box glyph rather than reallocated,
	// so drawing a screen of box art allocates nothing.
	boxSink canvasSink
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
		cache:           make(map[uint64]cacheEntry, 1024),
		pageKeys:        make(map[int][]uint64),
		maxCacheEntries: maxEntries,
		scaleFactor:     safeScale,
		scaleInv:        1.0 / safeScale,
	}, nil
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

func (r *Renderer) DrawLayoutTransformed(layout Layout, x, y float32,
	transform AffineTransform) {
	r.drawLayoutImpl(layout, x, y, transform, nil)
}

func (r *Renderer) DrawLayoutRotated(layout Layout, x, y, angle float32) {
	r.drawLayoutImpl(layout, x, y, AffineRotation(angle), nil)
}

func (r *Renderer) DrawLayoutWithGradient(layout Layout, x, y float32,
	gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, AffineIdentity(), gradient)
}

func (r *Renderer) DrawLayoutTransformedWithGradient(layout Layout,
	x, y float32, transform AffineTransform, gradient *GradientConfig) {
	r.drawLayoutImpl(layout, x, y, transform, gradient)
}

func (r *Renderer) DrawLayoutPlaced(layout Layout,
	placements []GlyphPlacement) {
	if len(placements) != len(layout.Glyphs) || len(layout.Glyphs) == 0 {
		return
	}

	ctx2d, ok := r.getMainContext()
	if !ok {
		return
	}

	for _, item := range layout.Items {
		if item.HasStroke && item.Color.A == 0 {
			continue
		}
		c := item.Color
		cssFont := item.CSSFont
		if cssFont == "" {
			continue
		}

		ctx2d.Set("font", cssFont)
		ctx2d.Set("textBaseline", "alphabetic")
		ctx2d.Set("globalAlpha", float64(c.A)/255.0)
		ctx2d.Set("fillStyle", cssColorString(c))

		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			if (g.Index & PangoGlyphUnknownFlag) != 0 {
				continue
			}
			p := placements[i]
			ch := glyphText(layout.Text, g)

			// A rotated placement gets the font glyph: pixel snapping is
			// meaningless once the cell no longer sits on the pixel grid.
			if p.Angle == 0 && r.drawBoxIfBuiltin(ctx2d, layout.Text, item,
				g, p.X, p.Y, float64(c.A)/255.0) {
				continue
			}

			if p.Angle != 0 {
				ctx2d.Call("save")
				ctx2d.Call("translate",
					float64(p.X), float64(p.Y))
				ctx2d.Call("rotate", float64(p.Angle))
				ctx2d.Call("fillText", ch, 0, 0)
				ctx2d.Call("restore")
			} else {
				ctx2d.Call("fillText", ch,
					float64(p.X), float64(p.Y))
			}
		}
	}
	ctx2d.Set("globalAlpha", 1.0)
}

func (r *Renderer) Atlas() *GlyphAtlas { return r.atlas }
