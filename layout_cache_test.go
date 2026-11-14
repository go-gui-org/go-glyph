package glyph

import (
	"testing"
	"time"
)

const benchCacheSize = 8192

func newCacheTestSystem(tb testing.TB) *TextSystem {
	tb.Helper()
	ts, err := NewTextSystem(newRecordingBackend())
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(ts.Free)
	return ts
}

// BenchmarkCommitFullCache measures the per-frame cost of Commit when the layout cache
// is full of fresh entries. Nothing expires, so every call scans the whole map.
func BenchmarkCommitFullCache(b *testing.B) {
	ts := newCacheTestSystem(b)
	now := time.Now().UnixMilli()
	for i := 0; i < benchCacheSize; i++ {
		ts.cache[uint64(i)] = &cachedLayout{lastAccess: now}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.Commit()
	}
}

// BenchmarkEvictOldestLayouts measures one eviction from a full cache. The refill reuses
// pre-built entries so its allocations stay out of the result.
func BenchmarkEvictOldestLayouts(b *testing.B) {
	ts := newCacheTestSystem(b)
	entries := make([]*cachedLayout, benchCacheSize)
	for i := range entries {
		entries[i] = &cachedLayout{}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j, e := range entries {
			e.lastAccess = int64(j + 1)
			ts.cache[uint64(j)] = e
		}
		b.StartTimer()
		ts.evictOldestLayouts()
	}
}

func TestLayoutCacheExpiresStaleEntries(t *testing.T) {
	ts := newCacheTestSystem(t)
	now := time.Now().UnixMilli()
	ts.cache[1] = &cachedLayout{lastAccess: now - ts.evictionAge - 1}
	ts.cache[2] = &cachedLayout{lastAccess: now}
	ts.lastPrune = 0 // Make sure the throttle lets the scan run.
	ts.pruneCache()
	if _, ok := ts.cache[1]; ok {
		t.Error("stale entry survived")
	}
	if _, ok := ts.cache[2]; !ok {
		t.Error("fresh entry was removed")
	}
}

func TestLayoutCacheEvictsOldestQuarter(t *testing.T) {
	ts := newCacheTestSystem(t)
	for i := 1; i <= 100; i++ {
		ts.cache[uint64(i)] = &cachedLayout{lastAccess: int64(i)}
	}
	ts.evictOldestLayouts()
	if len(ts.cache) != 75 {
		t.Fatalf("len = %d, want 75", len(ts.cache))
	}
	for k, item := range ts.cache {
		if item.lastAccess <= 25 {
			t.Errorf("entry %d (lastAccess %d) should be evicted", k, item.lastAccess)
		}
	}
}

func TestLayoutCachePruneThrottled(t *testing.T) {
	ts := newCacheTestSystem(t)
	now := time.Now().UnixMilli()
	// A fresh entry makes the first call scan, which sets lastPrune.
	ts.cache[1] = &cachedLayout{lastAccess: now}
	ts.lastPrune = 0
	ts.pruneCache()
	if ts.lastPrune == 0 {
		t.Fatal("first prune did not scan")
	}

	ts.cache[2] = &cachedLayout{lastAccess: now - ts.evictionAge - 1}
	ts.pruneCache()
	if _, ok := ts.cache[2]; !ok {
		t.Error("throttled prune removed the stale entry")
	}

	ts.lastPrune -= ts.evictionAge
	ts.pruneCache()
	if _, ok := ts.cache[2]; ok {
		t.Error("stale entry survived after the throttle interval")
	}
}
