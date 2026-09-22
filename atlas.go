package glyph

import (
	"fmt"
	"image"
	"math"
	"math/bits"

	"golang.org/x/image/vector"
)

// atlasGlyphPadding is the number of transparent pixels surrounding
// each glyph in the atlas texture. A 1-pixel border prevents texture
// sampling bleed from adjacent glyphs during bilinear/trilinear GPU
// filtering. Increasing to 2 would improve quality at high
// magnification but wastes ~1.5% more atlas space per glyph.
const atlasGlyphPadding = 1

// AtlasPage is a single texture page in a multi-page glyph atlas.
type AtlasPage struct {
	Shelves []Shelf
	// StagingFront is no longer used and stays nil.
	//
	// Deprecated: pages keep one staging buffer, StagingBack. Every
	// backend copies the pixels during UpdateTexture (glTexSubImage2D,
	// replaceRegion, WritePixels or copy), so a second buffer only
	// doubled the atlas heap.
	StagingFront []byte
	// StagingBack is the page's staging buffer: the CPU rasterization
	// target and the GPU upload source.
	StagingBack []byte
	// DirtyRect is the half-open pixel box rasterized into since the last
	// upload, in page coordinates. Meaningful only while Dirty is set; an
	// empty rect alongside Dirty is read as "all of it", so a caller that
	// only flips Dirty still gets a correct (if whole-page) upload.
	DirtyRect  image.Rectangle
	TextureID  TextureID
	Width      int
	Height     int
	Age        uint64 // Frame counter when last used.
	UsedPixels int64
	Dirty      bool
}

// markDirty unions the half-open box (x, y, w, h) into the page's pending
// upload region.
func (page *AtlasPage) markDirty(x, y, w, h int) {
	page.Dirty = true
	page.DirtyRect = page.DirtyRect.Union(image.Rect(x, y, x+w, y+h))
}

// pendingUpload returns the region to upload, clamped both to the page's
// declared bounds and to the rows the staging buffer actually holds. An
// empty DirtyRect means the whole page (see the field comment).
//
// The clamp is not paranoia about our own writers: AtlasPage's fields are
// exported, so Width/Height/DirtyRect/staging can be set independently by
// embedding code. Trusting them would turn a caller's mistake into an
// out-of-range slice in uploadPage or a zero-extent UpdateTextureRect
// call, so the region is reconciled with the buffers here instead.
func (page *AtlasPage) pendingUpload() image.Rectangle {
	rowBytes := page.Width * 4
	rows := page.Height
	if rowBytes <= 0 || rows <= 0 {
		return image.Rectangle{}
	}
	// A short buffer caps the region at whole rows we can address.
	if n := len(page.StagingBack) / rowBytes; n < rows {
		rows = n
	}
	full := image.Rect(0, 0, page.Width, rows)
	if page.DirtyRect.Empty() {
		return full
	}
	return page.DirtyRect.Intersect(full)
}

// Shelf is a horizontal strip within an atlas page.
type Shelf struct {
	Y       int // Vertical position of shelf top.
	Height  int // Shelf height (fixed at creation).
	CursorX int // Next free x position.
	Width   int // Shelf width (page width).
}

// GlyphAtlas manages a multi-page texture atlas for glyph bitmaps.
//
// Not safe for concurrent use. Accessed only through Renderer.
type GlyphAtlas struct {
	Backend      DrawBackend
	Pages        []AtlasPage
	Garbage      []TextureID // Textures pending deletion.
	MaxPages     int
	CurrentPage  int
	FrameCounter uint64
	// MaxGlyphDimension caps the width and height that InsertBitmap
	// accepts. The rasterizer clamps its own output to MaxGlyphSize
	// (256), so larger values only matter to direct InsertBitmap
	// callers with their own bitmaps.
	MaxGlyphDimension int
	LastFrame         uint64

	// initialHeight is the height the first page was created with. Reset
	// shrinks a grown first page back to it, so a purge gives back memory.
	initialHeight int

	// scratchAlpha and scratchRGBA are grow-only byte buffers reused
	// across rasterization calls to eliminate per-glyph allocations.
	// scratchRasterizer is a reused vector rasterizer.
	scratchAlpha  []byte
	scratchRGBA   []byte
	scratchRaster *vector.Rasterizer
}

