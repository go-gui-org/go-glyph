package glyph

// metricsCache is an empty metrics cache used for struct parity.
type metricsCache struct{}

func newMetricsCache(capacity int) metricsCache {
	return metricsCache{}
}

// cacheEntry is the glyph-cache value: the public CachedGlyph plus its
// last-touched frame. Folding age into the value keeps one map instead of a
// parallel cache/cacheAges pair — one hash per hit instead of two, and no
// duplicated key set (~48+ bytes/entry saved at the default 4096-entry cap).
// Untagged file so both the FreeType and WASM Renderer structs share it.
type cacheEntry struct {
	CachedGlyph
	age uint64
	// slot is the entry's index in its page's pageKeys list, so removing
	// one key is O(1) instead of a scan of every key on the page. Unused
	// for entries with no page (Page < 0) and on WASM.
	slot int
}
