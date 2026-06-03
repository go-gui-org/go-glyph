//go:build (linux || darwin || windows) && !ios && !android

// Package sdl2 provides an SDL2 DrawBackend for the glyph
// text rendering library.
package sdl2

import (
	"log"
	"unsafe"

	"github.com/go-gui-org/go-glyph"
	"github.com/veandco/go-sdl2/sdl"
)

// Backend implements glyph.DrawBackend using SDL2.
type Backend struct {
	renderer *sdl.Renderer
	textures map[glyph.TextureID]*sdl.Texture
	widths   map[glyph.TextureID]int
	heights  map[glyph.TextureID]int
	nextID   glyph.TextureID
	dpiScale float32
}

// New creates an SDL2 backend. renderer is the SDL renderer
// for the target window. dpiScale is the display scale factor
// (physical pixels / logical pixels).
func New(renderer *sdl.Renderer, dpiScale float32) *Backend {
	if dpiScale <= 0 {
		dpiScale = 1.0
	}
	_ = renderer.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	return &Backend{
		renderer: renderer,
		textures: make(map[glyph.TextureID]*sdl.Texture),
		widths:   make(map[glyph.TextureID]int),
		heights:  make(map[glyph.TextureID]int),
		dpiScale: dpiScale,
	}
}

// NewTexture allocates a new RGBA texture.
func (b *Backend) NewTexture(width, height int) glyph.TextureID {
	b.nextID++
	id := b.nextID
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "1")
	tex, err := b.renderer.CreateTexture(
		uint32(sdl.PIXELFORMAT_RGBA32),
		sdl.TEXTUREACCESS_STREAMING,
		int32(width), int32(height),
	)
	if err != nil {
		log.Printf("sdl2: CreateTexture %dx%d: %v", width, height, err)
		return 0
	}
	_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)
	b.textures[id] = tex
	b.widths[id] = width
	b.heights[id] = height
	return id
}

// UpdateTexture uploads RGBA data to an existing texture.
func (b *Backend) UpdateTexture(id glyph.TextureID, data []byte) {
	tex, ok := b.textures[id]
	if !ok || len(data) == 0 {
		return
	}
	w := b.widths[id]
	_ = tex.Update(nil, unsafe.Pointer(&data[0]), w*4)
}

// DeleteTexture releases a texture.
func (b *Backend) DeleteTexture(id glyph.TextureID) {
	if tex, ok := b.textures[id]; ok {
		_ = tex.Destroy()
		delete(b.textures, id)
		delete(b.widths, id)
		delete(b.heights, id)
	}
}

// DrawTexturedQuad draws a textured rectangle with color tinting.
func (b *Backend) DrawTexturedQuad(
	id glyph.TextureID, src, dst glyph.Rect, c glyph.Color,
) {
	tex, ok := b.textures[id]
	if !ok || b.renderer == nil {
		return
	}
	_ = tex.SetColorMod(c.R, c.G, c.B)
	_ = tex.SetAlphaMod(c.A)
	srcRect := &sdl.Rect{
		X: int32(src.X), Y: int32(src.Y),
		W: int32(src.Width), H: int32(src.Height),
	}
	s := b.dpiScale
	dstRect := &sdl.FRect{
		X: dst.X * s, Y: dst.Y * s,
		W: dst.Width * s, H: dst.Height * s,
	}
	_ = b.renderer.CopyF(tex, srcRect, dstRect)
}

// DrawFilledRect draws a filled rectangle.
func (b *Backend) DrawFilledRect(dst glyph.Rect, c glyph.Color) {
	if b.renderer == nil {
		return
	}
	w := dst.Width
	h := dst.Height
	if w <= 0 || h <= 0 {
		return
	}
	_ = b.renderer.SetDrawColor(c.R, c.G, c.B, c.A)
	s := b.dpiScale
	rect := sdl.FRect{
		X: dst.X * s, Y: dst.Y * s,
		W: w * s, H: h * s,
	}
	_ = b.renderer.FillRectF(&rect)
}

// DrawTexturedQuadTransformed draws with an affine transform.
func (b *Backend) DrawTexturedQuadTransformed(
	id glyph.TextureID, src, dst glyph.Rect,
	c glyph.Color, t glyph.AffineTransform,
) {
	tex, ok := b.textures[id]
	if !ok || b.renderer == nil {
		return
	}

	// Compute untransformed quad corners.
	x0, y0 := dst.X, dst.Y
	x1, y1 := dst.X+dst.Width, dst.Y
	x2, y2 := dst.X+dst.Width, dst.Y+dst.Height
	x3, y3 := dst.X, dst.Y+dst.Height

	// Apply affine transform then scale to physical pixels.
	s := b.dpiScale
	tx0, ty0 := t.Apply(x0, y0)
	tx1, ty1 := t.Apply(x1, y1)
	tx2, ty2 := t.Apply(x2, y2)
	tx3, ty3 := t.Apply(x3, y3)
	tx0 *= s
	ty0 *= s
	tx1 *= s
	ty1 *= s
	tx2 *= s
	ty2 *= s
	tx3 *= s
	ty3 *= s

	// Normalized texture coordinates.
	texW := float32(b.widths[id])
	texH := float32(b.heights[id])
	u0 := src.X / texW
	v0 := src.Y / texH
	u1 := (src.X + src.Width) / texW
	v1 := (src.Y + src.Height) / texH

	col := sdl.Color{R: c.R, G: c.G, B: c.B, A: c.A}

	verts := []sdl.Vertex{
		{Position: sdl.FPoint{X: tx0, Y: ty0}, Color: col, TexCoord: sdl.FPoint{X: u0, Y: v0}},
		{Position: sdl.FPoint{X: tx1, Y: ty1}, Color: col, TexCoord: sdl.FPoint{X: u1, Y: v0}},
		{Position: sdl.FPoint{X: tx2, Y: ty2}, Color: col, TexCoord: sdl.FPoint{X: u1, Y: v1}},
		{Position: sdl.FPoint{X: tx3, Y: ty3}, Color: col, TexCoord: sdl.FPoint{X: u0, Y: v1}},
	}
	indices := []int32{0, 1, 2, 0, 2, 3}

	_ = b.renderer.RenderGeometry(tex, verts, indices)
}

// DPIScale returns the display DPI scale factor.
func (b *Backend) DPIScale() float32 { return b.dpiScale }

// Destroy releases all textures held by the backend.
// Does not destroy the SDL renderer (caller-owned).
func (b *Backend) Destroy() {
	for _, tex := range b.textures {
		_ = tex.Destroy()
	}
	b.textures = nil
	b.widths = nil
	b.heights = nil
}
