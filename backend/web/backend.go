//go:build js && wasm

// Package web provides a Canvas2D DrawBackend for browser-based
// rendering of go-glyph text.
package web

import (
	"syscall/js"

	"github.com/go-gui-org/go-glyph"
)

// Backend implements glyph.DrawBackend using Canvas2D.
type Backend struct {
	canvas   js.Value
	ctx2d    js.Value
	textures map[glyph.TextureID]*textureData
	nextID   glyph.TextureID
	dpiScale float32
	width    int
	height   int
}

type textureData struct {
	data   []byte
	width  int
	height int
}

// New creates a Canvas2D backend from an HTML canvas element.
func New(canvas js.Value, dpiScale float32) *Backend {
	if dpiScale <= 0 {
		dpiScale = 1.0
	}
	ctx2d := canvas.Call("getContext", "2d")
	w := canvas.Get("width").Int()
	h := canvas.Get("height").Int()

	return &Backend{
		canvas:   canvas,
		ctx2d:    ctx2d,
		textures: make(map[glyph.TextureID]*textureData),
		nextID:   1,
		dpiScale: dpiScale,
		width:    w,
		height:   h,
	}
}

// BeginFrame clears the canvas with the given color.
func (b *Backend) BeginFrame(clearR, clearG, clearB, clearA float32) {
	b.width = b.canvas.Get("width").Int()
	b.height = b.canvas.Get("height").Int()

	b.ctx2d.Set("globalCompositeOperation", "source-over")
	b.ctx2d.Set("globalAlpha", 1.0)
	// Clear in pixel space (full physical canvas).
	b.ctx2d.Call("setTransform", 1, 0, 0, 1, 0, 0)

	r := int(clearR * 255)
	g := int(clearG * 255)
	bl := int(clearB * 255)
	b.ctx2d.Set("fillStyle", rgbaStyle(r, g, bl, 255))
	b.ctx2d.Call("fillRect", 0, 0, b.width, b.height)
}

// EndFrame is a no-op — Canvas2D is immediate mode.
func (b *Backend) EndFrame() {}

// Canvas2DContext returns the main CanvasRenderingContext2D for
// direct fillText rendering by the WASM Renderer.
func (b *Backend) Canvas2DContext() any { return b.ctx2d }

// DPIScale returns the display scale factor.
func (b *Backend) DPIScale() float32 { return b.dpiScale }

// NewTexture allocates a texture backed by an RGBA byte slice.
func (b *Backend) NewTexture(width, height int) glyph.TextureID {
	id := b.nextID
	b.nextID++
	b.textures[id] = &textureData{
		data:   make([]byte, width*height*4),
		width:  width,
		height: height,
	}
	return id
}

// UpdateTexture uploads RGBA pixel data.
func (b *Backend) UpdateTexture(id glyph.TextureID, data []byte) {
	td, ok := b.textures[id]
	if !ok {
		return
	}
	copy(td.data, data)
}

// DeleteTexture releases a texture.
func (b *Backend) DeleteTexture(id glyph.TextureID) {
	delete(b.textures, id)
}

// DrawTexturedQuad is a no-op; WASM renders via fillText.
func (b *Backend) DrawTexturedQuad(_ glyph.TextureID,
	_, _ glyph.Rect, _ glyph.Color) {
}

// DrawTexturedQuadTransformed is a no-op; WASM renders via fillText.
func (b *Backend) DrawTexturedQuadTransformed(_ glyph.TextureID,
	_, _ glyph.Rect, _ glyph.Color, _ glyph.AffineTransform) {
}

// DrawFilledRect draws an untextured filled rectangle.
func (b *Backend) DrawFilledRect(dst glyph.Rect, c glyph.Color) {
	b.ctx2d.Set("globalAlpha", float64(c.A)/255.0)
	b.ctx2d.Set("fillStyle",
		rgbaStyle(int(c.R), int(c.G), int(c.B), 255))
	b.ctx2d.Call("fillRect",
		float64(dst.X), float64(dst.Y),
		float64(dst.Width), float64(dst.Height))
	b.ctx2d.Set("globalAlpha", 1.0)
}

func rgbaStyle(r, g, b, a int) string {
	if a >= 255 {
		return "rgb(" + itoa(r) + "," + itoa(g) + "," + itoa(b) + ")"
	}
	return "rgba(" + itoa(r) + "," + itoa(g) + "," + itoa(b) +
		"," + ftoa(float64(a)/255.0) + ")"
}

func itoa(i int) string {
	if i < 0 {
		return "-" + uitoa(uint(-i))
	}
	return uitoa(uint(i))
}

func uitoa(u uint) string {
	if u == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	if f <= 0 {
		return "0"
	}
	if f >= 1 {
		return "1"
	}
	i := int(f * 100)
	return "0." + uitoa(uint(i/10)) + uitoa(uint(i%10))
}
