package glyph

// minLineHeightRatio floors the recommended baseline-to-baseline advance to
// this multiple of the em when a font's own metrics come out cramped (tight
// ascent/descent, or a zero line-gap hint). 1.15 approximates a tighter
// CSS "normal" line height for Latin text without looking loose.
const minLineHeightRatio = 1.15

// recommendedLineHeight returns the baseline-to-baseline advance for a font:
// ascent+descent+leading (the font's own hint), floored to
// minLineHeightRatio×em so fonts lacking a line-gap hint — or falling back to
// synthesized metrics — do not render with no discernible line spacing.
//
// All arguments and the result are in the same unit (physical or logical
// pixels); em is the font size in that unit.
func recommendedLineHeight(ascent, descent, leading, em float64) float64 {
	h := ascent + descent + leading
	if lo := minLineHeightRatio * em; h < lo {
		h = lo
	}
	return h
}
