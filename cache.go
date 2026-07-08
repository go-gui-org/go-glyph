//go:build !darwin

package glyph

// metricsCache is an empty metrics cache used for struct parity
// on non-Darwin platforms. Darwin uses ctMetricsCache instead.
type metricsCache struct{}

func newMetricsCache(capacity int) metricsCache {
	return metricsCache{}
}
