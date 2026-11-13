//go:build android || linux || darwin || windows || (js && wasm)

package glyph

import "cmp"

// parseSizeFromStyle returns the effective font size from a TextStyle. It is
// the same resolver as resolveFontSize, kept under this name for the layout
// call sites. One resolver means every backend clamps the same way.
func parseSizeFromStyle(s TextStyle) float32 {
	return resolveFontSize(s)
}

// mergeStyles merges run style on top of base style.
func mergeStyles(base, run TextStyle) TextStyle {
	result := run
	result.FontName = cmp.Or(result.FontName, base.FontName)
	if result.Size <= 0 {
		result.Size = base.Size
	}
	if result.Color.A == 0 {
		result.Color = base.Color
	}
	// Grid geometry falls back to the base style so a grid caller declares the
	// cell once on cfg.Style instead of repeating it on every run (#105).
	if result.CellWidth <= 0 {
		result.CellWidth = base.CellWidth
	}
	if result.CellHeight <= 0 {
		result.CellHeight = base.CellHeight
	}
	if result.EmojiBoxWidth <= 0 {
		result.EmojiBoxWidth = base.EmojiBoxWidth
	}
	// The bool has no unset sentinel, so the opt-out is sticky: a base opt-out
	// applies to every run and a run cannot opt back in.
	result.NoBuiltinBoxGlyphs = result.NoBuiltinBoxGlyphs || base.NoBuiltinBoxGlyphs
	return result
}
