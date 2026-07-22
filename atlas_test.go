package glyph

import (
	"testing"
)

// mockBackend records DrawBackend calls for testing.
type mockBackend struct {
	textures map[TextureID][]byte
	nextID   TextureID
}

func newMockBackend() *mockBackend {
	return &mockBackend{textures: make(map[TextureID][]byte)}
}

func (m *mockBackend) NewTexture(w, h int) TextureID {
	m.nextID++
	m.textures[m.nextID] = make([]byte, w*h*4)
	return m.nextID
}

func (m *mockBackend) UpdateTexture(id TextureID, data []byte) {
	if _, ok := m.textures[id]; ok {
		m.textures[id] = append([]byte(nil), data...)
	}
}

func (m *mockBackend) DeleteTexture(id TextureID) {
	delete(m.textures, id)
}

func (m *mockBackend) DrawTexturedQuad(TextureID, Rect, Rect, Color)                             {}
func (m *mockBackend) DrawFilledRect(Rect, Color)                                                {}
func (m *mockBackend) DrawTexturedQuadTransformed(TextureID, Rect, Rect, Color, AffineTransform) {}
func (m *mockBackend) DPIScale() float32                                                         { return 1.0 }

// makeSyntheticBitmap creates a solid-colored RGBA bitmap.
func makeSyntheticBitmap(w, h int, r, g, b, a byte) Bitmap {
	data := make([]byte, w*h*4)
	for i := 0; i < len(data); i += 4 {
		data[i+0] = r
		data[i+1] = g
		data[i+2] = b
		data[i+3] = a
	}
	return Bitmap{Width: w, Height: h, Channels: 4, Data: data}
}

func TestAtlasInsertSingle(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	bmp := makeSyntheticBitmap(10, 12, 255, 255, 255, 200)
	cached, reset, _, err := atlas.InsertBitmap(bmp, 3, 11)
	if err != nil {
		t.Fatal(err)
	}
	if reset {
		t.Error("unexpected page reset on first insert")
	}
	if cached.Width != 10 || cached.Height != 12 {
		t.Errorf("cached size = %dx%d, want 10x12", cached.Width, cached.Height)
	}
	if cached.Left != 3 || cached.Top != 11 {
		t.Errorf("cached bearing = (%d,%d), want (3,11)", cached.Left, cached.Top)
	}
	if cached.X != atlasGlyphPadding || cached.Y != atlasGlyphPadding {
		t.Errorf("cached pos = (%d,%d), want (%d,%d)",
			cached.X, cached.Y, atlasGlyphPadding, atlasGlyphPadding)
	}
}

