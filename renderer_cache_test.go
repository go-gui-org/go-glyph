//go:build android || linux || darwin || windows

package glyph

import "testing"

// seedCacheEntry inserts a synthetic cache entry (no rasterization) so
// eviction can be exercised deterministically at the map level.
func seedCacheEntry(r *Renderer, key uint64, page int, age uint64) {
	r.cache[key] = cacheEntry{
		CachedGlyph: CachedGlyph{Width: 1, Height: 1, Page: page},
		age:         age,
	}
	r.pageKeys[page] = append(r.pageKeys[page], key)
}

// TestEvictOldestGlyphRemovesOldest seeds fewer entries than the eviction
// sample size, so the sample covers the whole cache and the globally oldest
// entry must be the one evicted — from both the cache and pageKeys.
func TestEvictOldestGlyphRemovesOldest(t *testing.T) {
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	if evictSampleSize < 4 {
		t.Fatalf("evictSampleSize = %d; test assumes >= 4", evictSampleSize)
	}
	// Ages deliberately unordered relative to key values.
	seedCacheEntry(r, 1, 0, 40)
	seedCacheEntry(r, 2, 0, 10) // oldest
	seedCacheEntry(r, 3, 0, 30)
	seedCacheEntry(r, 4, 0, 20)

	r.evictOldestGlyph()

	if len(r.cache) != 3 {
		t.Fatalf("cache size = %d after eviction, want 3", len(r.cache))
	}
	if _, ok := r.cache[2]; ok {
		t.Error("oldest entry (key 2, age 10) survived eviction")
	}
	for _, k := range r.pageKeys[0] {
		if k == 2 {
			t.Error("evicted key 2 still present in pageKeys")
		}
	}
	if len(r.pageKeys[0]) != 3 {
		t.Errorf("pageKeys[0] size = %d, want 3", len(r.pageKeys[0]))
	}
}

// TestEvictOldestGlyphEmptyCache guards the no-entries edge: eviction on an
// empty cache must be a no-op, not a panic or a bogus delete.
func TestEvictOldestGlyphEmptyCache(t *testing.T) {
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.evictOldestGlyph()

	if len(r.cache) != 0 || len(r.pageKeys) != 0 {
		t.Errorf("eviction on empty cache mutated state: cache=%d pageKeys=%d",
			len(r.cache), len(r.pageKeys))
	}
}

// TestEvictOldestGlyphSampled verifies the approximate-LRU bound with more
// entries than the sample size: exactly one entry is evicted, and — because
// the evicted entry is the minimum of an evictSampleSize-sized sample of
// distinct ages 1..n — its age can never be among the (sampleSize-1) newest.
func TestEvictOldestGlyphSampled(t *testing.T) {
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	const n = 100
	for i := uint64(1); i <= n; i++ {
		seedCacheEntry(r, i, 0, i) // key i has age i
	}

	r.evictOldestGlyph()

	if len(r.cache) != n-1 {
		t.Fatalf("cache size = %d after eviction, want %d", len(r.cache), n-1)
	}
	var evictedAge uint64
	for i := uint64(1); i <= n; i++ {
		if _, ok := r.cache[i]; !ok {
			evictedAge = i // age == key
			break
		}
	}
	if evictedAge == 0 {
		t.Fatal("no entry was evicted")
	}
	// Min of evictSampleSize distinct values drawn from 1..n is at most
	// n - evictSampleSize + 1.
	if maxAge := uint64(n - evictSampleSize + 1); evictedAge > maxAge {
		t.Errorf("evicted age %d exceeds sampled-LRU bound %d",
			evictedAge, maxAge)
	}
	if len(r.pageKeys[0]) != n-1 {
		t.Errorf("pageKeys[0] size = %d, want %d", len(r.pageKeys[0]), n-1)
	}
}

// TestGlyphCacheCapRespected drives getOrLoadGlyph past the configured cap
// with distinct by-id keys (Ascent varies targetH, a key input, while the
// rendered bitmap stays small so no atlas page reset interferes) and checks
// the cap holds and cache/pageKeys stay consistent.
func TestGlyphCacheCapRespected(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")

	const maxEntries = 256 // minimum allowed by NewRendererWithConfig
	r, err := NewRendererWithConfig(newMockBackend(), 1.0, 1024, 1024,
		RendererConfig{MaxGlyphCacheEntries: maxEntries})
	if err != nil {
		t.Fatalf("NewRendererWithConfig: %v", err)
	}
	defer r.Free()

	g := Glyph{GlyphID: gid, Index: 0, Codepoint: 1}
	for ascent := 1; ascent <= maxEntries+50; ascent++ {
		item := Item{
			Style:    TextStyle{FontName: "Sans 8"},
			FontPath: path,
			Ascent:   float64(ascent),
		}
		r.getOrLoadGlyph("H", item, g, 0, 0)
		r.atlas.FrameCounter++ // distinct ages for LRU ordering
	}

	if len(r.cache) > maxEntries {
		t.Errorf("cache size = %d exceeds cap %d", len(r.cache), maxEntries)
	}
	if len(r.cache) == 0 {
		t.Fatal("cache unexpectedly empty; glyph loads all failed")
	}
	total := 0
	for _, keys := range r.pageKeys {
		total += len(keys)
	}
	if total != len(r.cache) {
		t.Errorf("pageKeys holds %d keys, cache holds %d; eviction desynced them",
			total, len(r.cache))
	}
}

