package glyph

import "testing"

// fakeFace returns a cachedFace with no parsed font; the LRU only inspects
// size and identity, so a bare struct is enough.
func fakeFace() *cachedFace { return &cachedFace{} }

// TestFaceLRUCapacity bounds the entry count even when the byte budget
// would allow more (many small fonts must not accumulate without limit).
func TestFaceLRUCapacity(t *testing.T) {
	lru := newFaceLRU(2, 1<<30)
	lru.add("a", fakeFace(), 10)
	lru.add("b", fakeFace(), 10)
	lru.add("c", fakeFace(), 10)
	if got := lru.ll.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2 (capacity bound)", got)
	}
	// Oldest (a) evicted; b, c retained.
	for _, path := range []string{"b", "c"} {
		if _, ok := lru.items[path]; !ok {
			t.Errorf("%s evicted despite capacity being the binding constraint", path)
		}
	}
	if _, ok := lru.items["a"]; ok {
		t.Error("a not evicted (should be the least recently used)")
	}
	if lru.used != 20 {
		t.Errorf("used = %d, want 20 (sizes of surviving entries)", lru.used)
	}
}

// TestFaceLRUBudgetEvictsOldest verifies the byte budget evicts the
// least-recently-used entry when the sum of resident sizes exceeds it.
func TestFaceLRUBudgetEvictsOldest(t *testing.T) {
	lru := newFaceLRU(16, 100)
	lru.add("a", fakeFace(), 60)
	lru.add("b", fakeFace(), 60)
	if lru.used != 60 || lru.ll.Len() != 1 {
		t.Fatalf("after over-budget add: used=%d Len=%d, want 60/1 (a evicted)", lru.used, lru.ll.Len())
	}
	if _, ok := lru.items["a"]; ok {
		t.Error("a retained despite pushing the cache over budget")
	}
	// b is the freshest entry and must survive its own size check.
	if _, ok := lru.items["b"]; !ok {
		t.Error("b evicted; the freshest entry must never be removed by its own size")
	}
}

// TestFaceLRUOversizedEntryStays keeps a single face bigger than the whole
// budget resident: evicting it would re-parse on every glyph. Once a second
// face arrives, though, the giant is the oldest entry and goes first.
func TestFaceLRUOversizedEntryStays(t *testing.T) {
	lru := newFaceLRU(4, 100)
	lru.add("huge", fakeFace(), 1000)
	if lru.ll.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (oversized entry must stay)", lru.ll.Len())
	}
	if lru.used != 1000 {
		t.Errorf("used = %d, want 1000 (overshoot recorded; entry is sole resident)", lru.used)
	}
	// A second face pushes the budget over; the giant is now the least
	// recently used, so it is the one evicted.
	lru.add("small", fakeFace(), 1)
	if _, ok := lru.items["huge"]; ok {
		t.Error("huge retained; it is the least recently used and over budget")
	}
	if lru.used != 1 || lru.ll.Len() != 1 {
		t.Errorf("used=%d Len=%d, want 1/1 after eviction", lru.used, lru.ll.Len())
	}
	// Re-add the giant: it is freshest, so the older small face is evicted
	// instead — the newest entry is never sacrificed by its own size.
	lru.add("huge", fakeFace(), 1000)
	if _, ok := lru.items["small"]; ok {
		t.Error("small retained; the freshest entry evicts older ones to make room")
	}
	if lru.used != 1000 || lru.ll.Len() != 1 {
		t.Errorf("used=%d Len=%d, want 1000/1 (giant sole resident)", lru.used, lru.ll.Len())
	}
	// Once the giant is no longer freshest, normal LRU order applies: the
	// next arrival evicts it and the budget returns to the newcomer.
	lru.add("small2", fakeFace(), 1)
	if _, ok := lru.items["huge"]; ok {
		t.Error("huge retained after becoming the least recently used entry")
	}
	if lru.used != 1 || lru.ll.Len() != 1 {
		t.Errorf("used=%d Len=%d, want 1/1 after eviction", lru.used, lru.ll.Len())
	}
}

// TestFaceLRUNegativeCacheFree verifies nil (negative) entries carry no
// size, so bad paths never push the budget.
func TestFaceLRUNegativeCacheFree(t *testing.T) {
	lru := newFaceLRU(16, 10)
	for _, p := range []string{"bad1", "bad2", "bad3", "bad4"} {
		lru.add(p, nil, 0)
	}
	if lru.used != 0 {
		t.Errorf("used = %d, want 0 (negative entries cost no budget)", lru.used)
	}
	if lru.ll.Len() != 4 {
		t.Errorf("Len = %d, want 4 (negative entries retained under budget)", lru.ll.Len())
	}
}

// TestFaceLRUReAddNoDoubleCount verifies a repeated add of the same path
// (concurrent parse race) refreshes recency without double-counting size.
func TestFaceLRUReAddNoDoubleCount(t *testing.T) {
	lru := newFaceLRU(4, 90)
	lru.add("a", fakeFace(), 50)
	lru.add("b", fakeFace(), 40)
	lru.add("a", fakeFace(), 50) // concurrent duplicate parse wins nothing
	if lru.used != 90 {
		t.Errorf("used = %d, want 90 (no double count)", lru.used)
	}
	if lru.ll.Len() != 2 {
		t.Errorf("Len = %d, want 2", lru.ll.Len())
	}
	// a must now be freshest: adding c (1) pushes the budget to 91, so the
	// oldest entry (b) is evicted, not a.
	lru.add("c", fakeFace(), 1)
	if _, ok := lru.items["a"]; !ok {
		t.Error("a evicted despite being most recently used")
	}
	if _, ok := lru.items["b"]; ok {
		t.Error("b retained; it should be least recently used")
	}
	if lru.used != 51 {
		t.Errorf("used = %d, want 51 (a+c)", lru.used)
	}
}

// TestFaceLRUBudgetZeroNoOp confirms budget 0 disables the byte bound
// (legacy behavior for callers that opt out).
func TestFaceLRUBudgetZeroNoOp(t *testing.T) {
	lru := newFaceLRU(8, 0)
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		lru.add(p, fakeFace(), 1<<30)
	}
	if lru.ll.Len() != 5 {
		t.Errorf("Len = %d, want 5 (budget disabled)", lru.ll.Len())
	}
}
