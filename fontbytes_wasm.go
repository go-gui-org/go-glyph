//go:build js

package glyph

// writeTempFontFile is a no-op under wasm: the browser has no
// filesystem. Load fonts via the FontFace API before creating the
// TextSystem. Returning an empty path signals AddFontBytes to skip
// registration.
func writeTempFontFile(_ []byte) (string, error) { return "", nil }