// TestGlyphCacheCapRespectedOnFailedLoads guards the negative-cache path: a
// stream of distinct un-rasterizable glyphs (here: a 300px glyph on a 64x64
// atlas, which InsertBitmap rejects) must evict like successful loads do, not
// grow the cache without bound. Also checks the negative entry itself: a
// repeat call hits the Page -1 entry, returns a zero CachedGlyph, and does
// not re-insert.
func TestGlyphCacheCapRespectedOnFailedLoads(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")

	const maxEntries = 256 // minimum allowed by NewRendererWithConfig
	r, err := NewRendererWithConfig(newMockBackend(), 1.0, 64, 64,
		RendererConfig{MaxGlyphCacheEntries: maxEntries})
	if err != nil {
		t.Fatalf("NewRendererWithConfig: %v", err)
	}
	defer r.Free()

	// Fill to the cap with synthetic entries so the very first failed load
	// must evict.
	for i := range maxEntries {
		seedCacheEntry(r, uint64(i)+1, 0, uint64(i))
	}

	g := Glyph{GlyphID: gid, Index: 0, Codepoint: 1}
	failing := func(ascent int) Item {
		return Item{
			Style:    TextStyle{FontName: "Sans 300"}, // too big for 64x64
			FontPath: path,
			Ascent:   float64(ascent),
		}
	}

	for ascent := 1; ascent <= 50; ascent++ {
		cg := r.getOrLoadGlyph("H", failing(ascent), g, 0, 0)
		if cg != (CachedGlyph{}) {
			t.Fatalf("ascent %d: failed load returned %+v, want zero value",
				ascent, cg)
		}
	}
	if len(r.cache) > maxEntries {
		t.Errorf("cache size = %d exceeds cap %d after failed loads",
			len(r.cache), maxEntries)
	}
	// Prove the negative path ran: at least one Page -1 entry must exist
	// (otherwise the loads succeeded and this test exercised nothing).
	negatives := 0
	for _, e := range r.cache {
		if e.Page < 0 {
			negatives++
		}
	}
	if negatives == 0 {
		t.Fatal("no Page -1 entries cached; loads did not fail as intended")
	}

	// Repeat call must hit the negative entry: zero value out, no growth.
	before := len(r.cache)
	if cg := r.getOrLoadGlyph("H", failing(1), g, 0, 0); cg != (CachedGlyph{}) {
		t.Errorf("negative-cache hit returned %+v, want zero value", cg)
	}
	if len(r.cache) != before {
		t.Errorf("negative-cache hit changed cache size %d -> %d",
			before, len(r.cache))
	}
}

// TestGlyphCacheAgeRefreshQuantized verifies the lazy age refresh on the hit
// path: within glyphAgeRefreshFrames the stored age must NOT move (hits stay
// a single map lookup), and once the entry is at least that stale a hit must
// store the current frame back — otherwise perpetually hot glyphs would look
// idle and sampled eviction would target them.
func TestGlyphCacheAgeRefreshQuantized(t *testing.T) {
	path, _, gid := resolveTestGlyph(t, "H")

	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	item := Item{
		Style:    TextStyle{FontName: "Sans 8"},
		FontPath: path,
		Ascent:   10,
	}
	g := Glyph{GlyphID: gid, Index: 0, Codepoint: 1}

	r.getOrLoadGlyph("H", item, g, 0, 0)
	if len(r.cache) != 1 {
		t.Fatalf("expected exactly 1 cache entry, got %d", len(r.cache))
	}
	entryAge := func() uint64 {
		for _, e := range r.cache {
			return e.age
		}
		t.Fatal("cache entry vanished")
		return 0
	}
	insertAge := entryAge()

	// A hit inside the refresh window must not touch the stored age.
	r.atlas.FrameCounter = insertAge + glyphAgeRefreshFrames - 1
	r.getOrLoadGlyph("H", item, g, 0, 0)
	if got := entryAge(); got != insertAge {
		t.Errorf("age refreshed too early: %d -> %d (within window)",
			insertAge, got)
	}

	// A hit at/after the staleness threshold must refresh to the current
	// frame.
	r.atlas.FrameCounter = insertAge + glyphAgeRefreshFrames
	r.getOrLoadGlyph("H", item, g, 0, 0)
	if got := entryAge(); got != r.atlas.FrameCounter {
		t.Errorf("age not refreshed at threshold: got %d, want %d",
			got, r.atlas.FrameCounter)
	}
}

// BenchmarkDrawLayoutPlacedCached measures the steady-state (fully cached)
// per-frame draw cost — the hot path issue #70 targets: one map operation per
// cache hit instead of two.
func BenchmarkDrawLayoutPlacedCached(b *testing.B) {
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText(
		"The quick brown fox jumps over the lazy dog", TextConfig{})
	if err != nil {
		b.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		b.Skip("no font path resolved on this host; skipping")
	}

	positions := layout.GlyphPositions()
	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for _, p := range positions {
		placements[p.Index] = GlyphPlacement{X: p.X, Y: p.Y}
	}

	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		b.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	// Warm the cache so the loop measures pure hit-path cost.
	r.DrawLayoutPlaced(layout, placements)
	r.Commit()

	b.ResetTimer()
	for b.Loop() {
		r.DrawLayoutPlaced(layout, placements)
		r.Commit()
	}
}
