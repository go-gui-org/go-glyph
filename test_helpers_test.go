package glyph

import "testing"

// recordingBackend extends mockBackend with draw call recording.
type recordingBackend struct {
	mockBackend
	drawCalls   []drawCall
	filledRects []filledRectCall
}

type drawCall struct {
	TextureID TextureID
	Src       Rect
	Dst       Rect
	Color     Color
}

type filledRectCall struct {
	Dst   Rect
	Color Color
}

func newRecordingBackend() *recordingBackend {
	return &recordingBackend{
		mockBackend: *newMockBackend(),
	}
}

func (r *recordingBackend) DrawTexturedQuad(id TextureID, src, dst Rect, c Color) {
	r.drawCalls = append(r.drawCalls, drawCall{id, src, dst, c})
}

func (r *recordingBackend) DrawFilledRect(dst Rect, c Color) {
	r.filledRects = append(r.filledRects, filledRectCall{dst, c})
}

func (r *recordingBackend) DrawTexturedQuadTransformed(id TextureID,
	src, dst Rect, c Color, t AffineTransform) {
	r.drawCalls = append(r.drawCalls, drawCall{id, src, dst, c})
}

func TestNewTextSystem(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	if ts.ctx == nil {
		t.Error("nil context")
	}
	if ts.renderer == nil {
		t.Error("nil renderer")
	}
}

func TestTextSystemDrawText(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{
			FontName: "Sans 16",
			Color:    Color{0, 0, 0, 255},
		},
	}
	err = ts.DrawText(100, 200, "Hello", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls after DrawText + Commit")
	}
}

func TestTextSystemEmptyText(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 14"}}
	err = ts.DrawText(0, 0, "", cfg)
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func TestTextSystemAddFontFile(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	err = ts.AddFontFile("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestTextSystemAddFontBytes(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	if err := ts.AddFontBytes(nil); err == nil {
		t.Error("expected error for empty font data")
	}
	if len(ts.tempFontFiles) != 0 {
		t.Errorf("no temp files expected after failure, got %d",
			len(ts.tempFontFiles))
	}
}

func TestTextSystemPurge(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{
			FontName: "Sans 16",
			Color:    Color{0, 0, 0, 255},
		},
	}
	err = ts.DrawText(100, 200, "Hello", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts.Commit()

	if len(ts.cache) == 0 {
		t.Error("layout cache should have entry after DrawText")
	}
	if len(ts.renderer.cache) == 0 {
		t.Error("glyph cache should have entries after DrawText")
	}

	ts.Purge()

	if len(ts.cache) != 0 {
		t.Error("layout cache should be empty after Purge")
	}
	if len(ts.renderer.cache) != 0 {
		t.Error("glyph cache should be empty after Purge")
	}
	if len(ts.renderer.pageKeys) != 0 {
		t.Error("pageKeys should be empty after Purge")
	}
	// Verify atlas pages are reset (shelves cleared, staging zeroed).
	for i, page := range ts.renderer.atlas.Pages {
		if len(page.Shelves) != 0 {
			t.Errorf("page %d shelves not empty after Purge", i)
		}
		if page.UsedPixels != 0 {
			t.Errorf("page %d UsedPixels = %d, want 0", i, page.UsedPixels)
		}
	}
}

func testLayout() Layout {
	charRects := []CharRect{
		{Rect: Rect{X: 0, Y: 0, Width: 10, Height: 20}, Index: 0},
		{Rect: Rect{X: 10, Y: 0, Width: 10, Height: 20}, Index: 1},
		{Rect: Rect{X: 20, Y: 0, Width: 10, Height: 20}, Index: 2},
		{Rect: Rect{X: 30, Y: 0, Width: 10, Height: 20}, Index: 3},
		{Rect: Rect{X: 40, Y: 0, Width: 10, Height: 20}, Index: 4},
		{Rect: Rect{X: 0, Y: 20, Width: 10, Height: 20}, Index: 6},
		{Rect: Rect{X: 10, Y: 20, Width: 10, Height: 20}, Index: 7},
		{Rect: Rect{X: 20, Y: 20, Width: 10, Height: 20}, Index: 8},
		{Rect: Rect{X: 30, Y: 20, Width: 10, Height: 20}, Index: 9},
		{Rect: Rect{X: 40, Y: 20, Width: 10, Height: 20}, Index: 10},
	}
	charRectByIndex := map[int]int{
		0: 0, 1: 1, 2: 2, 3: 3, 4: 4,
		6: 5, 7: 6, 8: 7, 9: 8, 10: 9,
	}
	lines := []Line{
		{StartIndex: 0, Length: 5, Rect: Rect{X: 0, Y: 0, Width: 50, Height: 20}},
		{StartIndex: 6, Length: 5, Rect: Rect{X: 0, Y: 20, Width: 50, Height: 20}},
	}
	logAttrs := []LogAttr{
		{IsCursorPosition: true, IsWordStart: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true, IsWordEnd: true},
		{IsCursorPosition: true, IsWordStart: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true},
		{IsCursorPosition: true, IsWordEnd: true},
	}
	logAttrByIndex := map[int]int{
		0: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5,
		6: 6, 7: 7, 8: 8, 9: 9, 10: 10, 11: 11,
	}
	return Layout{
		Text:            "Hello\nWorld",
		CharRects:       charRects,
		CharRectByIndex: charRectByIndex,
		Lines:           lines,
		LogAttrs:        logAttrs,
		LogAttrByIndex:  logAttrByIndex,
		Width:           50,
		Height:          40,
	}
}
