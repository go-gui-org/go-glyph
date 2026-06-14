package glyph

import "testing"

// callRecordingBackend extends mockBackend and records ALL draw call
// parameters including the AffineTransform on DrawTexturedQuadTransformed.
type texturedQuadCall struct {
	TextureID TextureID
	Src       Rect
	Dst       Rect
	Color     Color
}

type transformedQuadCall struct {
	TextureID TextureID
	Src       Rect
	Dst       Rect
	Color     Color
	Transform AffineTransform
}

type filledRectRecord struct {
	Dst   Rect
	Color Color
}

type callRecordingBackend struct {
	mockBackend
	texturedQuads    []texturedQuadCall
	transformedQuads []transformedQuadCall
	filledRects      []filledRectRecord
}

func (b *callRecordingBackend) DrawTexturedQuad(id TextureID, src, dst Rect, c Color) {
	b.texturedQuads = append(b.texturedQuads, texturedQuadCall{id, src, dst, c})
}

func (b *callRecordingBackend) DrawFilledRect(dst Rect, c Color) {
	b.filledRects = append(b.filledRects, filledRectRecord{dst, c})
}

func (b *callRecordingBackend) DrawTexturedQuadTransformed(id TextureID, src, dst Rect, c Color, t AffineTransform) {
	b.transformedQuads = append(b.transformedQuads, transformedQuadCall{id, src, dst, c, t})
}

// --- Backend interface contract tests ---

func TestDrawBackend_NewTexture(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	id := b.NewTexture(100, 50)
	if id == 0 {
		t.Error("NewTexture should return non-zero TextureID")
	}
	if _, ok := b.textures[id]; !ok {
		t.Error("NewTexture should store texture data")
	}
}

func TestDrawBackend_UpdateTexture(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	id := b.NewTexture(4, 1)
	data := []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 0, 0, 0, 0}
	b.UpdateTexture(id, data)
	stored := b.textures[id]
	if len(stored) != len(data) {
		t.Fatalf("texture data len = %d, want %d", len(stored), len(data))
	}
	for i := range data {
		if stored[i] != data[i] {
			t.Fatalf("texture data[%d] = %d, want %d", i, stored[i], data[i])
		}
	}
}

func TestDrawBackend_DeleteTexture(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	id := b.NewTexture(10, 10)
	b.DeleteTexture(id)
	if _, ok := b.textures[id]; ok {
		t.Error("DeleteTexture should remove texture from store")
	}
}

func TestDrawBackend_DPIScale(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	if got := b.DPIScale(); got != 1.0 {
		t.Errorf("DPIScale = %f, want 1.0", got)
	}
}

func TestDrawBackend_DeleteTextureInvalidNoPanic(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	// Delete non-existent texture — must not panic.
	b.DeleteTexture(99999)
}

func TestCallRecording_TexturedQuad(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	id := b.NewTexture(10, 10)
	want := texturedQuadCall{
		TextureID: id,
		Src:       Rect{X: 0, Y: 0, Width: 5, Height: 5},
		Dst:       Rect{X: 10, Y: 20, Width: 50, Height: 50},
		Color:     Color{255, 0, 0, 255},
	}
	b.DrawTexturedQuad(want.TextureID, want.Src, want.Dst, want.Color)

	if len(b.texturedQuads) != 1 {
		t.Fatalf("texturedQuads = %d, want 1", len(b.texturedQuads))
	}
	got := b.texturedQuads[0]
	if got.TextureID != want.TextureID || got.Src != want.Src ||
		got.Dst != want.Dst || got.Color != want.Color {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCallRecording_TransformedQuad(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	id := b.NewTexture(10, 10)
	transform := AffineRotation(0.5)
	b.DrawTexturedQuadTransformed(id, Rect{0, 0, 10, 10}, Rect{5, 5, 20, 20},
		Color{0, 255, 0, 255}, transform)

	if len(b.transformedQuads) != 1 {
		t.Fatalf("transformedQuads = %d, want 1", len(b.transformedQuads))
	}
	got := b.transformedQuads[0]
	if got.Transform != transform {
		t.Errorf("Transform not recorded: got %+v, want %+v", got.Transform, transform)
	}
}

func TestCallRecording_FilledRect(t *testing.T) {
	b := &callRecordingBackend{mockBackend: *newMockBackend()}
	want := filledRectRecord{
		Dst:   Rect{X: 0, Y: 0, Width: 100, Height: 20},
		Color: Color{0, 0, 255, 128},
	}
	b.DrawFilledRect(want.Dst, want.Color)

	if len(b.filledRects) != 1 {
		t.Fatalf("filledRects = %d, want 1", len(b.filledRects))
	}
	got := b.filledRects[0]
	if got.Dst != want.Dst || got.Color != want.Color {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
