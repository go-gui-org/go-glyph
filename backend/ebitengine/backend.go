// Package ebitengine provides an Ebitengine DrawBackend for the glyph
// text rendering library.
package ebitengine

import (
	"image"
	"image/color"

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
}

// New creates an Ebitengine backend. target is the destination
// image (usually the screen from Game.Draw). dpiScale is the
// display scale factor (e.g. ebiten.Monitor().DeviceScaleFactor()).
func New(target *ebiten.Image, dpiScale float32) *Backend {
	if dpiScale <= 0 {
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

// NewTexture allocates a new RGBA texture.
func (b *Backend) NewTexture(width, height int) glyph.TextureID {
	b.nextID++
	id := b.nextID
	img := ebiten.NewImage(width, height)
	b.textures[id] = img
	b.widths[id] = width
	b.heights[id] = height
	return id
}

// UpdateTexture uploads RGBA data to an existing texture.
func (b *Backend) UpdateTexture(id glyph.TextureID, data []byte) {
	img, ok := b.textures[id]
	if !ok || len(data) == 0 {
		return
	}
	w := b.widths[id]
	h := b.heights[id]
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

// DrawFilledRect draws a filled rectangle.
func (b *Backend) DrawFilledRect(dst glyph.Rect, c glyph.Color) {
	if b.target == nil {
		return
	}
	w := int(dst.Width)
	h := int(dst.Height)
	if w <= 0 || h <= 0 {
		return
	}
	// Use a 1x1 white pixel stretched to fill.
	pixel := ebiten.NewImage(1, 1)
	pixel.Fill(color.White)

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

// DrawTexturedQuadTransformed draws with an affine transform applied.
func (b *Backend) DrawTexturedQuadTransformed(id glyph.TextureID,
	src, dst glyph.Rect, c glyph.Color, t glyph.AffineTransform) {

	img, ok := b.textures[id]
	if !ok || b.target == nil {
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
