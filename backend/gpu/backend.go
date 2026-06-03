package gpu

import (
	"unsafe"

	"github.com/go-gui-org/go-glyph"
)

// Backend implements glyph.DrawBackend using a GPU backend via CGo.
// SDL2 provides the window; rendering bypasses SDL2's renderer.
type Backend struct {
	gpu      *gpuCtx
	widths   map[glyph.TextureID]int
	heights  map[glyph.TextureID]int
	batch    batch
	dpiScale float32
}

// New creates a GPU backend. sdlWindow is an unsafe.Pointer to
// the SDL_Window. dpiScale is physical pixels / logical pixels.
func New(sdlWindow unsafe.Pointer, dpiScale float32) (*Backend, error) {
	if dpiScale <= 0 {
		dpiScale = 1.0
	}
	g, err := gpuInitGo(sdlWindow, dpiScale)
	if err != nil {
		return nil, err
	}
	return &Backend{
		gpu:      g,
		widths:   make(map[glyph.TextureID]int),
		heights:  make(map[glyph.TextureID]int),
		dpiScale: dpiScale,
	}, nil
}

// NewTexture allocates a new RGBA texture.
func (b *Backend) NewTexture(width, height int) glyph.TextureID {
	id := glyph.TextureID(b.gpu.newTexture(width, height))
	b.widths[id] = width
	b.heights[id] = height
	return id
}

// UpdateTexture uploads RGBA data to an existing texture.
func (b *Backend) UpdateTexture(id glyph.TextureID, data []byte) {
	w := b.widths[id]
	h := b.heights[id]
	b.gpu.updateTexture(uint64(id), data, w, h)
}

// DeleteTexture releases a texture.
func (b *Backend) DeleteTexture(id glyph.TextureID) {
	b.gpu.deleteTexture(uint64(id))
	delete(b.widths, id)
	delete(b.heights, id)
}

// DrawTexturedQuad draws a textured rectangle with color tinting.
func (b *Backend) DrawTexturedQuad(
	id glyph.TextureID, src, dst glyph.Rect, c glyph.Color,
) {
	texW := float32(b.widths[id])
	texH := float32(b.heights[id])
	if texW == 0 || texH == 0 {
		return
	}
	u0 := src.X / texW
	v0 := src.Y / texH
	u1 := (src.X + src.Width) / texW
	v1 := (src.Y + src.Height) / texH

	x0, y0 := dst.X, dst.Y
	x1, y1 := dst.X+dst.Width, dst.Y+dst.Height

	b.batch.append6(uint64(id),
		Vertex{x0, y0, c.R, c.G, c.B, c.A, u0, v0},
		Vertex{x1, y0, c.R, c.G, c.B, c.A, u1, v0},
		Vertex{x1, y1, c.R, c.G, c.B, c.A, u1, v1},
		Vertex{x0, y1, c.R, c.G, c.B, c.A, u0, v1},
	)
}

// DrawFilledRect draws a filled rectangle (textureID=0 → white tex).
func (b *Backend) DrawFilledRect(dst glyph.Rect, c glyph.Color) {
	if dst.Width <= 0 || dst.Height <= 0 {
		return
	}
	x0, y0 := dst.X, dst.Y
	x1, y1 := dst.X+dst.Width, dst.Y+dst.Height

	b.batch.append6(0,
		Vertex{x0, y0, c.R, c.G, c.B, c.A, 0, 0},
		Vertex{x1, y0, c.R, c.G, c.B, c.A, 1, 0},
		Vertex{x1, y1, c.R, c.G, c.B, c.A, 1, 1},
		Vertex{x0, y1, c.R, c.G, c.B, c.A, 0, 1},
	)
}

// DrawTexturedQuadTransformed draws a textured quad with an
// affine transform applied CPU-side.
func (b *Backend) DrawTexturedQuadTransformed(
	id glyph.TextureID, src, dst glyph.Rect,
	c glyph.Color, t glyph.AffineTransform,
) {
	texW := float32(b.widths[id])
	texH := float32(b.heights[id])
	if texW == 0 || texH == 0 {
		return
	}
	u0 := src.X / texW
	v0 := src.Y / texH
	u1 := (src.X + src.Width) / texW
	v1 := (src.Y + src.Height) / texH

	// Quad corners, transformed.
	x0, y0 := t.Apply(dst.X, dst.Y)
	x1, y1 := t.Apply(dst.X+dst.Width, dst.Y)
	x2, y2 := t.Apply(dst.X+dst.Width, dst.Y+dst.Height)
	x3, y3 := t.Apply(dst.X, dst.Y+dst.Height)

	b.batch.append6(uint64(id),
		Vertex{x0, y0, c.R, c.G, c.B, c.A, u0, v0},
		Vertex{x1, y1, c.R, c.G, c.B, c.A, u1, v0},
		Vertex{x2, y2, c.R, c.G, c.B, c.A, u1, v1},
		Vertex{x3, y3, c.R, c.G, c.B, c.A, u0, v1},
	)
}

// DPIScale returns the display DPI scale factor.
func (b *Backend) DPIScale() float32 { return b.dpiScale }

// BeginFrame resets vertex/command buffers for a new frame.
func (b *Backend) BeginFrame() {
	b.batch.reset()
}

// EndFrame flushes batched draw commands to the GPU and presents.
func (b *Backend) EndFrame(clearR, clearG, clearB, clearA float32,
	logicalW, logicalH int) error {
	return b.gpu.render(b.batch.verts, b.batch.cmds,
		clearR, clearG, clearB, clearA,
		logicalW, logicalH)
}

// DrawableSize returns the physical drawable size in pixels.
func (b *Backend) DrawableSize() (int, int) {
	return b.gpu.drawableSize()
}

// Destroy releases all GPU resources.
func (b *Backend) Destroy() {
	b.gpu.destroy()
	b.widths = nil
	b.heights = nil
}
