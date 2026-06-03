package glyph

import (
	"fmt"
	"math"
)

// atlasGlyphPadding is the number of transparent pixels surrounding
// each glyph in the atlas texture. A 1-pixel border prevents texture
// sampling bleed from adjacent glyphs during bilinear/trilinear GPU
// filtering. Increasing to 2 would improve quality at high
// magnification but wastes ~1.5% more atlas space per glyph.
const atlasGlyphPadding = 1

// AtlasPage is a single texture page in a multi-page glyph atlas.
type AtlasPage struct {
	Shelves      []Shelf
	StagingFront []byte // GPU upload source.
	StagingBack  []byte // CPU rasterization target.
	TextureID    TextureID
	Width        int
	Height       int
	Age          uint64 // Frame counter when last used.
	UsedPixels   int64
	Dirty        bool
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
	Backend           DrawBackend
	Pages             []AtlasPage
	Garbage           []TextureID // Textures pending deletion.
	MaxPages          int
	CurrentPage       int
	FrameCounter      uint64
	MaxGlyphDimension int
	LastFrame         uint64
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
// Returns n unchanged if already a power of two.
func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
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
	}, nil
}

// Free releases all atlas textures.
func (atlas *GlyphAtlas) Free() {
	for _, page := range atlas.Pages {
		atlas.Backend.DeleteTexture(page.TextureID)
	}
	for _, id := range atlas.Garbage {
		atlas.Backend.DeleteTexture(id)
	}
	atlas.Pages = nil
	atlas.Garbage = nil
}

// Cleanup removes stale textures from previous frames.
func (atlas *GlyphAtlas) Cleanup(frame uint64) {
	if frame > atlas.LastFrame {
		for _, id := range atlas.Garbage {
			atlas.Backend.DeleteTexture(id)
		}
		atlas.Garbage = atlas.Garbage[:0]
		atlas.LastFrame = frame
	}
}

// InsertBitmap places a bitmap into the atlas using shelf-based
// best-height-fit with multi-page support.
// Returns the CachedGlyph, whether a page reset occurred, and
// the index of the reset page.
func (atlas *GlyphAtlas) InsertBitmap(bmp Bitmap, left, top int) (CachedGlyph, bool, int, error) {
	glyphW := bmp.Width
	glyphH := bmp.Height
	paddedW := glyphW + atlasGlyphPadding*2
	paddedH := glyphH + atlasGlyphPadding*2

	if glyphW > atlas.MaxGlyphDimension || glyphH > atlas.MaxGlyphDimension {
		return CachedGlyph{}, false, 0, fmt.Errorf(
			"glyph dimensions (%dx%d) exceed max atlas size (%d)",
			glyphW, glyphH, atlas.MaxGlyphDimension)
	}
	if glyphW <= 0 || glyphH <= 0 {
		return CachedGlyph{}, false, 0, nil // empty glyph
	}

	page := &atlas.Pages[atlas.CurrentPage]
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
				newPage, err := newAtlasPage(atlas.Backend, page.Width, 1024)
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
	page.Dirty = true
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

// SwapAndUpload swaps staging buffers and uploads dirty pages
// to the GPU.
func (atlas *GlyphAtlas) SwapAndUpload() {
	for i := range atlas.Pages {
		page := &atlas.Pages[i]
		if !page.Dirty {
			continue
		}
		// Swap front/back.
		page.StagingFront, page.StagingBack = page.StagingBack, page.StagingFront
		// Copy front→back to preserve accumulated data.
		copy(page.StagingBack, page.StagingFront)

		atlas.Backend.UpdateTexture(page.TextureID, page.StagingFront)
		page.Dirty = false
		page.Age = atlas.FrameCounter
	}
}

// --- internal helpers ---

func newAtlasPage(backend DrawBackend, w, h int) (AtlasPage, error) {
	if w <= 0 || h <= 0 {
		return AtlasPage{}, fmt.Errorf("atlas page dimensions must be positive: %dx%d", w, h)
	}
	size, err := checkAllocationSize(w, h, 4)
	if err != nil {
		return AtlasPage{}, err
	}
	texID := backend.NewTexture(w, h)
	return AtlasPage{
		TextureID:    texID,
		Width:        w,
		Height:       h,
		StagingFront: make([]byte, size),
		StagingBack:  make([]byte, size),
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
	page := &atlas.Pages[pageIdx]
	page.Shelves = page.Shelves[:0]
	page.UsedPixels = 0
	page.Age = atlas.FrameCounter

	// Zero out staging buffers.
	clear(page.StagingFront)
	clear(page.StagingBack)
	page.Dirty = true
}

func (atlas *GlyphAtlas) growPage(pageIdx, newHeight int) error {
	page := &atlas.Pages[pageIdx]
	if newHeight <= page.Height {
		return nil
	}
	newSize, err := checkAllocationSize(page.Width, newHeight, 4)
	if err != nil {
		return err
	}
	oldSize := int64(page.Width) * int64(page.Height) * 4

	// Reallocate staging buffers, preserving existing data.
	newFront := make([]byte, newSize)
	newBack := make([]byte, newSize)
	copy(newBack, page.StagingBack[:oldSize])

	page.StagingFront = newFront
	page.StagingBack = newBack
	page.Height = newHeight

	// Replace texture (old one goes to garbage for deferred deletion).
	atlas.Garbage = append(atlas.Garbage, page.TextureID)
	page.TextureID = atlas.Backend.NewTexture(page.Width, newHeight)
	page.Dirty = true
	return nil
}

func copyBitmapToPage(page *AtlasPage, bmp Bitmap, x, y int) error {
	if x < 0 || y < 0 || x+bmp.Width > page.Width || y+bmp.Height > page.Height {
		return fmt.Errorf("bitmap copy out of bounds: pos(%d,%d) size(%dx%d) page(%dx%d)",
			x, y, bmp.Width, bmp.Height, page.Width, page.Height)
	}
	if bmp.Width <= 0 || bmp.Height <= 0 || len(bmp.Data) == 0 {
		return nil
	}
	rowBytes := bmp.Width * 4
	for row := range bmp.Height {
		srcOff := row * rowBytes
		dstOff := ((y+row)*page.Width + x) * 4
		copy(page.StagingBack[dstOff:dstOff+rowBytes], bmp.Data[srcOff:srcOff+rowBytes])
	}
	return nil
}
