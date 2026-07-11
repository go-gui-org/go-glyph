package glyph

// metricsCache is an empty metrics cache used for struct parity.
type metricsCache struct{}

func newMetricsCache(capacity int) metricsCache {
	return metricsCache{}
}
