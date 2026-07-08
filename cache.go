//go:build !darwin

package glyph

import "container/list"

// FontMetricsEntry stores cached font metrics.
type FontMetricsEntry struct {
	Ascent  int
	Descent int
	LineGap int
}

type metricsCacheEntry struct {
	key   uint64
	value FontMetricsEntry
}

// metricsCache is an LRU cache for font metrics keyed by
// (face pointer XOR size) tuple. Not safe for concurrent use.
type metricsCache struct {
	entries  map[uint64]*list.Element
	order    *list.List
	capacity int
}

func newMetricsCache(capacity int) metricsCache {
	return metricsCache{
		entries:  make(map[uint64]*list.Element, capacity),
		order:    list.New(),
		capacity: capacity,
	}
}

func (c *metricsCache) get(key uint64) (FontMetricsEntry, bool) {
	elem, ok := c.entries[key]
	if !ok {
		return FontMetricsEntry{}, false
	}
	c.order.MoveToBack(elem)
	return elem.Value.(metricsCacheEntry).value, true
}

func (c *metricsCache) put(key uint64, entry FontMetricsEntry) {
	if elem, exists := c.entries[key]; exists {
		elem.Value = metricsCacheEntry{key: key, value: entry}
		c.order.MoveToBack(elem)
		return
	}
	if len(c.entries) >= c.capacity {
		front := c.order.Front()
		if front != nil {
			evictKey := front.Value.(metricsCacheEntry).key
			delete(c.entries, evictKey)
			c.order.Remove(front)
		}
	}
	elem := c.order.PushBack(metricsCacheEntry{key: key, value: entry})
	c.entries[key] = elem
}
