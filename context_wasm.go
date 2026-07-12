//go:build js && wasm

package glyph

import (
	"cmp"
	"fmt"
	"strings"
	"syscall/js"
)

// Context holds a Canvas2D context for text measurement in WASM.
//
// Not safe for concurrent use.
type Context struct {
	canvas      js.Value // OffscreenCanvas for measurement.
	ctx2d       js.Value // CanvasRenderingContext2D.
	scaleFactor float32
	scaleInv    float32
	metrics     metricsCache
}

// NewContext creates a WASM text context using an offscreen canvas
// for measureText calls.
func NewContext(scaleFactor float32) (*Context, error) {
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}

	doc := js.Global().Get("document")
	var canvas js.Value
	if !doc.IsUndefined() && !doc.IsNull() {
		canvas = doc.Call("createElement", "canvas")
		canvas.Set("width", 1)
		canvas.Set("height", 1)
	} else {
		// OffscreenCanvas for worker context.
		canvas = js.Global().Get("OffscreenCanvas").New(1, 1)
	}

	ctx2d := canvas.Call("getContext", "2d")
	if ctx2d.IsNull() || ctx2d.IsUndefined() {
		return nil, fmt.Errorf("failed to create 2d context for measurement")
	}

	return &Context{
		canvas:      canvas,
		ctx2d:       ctx2d,
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
	}, nil
}

// Free releases resources.
func (ctx *Context) Free() {
	ctx.canvas = js.Undefined()
	ctx.ctx2d = js.Undefined()
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile is a no-op under WASM. Use FontFace API to load fonts
// before creating the TextSystem.
func (ctx *Context) AddFontFile(_ string) error { return nil }

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	cssFont := buildCSSFont(cfg.Style)
	ctx.ctx2d.Set("font", cssFont)

	m := ctx.ctx2d.Call("measureText", "Hg")
	ascent := float32(m.Get("fontBoundingBoxAscent").Float())
	descent := float32(m.Get("fontBoundingBoxDescent").Float())
	return ascent + descent, nil
}

// FontMetrics returns detailed font metrics.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	cssFont := buildCSSFont(cfg.Style)
	ctx.ctx2d.Set("font", cssFont)

	m := ctx.ctx2d.Call("measureText", "Hg")
	ascent := float32(m.Get("fontBoundingBoxAscent").Float())
	descent := float32(m.Get("fontBoundingBoxDescent").Float())
	// Canvas exposes no line-gap; leading is 0 and the em floor guards
	// against cramped stacking.
	lineHeight := recommendedLineHeight(
		float64(ascent), float64(descent), 0, cssFontSize(cfg.Style))
	return TextMetrics{
		Ascender:   ascent,
		Descender:  descent,
		Height:     ascent + descent,
		LineHeight: float32(lineHeight),
	}, nil
}

// ResolveFontName returns the input name unchanged under WASM.
func (ctx *Context) ResolveFontName(name string) (string, error) {
	return name, nil
}

// cssFontSize resolves the font size (logical px) for a style, falling back
// to the size embedded in a Pango font name, then to 16.
func cssFontSize(style TextStyle) float64 {
	size := style.Size
	if size <= 0 {
		size = parseSizeFromFontName(style.FontName)
	}
	if size <= 0 {
		size = 16
	}
	return float64(size)
}

// buildCSSFont constructs a CSS font string from TextStyle.
func buildCSSFont(style TextStyle) string {
	size := cssFontSize(style)

	family := cmp.Or(parseFamilyFromFontName(style.FontName), "sans-serif")

	var sb strings.Builder

	// Style.
	switch style.Typeface {
	case TypefaceItalic, TypefaceBoldItalic:
		sb.WriteString("italic ")
	}

	// Weight.
	switch style.Typeface {
	case TypefaceBold, TypefaceBoldItalic:
		sb.WriteString("bold ")
	}

	// Also check FontName for "Bold"/"Italic".
	lower := strings.ToLower(style.FontName)
	if style.Typeface == TypefaceRegular {
		if strings.Contains(lower, " bold") {
			sb.WriteString("bold ")
		}
		if strings.Contains(lower, " italic") {
			sb.WriteString("italic ")
		}
	}

	fmt.Fprintf(&sb, "%gpx ", size)
	sb.WriteString(mapFontFamily(family))
	return sb.String()
}

// parseSizeFromFontName extracts trailing numeric size from Pango
// font name like "Sans Bold 18".
func parseSizeFromFontName(name string) float32 {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return 0
	}
	last := parts[len(parts)-1]
	var sz float32
	if _, err := fmt.Sscanf(last, "%f", &sz); err == nil && sz > 0 {
		return sz
	}
	return 0
}

// parseFamilyFromFontName extracts the family portion from a Pango
// font name, stripping trailing size and style keywords.
func parseFamilyFromFontName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}

	// Strip trailing number (size).
	end := len(parts)
	var sz float32
	if _, err := fmt.Sscanf(parts[end-1], "%f", &sz); err == nil && sz > 0 {
		end--
	}

	// Strip style keywords.
	styleWords := map[string]bool{
		"bold": true, "italic": true, "oblique": true,
		"light": true, "medium": true, "semibold": true,
		"heavy": true, "ultrabold": true, "ultralight": true,
		"condensed": true, "expanded": true, "regular": true,
	}
	for end > 0 && styleWords[strings.ToLower(parts[end-1])] {
		end--
	}
	if end == 0 {
		end = 1 // Keep at least one word.
	}
	return strings.Join(parts[:end], " ")
}

// mapFontFamily maps generic Pango families to CSS equivalents.
func mapFontFamily(family string) string {
	switch strings.ToLower(family) {
	case "sans", "sans-serif":
		return "sans-serif"
	case "serif":
		return "serif"
	case "monospace", "mono":
		return "monospace"
	default:
		clean := strings.ReplaceAll(family, "'", "")
		return "'" + clean + "', sans-serif"
	}
}
