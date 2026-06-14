//go:build !android && !windows && !(darwin && !glyph_pango) && !(js && wasm)

package glyph

// parseSizeFromFontName is a stub for platforms that don't encode font size
// in font names (FreeType/Pango on Linux, etc.). On these platforms the
// TextStyle.Size field is always set explicitly, so this fallback is never
// reached. Required by layout_shared.go.
func parseSizeFromFontName(name string) float32 { return 0 }
