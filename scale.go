package glyph

import "math"

// DPI rescaling. A TextSystem takes its scale from the backend once, at
// construction (NewTextSystem), and every size below it — the pixel size a
// face is opened at, the atlas bitmaps, the cached layouts — is derived from
// that one value. A window that moves to a monitor with a different scale
// factor must therefore push the new value down, or text keeps the density of
// the monitor it was created on while the rest of the frame follows the new
// one.
//
// The Context and Renderer field names are identical in the FreeType and WASM
// builds, so these setters stay in one untagged file (as cache.go does).

// setScaleFactor re-points the shaping context at a new device scale.
// Faces are cached by path and opened per call at a size derived from
// this value, so no face invalidation is needed here.
func (ctx *Context) setScaleFactor(s float32) {
	ctx.scaleFactor = s
	ctx.scaleInv = 1.0 / s
}

// setScaleFactor re-points the renderer at a new device scale. The caller
// purges the glyph cache: entries rasterized at the old density stay
// addressable but no longer match what the atlas should hold.
func (r *Renderer) setScaleFactor(s float32) {
	r.scaleFactor = s
	r.scaleInv = 1.0 / s
}

// SetDPIScale updates the device scale factor for shaping, rasterization and
// placement, and drops the caches keyed at the old one. Call it when the
// window moves to a display with a different scale factor, together with the
// matching setter on the DrawBackend.
//
// Values of zero or less (NaN included) and non-finite values are rejected,
// and values above 10× are ignored to bound rasterization cost; a scale equal
// to the current one is a no-op — so calling this on every resize costs
// nothing.
func (ts *TextSystem) SetDPIScale(s float32) {
	if !(s > 0) || math.IsInf(float64(s), 0) || s > 10 || ts.ctx == nil {
		return
	}
	if s == ts.ctx.scaleFactor {
		return
	}
	ts.ctx.setScaleFactor(s)
	if ts.renderer != nil {
		ts.renderer.setScaleFactor(s)
	}
	// The layout cache key (getCacheKey) does not hash the scale, so every
	// cached layout describes the old density and must go. Purge also clears
	// the glyph cache and resets the atlas pages.
	ts.Purge()
}

// DPIScale returns the device scale factor currently in effect.
func (ts *TextSystem) DPIScale() float32 {
	if ts.ctx == nil {
		return 0
	}
	return ts.ctx.scaleFactor
}
