//go:build darwin && !glyph_pango

package glyph

import "testing"

func TestCTMetricsCache_ZeroCapacityDefaultsTo32(t *testing.T) {
	c := newCTMetricsCache(0)
	if c.capacity != 32 {
		t.Errorf("capacity = %d, want 32", c.capacity)
	}
	k := ctMetricsKey{family: "Sans", size: 16}
	m := ctFontMetrics{ascent: 10, descent: 3, leading: 1}
	c.put(k, m)
	got, ok := c.get(k)
	if !ok || got != m {
		t.Errorf("get after put = %v, %v; want %v, true", got, ok, m)
	}
}

func TestCTMetricsCache_NegativeCapacityDefaultsTo32(t *testing.T) {
	c := newCTMetricsCache(-5)
	if c.capacity != 32 {
		t.Errorf("capacity = %d, want 32", c.capacity)
	}
}

func TestCTMetricsCache_EvictsAtCapacity(t *testing.T) {
	const cap = 2
	c := newCTMetricsCache(cap)
	k1 := ctMetricsKey{family: "A", size: 10}
	k2 := ctMetricsKey{family: "B", size: 12}
	k3 := ctMetricsKey{family: "C", size: 14}
	m := ctFontMetrics{ascent: 1}

	c.put(k1, m)
	c.put(k2, m)
	c.put(k3, m) // triggers eviction

	if len(c.entries) != cap {
		t.Errorf("len(entries) = %d after eviction, want %d", len(c.entries), cap)
	}
	if _, ok := c.get(k3); !ok {
		t.Error("newly inserted entry must be present after eviction")
	}
}

func TestCTMetricsCache_GetMiss(t *testing.T) {
	c := newCTMetricsCache(4)
	k := ctMetricsKey{family: "Missing", size: 12}
	got, ok := c.get(k)
	if ok || got != (ctFontMetrics{}) {
		t.Errorf("get miss = %v, %v; want zero, false", got, ok)
	}
}

func TestCTMetricsCache_NilEntriesNoOp(t *testing.T) {
	// Zero-value assigned by ctx.Free(); must not panic.
	var c ctMetricsCache
	k := ctMetricsKey{family: "Sans", size: 16}
	m := ctFontMetrics{ascent: 1}

	got, ok := c.get(k)
	if ok || got != (ctFontMetrics{}) {
		t.Errorf("get on nil entries = %v, %v; want zero, false", got, ok)
	}
	c.put(k, m) // must silently no-op, not panic
}