// CachedGlyph stores atlas coordinates and bearing info for a
// rasterized glyph.
type CachedGlyph struct {
	X      int
	Y      int
	Width  int
	Height int
	Left   int // Bitmap left bearing.
	Top    int // Bitmap top bearing.
	Page   int // Atlas page index.
}

// nextPowerOfTwo rounds n up to the next power of two.
// It returns n unchanged when n is already a power of two.
// It saturates at math.MaxInt when the next power of two does not
// fit in an int, so callers never see a wrapped negative value.
func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	k := bits.Len(uint(n - 1))
	if k >= bits.UintSize-1 {
		return math.MaxInt
	}
	return 1 << k
}

// NewGlyphAtlas creates a new glyph atlas with one initial page.
// Dimensions are rounded up to the next power of two to satisfy GPU
// texture alignment requirements (most drivers silently round up
// non-power-of-two textures, wasting VRAM). Dimensions below 64 are
// clamped to 64.
func NewGlyphAtlas(backend DrawBackend, w, h int) (*GlyphAtlas, error) {
	w = max(nextPowerOfTwo(w), 64)
	h = max(nextPowerOfTwo(h), 64)
	page, err := newAtlasPage(backend, w, h)
	if err != nil {
		return nil, err
	}
	return &GlyphAtlas{
		Backend:           backend,
		Pages:             []AtlasPage{page},
		MaxPages:          4,
		CurrentPage:       0,
		MaxGlyphDimension: 4096,
		initialHeight:     h,
	}, nil
}

// Reset clears the atlas and gives back the memory it grew into. It
// keeps only the first page, cleared and shrunk to its initial height.
// Use it to reclaim atlas space mid-session while keeping the TextSystem
// alive.
//
// The textures of dropped or shrunk pages go to Garbage, not straight to
// DeleteTexture: quads already emitted this frame can still refer to
// them. The next Cleanup with a new frame number deletes them.
//
// Reset invalidates every CachedGlyph handed out before the call.
// Do not draw with old coordinates after Reset. To clear the
// Renderer cache at the same time, call PurgeGlyphCache instead
// of calling Reset directly.
func (atlas *GlyphAtlas) Reset() {
	if len(atlas.Pages) == 0 {
		return
	}
	for i := 1; i < len(atlas.Pages); i++ {
		atlas.Garbage = append(atlas.Garbage, atlas.Pages[i].TextureID)
		atlas.Pages[i] = AtlasPage{} // drop the staging buffer now
	}
	atlas.Pages = atlas.Pages[:1]
	atlas.CurrentPage = 0

	page := &atlas.Pages[0]
	if h := atlas.initialHeight; h > 0 && page.Height > h && atlas.Backend != nil {
		// A new, smaller texture replaces the grown one. On allocation
		// failure keep the grown page; resetPage below still clears it.
		if size, err := checkAllocationSize(page.Width, h, 4); err == nil {
			atlas.Garbage = append(atlas.Garbage, page.TextureID)
			page.TextureID = atlas.Backend.NewTexture(page.Width, h)
			page.Height = h
			page.StagingBack = make([]byte, size)
		}
	}
	atlas.resetPage(0)
}

// Free releases all atlas textures. It is safe to call Free twice.
// After Free the atlas holds no pages, so InsertBitmap returns an
// error until the atlas is discarded.
func (atlas *GlyphAtlas) Free() {
	if atlas.Backend != nil {
		for _, page := range atlas.Pages {
			atlas.Backend.DeleteTexture(page.TextureID)
		}
		for _, id := range atlas.Garbage {
			atlas.Backend.DeleteTexture(id)
		}
	}
	atlas.Pages = nil
	atlas.Garbage = nil
	atlas.CurrentPage = 0
}

// Cleanup removes stale textures from previous frames. It collects
// when frame differs from the last cleanup, forward or backward, so
// a repeated call within one frame stays cheap and a backward jump
// in frame numbers cannot leak textures.
func (atlas *GlyphAtlas) Cleanup(frame uint64) {
	if frame == atlas.LastFrame {
		return
	}
	if atlas.Backend != nil {
		for _, id := range atlas.Garbage {
			atlas.Backend.DeleteTexture(id)
		}
	}
	atlas.Garbage = atlas.Garbage[:0]
	atlas.LastFrame = frame
}

