package glyph

import (
	"image"
	"testing"
)

// rectMockBackend is a mockBackend that also implements
// RectTextureUpdater, so it exercises the mid-frame upload path. It
// applies each sub-rectangle into its texture the way a GPU would,
// letting a test assert both the region that was uploaded and the pixels
// that landed.
type rectMockBackend struct {
	mockBackend
	rectCalls []rectCall
	fullCalls int
}

type rectCall struct {
	id         TextureID
	x, y, w, h int
}

func newRectMockBackend() *rectMockBackend {
	return &rectMockBackend{
		mockBackend: mockBackend{textures: make(map[TextureID][]byte)},
	}
}

func (m *rectMockBackend) UpdateTexture(id TextureID, data []byte) {
	m.fullCalls++
	m.mockBackend.UpdateTexture(id, data)
}

func (m *rectMockBackend) UpdateTextureRect(id TextureID, data []byte,
	srcStride, x, y, w, h int) {

	tex, ok := m.textures[id]
	if !ok {
		return
	}
	m.rectCalls = append(m.rectCalls, rectCall{id, x, y, w, h})
	for row := range h {
		src := (y+row)*srcStride + x*4
		dst := (y+row)*srcStride + x*4
		copy(tex[dst:dst+w*4], data[src:src+w*4])
	}
}

// texelAt reads one RGBA pixel out of a recorded texture.
func (m *rectMockBackend) texelAt(id TextureID, stride, x, y int) [4]byte {
	tex := m.textures[id]
	off := y*stride + x*4
	return [4]byte{tex[off], tex[off+1], tex[off+2], tex[off+3]}
}

