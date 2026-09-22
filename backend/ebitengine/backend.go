// Package ebitengine provides an Ebitengine DrawBackend for the glyph
// text rendering library.
package ebitengine

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/go-gui-org/go-glyph"
)

// Backend implements glyph.DrawBackend using Ebitengine.
type Backend struct {
	target   *ebiten.Image
	textures map[glyph.TextureID]*ebiten.Image
	widths   map[glyph.TextureID]int
	heights  map[glyph.TextureID]int
	nextID   glyph.TextureID
	dpiScale float32
	// pixel is a 1x1 white image, stretched to draw filled rects.
	// Built once on first use: ebiten.NewImage allocates a GPU
	// texture, too costly to repeat per background or underline.
	pixel *ebiten.Image
}

// New creates an Ebitengine backend. target is the destination
// image (usually the screen from Game.Draw). dpiScale is the
// display scale factor (e.g. ebiten.Monitor().DeviceScaleFactor()).
func New(target *ebiten.Image, dpiScale float32) *Backend {
	// !(x > 0) also catches NaN, which a <= 0 guard lets through.
	if !(dpiScale > 0) {
		dpiScale = 1.0
	}
	return &Backend{
		target:   target,
		textures: make(map[glyph.TextureID]*ebiten.Image),
		widths:   make(map[glyph.TextureID]int),
		heights:  make(map[glyph.TextureID]int),
		dpiScale: dpiScale,
	}
}

// SetTarget updates the draw target (call each frame with screen).
func (b *Backend) SetTarget(target *ebiten.Image) {
	b.target = target
}

// NewTexture allocates a new RGBA texture. Non-positive sizes
// return 0 (invalid): ebiten.NewImage panics on them, and a
// zero-size texture is never drawable.
func (b *Backend) NewTexture(width, height int) glyph.TextureID {
	if width <= 0 || height <= 0 {
		return 0
	}
	b.nextID++
	id := b.nextID
	img := ebiten.NewImage(width, height)
	b.textures[id] = img
	b.widths[id] = width
	b.heights[id] = height
	return id
}

// UpdateTexture uploads RGBA data to an existing texture. A
// short buffer or unknown size is ignored: WritePixels would
// panic on a short slice. int64 arithmetic avoids overflow on
// the product.
func (b *Backend) UpdateTexture(id glyph.TextureID, data []byte) {
	img, ok := b.textures[id]
	if !ok || len(data) == 0 {
		return
	}
	w := b.widths[id]
	h := b.heights[id]
	if w <= 0 || h <= 0 ||
		int64(len(data)) < int64(w)*int64(h)*4 {
		return
	}
	img.WritePixels(data[:w*h*4])
}

// DeleteTexture releases a texture.
func (b *Backend) DeleteTexture(id glyph.TextureID) {
	if img, ok := b.textures[id]; ok {
		img.Deallocate()
		delete(b.textures, id)
		delete(b.widths, id)
		delete(b.heights, id)
	}
}

// DrawTexturedQuad draws a textured rectangle with color tinting.
func (b *Backend) DrawTexturedQuad(id glyph.TextureID, src, dst glyph.Rect, c glyph.Color) {
	img, ok := b.textures[id]
	if !ok || b.target == nil {
		return
	}
	if !finiteRect(src) || !finiteRect(dst) {
		return
	}

	sub := img.SubImage(image.Rect(
		int(src.X), int(src.Y),
		int(src.X+src.Width), int(src.Y+src.Height),
	)).(*ebiten.Image)

	op := &ebiten.DrawImageOptions{}

	// Scale sub-image to dst size.
	if src.Width > 0 && src.Height > 0 {
		sx := float64(dst.Width) / float64(src.Width)
		sy := float64(dst.Height) / float64(src.Height)
		op.GeoM.Scale(sx, sy)
	}
	op.GeoM.Translate(float64(dst.X), float64(dst.Y))

	// Scale logical coordinates to physical pixels.
	if b.dpiScale != 1.0 {
		op.GeoM.Scale(float64(b.dpiScale), float64(b.dpiScale))
	}

	// Color tinting via ColorScale.
	op.ColorScale.Scale(
		float32(c.R)/255.0,
		float32(c.G)/255.0,
		float32(c.B)/255.0,
		float32(c.A)/255.0,
	)

	b.target.DrawImage(sub, op)
}

// whitePixel returns the shared 1x1 white fill image, built on
// first use.
func (b *Backend) whitePixel() *ebiten.Image {
	if b.pixel == nil {
		b.pixel = ebiten.NewImage(1, 1)
		b.pixel.Fill(color.White)
	}
	return b.pixel
}

