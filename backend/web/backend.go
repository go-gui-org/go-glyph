//go:build js && wasm

// Package web provides a Canvas2D DrawBackend for browser-based
// rendering of go-glyph text.
package web

import (
	"math"
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
	// !(x > 0) also catches NaN, which a <= 0 guard lets through; a NaN
	// base transform would blank every frame.
	if !(dpiScale > 0) {
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

	// Leave the frame's base transform at the device pixel ratio, so
	// everything drawn afterwards — text, rects, built-in box glyphs —
	// works in logical units while landing on the physical grid. On a 1x
	// canvas this is the identity.
	s := float64(b.dpiScale)
	b.ctx2d.Call("setTransform", s, 0, 0, s, 0, 0)
}

// EndFrame is a no-op — Canvas2D is immediate mode.
func (b *Backend) EndFrame() {}

// Canvas2DContext returns the main CanvasRenderingContext2D for
// direct fillText rendering by the WASM Renderer.
func (b *Backend) Canvas2DContext() any { return b.ctx2d }

// DPIScale returns the display scale factor.
func (b *Backend) DPIScale() float32 { return b.dpiScale }

// SetDPIScale updates the display scale factor, which the backend applies
// when it converts glyph's logical coordinates to physical pixels. Call it
// when the window moves to a display with a different scale factor, paired
// with (*glyph.TextSystem).SetDPIScale so shaping and rasterization follow.
// Values of zero or less (NaN included) and non-finite or >10× values are ignored,
// as in New.
func (b *Backend) SetDPIScale(dpiScale float32) {
	if !(dpiScale > 0) || math.IsInf(float64(dpiScale), 0) || dpiScale > 10 {
		return
	}
	b.dpiScale = dpiScale
}

// NewTexture allocates a texture backed by an RGBA byte slice.
// Non-positive sizes return 0 (invalid) instead of panicking
// in make.
func (b *Backend) NewTexture(width, height int) glyph.TextureID {
	if width <= 0 || height <= 0 {
		return 0
	}
	id := b.nextID
	b.nextID++
	b.textures[id] = &textureData{
		data:   make([]byte, width*height*4),
		width:  width,
		height: height,
	}
	return id
}

// UpdateTexture uploads RGBA pixel data. Unknown ids and short
// buffers are ignored: a short copy would leave stale pixels in
// the rows it does not reach.
func (b *Backend) UpdateTexture(id glyph.TextureID, data []byte) {
	td, ok := b.textures[id]
	if !ok {
		return
	}
	if int64(len(data)) < int64(td.width)*int64(td.height)*4 {
		return
	}
	copy(td.data, data)
}

// UpdateTextureRect copies the (x, y, w, h) region of data, a whole
// page with srcStride bytes per row, into a texture. It implements
// glyph.RectTextureUpdater for parity with the native backends. Unknown
// ids, invalid regions and short buffers are ignored.
func (b *Backend) UpdateTextureRect(id glyph.TextureID, data []byte,
	srcStride, x, y, w, h int) {

	td, ok := b.textures[id]
	if !ok || !validTextureRect(td.width, td.height, len(data),
		srcStride, x, y, w, h) || len(td.data) < td.width*td.height*4 {
		return
	}
	dstStride := td.width * 4
	for row := range h {
		src := (y+row)*srcStride + x*4
		dst := (y+row)*dstStride + x*4
		copy(td.data[dst:dst+w*4], data[src:src+w*4])
	}
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
	if dst.Width <= 0 || dst.Height <= 0 || !finiteRect(dst) {
		return
	}
	b.ctx2d.Set("globalAlpha", float64(c.A)/255.0)
	b.ctx2d.Set("fillStyle",
		rgbaStyle(int(c.R), int(c.G), int(c.B), 255))
	b.ctx2d.Call("fillRect",
		float64(dst.X), float64(dst.Y),
		float64(dst.Width), float64(dst.Height))
	b.ctx2d.Set("globalAlpha", 1.0)
}

// DrawFilledRectTransformed draws a filled rect with an affine
// transform applied. Implements glyph.TransformedFillBackend,
// so rotated backgrounds and decorations rotate with the glyphs
// instead of staying axis-aligned. t already holds the draw
// origin (folded in by the caller), and the device-pixel-ratio
// scale is folded into the canvas matrix the same way
// setCanvasTransform does it on the renderer side.
func (b *Backend) DrawFilledRectTransformed(dst glyph.Rect,
	c glyph.Color, t glyph.AffineTransform) {

	if dst.Width <= 0 || dst.Height <= 0 {
		return
	}
	if !t.IsFinite() || !finiteRect(dst) {
		return
	}
	s := float64(b.dpiScale)
	b.ctx2d.Call("save")
	b.ctx2d.Call("setTransform",
		float64(t.XX)*s, float64(t.YX)*s,
		float64(t.XY)*s, float64(t.YY)*s,
		float64(t.X0)*s, float64(t.Y0)*s)
	b.ctx2d.Set("globalAlpha", float64(c.A)/255.0)
	b.ctx2d.Set("fillStyle",
		rgbaStyle(int(c.R), int(c.G), int(c.B), 255))
	b.ctx2d.Call("fillRect",
		float64(dst.X), float64(dst.Y),
		float64(dst.Width), float64(dst.Height))
	// restore brings back the base transform (device-pixel
	// ratio), the fill style, and the alpha in one call.
	b.ctx2d.Call("restore")
}

// finiteRect reports whether all rect fields are finite. Canvas
// ignores non-finite fillRect args, which would silently drop
// the fill and desync later draws, so callers drop them first.
func finiteRect(r glyph.Rect) bool {
	return finiteF32(r.X) && finiteF32(r.Y) &&
		finiteF32(r.Width) && finiteF32(r.Height)
}

// finiteF32 reports whether v is finite (no NaN, no infinite).
func finiteF32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
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

// validTextureRect reports whether the region (x, y, w, h) lies inside a
// texW x texH texture and data, read with srcStride bytes per row, holds
// every pixel of it. int64 math keeps huge inputs from wrapping past the
// checks.
func validTextureRect(texW, texH, dataLen, srcStride, x, y, w, h int) bool {
	if w <= 0 || h <= 0 || x < 0 || y < 0 || srcStride <= 0 ||
		srcStride%4 != 0 {
		return false
	}
	if int64(x)+int64(w) > int64(texW) || int64(y)+int64(h) > int64(texH) {
		return false
	}
	if (int64(x)+int64(w))*4 > int64(srcStride) {
		return false
	}
	end := (int64(y)+int64(h)-1)*int64(srcStride) + (int64(x)+int64(w))*4
	return end <= int64(dataLen)
}
