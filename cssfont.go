package glyph

import (
	"cmp"
	"math"
	"strconv"
	"strings"
)

// The CSS font helpers below back the WASM Canvas2D context
// (context_wasm.go). They have no build tag, so the native test suite
// covers them too: they are pure string code with no syscall/js use.

// resolveFontSize returns the font size (logical px) for a style: the
// explicit Size, else the size embedded in a Pango font name, else 16.
// NaN, infinite, zero, and negative sizes count as "not set". A NaN size
// would give the CSS string "NaNpx", which the canvas rejects without an
// error, so the previous font would stay active and measure wrongly.
func resolveFontSize(style TextStyle) float32 {
	size := style.Size
	if !(size > 0) || math.IsInf(float64(size), 0) {
		size = parseSizeFromFontName(style.FontName)
	}
	if !(size > 0) {
		size = 16
	}
	return size
}

// cssFontSize is resolveFontSize as a float64, the unit the line-height
// math uses.
func cssFontSize(style TextStyle) float64 {
	return float64(resolveFontSize(style))
}

// CSS font-weight values for the Pango weight keywords.
const (
	cssWeightNormal = 400
	cssWeightBold   = 700
)

// pangoCSSWeights maps Pango weight keywords to CSS font-weight values.
var pangoCSSWeights = map[string]int{
	"ultralight": 200,
	"light":      300,
	"regular":    cssWeightNormal,
	"medium":     500,
	"semibold":   600,
	"bold":       cssWeightBold,
	"ultrabold":  800,
	"heavy":      900,
}

// fontNameStyle reads the weight and slant keywords at the end of a Pango
// font name ("Fira Code Light 11" → 300, upright). It scans the same
// trailing words that parseFamilyFromFontName strips, so every keyword
// removed from the family is applied here and none is lost. weight is
// cssWeightNormal when the name has no weight keyword.
func fontNameStyle(name string) (weight int, italic bool) {
	weight = cssWeightNormal
	parts := strings.Fields(name)
	end := len(parts)
	if end > 0 && parseFontSize(parts[end-1]) > 0 {
		end--
	}
	// Keep at least one word, as parseFamilyFromFontName does: "Bold"
	// alone is a family name, not a keyword.
	for end > 1 {
		w := strings.ToLower(parts[end-1])
		if !fontStyleWords[w] {
			break
		}
		if v, ok := pangoCSSWeights[w]; ok {
			weight = v
		}
		if w == "italic" || w == "oblique" {
			italic = true
		}
		end--
	}
	return weight, italic
}

// buildCSSFont constructs a CSS font shorthand string from TextStyle,
// such as "italic bold 16px 'Noto Sans', sans-serif".
func buildCSSFont(style TextStyle) string {
	family := cmp.Or(parseFamilyFromFontName(style.FontName), "sans-serif")

	// The Typeface field wins. Keywords in the font name apply only for
	// TypefaceRegular, which is also the zero value.
	weight, italic := cssWeightNormal, false
	switch style.Typeface {
	case TypefaceBold:
		weight = cssWeightBold
	case TypefaceItalic:
		italic = true
	case TypefaceBoldItalic:
		weight, italic = cssWeightBold, true
	default:
		weight, italic = fontNameStyle(style.FontName)
	}

	var sb strings.Builder
	if italic {
		sb.WriteString("italic ")
	}
	switch weight {
	case cssWeightNormal:
	case cssWeightBold:
		sb.WriteString("bold ")
	default:
		sb.WriteString(strconv.Itoa(weight))
		sb.WriteByte(' ')
	}
	sb.WriteString(strconv.FormatFloat(cssFontSize(style), 'g', -1, 32))
	sb.WriteString("px ")
	sb.WriteString(mapFontFamily(family))
	return sb.String()
}

// mapFontFamily maps generic Pango families to CSS equivalents. Other
// names become a quoted CSS string with a sans-serif fallback.
func mapFontFamily(family string) string {
	switch strings.ToLower(family) {
	case "sans", "sans-serif":
		return "sans-serif"
	case "serif":
		return "serif"
	case "monospace", "mono":
		return "monospace"
	default:
		return cssQuote(family) + ", sans-serif"
	}
}

// cssQuote returns s as a single-quoted CSS string. A backslash and a
// quote are escaped, and control characters (a newline ends a CSS string)
// are dropped. Only removing the quote was not enough: a trailing
// backslash then escaped the closing quote, the font string became
// invalid, and the canvas kept the previous font without an error.
func cssQuote(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + 2)
	sb.WriteByte('\'')
	for _, r := range s {
		switch {
		case r == '\\' || r == '\'':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			// Drop control characters.
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('\'')
	return sb.String()
}
