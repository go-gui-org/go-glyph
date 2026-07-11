//go:build android || linux || darwin

package glyph

// LoadGlyphConfig holds parameters for Android glyph rasterization.
type LoadGlyphConfig struct {
	Index        uint32
	Codepoint    uint32
	TargetHeight int
	SubpixelBin  int
}

// LoadGlyphResult holds the output of a glyph load operation.
type LoadGlyphResult struct {
	Cached        CachedGlyph
	ResetOccurred bool
	ResetPage     int
}
