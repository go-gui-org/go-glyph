package glyph

// Canvas2D style helpers for the WASM text path. They hold no syscall/js
// code, so they build and test on every platform.

// canvasFillMode says how the WASM fill pass colors one item's glyphs.
type canvasFillMode uint8

const (
	// canvasFillFlat: one fillStyle and globalAlpha from the item color.
	canvasFillFlat canvasFillMode = iota
	// canvasFillGradient: a Canvas2D linear gradient, composited per
	// pixel by the browser. Vertical gradients only.
	canvasFillGradient
	// canvasFillPerGlyph: one flat color per glyph from
	// gradientColorForGlyph, as the atlas path does. Horizontal and
	// diagonal gradients.
	canvasFillPerGlyph
)

// canvasFillModeFor picks the fill mode for gradient. Every direction must
// map to a mode that sets fillStyle, or the fill pass draws with the style
// the previous item left behind.
func canvasFillModeFor(gradient *GradientConfig) canvasFillMode {
	if gradient == nil || len(gradient.Stops) == 0 {
		return canvasFillFlat
	}
	if gradient.Direction == GradientVertical {
		return canvasFillGradient
	}
	return canvasFillPerGlyph
}

// canvasSpanY returns the Y range of a vertical Canvas2D gradient. A
// canvas gradient resolves in the matrix that is current when the fill
// runs. Under identity that is the base matrix, so the span needs the draw
// origin oy. Under a transform, setCanvasTransform has already put the
// origin into the matrix, so the span stays in layout coords; adding oy
// again would move the gradient down by oy.
func (e gradientExtents) canvasSpanY(oy float32, isIdentity bool) (y0, y1 float32) {
	y0 = e.yOff
	if isIdentity {
		y0 += oy
	}
	return y0, y0 + e.h
}

// One-entry cache for cssColorRGB. Runs of text share one color, so this
// removes most string allocations in the fill pass. WASM runs the renderer
// on one thread, so the cache needs no lock.
var (
	lastRGBColor Color
	lastRGBCSS   string
)

// cssColorRGB returns c as an opaque CSS rgb() string and ignores c.A.
// The text path sets alpha through globalAlpha; an rgba() style with the
// same alpha would apply it twice, so a 50% color would draw at 25%. The
// web backend's DrawFilledRect uses the same split.
func cssColorRGB(c Color) string {
	c.A = 0
	if c == lastRGBColor && lastRGBCSS != "" {
		return lastRGBCSS
	}
	s := "rgb(" + jsItoa(int(c.R)) + "," + jsItoa(int(c.G)) + "," +
		jsItoa(int(c.B)) + ")"
	lastRGBColor = c
	lastRGBCSS = s
	return s
}

// cssColorRGBA returns c as a CSS rgba() string with its alpha. Use it
// only where globalAlpha is 1, such as gradient color stops.
func cssColorRGBA(c Color) string {
	return "rgba(" + jsItoa(int(c.R)) + "," + jsItoa(int(c.G)) + "," +
		jsItoa(int(c.B)) + "," + jsAlpha(c.A) + ")"
}

// jsAlpha formats a/255 with three decimals. Three decimals keep every
// nonzero alpha nonzero: 1/255 is 0.004, and two decimals would round it
// to 0.
func jsAlpha(a uint8) string {
	switch a {
	case 255:
		return "1"
	case 0:
		return "0"
	}
	v := (int(a)*1000 + 127) / 255 // Rounded thousandths, 4..996.
	return "0." + jsItoa(v/100) + jsItoa(v/10%10) + jsItoa(v%10)
}

// jsItoa formats an int in base 10 without strconv.
func jsItoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + jsItoa(-i)
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