// InsertBitmap places a bitmap into the atlas using shelf-based
// best-height-fit with multi-page support.
// Returns the CachedGlyph, whether a page reset occurred, and
// the index of the reset page.
func (atlas *GlyphAtlas) InsertBitmap(bmp Bitmap, left, top int) (CachedGlyph, bool, int, error) {
	glyphW := bmp.Width
	glyphH := bmp.Height

	if glyphW > atlas.MaxGlyphDimension || glyphH > atlas.MaxGlyphDimension {
		return CachedGlyph{}, false, 0, fmt.Errorf(
			"glyph dimensions (%dx%d) exceed max atlas size (%d)",
			glyphW, glyphH, atlas.MaxGlyphDimension)
	}
	if glyphW <= 0 || glyphH <= 0 {
		return CachedGlyph{}, false, 0, nil // empty glyph
	}
	if atlas.Backend == nil {
		return CachedGlyph{}, false, 0, fmt.Errorf("glyph atlas has no backend")
	}
	if len(atlas.Pages) == 0 || atlas.CurrentPage < 0 ||
		atlas.CurrentPage >= len(atlas.Pages) {
		return CachedGlyph{}, false, 0, fmt.Errorf(
			"glyph atlas has no usable page (Free was called?)")
	}
	paddedW := glyphW + atlasGlyphPadding*2
	paddedH := glyphH + atlasGlyphPadding*2

	page := &atlas.Pages[atlas.CurrentPage]
	if paddedW > page.Width {
		return CachedGlyph{}, false, 0, fmt.Errorf(
			"glyph too wide for atlas page: width %d exceeds page width %d",
			paddedW, page.Width)
	}
	resetOccurred := false
	resetPageIdx := 0

	shelfIdx := page.findBestShelf(paddedW, paddedH)

	if shelfIdx < 0 {
		newY := page.getNextShelfY()
		if newY+paddedH > page.Height {
			// Page full — try grow, add page, or reset.
			if page.Height < atlas.MaxGlyphDimension {
				newHeight := page.Height * 2
				if newHeight == 0 {
					newHeight = 1024
				}
				if newHeight > atlas.MaxGlyphDimension {
					newHeight = atlas.MaxGlyphDimension
				}
				if err := atlas.growPage(atlas.CurrentPage, newHeight); err != nil {
					return CachedGlyph{}, false, 0, err
				}
			} else if len(atlas.Pages) < atlas.MaxPages {
				// This branch runs only when the page has grown to
				// MaxGlyphDimension. Cap new pages at 1024 rows so the
				// default 4096 limit does not allocate a full-height page
				// at once; the new page grows on demand like the first.
				// Small atlases (limit below 1024) stay at their height.
				newPage, err := newAtlasPage(atlas.Backend, page.Width,
					min(page.Height, 1024))
				if err != nil {
					return CachedGlyph{}, false, 0, err
				}
				atlas.Pages = append(atlas.Pages, newPage)
				atlas.CurrentPage = len(atlas.Pages) - 1
			} else {
				oldestIdx := atlas.findOldestPage()
				atlas.resetPage(oldestIdx)
				atlas.CurrentPage = oldestIdx
				resetOccurred = true
				resetPageIdx = oldestIdx
			}

			page = &atlas.Pages[atlas.CurrentPage]
			shelfIdx = page.findBestShelf(paddedW, paddedH)
		}

		if shelfIdx < 0 {
			newY = page.getNextShelfY()
			if newY+paddedH > page.Height {
				return CachedGlyph{}, false, 0, fmt.Errorf("glyph too large for atlas page")
			}
			page.Shelves = append(page.Shelves, Shelf{
				Y:       newY,
				Height:  paddedH,
				CursorX: 0,
				Width:   page.Width,
			})
			shelfIdx = len(page.Shelves) - 1
		}
	}

	shelf := &page.Shelves[shelfIdx]
	x := shelf.CursorX
	y := shelf.Y
	shelf.CursorX += paddedW

	if err := copyBitmapToPage(
		page, bmp, x+atlasGlyphPadding, y+atlasGlyphPadding,
	); err != nil {
		return CachedGlyph{}, false, 0, err
	}
	// Mark the padded box, not just the glyph: the transparent border is
	// part of what this insert reserved, and uploading it keeps the ring
	// on the GPU consistent with staging.
	page.markDirty(x, y, paddedW, paddedH)
	page.UsedPixels = page.calculateShelfUsedPixels()

	cached := CachedGlyph{
		X:      x + atlasGlyphPadding,
		Y:      y + atlasGlyphPadding,
		Width:  glyphW,
		Height: glyphH,
		Left:   left,
		Top:    top,
		Page:   atlas.CurrentPage,
	}
	return cached, resetOccurred, resetPageIdx, nil
}