func TestAtlasInsertMultipleSameShelf(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	bmp1 := makeSyntheticBitmap(10, 12, 255, 0, 0, 255)
	bmp2 := makeSyntheticBitmap(8, 12, 0, 255, 0, 255)

	c1, _, _, err := atlas.InsertBitmap(bmp1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	c2, _, _, err := atlas.InsertBitmap(bmp2, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Both should be on shelf 0, same Y.
	if c1.Y != c2.Y {
		t.Errorf("expected same shelf: Y1=%d Y2=%d", c1.Y, c2.Y)
	}
	wantX := c1.X + c1.Width + atlasGlyphPadding*2
	if c2.X != wantX {
		t.Errorf("expected padded adjacency: c2.X=%d, want %d", c2.X, wantX)
	}
}

func TestAtlasNewShelfForTallGlyph(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	bmp1 := makeSyntheticBitmap(10, 12, 255, 0, 0, 255)
	bmp2 := makeSyntheticBitmap(10, 30, 0, 255, 0, 255)

	c1, _, _, err := atlas.InsertBitmap(bmp1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	c2, _, _, err := atlas.InsertBitmap(bmp2, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// bmp2 is much taller, should get a new shelf.
	if c2.Y <= c1.Y {
		t.Errorf("expected new shelf: c2.Y=%d should be > c1.Y=%d", c2.Y, c1.Y)
	}
}

func TestAtlasPageGrow(t *testing.T) {
	backend := newMockBackend()
	// Small page that will need to grow.
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	// Fill with 32px tall glyphs — first two fit (64px), third triggers grow.
	for i := range 5 {
		bmp := makeSyntheticBitmap(60, 32, 255, 255, 255, 255)
		_, _, _, err := atlas.InsertBitmap(bmp, 0, 0)
		if err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	page := atlas.Pages[atlas.CurrentPage]
	if page.Height <= 64 {
		t.Errorf("expected page growth: height=%d", page.Height)
	}
}

func TestAtlasMultiPage(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	atlas.MaxGlyphDimension = 128 // Limit growth to force new pages.

	// Fill until we get multiple pages.
	for i := range 20 {
		bmp := makeSyntheticBitmap(60, 32, 255, 255, 255, 255)
		_, _, _, err := atlas.InsertBitmap(bmp, 0, 0)
		if err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	if len(atlas.Pages) < 2 {
		t.Errorf("expected multiple pages, got %d", len(atlas.Pages))
	}
}

func TestAtlasPageReset(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	atlas.MaxPages = 2
	atlas.MaxGlyphDimension = 64 // No growth allowed — forces new pages/resets sooner.

	resetSeen := false
	for i := range 50 {
		bmp := makeSyntheticBitmap(60, 30, 255, 255, 255, 255)
		_, reset, _, err := atlas.InsertBitmap(bmp, 0, 0)
		if err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
		if reset {
			resetSeen = true
		}
	}

	if !resetSeen {
		t.Error("expected at least one page reset")
	}
}

func TestAtlasSwapAndUpload(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	bmp := makeSyntheticBitmap(10, 10, 128, 64, 32, 255)
	_, _, _, err = atlas.InsertBitmap(bmp, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if !atlas.Pages[0].Dirty {
		t.Error("expected page dirty after insert")
	}

	atlas.SwapAndUpload()

	if atlas.Pages[0].Dirty {
		t.Error("expected page clean after upload")
	}
}

func TestAtlasEmptyGlyph(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	bmp := Bitmap{Width: 0, Height: 0, Channels: 4, Data: nil}
	cached, reset, _, err := atlas.InsertBitmap(bmp, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if reset {
		t.Error("unexpected reset for empty glyph")
	}
	if cached.Width != 0 || cached.Height != 0 {
		t.Errorf("expected zero-size cached glyph, got %dx%d",
			cached.Width, cached.Height)
	}
}

func TestAtlasOversizedGlyph(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	bmp := makeSyntheticBitmap(5000, 5000, 255, 255, 255, 255)
	_, _, _, err = atlas.InsertBitmap(bmp, 0, 0)
	if err == nil {
		t.Error("expected error for oversized glyph")
	}
}

func TestAtlasCleanup(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	// Manually add garbage.
	atlas.Garbage = append(atlas.Garbage, TextureID(999))
	backend.textures[TextureID(999)] = []byte{1, 2, 3}

	atlas.Cleanup(1)
	if len(atlas.Garbage) != 0 {
		t.Error("expected garbage cleared after cleanup")
	}
	if _, ok := backend.textures[TextureID(999)]; ok {
		t.Error("expected texture 999 deleted")
	}
}

func TestAtlasFree(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}

	texCount := len(backend.textures)
	if texCount == 0 {
		t.Fatal("expected at least one texture after creation")
	}

	atlas.Free()

	if len(backend.textures) != 0 {
		t.Errorf("expected all textures freed, got %d", len(backend.textures))
	}
}

func TestAtlasReset(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	// Insert some glyphs to populate shelves.
	bmp := makeSyntheticBitmap(10, 12, 255, 255, 255, 200)
	for range 10 {
		_, _, _, err := atlas.InsertBitmap(bmp, 3, 11)
		if err != nil {
			t.Fatal(err)
		}
	}

	page := &atlas.Pages[0]
	if len(page.Shelves) == 0 {
		t.Fatal("expected shelves populated before Reset")
	}

	texID := page.TextureID
	atlas.Reset()

	if len(page.Shelves) != 0 {
		t.Error("expected shelves cleared after Reset")
	}
	if page.UsedPixels != 0 {
		t.Errorf("UsedPixels = %d, want 0", page.UsedPixels)
	}
	if page.TextureID != texID {
		t.Error("texture should not be deleted after Reset")
	}
	if _, ok := backend.textures[texID]; !ok {
		t.Error("texture should still exist after Reset")
	}
}

func TestAtlasPageEvictionPressure(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	atlas.MaxPages = 2
	atlas.MaxGlyphDimension = 64

	resetCount := 0
	for i := range 100 {
		bmp := makeSyntheticBitmap(60, 30, 255, 255, 255, 255)
		_, reset, _, err := atlas.InsertBitmap(bmp, 0, 0)
		if err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
		if reset {
			resetCount++
		}
		// Advance frame counter so eviction can distinguish ages.
		atlas.FrameCounter++
	}
	if resetCount < 2 {
		t.Errorf("expected multiple page resets under pressure, got %d",
			resetCount)
	}
}

func TestAtlasPageEvictionSelectsOldest(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	atlas.MaxPages = 3
	atlas.MaxGlyphDimension = 64

	// Fill all 3 pages.
	for range 30 {
		bmp := makeSyntheticBitmap(60, 30, 255, 255, 255, 255)
		_, _, _, err := atlas.InsertBitmap(bmp, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		atlas.FrameCounter++
	}

	if len(atlas.Pages) < 2 {
		t.Fatalf("expected multiple pages, got %d", len(atlas.Pages))
	}

	// Set page ages manually.
	atlas.Pages[0].Age = 100
	atlas.Pages[1].Age = 50 // oldest

	oldestIdx := atlas.findOldestPage()
	if oldestIdx != 1 {
		t.Errorf("findOldestPage() = %d, want 1 (age=50)", oldestIdx)
	}
}

func TestNewGlyphAtlasRoundsUp(t *testing.T) {
	backend := newMockBackend()

	// Non-power-of-two rounds up.
	atlas, err := NewGlyphAtlas(backend, 1000, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atlas.Pages[0].Width != 1024 {
		t.Errorf("width = %d, want 1024 (rounded up from 1000)",
			atlas.Pages[0].Width)
	}
	atlas.Free()

	// Small dimensions clamped to 64.
	atlas, err = NewGlyphAtlas(backend, 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atlas.Pages[0].Width != 64 || atlas.Pages[0].Height != 64 {
		t.Errorf("dimensions = %dx%d, want 64x64",
			atlas.Pages[0].Width, atlas.Pages[0].Height)
	}
	atlas.Free()

	// Zero dimensions clamped to 64.
	atlas, err = NewGlyphAtlas(backend, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atlas.Pages[0].Width != 64 || atlas.Pages[0].Height != 64 {
		t.Errorf("dimensions = %dx%d, want 64x64",
			atlas.Pages[0].Width, atlas.Pages[0].Height)
	}
	atlas.Free()

	// Already power-of-two passes through.
	atlas, err = NewGlyphAtlas(backend, 256, 512)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atlas.Pages[0].Width != 256 || atlas.Pages[0].Height != 512 {
		t.Errorf("dimensions = %dx%d, want 256x512",
			atlas.Pages[0].Width, atlas.Pages[0].Height)
	}
	atlas.Free()
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, 1}, {1, 1}, {2, 2}, {3, 4}, {5, 8},
		{63, 64}, {64, 64}, {65, 128}, {1000, 1024},
		{1024, 1024}, {1025, 2048},
	}
	for _, tt := range tests {
		got := nextPowerOfTwo(tt.in)
		if got != tt.want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d",
				tt.in, got, tt.want)
		}
	}
}

func BenchmarkAtlasInsertBitmap(b *testing.B) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 1024, 1024)
	if err != nil {
		b.Fatal(err)
	}
	defer atlas.Free()
	bmp := makeSyntheticBitmap(16, 20, 255, 255, 255, 200)
	b.ResetTimer()
	for b.Loop() {
		_, _, _, err := atlas.InsertBitmap(bmp, 3, 11)
		if err != nil {
			// Reset when page fills up.
			atlas.resetPage(atlas.CurrentPage)
		}
	}
}

func BenchmarkFindBestShelf(b *testing.B) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 1024, 1024)
	if err != nil {
		b.Fatal(err)
	}
	defer atlas.Free()
	// Populate shelves.
	for range 30 {
		bmp := makeSyntheticBitmap(30, 14+int(b.N%8), 255, 0, 0, 255)
		if _, _, _, err := atlas.InsertBitmap(bmp, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
	page := &atlas.Pages[0]
	b.ResetTimer()
	for b.Loop() {
		page.findBestShelf(18, 16)
	}
}

func TestAtlasCopyBitmapData(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	// Insert a 2x2 red bitmap.
	bmp := makeSyntheticBitmap(2, 2, 255, 0, 0, 255)
	_, _, _, err = atlas.InsertBitmap(bmp, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	page := &atlas.Pages[0]
	// Transparent padding should surround the bitmap.
	idx := 0 // (0,0)
	if page.StagingBack[idx] != 0 || page.StagingBack[idx+3] != 0 {
		t.Errorf("pixel (0,0): R=%d A=%d, want R=0 A=0",
			page.StagingBack[idx], page.StagingBack[idx+3])
	}
	// Check pixel at the padded bitmap origin.
	idx = ((atlasGlyphPadding * 64) + atlasGlyphPadding) * 4
	if page.StagingBack[idx] != 255 || page.StagingBack[idx+1] != 0 {
		t.Errorf("padded pixel: R=%d G=%d, want R=255 G=0",
			page.StagingBack[idx], page.StagingBack[idx+1])
	}
}
