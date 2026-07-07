//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

// FontMetricsEntry stores cached font metrics in Pango units.
type FontMetricsEntry struct {
	Ascent  int // Pango units.
	Descent int // Pango units.
	LineGap int // Pango units (0 if not available).
}

type metricsCacheEntry struct {
	key   uint64
	value FontMetricsEntry
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