// rectUpdater reports the backend's optional sub-rectangle upload
// support, or nil. Resolved per call rather than cached at construction
// so that reassigning the exported Backend field cannot leave a stale
// answer behind; an interface type assertion costs no allocation.
func (atlas *GlyphAtlas) rectUpdater() RectTextureUpdater {
	ru, _ := atlas.Backend.(RectTextureUpdater)
	return ru
}

// SwapAndUpload uploads dirty pages to the GPU. The name predates the
// single staging buffer; nothing is swapped any more.
// Called at the frame boundary by (*Renderer).Commit, which is what makes
// it the backstop: whatever a mid-frame UploadDirtyRects left pending — or
// everything, on a backend without RectTextureUpdater — is sent here.
//
// On a backend that does implement RectTextureUpdater each page sends only
// its pending region rather than its full extent, so the cost tracks what
// was rasterized. Backends without it still receive whole pages.
//
// With no backend set, SwapAndUpload keeps the pages dirty and sends
// nothing, so a later call with a backend still uploads the pixels.
func (atlas *GlyphAtlas) SwapAndUpload() {
	if atlas.Backend == nil {
		return
	}
	for i := range atlas.Pages {
		if page := &atlas.Pages[i]; page.Dirty {
			atlas.uploadPage(page)
		}
	}
}

// UploadDirtyRects uploads pending glyph rasterization mid-frame, so
// quads emitted after it sample texels that are already on the GPU.
//
// It is deliberately a no-op unless the backend implements
// RectTextureUpdater. Without sub-rectangle support the only available
// upload is the whole page, and doing that per draw call would turn every
// frame that introduces a glyph into one multi-megabyte page transfer per
// call — a terminal frame issues hundreds. Such backends keep batching to
// SwapAndUpload at the frame boundary, which is correct for them because
// their draw calls do not sample the texture until present time.
func (atlas *GlyphAtlas) UploadDirtyRects() {
	if atlas.rectUpdater() == nil {
		return
	}
	atlas.SwapAndUpload()
}

// uploadPage uploads one dirty page's pending region and clears its
// dirty state.
func (atlas *GlyphAtlas) uploadPage(page *AtlasPage) {
	region := page.pendingUpload()
	if region.Empty() {
		// Nothing addressable to send — a DirtyRect entirely outside the
		// page, or a degenerate page. Clear the flag so the page does not
		// re-enter this path every frame.
		page.Dirty = false
		page.DirtyRect = image.Rectangle{}
		return
	}

	// Upload straight from the one staging buffer. Every backend copies
	// the pixels before UpdateTexture/UpdateTextureRect returns, so the
	// buffer is free to take the next glyph at once.
	rowBytes := page.Width * 4
	if ru := atlas.rectUpdater(); ru != nil {
		ru.UpdateTextureRect(page.TextureID, page.StagingBack, rowBytes,
			region.Min.X, region.Min.Y, region.Dx(), region.Dy())
	} else {
		atlas.Backend.UpdateTexture(page.TextureID, page.StagingBack)
	}

	page.Dirty = false
	page.DirtyRect = image.Rectangle{}
	page.Age = atlas.FrameCounter
}

// --- internal helpers ---

func newAtlasPage(backend DrawBackend, w, h int) (AtlasPage, error) {
	if backend == nil {
		return AtlasPage{}, fmt.Errorf("atlas backend must not be nil")
	}
	if w <= 0 || h <= 0 {
		return AtlasPage{}, fmt.Errorf("atlas page dimensions must be positive: %dx%d", w, h)
	}
	size, err := checkAllocationSize(w, h, 4)
	if err != nil {
		return AtlasPage{}, err
	}
	texID := backend.NewTexture(w, h)
	return AtlasPage{
		TextureID:   texID,
		Width:       w,
		Height:      h,
		StagingBack: make([]byte, size),
	}, nil
}

