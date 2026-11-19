package glyph

import "math"

// coverageCurve selects how raw glyph coverage is remapped before it goes
// into the atlas (issue #189).
//
// The backends blend glyph alpha in sRGB space, not in linear light. For
// light text on a dark background, sRGB blending makes the partial-coverage
// edge pixels too dark, so the text looks thin, dim and soft. For dark text
// on a light background, the error goes the other way and is small. One
// fixed curve cannot correct both cases, so the curve is chosen by the
// luminance of the text color. The choice is part of the glyph cache key,
// because one atlas raster cannot serve two curves.
type coverageCurve uint8

const (
	// curveNone keeps the raw coverage. Used for dark text, color glyphs and
	// emoji.
	curveNone coverageCurve = iota
	// curveMid raises partial coverage a little, for mid-luminance text.
	curveMid
	// curveLight raises partial coverage the most, for light text.
	curveLight
)

// Luminance limits for the buckets, as relative luminance (0 = black,
// 1 = white). Below curveMidMinLum the text is dark and keeps raw coverage.
// Above curveLightMinLum the text is light and gets the full correction.
// These are first values. Tune them against screenshots on Windows at 125%.
const (
	curveMidMinLum   = 0.25
	curveLightMinLum = 0.6
)

// Gamma values of the two correction curves. Each curve maps coverage a to
// a^(1/gamma). 1.8 is the value the old Windows GDI backend used (and the V
// implementation before it).
const (
	coverageMidGamma   = 1.4
	coverageLightGamma = 1.8
)

// coverageMid and coverageLight are the lookup tables for curveMid and
// curveLight. Built once at init so the per-pixel remap is one table read.
var (
	coverageMid   [256]byte
	coverageLight [256]byte
)

func init() {
	fillGammaTable(&coverageMid, coverageMidGamma)
	fillGammaTable(&coverageLight, coverageLightGamma)
}

// fillGammaTable fills tab with round(255 * (i/255)^(1/gamma)).
func fillGammaTable(tab *[256]byte, gamma float64) {
	inv := 1.0 / gamma
	for i := range 256 {
		tab[i] = byte(math.Round(math.Pow(float64(i)/255.0, inv) * 255.0))
	}
}

// curveFor returns the coverage curve for text drawn in c. It uses the
// relative luminance of c (sRGB channels made linear, Rec. 709 weights), so
// pure blue counts as dark and pure yellow as light. Alpha is ignored: a
// translucent color has the same hue on screen, only weaker.
func curveFor(c Color) coverageCurve {
	l := 0.2126*srgbToLinear(c.R) + 0.7152*srgbToLinear(c.G) +
		0.0722*srgbToLinear(c.B)
	switch {
	case l > curveLightMinLum:
		return curveLight
	case l >= curveMidMinLum:
		return curveMid
	default:
		return curveNone
	}
}

// srgbToLinear converts one 8-bit sRGB channel to linear light (0..1).
func srgbToLinear(v uint8) float64 {
	c := float64(v) / 255.0
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// applyCoverageCurve remaps the alpha byte of every pixel in an RGBA
// buffer, in place. The RGB bytes are not changed. It does not allocate.
func applyCoverageCurve(rgba []byte, curve coverageCurve) {
	var tab *[256]byte
	switch curve {
	case curveMid:
		tab = &coverageMid
	case curveLight:
		tab = &coverageLight
	default:
		return
	}
	for i := 3; i < len(rgba); i += 4 {
		rgba[i] = tab[rgba[i]]
	}
}

// gradientCurve returns the coverage curve for text tinted by a gradient
// with these stops: the curve of the mean stop color. Not exact for every
// band, but it keeps one atlas entry per glyph for the whole draw.
func gradientCurve(stops []GradientStop) coverageCurve {
	if len(stops) == 0 {
		return curveNone
	}
	var r, g, b int
	for _, s := range stops {
		r += int(s.Color.R)
		g += int(s.Color.G)
		b += int(s.Color.B)
	}
	n := len(stops)
	return curveFor(Color{uint8(r / n), uint8(g / n), uint8(b / n), 255})
}
