// Package gpu provides a raw GPU DrawBackend for the glyph text
// rendering library: Metal on macOS, native GLX/OpenGL on Linux, and
// OpenGL on Windows. The caller owns the window; gpu.New takes a native
// handle and renders into it.
package gpu

// Vertex matches the MSL vertex layout (20 bytes).
type Vertex struct {
	PosX, PosY float32 // 8 bytes
	R, G, B, A uint8   // 4 bytes (packed RGBA)
	TexU, TexV float32 // 8 bytes
}

// drawCmd represents a batched draw call.
type drawCmd struct {
	textureID uint64
	firstVert int32
	vertCount int32
}

// batch accumulates vertices and draw commands per frame.
type batch struct {
	verts []Vertex
	cmds  []drawCmd
}

func (b *batch) reset() {
	b.verts = b.verts[:0]
	b.cmds = b.cmds[:0]
}

// append6 adds 6 vertices (two triangles for a quad) and a
// draw command for the given texture.
func (b *batch) append6(texID uint64, v0, v1, v2, v3 Vertex) {
	first := int32(len(b.verts))
	b.verts = append(b.verts,
		v0, v1, v2, // tri 1
		v0, v2, v3, // tri 2
	)
	b.cmds = append(b.cmds, drawCmd{
		textureID: texID,
		firstVert: first,
		vertCount: 6,
	})
}