func (page *AtlasPage) findBestShelf(glyphW, glyphH int) int {
	bestIdx := -1
	bestWaste := math.MaxInt32

	for i := range page.Shelves {
		s := &page.Shelves[i]
		if glyphH > s.Height {
			continue
		}
		if s.CursorX+glyphW > s.Width {
			continue
		}
		waste := s.Height - glyphH
		if waste < bestWaste {
			bestWaste = waste
			bestIdx = i
		}
	}
	// Reject a shelf match when vertical waste exceeds 50% of the
	// glyph height. This balances reusing existing shelves (fewer
	// shelves = less Y-axis fragmentation) vs. wasting vertical
	// space (tall shelves filled with short glyphs). Empirically
	// yields ~85% atlas utilization for mixed Latin+CJK workloads.
	if bestIdx >= 0 && bestWaste > glyphH/2 {
		return -1
	}
	return bestIdx
}

func (page *AtlasPage) getNextShelfY() int {
	if len(page.Shelves) == 0 {
		return 0
	}
	last := page.Shelves[len(page.Shelves)-1]
	return last.Y + last.Height
}

func (page *AtlasPage) calculateShelfUsedPixels() int64 {
	var used int64
	for _, s := range page.Shelves {
		used += int64(s.CursorX) * int64(s.Height)
	}
	return used
}

func (atlas *GlyphAtlas) findOldestPage() int {
	if len(atlas.Pages) == 0 {
		return 0
	}
	oldestIdx := 0
	oldestAge := atlas.Pages[0].Age
	for i, p := range atlas.Pages {
		if p.Age < oldestAge {
			oldestAge = p.Age
			oldestIdx = i
		}
	}
	return oldestIdx
}

func (atlas *GlyphAtlas) resetPage(pageIdx int) {
	if pageIdx < 0 || pageIdx >= len(atlas.Pages) {
		return
	}
	page := &atlas.Pages[pageIdx]
	page.Shelves = page.Shelves[:0]
	page.UsedPixels = 0
	page.Age = atlas.FrameCounter

	// Zero out the staging buffer. The whole page is now dirty: the GPU
	// copy still holds the evicted glyphs and must be cleared with it.
	clear(page.StagingBack)
	page.DirtyRect = image.Rectangle{}
	page.markDirty(0, 0, page.Width, page.Height)
}

func (atlas *GlyphAtlas) growPage(pageIdx, newHeight int) error {
	if pageIdx < 0 || pageIdx >= len(atlas.Pages) {
		return fmt.Errorf("atlas page index out of range: %d", pageIdx)
	}
	page := &atlas.Pages[pageIdx]
	if newHeight <= page.Height {
		return nil
	}
	newSize, err := checkAllocationSize(page.Width, newHeight, 4)
	if err != nil {
		return err
	}
	oldSize := int64(page.Width) * int64(page.Height) * 4

	// Reallocate the staging buffer, preserving existing data.
	newBack := make([]byte, newSize)
	// The staging buffer is exported, so outside code can shrink it.
	// Copy only what the old buffer still holds. When it is short, the
	// old pixels are already lost and the full-page dirty mark below
	// still makes the GPU copy match the new buffer.
	if int64(len(page.StagingBack)) >= oldSize {
		copy(newBack, page.StagingBack[:oldSize])
	}

	page.StagingBack = newBack
	page.Height = newHeight

	// Replace texture (old one goes to garbage for deferred deletion).
	atlas.Garbage = append(atlas.Garbage, page.TextureID)
	page.TextureID = atlas.Backend.NewTexture(page.Width, newHeight)
	// The whole (larger) page is dirty: the replacement texture is
	// untouched, so only a full-page upload puts the preserved glyphs
	// on it.
	page.DirtyRect = image.Rectangle{}
	page.markDirty(0, 0, page.Width, newHeight)
	return nil
}

