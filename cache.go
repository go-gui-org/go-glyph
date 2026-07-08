//go:build !darwin

package glyph

// metricsCache is an empty metrics cache used for struct parity
// on non-Darwin platforms. Darwin uses ctMetricsCache instead.
type metricsCache struct{}

func newMetricsCache(capacity int) metricsCache {
	return metricsCache{}
}

func (c *metricsCache) get(key uint64) (struct{ Ascent, Descent, LineGap int }, bool) {
	return struct{ Ascent, Descent, LineGap int }{}, false
}

func (c *metricsCache) put(key uint64, entry struct{ Ascent, Descent, LineGap int }) {}
