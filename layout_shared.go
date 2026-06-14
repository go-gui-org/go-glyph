package glyph

import "cmp"

// parseSizeFromStyle returns the effective font size from a TextStyle.
func parseSizeFromStyle(s TextStyle) float32 {
	if s.Size > 0 {
		return s.Size
	}
	sz := parseSizeFromFontName(s.FontName)
	if sz > 0 {
		return sz
	}
	return 16
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
	return result
}