func copyBitmapToPage(page *AtlasPage, bmp Bitmap, x, y int) error {
	if bmp.Width <= 0 || bmp.Height <= 0 || len(bmp.Data) == 0 {
		return nil
	}
	if bmp.Channels != 4 {
		return fmt.Errorf("unsupported bitmap channels: %d (want 4 RGBA)",
			bmp.Channels)
	}
	// Compare in int64 so a crafted Bitmap with huge dimensions
	// cannot wrap the arithmetic and pass the bounds check.
	if x < 0 || y < 0 ||
		int64(x)+int64(bmp.Width) > int64(page.Width) ||
		int64(y)+int64(bmp.Height) > int64(page.Height) {
		return fmt.Errorf("bitmap copy out of bounds: pos(%d,%d) size(%dx%d) page(%dx%d)",
			x, y, bmp.Width, bmp.Height, page.Width, page.Height)
	}
	need := int64(bmp.Width) * int64(bmp.Height) * 4
	if int64(len(bmp.Data)) < need {
		return fmt.Errorf("bitmap data too short: have %d bytes, need %d for a %dx%d bitmap",
			len(bmp.Data), need, bmp.Width, bmp.Height)
	}
	// The staging buffers are exported, so outside code can shrink
	// them. Check the last row end, which is the highest address
	// the loop below touches.
	rowBytes := bmp.Width * 4
	lastEnd := int64(y+bmp.Height-1)*int64(page.Width)*4 +
		int64(x)*4 + int64(rowBytes)
	if lastEnd > int64(len(page.StagingBack)) {
		return fmt.Errorf("atlas staging buffer too short: need %d bytes, have %d",
			lastEnd, len(page.StagingBack))
	}
	for row := range bmp.Height {
		srcOff := row * rowBytes
		dstOff := ((y+row)*page.Width + x) * 4
		copy(page.StagingBack[dstOff:dstOff+rowBytes], bmp.Data[srcOff:srcOff+rowBytes])
	}
	return nil
}

// ensureAlpha grows the scratch-alpha buffer if needed and returns an
// *image.Alpha wrapping the buffer at the requested dimensions. The
// contents are not zeroed; the caller must fully overwrite via draw.Src.
// The image aliases atlas scratch storage. Copy the pixels (with
// InsertBitmap) before the next ensure call reuses the buffer.
// A nil return means the dimensions are invalid; the caller must not use
// the result.
func (atlas *GlyphAtlas) ensureAlpha(w, h int) *image.Alpha {
	if _, err := checkAllocationSize(w, h, 1); err != nil {
		return nil
	}
	size := w * h
	if cap(atlas.scratchAlpha) < size {
		atlas.scratchAlpha = make([]byte, size)
	}
	return &image.Alpha{
		Pix:    atlas.scratchAlpha[:size],
		Stride: w,
		Rect:   image.Rect(0, 0, w, h),
	}
}

// ensureRGBA grows the scratch-RGBA buffer if needed and returns an
// *image.RGBA wrapping the buffer at the requested dimensions. The
// contents are not zeroed; the caller must fully overwrite via draw.Src.
// The image aliases atlas scratch storage. Copy the pixels (with
// InsertBitmap) before the next ensure call reuses the buffer.
// A nil return means the dimensions are invalid; the caller must not use
// the result.
func (atlas *GlyphAtlas) ensureRGBA(w, h int) *image.RGBA {
	if _, err := checkAllocationSize(w, h, 4); err != nil {
		return nil
	}
	size := w * h * 4
	if cap(atlas.scratchRGBA) < size {
		atlas.scratchRGBA = make([]byte, size)
	}
	return &image.RGBA{
		Pix:    atlas.scratchRGBA[:size],
		Stride: w * 4,
		Rect:   image.Rect(0, 0, w, h),
	}
}

// ensureRasterizer returns a *vector.Rasterizer of the given dimensions,
// reusing the atlas's rasterizer when present. The rasterizer's DrawOp is
// NOT set; the caller must assign it. A nil return means the dimensions
// are invalid; the caller must not use the result.
func (atlas *GlyphAtlas) ensureRasterizer(w, h int) *vector.Rasterizer {
	if _, err := checkAllocationSize(w, h, 1); err != nil {
		return nil
	}
	if atlas.scratchRaster == nil {
		atlas.scratchRaster = vector.NewRasterizer(w, h)
	} else {
		atlas.scratchRaster.Reset(w, h)
	}
	return atlas.scratchRaster
}
