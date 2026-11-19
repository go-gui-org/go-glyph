//go:build linux || darwin || windows

package glyph

import (
	"math"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// Variable fonts on the pure-Go path.
//
// A variable font holds a range of designs on one or more axes (weight,
// width, optical size, ...). go-text draws the default instance unless the
// face gets variation coordinates. Before this file, the path never set them,
// so TextStyle.Features.VariationAxes had no effect, and every variable font
// drew its default instance at every size.
//
// The optical size axis (opsz) matters most. Its value is the point size the
// design is for: small sizes get wider letters, looser spacing and heavier
// thin strokes, so they stay legible. CoreText, DirectWrite and browsers
// (font-optical-sizing: auto) set opsz to the point size by themselves. The
// macOS system font (SF, SFNS.ttf) has opsz 17..28 with default 28, so its
// default instance is the tight Display design. At 13 pt CoreText uses opsz
// 17 (the Text design), which is about 11% wider. Without the auto opsz our
// UI text was narrower and tighter than the same text in native apps.
//
// Here, the axis values for a face come from varSpec: opsz is the point size
// (device px / scale factor) unless the caller sets it, and the caller's
// VariationAxes set the other axes. Each set of values gets its own
// cachedFace (an "instance"), which shares the parsed *font.Font of its base
// face. Layout and raster derive the instance from the same size and style,
// so the advances layout shapes with are the advances of the outlines the
// raster draws.

// maxVarAxes is the most axes a variable font may have for its variations to
// be used. Real fonts have fewer than ten. A larger fvar (malformed or
// hostile) is ignored, and the font draws its default instance.
const maxVarAxes = 16

// maxFaceInstances bounds the instances kept per base face. With the auto
// opsz quantized to half points, one font at UI sizes needs a few. Arbitrary
// caller axes could make many, so the map starts again when it is full. An
// instance a live ftFont still holds stays valid; it is only no longer shared.
const maxFaceInstances = 64

var opszTag = ot.MustNewTag("opsz")

// varSpec is what selects the instance of a variable font.
type varSpec struct {
	// scale is device px per point (the Context or Renderer scale factor).
	// It turns the device px size into the point size that the auto opsz
	// uses. 0 turns the auto opsz off.
	scale float64
	// axes are the caller's axis values (TextStyle.Features.VariationAxes),
	// in design units. A tag the font does not have is ignored. An opsz here
	// replaces the auto value.
	axes []FontAxis
}

// varSpecFor returns the varSpec of a style drawn at scaleFactor.
func varSpecFor(style TextStyle, scaleFactor float32) varSpec {
	v := varSpec{scale: float64(scaleFactor)}
	if style.Features != nil {
		v.axes = style.Features.VariationAxes
	}
	return v
}

// ptem returns the point size of text drawn at size device px, for the
// shaper (see getHB). Without a scale, the px size is used.
func (v varSpec) ptem(size float64) float32 {
	if v.scale > 0 {
		return float32(size / v.scale)
	}
	return float32(size)
}

// hash returns a hash of v for map keys.
func (v varSpec) hash() uint64 {
	h := fnvHashF32(fnvOffsetBasis, float32(v.scale))
	for _, a := range v.axes {
		h = fnvHashString(h, a.Tag)
		h = fnvHashF32(h, a.Value)
	}
	return h
}

// instKey holds the design value of each axis of a font, in fvar order. It
// is the map key of an instance.
type instKey [maxVarAxes]float32

// parseVarAxes returns the axes of the font's fvar table, or nil when the
// font is not variable or its fvar cannot be used.
func parseVarAxes(ld *ot.Loader) []tables.VariationAxisRecord {
	raw, err := ld.RawTable(ot.MustNewTag("fvar"))
	if err != nil {
		return nil
	}
	fv, _, err := tables.ParseFvar(raw)
	if err != nil {
		return nil
	}
	axes := fv.Axis
	if len(axes) == 0 || len(axes) > maxVarAxes {
		return nil
	}
	for _, a := range axes {
		// NaN fails these comparisons too.
		if !(a.Minimum <= a.Default && a.Default <= a.Maximum) {
			return nil
		}
	}
	return axes
}

// autoOpsz returns the opsz value for text of pt points. It is rounded to
// half a point, so sizes that differ only by float noise (fit factors, DPI
// math) share one instance, and the number of instances stays small. Half a
// point of opsz changes a line of SF by about 0.5%.
func autoOpsz(pt float64) float32 {
	return float32(math.Round(pt*2) / 2)
}

// loadFace returns the face of path to shape and draw size (device px) with
// the variations of v. For a font that is not variable, and when every axis
// is at its default, it is the cached base face itself.
func loadFace(path string, size float64, v varSpec) *cachedFace {
	cf := loadCachedFace(path)
	if cf == nil || len(cf.axes) == 0 {
		return cf
	}
	return cf.instance(size, v)
}

// designValues returns the design value of each axis for size and v, and
// whether all of them are the defaults.
func (cf *cachedFace) designValues(size float64, v varSpec) (key instKey, isDefault bool) {
	isDefault = true
	for i, a := range cf.axes {
		val := a.Default
		if a.Tag == opszTag && v.scale > 0 && size > 0 {
			val = autoOpsz(size / v.scale)
		}
		for _, ua := range v.axes {
			if len(ua.Tag) == 4 && ot.MustNewTag(ua.Tag) == a.Tag {
				val = ua.Value
			}
		}
		// Clamp to the axis, as the font spec says. NaN goes to the minimum.
		if !(val >= a.Minimum) {
			val = a.Minimum
		} else if val > a.Maximum {
			val = a.Maximum
		}
		key[i] = val
		if val != a.Default {
			isDefault = false
		}
	}
	return key, isDefault
}

// instance returns the instance of cf for size and v, and makes it on the
// first use. It returns cf when all axes are at their defaults, or when the
// instance cannot be made.
func (cf *cachedFace) instance(size float64, v varSpec) *cachedFace {
	key, isDefault := cf.designValues(size, v)
	if isDefault {
		return cf
	}
	cf.instMu.Lock()
	defer cf.instMu.Unlock()
	if inst, ok := cf.instances[key]; ok {
		return inst
	}
	inst := cf.newInstance(key[:len(cf.axes)])
	if inst == nil {
		return cf
	}
	if cf.instances == nil || len(cf.instances) >= maxFaceInstances {
		cf.instances = make(map[instKey]*cachedFace)
	}
	cf.instances[key] = inst
	return inst
}

// newInstance makes the face of cf at the design values. It shares cf's
// parsed font, which is read-only after parse. It returns nil when go-text
// panics on the font's variation tables.
func (cf *cachedFace) newInstance(design []float32) (inst *cachedFace) {
	defer func() {
		if recover() != nil {
			inst = nil
		}
	}()
	// NormalizeVariations maps to [-1, 1] and applies avar. It returns a new
	// slice and does not keep design.
	coords := cf.face.NormalizeVariations(design)
	face := font.NewFace(cf.face.Font)
	face.SetCoords(coords)
	return &cachedFace{
		face:  face,
		upem:  cf.upem,
		color: cf.color,
		sbix:  cf.sbix,
		base:  cf,
	}
}
