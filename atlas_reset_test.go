package glyph

import "testing"

// growAtlasPastFirstPage fills a 64x64 atlas until it has grown its first
// page and added more pages, so Reset has something to reclaim.
func growAtlasPastFirstPage(t *testing.T, atlas *GlyphAtlas) {
	t.Helper()
	atlas.MaxGlyphDimension = 128
	for i := range 20 {
		bmp := makeSyntheticBitmap(60, 32, 255, 255, 255, 255)
		if _, _, _, err := atlas.InsertBitmap(bmp, 0, 0); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if len(atlas.Pages) < 2 || atlas.Pages[0].Height <= 64 {
		t.Fatalf("setup: want grown multi-page atlas, got %d pages, "+
			"page 0 height %d", len(atlas.Pages), atlas.Pages[0].Height)
	}
}

// TestAtlasResetKeepsPendingGarbage is the regression test for a texture
// leak: growPage queues the old texture in Garbage, and Reset used to
// truncate Garbage without deleting it, so the texture was never freed.
func TestAtlasResetKeepsPendingGarbage(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	oldTex := atlas.Pages[0].TextureID
	if err := atlas.growPage(0, 128); err != nil {
		t.Fatal(err)
	}
	atlas.Reset()
	atlas.Cleanup(atlas.LastFrame + 1)

	if _, ok := backend.textures[oldTex]; ok {
		t.Errorf("texture %d replaced by growPage leaked after Reset", oldTex)
	}
}

// TestAtlasResetReclaimsGrownPages checks that Reset (and so
// PurgeGlyphCache) gives back memory: extra pages go away, the first page
// returns to its initial height, and the dropped textures are deleted at
// the next Cleanup.
func TestAtlasResetReclaimsGrownPages(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()
	growAtlasPastFirstPage(t, atlas)

	atlas.Reset()

	if len(atlas.Pages) != 1 {
		t.Fatalf("pages after Reset = %d, want 1", len(atlas.Pages))
	}
	page := &atlas.Pages[0]
	if page.Height != 64 {
		t.Errorf("page height after Reset = %d, want 64", page.Height)
	}
	if want := 64 * 64 * 4; len(page.StagingBack) != want {
		t.Errorf("staging after Reset = %d bytes, want %d",
			len(page.StagingBack), want)
	}
	if atlas.CurrentPage != 0 {
		t.Errorf("CurrentPage after Reset = %d, want 0", atlas.CurrentPage)
	}

	atlas.Cleanup(atlas.LastFrame + 1)
	if len(backend.textures) != 1 {
		t.Errorf("live textures after Reset+Cleanup = %d, want 1",
			len(backend.textures))
	}
	if _, ok := backend.textures[page.TextureID]; !ok {
		t.Error("the remaining page's texture was deleted")
	}

	// The reset atlas must still accept glyphs.
	if _, _, _, err := atlas.InsertBitmap(
		makeSyntheticBitmap(8, 8, 1, 2, 3, 4), 0, 0); err != nil {
		t.Fatalf("insert after Reset: %v", err)
	}
}

// TestAtlasSingleStagingBuffer checks that pages keep one staging buffer.
// Every backend copies the pixels during UpdateTexture (glTexSubImage2D,
// replaceRegion, WritePixels, copy), so a second buffer only doubled the
// atlas heap.
func TestAtlasSingleStagingBuffer(t *testing.T) {
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	if atlas.Pages[0].StagingFront != nil {
		t.Error("new page allocated StagingFront")
	}
	if err := atlas.growPage(0, 128); err != nil {
		t.Fatal(err)
	}
	if atlas.Pages[0].StagingFront != nil {
		t.Error("grown page allocated StagingFront")
	}

	bmp := makeSyntheticBitmap(4, 4, 9, 8, 7, 255)
	cg, _, _, err := atlas.InsertBitmap(bmp, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	atlas.SwapAndUpload()
	page := &atlas.Pages[0]
	off := (cg.Y*page.Width + cg.X) * 4
	if got := backend.textures[page.TextureID][off]; got != 9 {
		t.Errorf("uploaded texel = %d, want 9", got)
	}
	if page.StagingBack[off] != 9 {
		t.Errorf("staging lost the glyph after upload: %d", page.StagingBack[off])
	}
}
