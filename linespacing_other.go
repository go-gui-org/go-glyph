//go:build linux || darwin

package glyph

// preferTypoLineBox is off outside Windows: line boxes come from hhea unless
// the font sets USE_TYPO_METRICS (see linespacing_windows.go).
const preferTypoLineBox = false
