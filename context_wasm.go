//go:build js && wasm

package glyph

import (
	"errors"
	"fmt"
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
	scaleFactor = sanitizeScale(scaleFactor)

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
		return nil, fmt.Errorf("glyph: failed to create 2d context for measurement")
	}

	return &Context{
		canvas:      canvas,
		ctx2d:       ctx2d,
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
	}, nil
}

// Free releases resources. FontHeight and FontMetrics on a freed
// Context return an error instead of calling into an undefined JS value.
func (ctx *Context) Free() {
	ctx.canvas = js.Undefined()
	ctx.ctx2d = js.Undefined()
	ctx.metrics = metricsCache{}
}

// errFreedContext is returned by measurement calls after Free.
var errFreedContext = errors.New("glyph: Context used after Free")

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile is a no-op under WASM. Use FontFace API to load fonts
// before creating the TextSystem.
func (ctx *Context) AddFontFile(_ string) error { return nil }

// listFontFamilies returns nil under WASM: the Canvas2D backend keeps no
// discovered font catalog. Backs (*TextSystem).ListFontFamilies.
func (ctx *Context) listFontFamilies() []string { return nil }

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	if ctx.ctx2d.IsUndefined() {
		return 0, errFreedContext
	}
	cssFont := buildCSSFont(cfg.Style)
	ctx.ctx2d.Set("font", cssFont)

	m := ctx.ctx2d.Call("measureText", "Hg")
	ascent := float32(m.Get("fontBoundingBoxAscent").Float())
	descent := float32(m.Get("fontBoundingBoxDescent").Float())
	return ascent + descent, nil
}

// FontMetrics returns detailed font metrics.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	if ctx.ctx2d.IsUndefined() {
		return TextMetrics{}, errFreedContext
	}
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
