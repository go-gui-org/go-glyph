package glyph

// TextureID is an opaque handle to a GPU texture managed by a DrawBackend.
type TextureID uint64

// DrawBackend abstracts the GPU rendering backend. Implementations
// provide texture management and drawing primitives. The Renderer
// calls only these methods — never a specific backend directly.
type DrawBackend interface {
	// NewTexture allocates a new RGBA texture of the given size.
	NewTexture(width, height int) TextureID

	// UpdateTexture uploads RGBA pixel data to an existing texture.
	// data must be width*height*4 bytes.
	UpdateTexture(id TextureID, data []byte)

	// DeleteTexture releases a texture.
	DeleteTexture(id TextureID)

	// DrawTexturedQuad draws a textured rectangle with color tinting.
	DrawTexturedQuad(id TextureID, src, dst Rect, c Color)

	// DrawFilledRect draws an untextured filled rectangle.
	DrawFilledRect(dst Rect, c Color)

	// DrawTexturedQuadTransformed draws a textured quad with an
	// affine transform applied.
	DrawTexturedQuadTransformed(id TextureID, src, dst Rect, c Color, t AffineTransform)

	// DPIScale returns the display DPI scale factor.
	DPIScale() float32
}

// RectTextureUpdater is an optional DrawBackend extension for uploading
// just the changed sub-rectangle of a texture.
//
// Implement it on any backend whose draw calls sample a texture at the
// moment they are issued. OpenGL is the motivating case: glDrawArrays
// reads the texture as it stands at that point in the command stream, so
// a glyph rasterized during a frame but uploaded at the end of it renders
// blank for one frame — the quads were already issued against an atlas
// page that did not yet contain the glyph. Backends that merely record
// commands and rasterize at present time (Metal, the software renderer)
// do not have the problem, which is why it shows up only on GL.
//
// A backend that implements this lets the Renderer upload newly
// rasterized glyphs mid-frame, immediately after it resolves a layout and
// before it emits that layout's quads, at a cost proportional to the new
// glyphs instead of to the whole page. Backends that do not implement it
// keep the previous behavior: uploads batch to (*Renderer).Commit.
type RectTextureUpdater interface {
	// UpdateTextureRect uploads the pixel rectangle (x, y, w, h) of a
	// texture.
	//
	// data holds the *entire* page, not just the rectangle: source pixels
	// begin at y*srcStride + x*4 and each subsequent row advances by
	// srcStride bytes. This matches glPixelStorei(GL_UNPACK_ROW_LENGTH)
	// plus glTexSubImage2D, and Metal's replaceRegion:bytesPerRow:, so no
	// implementation has to compact the rows first.
	//
	// The rectangle is always non-empty and within the texture bounds.
	UpdateTextureRect(id TextureID, data []byte, srcStride, x, y, w, h int)
}
