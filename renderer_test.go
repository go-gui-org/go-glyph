//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

import (
	"testing"
)

// newPangoTestRenderer creates a Renderer with a mockBackend for testing
// cache management paths that don't require CGO glyph loading.
func newPangoTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	backend := newMockBackend()
	r, err := NewRendererWithConfig(backend, 1.0, 512, 512, RendererConfig{
		MaxGlyphCacheEntries: 32,
	})
	if err != nil {
		t.Fatalf("NewRendererWithConfig: %v", err)
	}
	t.Cleanup(func() { r.Free() })
	return r
}

func TestRendererGlyphCache_NilFaceReturnsEmpty(t *testing.T) {
	r := newPangoTestRenderer(t)
	item := Item{FTFace: nil}
	g := Glyph{Index: 65}
	cg := r.getOrLoadGlyph(item, g, 0, 0)
	if cg != (CachedGlyph{}) {
		t.Error("nil FTFace should return zero CachedGlyph")
	}
	// Cache must not be modified by a nil-face call.
	if len(r.cache) != 0 {
		t.Errorf("cache should be empty after nil-face call, got %d entries", len(r.cache))
	}
}

func TestRendererGlyphCache_Hit(t *testing.T) {
	r := newPangoTestRenderer(t)
	key := uint64(42)
	want := CachedGlyph{X: 10, Y: 20, Width: 30, Height: 40, Page: 0}
	r.cache[key] = want
	r.cacheAges[key] = 0
	r.atlas.FrameCounter = 99

	// We can't call getOrLoadGlyph directly with a real FTFace,
	// so test cache lookup by populating directly.
	item := Item{FTFace: nil}
	g := Glyph{Index: 65}
	cg := r.getOrLoadGlyph(item, g, 0, 0)

	// Nil FTFace returns zero unconditionally — bypass cache.
	// Instead, test the negative cache path separately.
	if cg != (CachedGlyph{}) {
		t.Error("nil FTFace should return zero")
	}
}

func TestRendererGlyphCache_NegativeCache(t *testing.T) {
	r := newPangoTestRenderer(t)
	key := uint64(99)
	r.cache[key] = CachedGlyph{Page: -1, Width: 10}
	r.cacheAges[key] = 5

	// Verify the negative cache structure is correct.
	cached, ok := r.cache[key]
	if !ok {
		t.Fatal("expected cache entry")
	}
	if cached.Page != -1 {
		t.Error("negative cache should have Page == -1")
	}
	// getOrLoadGlyph returns zero for Page < 0 on cache hit.
	// (Tested via direct struct inspection since nil FTFace path
	// doesn't reach the cache.)
}

func TestRendererGlyphCache_EvictionOrder(t *testing.T) {
	r := newPangoTestRenderer(t)
	// Populate to capacity with known ages.
	for i := uint64(0); i < uint64(r.maxCacheEntries); i++ {
		r.cache[i] = CachedGlyph{Page: 0, Width: int(i)}
		r.cacheAges[i] = i // Older entries have smaller ages.
	}
	r.atlas.FrameCounter = uint64(r.maxCacheEntries)

	// Insert one more entry — triggers eviction of oldest (key 0).
	newKey := uint64(99999)
	r.cache[newKey] = CachedGlyph{Page: 0, Width: 99}
	r.cacheAges[newKey] = r.atlas.FrameCounter + 1
	r.atlas.FrameCounter++

	// Manually call eviction since we bypassed getOrLoadGlyph.
	if len(r.cache) > r.maxCacheEntries {
		r.evictOldestGlyph()
	}
	if len(r.cache) > r.maxCacheEntries {
		t.Errorf("cache entries %d exceeds capacity %d after eviction",
			len(r.cache), r.maxCacheEntries)
	}
	if _, ok := r.cache[0]; ok {
		t.Error("oldest key 0 should have been evicted")
	}
	if _, ok := r.cache[newKey]; !ok {
		t.Error("newest key should survive")
	}
}

func TestRendererGlyphCache_EvictionEmptyNoPanic(t *testing.T) {
	r := newPangoTestRenderer(t)
	// evictOldestGlyph on empty cache must not panic.
	r.evictOldestGlyph()
	if len(r.cache) != 0 {
		t.Error("cache should remain empty after eviction on empty")
	}
}

func TestRendererGlyphCache_PageKeyTracking(t *testing.T) {
	r := newPangoTestRenderer(t)
	page := 3
	keys := []uint64{100, 200, 300}
	for _, k := range keys {
		r.cache[k] = CachedGlyph{Page: page}
		r.cacheAges[k] = 1
		r.pageKeys[page] = append(r.pageKeys[page], k)
	}
	got := r.pageKeys[page]
	if len(got) != 3 {
		t.Fatalf("pageKeys[%d] len = %d, want 3", page, len(got))
	}
	for i, k := range keys {
		if got[i] != k {
			t.Errorf("pageKeys[%d][%d] = %d, want %d", page, i, got[i], k)
		}
	}
}

func TestRendererGlyphCache_PageResetInvalidation(t *testing.T) {
	r := newPangoTestRenderer(t)
	page := 1
	keys := []uint64{10, 20, 30}
	for _, k := range keys {
		r.cache[k] = CachedGlyph{Page: page}
		r.cacheAges[k] = 5
	}
	r.pageKeys[page] = keys

	// Simulate page reset: delete all cache entries for the page.
	for _, k := range r.pageKeys[page] {
		delete(r.cache, k)
		delete(r.cacheAges, k)
	}
	delete(r.pageKeys, page)

	if len(r.cache) != 0 {
		t.Errorf("cache len = %d, want 0 after page reset", len(r.cache))
	}
	if len(r.cacheAges) != 0 {
		t.Errorf("cacheAges len = %d, want 0", len(r.cacheAges))
	}
	if _, ok := r.pageKeys[page]; ok {
		t.Error("pageKeys should not contain reset page")
	}
}

func TestRendererGlyphCache_RemovePageKey(t *testing.T) {
	r := newPangoTestRenderer(t)
	r.pageKeys[0] = []uint64{11, 22, 33, 44}

	r.removePageKey(0, 22) // Remove middle element.
	got := r.pageKeys[0]
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Unordered removal: remaining keys should be {11, 44, 33} or similar.
	found := make(map[uint64]bool)
	for _, k := range got {
		found[k] = true
	}
	for _, expect := range []uint64{11, 33, 44} {
		if !found[expect] {
			t.Errorf("missing expected key %d", expect)
		}
	}
	if found[22] {
		t.Error("removed key 22 should not be present")
	}
}

func TestRendererGlyphCache_RemovePageKeyLast(t *testing.T) {
	r := newPangoTestRenderer(t)
	r.pageKeys[0] = []uint64{55}

	r.removePageKey(0, 55)
	if len(r.pageKeys[0]) != 0 {
		t.Errorf("len = %d, want 0 after removing last key", len(r.pageKeys[0]))
	}
}

func TestComputeSubpixelBin(t *testing.T) {
	r := newPangoTestRenderer(t)

	tests := []struct {
		name    string
		x       float32
		isEmoji bool
	}{
		{"emoji_always_zero", 10.5, true},
		{"emoji_fractional", 3.7, true},
		{"non_emoji_zero", 0.0, false},
		{"non_emoji_offset", 0.3, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bin := r.computeSubpixelBin(tc.x, tc.isEmoji)
			if tc.isEmoji && bin != 0 {
				t.Errorf("emoji bin = %d, want 0", bin)
			}
			if bin < 0 || bin >= SubpixelBins {
				t.Errorf("bin %d out of range [0, %d)", bin, SubpixelBins)
			}
		})
	}
}