func TestAtlasInsertTracksDirtyRect(t *testing.T) {
	atlas, err := NewGlyphAtlas(newRectMockBackend(), 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	if _, _, _, err = atlas.InsertBitmap(
		makeSyntheticBitmap(10, 12, 1, 2, 3, 255), 0, 0); err != nil {
		t.Fatal(err)
	}

	// The padded box: a 10x12 glyph with a 1px ring, placed at the
	// origin of the first shelf.
	want := image.Rect(0, 0, 10+atlasGlyphPadding*2, 12+atlasGlyphPadding*2)
	if got := atlas.Pages[0].DirtyRect; got != want {
		t.Errorf("DirtyRect = %v, want %v", got, want)
	}
}

func TestAtlasUploadDirtyRectsNoopWithoutRectSupport(t *testing.T) {
	// The plain mock implements only DrawBackend. Uploading whole pages
	// per draw call is what UploadDirtyRects exists to avoid, so it must
	// leave the page pending for the frame-boundary Commit.
	backend := newMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	if _, _, _, err = atlas.InsertBitmap(
		makeSyntheticBitmap(8, 8, 9, 9, 9, 255), 0, 0); err != nil {
		t.Fatal(err)
	}

	atlas.UploadDirtyRects()
	if !atlas.Pages[0].Dirty {
		t.Error("page should still be pending without rect support")
	}

	atlas.SwapAndUpload()
	if atlas.Pages[0].Dirty {
		t.Error("SwapAndUpload should have cleared the page")
	}
}

func TestAtlasUploadDirtyRectsUploadsOnlyChangedRegion(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	if _, _, _, err = atlas.InsertBitmap(
		makeSyntheticBitmap(10, 12, 11, 22, 33, 255), 0, 0); err != nil {
		t.Fatal(err)
	}
	atlas.UploadDirtyRects()

	if backend.fullCalls != 0 {
		t.Errorf("full-page uploads = %d, want 0", backend.fullCalls)
	}
	if len(backend.rectCalls) != 1 {
		t.Fatalf("rect uploads = %d, want 1", len(backend.rectCalls))
	}
	got := backend.rectCalls[0]
	want := rectCall{atlas.Pages[0].TextureID, 0, 0,
		10 + atlasGlyphPadding*2, 12 + atlasGlyphPadding*2}
	if got != want {
		t.Errorf("rect upload = %+v, want %+v", got, want)
	}
	if atlas.Pages[0].Dirty {
		t.Error("page should be clean after UploadDirtyRects")
	}

	// The glyph's own texels must have reached the texture, offset past
	// the padding ring.
	page := &atlas.Pages[0]
	texel := backend.texelAt(page.TextureID, page.Width*4,
		atlasGlyphPadding, atlasGlyphPadding)
	if texel != [4]byte{11, 22, 33, 255} {
		t.Errorf("uploaded texel = %v, want {11 22 33 255}", texel)
	}
}

// TestAtlasIncrementalUploadsAccumulate is the load-bearing case for the
// partial copy-back in uploadPage: uploading only the dirty rows must not
// drop glyphs uploaded earlier, either from the staging buffers or from
// the texture. A second glyph is placed on a second shelf so its dirty
// rows do not overlap the first's.
func TestAtlasIncrementalUploadsAccumulate(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	// A short glyph, then a much taller one — the height mismatch forces
	// findBestShelf to open a new shelf rather than reuse the first.
	first, _, _, err := atlas.InsertBitmap(
		makeSyntheticBitmap(8, 4, 200, 0, 0, 255), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	atlas.UploadDirtyRects()

	second, _, _, err := atlas.InsertBitmap(
		makeSyntheticBitmap(8, 20, 0, 200, 0, 255), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	atlas.UploadDirtyRects()

	if first.Y == second.Y {
		t.Fatalf("test needs two shelves; both glyphs at y=%d", first.Y)
	}

	page := &atlas.Pages[0]
	stride := page.Width * 4

	// Both glyphs are on the GPU.
	if got := backend.texelAt(page.TextureID, stride, first.X, first.Y); got !=
		[4]byte{200, 0, 0, 255} {
		t.Errorf("first glyph texel = %v, want {200 0 0 255}", got)
	}
	if got := backend.texelAt(page.TextureID, stride, second.X, second.Y); got !=
		[4]byte{0, 200, 0, 255} {
		t.Errorf("second glyph texel = %v, want {0 200 0 255}", got)
	}

	// And the staging buffers agree with each other, so the next
	// incremental upload starts from a consistent page.
	for i := range page.StagingFront {
		if page.StagingFront[i] != page.StagingBack[i] {
			t.Fatalf("staging buffers diverged at byte %d: front=%d back=%d",
				i, page.StagingFront[i], page.StagingBack[i])
		}
	}
	// StagingBack is the rasterization target other code reads; it must
	// still hold the first glyph.
	off := first.Y*stride + first.X*4
	if page.StagingBack[off] != 200 {
		t.Errorf("StagingBack lost the first glyph: got %d, want 200",
			page.StagingBack[off])
	}
}

func TestAtlasResetMarksWholePageDirty(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	if _, _, _, err = atlas.InsertBitmap(
		makeSyntheticBitmap(8, 8, 255, 255, 255, 255), 0, 0); err != nil {
		t.Fatal(err)
	}
	atlas.UploadDirtyRects()

	atlas.Reset()
	page := &atlas.Pages[0]
	want := image.Rect(0, 0, page.Width, page.Height)
	if page.DirtyRect != want {
		t.Errorf("DirtyRect after Reset = %v, want %v", page.DirtyRect, want)
	}

	// Uploading must clear the evicted glyph from the texture, not just
	// from staging.
	atlas.UploadDirtyRects()
	if got := backend.texelAt(page.TextureID, page.Width*4,
		atlasGlyphPadding, atlasGlyphPadding); got != [4]byte{} {
		t.Errorf("texel after reset+upload = %v, want zeroed", got)
	}
}

// TestAtlasDirtyBoolAloneUploadsWholePage covers the documented fallback:
// AtlasPage is exported, so code outside the package may flip Dirty
// without a rect. That must upload everything rather than nothing.
func TestAtlasDirtyBoolAloneUploadsWholePage(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	page := &atlas.Pages[0]
	page.Dirty = true
	page.DirtyRect = image.Rectangle{}
	atlas.SwapAndUpload()

	if len(backend.rectCalls) != 1 {
		t.Fatalf("rect uploads = %d, want 1", len(backend.rectCalls))
	}
	got := backend.rectCalls[0]
	if got.x != 0 || got.y != 0 || got.w != page.Width || got.h != page.Height {
		t.Errorf("upload = %+v, want the full %dx%d page",
			got, page.Width, page.Height)
	}
}

// TestAtlasGrowPageMarksWholePageDirty covers the reallocation path:
// growPage hands the page a brand-new texture and a zeroed StagingFront,
// so anything short of a full-page region would leave the GPU copy and
// the two staging buffers disagreeing.
func TestAtlasGrowPageMarksWholePageDirty(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	if _, _, _, err = atlas.InsertBitmap(
		makeSyntheticBitmap(8, 8, 200, 200, 200, 255), 0, 0); err != nil {
		t.Fatal(err)
	}
	atlas.UploadDirtyRects()

	page := &atlas.Pages[0]
	oldTexture := page.TextureID
	if err = atlas.growPage(0, 128); err != nil {
		t.Fatal(err)
	}
	if page.TextureID == oldTexture {
		t.Fatal("growPage did not replace the texture")
	}
	want := image.Rect(0, 0, page.Width, 128)
	if page.DirtyRect != want {
		t.Errorf("DirtyRect after growPage = %v, want %v", page.DirtyRect, want)
	}

	// The upload must carry the preserved glyph onto the new texture and
	// leave both staging buffers identical again.
	atlas.UploadDirtyRects()
	stride := page.Width * 4
	if got := backend.texelAt(page.TextureID, stride,
		atlasGlyphPadding, atlasGlyphPadding); got[0] != 200 {
		t.Errorf("texel on grown texture = %v, want red 200", got)
	}
	for i := range page.StagingFront {
		if page.StagingFront[i] != page.StagingBack[i] {
			t.Fatalf("staging buffers diverged at byte %d after upload", i)
		}
	}
}

// TestAtlasPendingUploadClampsOutOfBounds covers a DirtyRect set by
// embedding code to a box that does not overlap the page. There is
// nothing addressable to send, so uploadPage must drop the request
// instead of issuing a zero-extent rect upload, and must clear Dirty so
// the page does not retry forever.
func TestAtlasPendingUploadClampsOutOfBounds(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	page := &atlas.Pages[0]
	page.Dirty = true
	page.DirtyRect = image.Rect(500, 500, 600, 600)
	atlas.SwapAndUpload()

	if len(backend.rectCalls) != 0 || backend.fullCalls != 0 {
		t.Errorf("uploads = %d rect / %d full, want none",
			len(backend.rectCalls), backend.fullCalls)
	}
	if page.Dirty {
		t.Error("Dirty still set after an unaddressable region")
	}
}

// TestAtlasPendingUploadClampsToShortBuffer covers Width/Height being
// enlarged without the staging buffers following: the region must shrink
// to the rows that actually exist rather than slicing past the buffer.
func TestAtlasPendingUploadClampsToShortBuffer(t *testing.T) {
	backend := newRectMockBackend()
	atlas, err := NewGlyphAtlas(backend, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.Free()

	page := &atlas.Pages[0]
	page.Height = 256 // Buffers still hold only 64 rows.
	page.Dirty = true
	page.DirtyRect = image.Rectangle{}

	if got := page.pendingUpload(); got != image.Rect(0, 0, 64, 64) {
		t.Errorf("pendingUpload = %v, want the 64 rows the buffers hold", got)
	}
	atlas.SwapAndUpload() // Must not panic.
}