// DrawFilledRect draws a filled rectangle.
func (b *Backend) DrawFilledRect(dst glyph.Rect, c glyph.Color) {
	if b.target == nil {
		return
	}
	if !finiteRect(dst) {
		return
	}
	w := int(dst.Width)
	h := int(dst.Height)
	if w <= 0 || h <= 0 {
		return
	}
	pixel := b.whitePixel()

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(w), float64(h))
	op.GeoM.Translate(float64(dst.X), float64(dst.Y))

	// Scale logical coordinates to physical pixels.
	if b.dpiScale != 1.0 {
		op.GeoM.Scale(float64(b.dpiScale), float64(b.dpiScale))
	}

	op.ColorScale.Scale(
		float32(c.R)/255.0,
		float32(c.G)/255.0,
		float32(c.B)/255.0,
		float32(c.A)/255.0,
	)

	b.target.DrawImage(pixel, op)
}

// DrawFilledRectTransformed draws a filled rect with an affine
// transform applied. Implements glyph.TransformedFillBackend,
// so rotated backgrounds and decorations rotate with the glyphs
// instead of staying axis-aligned.
func (b *Backend) DrawFilledRectTransformed(dst glyph.Rect,
	c glyph.Color, t glyph.AffineTransform) {

	if b.target == nil {
		return
	}
	if dst.Width <= 0 || dst.Height <= 0 {
		return
	}
	if !t.IsFinite() || !finiteRect(dst) {
		return
	}
	pixel := b.whitePixel()

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(dst.Width), float64(dst.Height))
	op.GeoM.Translate(float64(dst.X), float64(dst.Y))

	// Apply affine transform. The glyph AffineTransform is:
	//   [ XX XY X0 ]
	//   [ YX YY Y0 ]
	// Ebitengine GeoM is row-major [a,b,tx; c,d,ty].
	var m ebiten.GeoM
	m.SetElement(0, 0, float64(t.XX))
	m.SetElement(0, 1, float64(t.XY))
	m.SetElement(1, 0, float64(t.YX))
	m.SetElement(1, 1, float64(t.YY))
	m.SetElement(0, 2, float64(t.X0))
	m.SetElement(1, 2, float64(t.Y0))
	op.GeoM.Concat(m)

	// Scale logical coordinates to physical pixels.
	if b.dpiScale != 1.0 {
		op.GeoM.Scale(float64(b.dpiScale), float64(b.dpiScale))
	}

	op.ColorScale.Scale(
		float32(c.R)/255.0,
		float32(c.G)/255.0,
		float32(c.B)/255.0,
		float32(c.A)/255.0,
	)

	b.target.DrawImage(pixel, op)
}

// finiteRect reports whether all rect fields are finite. A
// rect with NaN or infinite coords would poison the draw with
// bad verts, so callers drop it before drawing.
func finiteRect(r glyph.Rect) bool {
	return finiteF32(r.X) && finiteF32(r.Y) &&
		finiteF32(r.Width) && finiteF32(r.Height)
}

// finiteF32 reports whether v is finite (no NaN, no infinite).
func finiteF32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

// DrawTexturedQuadTransformed draws with an affine transform applied.
func (b *Backend) DrawTexturedQuadTransformed(id glyph.TextureID,
	src, dst glyph.Rect, c glyph.Color, t glyph.AffineTransform) {

	img, ok := b.textures[id]
	if !ok || b.target == nil {
		return
	}
	if !t.IsFinite() || !finiteRect(src) || !finiteRect(dst) {
		return
	}

	sub := img.SubImage(image.Rect(
		int(src.X), int(src.Y),
		int(src.X+src.Width), int(src.Y+src.Height),
	)).(*ebiten.Image)

	op := &ebiten.DrawImageOptions{}

	// Scale to dst size.
	if src.Width > 0 && src.Height > 0 {
		sx := float64(dst.Width) / float64(src.Width)
		sy := float64(dst.Height) / float64(src.Height)
		op.GeoM.Scale(sx, sy)
	}
	op.GeoM.Translate(float64(dst.X), float64(dst.Y))

	// Apply affine transform.
	// The glyph AffineTransform is:
	//   [ XX XY X0 ]
	//   [ YX YY Y0 ]
	// Ebitengine GeoM is row-major [a,b,tx; c,d,ty].
	var m ebiten.GeoM
	m.SetElement(0, 0, float64(t.XX))
	m.SetElement(0, 1, float64(t.XY))
	m.SetElement(1, 0, float64(t.YX))
	m.SetElement(1, 1, float64(t.YY))
	m.SetElement(0, 2, float64(t.X0))
	m.SetElement(1, 2, float64(t.Y0))
	op.GeoM.Concat(m)

	// Scale logical coordinates to physical pixels.
	if b.dpiScale != 1.0 {
		op.GeoM.Scale(float64(b.dpiScale), float64(b.dpiScale))
	}

	op.ColorScale.Scale(
		float32(c.R)/255.0,
		float32(c.G)/255.0,
		float32(c.B)/255.0,
		float32(c.A)/255.0,
	)

	b.target.DrawImage(sub, op)
}

// DPIScale returns the display DPI scale factor.
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
