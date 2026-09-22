package glyph

import (
	"math"
	"strconv"
	"strings"
)

// This file holds the Pango font-name helpers that every build shares.
// The native and WASM builds had diverged copies: native truncated
// "Sans 12.5" to 12, WASM kept 12.5, and WASM also took "12px" as a size
// because fmt.Sscanf accepts a numeric prefix.

// sanitizeScale returns a usable DPI scale factor. It maps zero,
// negative, NaN, and infinite values to 1. A NaN scale would make every
// metric NaN, and an infinite scale gives an inverse scale of 0.
func sanitizeScale(scale float32) float32 {
	if !(scale > 0) || math.IsInf(float64(scale), 0) {
		return 1
	}
	return scale
}

// parseFontSize parses a font-name token as a point size. It accepts
// only a finite positive number that fills the whole token ("12",
// "10.5"), so a family word such as "12px" or "Inf" is not taken as a
// size. It returns 0 when the token is not a size. strconv.ParseFloat
// does not allocate, unlike fmt.Sscanf.
func parseFontSize(tok string) float32 {
	if tok == "" || (tok[0] < '0' || tok[0] > '9') && tok[0] != '.' {
		// Fast reject, and it also rejects "Inf", "NaN", signs, and hex
		// forms, which ParseFloat would accept.
		return 0
	}
	v, err := strconv.ParseFloat(tok, 32)
	if err != nil || !(v > 0) || math.IsInf(v, 0) {
		return 0
	}
	return float32(v)
}

// parseSizeFromFontName extracts the trailing numeric size from a Pango
// font name like "Sans Bold 18". Returns 0 when there is no size.
func parseSizeFromFontName(name string) float32 {
	return parseFontSize(lastField(name))
}

// lastField returns the last whitespace-separated token of s, without
// allocating the token slice that strings.Fields builds.
func lastField(s string) string {
	s = strings.TrimRight(s, " \t\n\r\v\f")
	i := strings.LastIndexAny(s, " \t\n\r\v\f")
	return s[i+1:]
}

// fontStyleWords are the Pango style and weight keywords that
// parseFamilyFromFontName strips from the end of a font name. It is a
// package variable so the map is built once, not on every call.
var fontStyleWords = map[string]bool{
	"bold": true, "italic": true, "oblique": true,
	"light": true, "medium": true, "semibold": true,
	"heavy": true, "ultrabold": true, "ultralight": true,
	"condensed": true, "expanded": true, "regular": true,
}

// parseFamilyFromFontName extracts the family portion from a Pango font
// name, stripping the trailing size and style keywords. At least one word
// is kept, so "Bold" alone stays "Bold".
func parseFamilyFromFontName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	end := len(parts)
	if parseFontSize(parts[end-1]) > 0 {
		end--
	}
	for end > 0 && fontStyleWords[strings.ToLower(parts[end-1])] {
		end--
	}
	if end == 0 {
		end = 1
	}
	return strings.Join(parts[:end], " ")
}
