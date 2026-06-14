//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

import (
	"testing"
)

func TestMetricsCache_GetHit(t *testing.T) {
	c := newMetricsCache(10)
	entry := FontMetricsEntry{Ascent: 100, Descent: 20, LineGap: 5}
	c.put(42, entry)

	got, ok := c.get(42)
	if !ok {
		t.Fatal("expected hit for key 42")
	}
	if got.Ascent != 100 || got.Descent != 20 || got.LineGap != 5 {
		t.Errorf("entry mismatch: got %+v, want %+v", got, entry)
	}
}

func TestMetricsCache_GetMiss(t *testing.T) {
	c := newMetricsCache(10)
	got, ok := c.get(99)
	if ok {
		t.Error("expected miss for unknown key")
	}
	if got != (FontMetricsEntry{}) {
		t.Errorf("expected zero value on miss, got %+v", got)
	}
}

func TestMetricsCache_EvictionOnOverflow(t *testing.T) {
	const cap = 4
	c := newMetricsCache(cap)
	for i := uint64(0); i < cap+1; i++ {
		c.put(i, FontMetricsEntry{Ascent: int(i)})
	}
	if len(c.entries) != cap {
		t.Errorf("entries = %d, want %d", len(c.entries), cap)
	}
	if _, ok := c.get(0); ok {
		t.Error("oldest key 0 should have been evicted")
	}
	if _, ok := c.get(cap); !ok {
		t.Error("newest key should be present after overflow")
	}
}

func TestMetricsCache_GetRefreshesLRU(t *testing.T) {
	const cap = 3
	c := newMetricsCache(cap)
	c.put(0, FontMetricsEntry{Ascent: 0})
	c.put(1, FontMetricsEntry{Ascent: 1})
	c.put(2, FontMetricsEntry{Ascent: 2})
	// Access key 0 to refresh it — now key 1 is oldest.
	c.get(0)
	// Insert key 3 — evicts key 1 (now the oldest).
	c.put(3, FontMetricsEntry{Ascent: 3})

	if _, ok := c.get(0); !ok {
		t.Error("key 0 should survive after get-refresh")
	}
	if _, ok := c.get(1); ok {
		t.Error("key 1 should have been evicted")
	}
	if _, ok := c.get(2); !ok {
		t.Error("key 2 should survive")
	}
}

func TestMetricsCache_PutExistingRefreshesLRU(t *testing.T) {
	const cap = 3
	c := newMetricsCache(cap)
	c.put(0, FontMetricsEntry{Ascent: 0})
	c.put(1, FontMetricsEntry{Ascent: 1})
	c.put(2, FontMetricsEntry{Ascent: 2})
	// Put existing key 0 with new value.
	c.put(0, FontMetricsEntry{Ascent: 99})
	// Insert key 3 — evicts key 1 (now the oldest, not key 0).
	c.put(3, FontMetricsEntry{Ascent: 3})

	if v, ok := c.get(0); !ok {
		t.Error("key 0 should survive after put-existing refresh")
	} else if v.Ascent != 99 {
		t.Errorf("key 0 Ascent = %d, want 99", v.Ascent)
	}
	if _, ok := c.get(1); ok {
		t.Error("key 1 should have been evicted")
	}
}

func TestMetricsCache_ZeroCapacity(t *testing.T) {
	// newMetricsCache(0) creates a valid map but capacity=0 means
	// every put triggers eviction. With an empty cache, the first
	// put succeeds (front is nil, so eviction skipped), then
	// subsequent puts evict the previous entry. Net effect:
	// capacity 0 behaves as a leaky capacity 1.
	c := newMetricsCache(0)
	c.put(1, FontMetricsEntry{Ascent: 10})
	// First put succeeds despite capacity=0 (empty list, no front to evict).
	if _, ok := c.get(1); !ok {
		t.Error("first put should succeed (nil front, no eviction)")
	}
	c.put(2, FontMetricsEntry{Ascent: 20})
	// Second put evicts key 1 (front of one-element list).
	if _, ok := c.get(1); ok {
		t.Error("key 1 should be evicted by second put")
	}
	if _, ok := c.get(2); !ok {
		t.Error("key 2 should be present")
	}
}

func TestMetricsCache_ZeroValueStruct(t *testing.T) {
	var c metricsCache // zero-value, order is nil
	// Get on nil map returns zero value and false.
	_, ok := c.get(1)
	if ok {
		t.Error("get should miss on zero-value cache")
	}
	// put on zero-value cache panics (nil order list).
	// Documented: must call newMetricsCache to construct.
}
