package glyph

// decorationMetrics holds underline and strikethrough geometry in the
// convention of the Item fields of the same names:
//
//   - ulOffset is the distance from the baseline down to the bottom of
//     the underline, so its top is at baseline + ulOffset - ulThick.
//   - stOffset is the distance from the baseline up to the top of the
//     strikethrough plus its thickness, so its top is at
//     baseline - stOffset + stThick.
//
// These odd-looking conventions are the ones the renderers (and any host
// that draws the lines itself from Item) already read.
type decorationMetrics struct {
	ulOffset, ulThick float64
	stOffset, stThick float64
}

// Em-based fallbacks for fonts without underline (post) or strikeout
// (OS/2) metrics. They are close to what common text faces declare.
const (
	fallbackUnderlinePosEm  = -0.1 // top of the underline, above baseline
	fallbackStrikePosEm     = 0.3  // top of the strikethrough, above baseline
	fallbackDecorationThkEm = 0.05 // line thickness
)

// newDecorationMetrics converts font-style metrics into Item fields. ulPos
// and stPos are the distance above the baseline of the top of each line
// (negative is below), as in the post and OS/2 tables; ulThick and
// stThick are line thicknesses. All values, em and minThick share one
// unit. A metric with a thickness that is not positive and finite is
// treated as missing and replaced by the em-based fallback. Thickness is
// raised to minThick (one device pixel) with the top edge kept in place,
// so a tiny run still shows its line.
func newDecorationMetrics(ulPos, ulThick, stPos, stThick, em,
	minThick float64) decorationMetrics {

	if !(ulThick > 0) || !finiteF64(ulThick) || !finiteF64(ulPos) {
		ulPos = fallbackUnderlinePosEm * em
		ulThick = fallbackDecorationThkEm * em
	}
	if !(stThick > 0) || !finiteF64(stThick) || !finiteF64(stPos) {
		stPos = fallbackStrikePosEm * em
		stThick = fallbackDecorationThkEm * em
	}
	ulThick = max(ulThick, minThick)
	stThick = max(stThick, minThick)
	return decorationMetrics{
		ulOffset: -ulPos + ulThick,
		ulThick:  ulThick,
		stOffset: stPos + stThick,
		stThick:  stThick,
	}
}

func finiteF64(v float64) bool {
	return v == v && v <= 1e300 && v >= -1e300
}
