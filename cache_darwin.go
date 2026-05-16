//go:build darwin && !glyph_pango

package glyph

// ctMetricsKey identifies a unique (family, size, traits) tuple. Keyed
// on resolved family + scaled size + style flags so two newCTFont
// calls with identical params share a cache entry even when CoreText
// hands back different CTFontRefs.
type ctMetricsKey struct {
	family string
	size   float64
	bold   bool
	italic bool
}

// ctFontMetrics is the cached payload — ascent, descent, leading in
// raw Core Text units (before scaleFactor division).
type ctFontMetrics struct {
	ascent  float64
	descent float64
	leading float64
}

// ctMetricsCache memoizes ctFont.metrics() results. Bounded with a
// drop-one-on-overflow policy: typical sessions touch a handful of
// (family, size) tuples; the cap exists to bound memory in
// pathological cases, not to enforce strict LRU.
type ctMetricsCache struct {
	entries  map[ctMetricsKey]ctFontMetrics
	capacity int
}

func newCTMetricsCache(capacity int) ctMetricsCache {
	if capacity <= 0 {
		capacity = 32
	}
	return ctMetricsCache{
		entries:  make(map[ctMetricsKey]ctFontMetrics, capacity),
		capacity: capacity,
	}
}

func (c *ctMetricsCache) get(k ctMetricsKey) (ctFontMetrics, bool) {
	if c.entries == nil {
		return ctFontMetrics{}, false
	}
	m, ok := c.entries[k]
	return m, ok
}

func (c *ctMetricsCache) put(k ctMetricsKey, m ctFontMetrics) {
	if c.entries == nil {
		return
	}
	if c.capacity > 0 && len(c.entries) >= c.capacity {
		for evict := range c.entries {
			delete(c.entries, evict)
			break
		}
	}
	c.entries[k] = m
}
