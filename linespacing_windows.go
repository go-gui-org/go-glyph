//go:build windows

package glyph

// preferTypoLineBox builds the line box from OS/2 typo metrics for fonts that
// do not set USE_TYPO_METRICS, instead of hhea. Windows only: Segoe UI, the
// default UI font there, has an hhea box of 1.33 em (issue #195).
const preferTypoLineBox = true
