//go:build !darwin || glyph_pango

package glyph

import "container/list"

// metricsCache is an LRU cache for font metrics keyed by
// (face pointer XOR size) tuple. Not safe for concurrent use.
// get/put live in cache_pango.go (the only consumer); other
// platform contexts still construct an empty cache for parity.
type metricsCache struct {
	entries  map[uint64]*list.Element
	order    *list.List // Front = oldest, Back = newest.
	capacity int
}

func newMetricsCache(capacity int) metricsCache {
	return metricsCache{
		entries:  make(map[uint64]*list.Element, capacity),
		order:    list.New(),
		capacity: capacity,
	}
}
