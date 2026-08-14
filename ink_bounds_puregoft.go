//go:build android || linux || darwin || windows

package glyph

import (
	"math"
	"unicode/utf8"

	"github.com/go-text/typesetting/font"
)

// LayoutInkBounds returns the union of every glyph's outline bounding box
// in the layout, in layout coordinates (origin at the layout's top-left,
// Y downward) and logical pixels — the same space Layout.Width/Height and
// Item.X/Y use.
//
// It walks the pen exactly as the draw path does (see drawLayoutImpl):
// the pen starts at the item origin, each glyph paints at
// (pen.X+XOffset, pen.Y-YOffset), and the pen then advances by
// (XAdvance, -YAdvance).
//
// ok is false when any glyph's outline cannot be measured — an unloadable
// face, a bitmap-only color glyph, a cluster whose id layout did not
// resolve — because a partial union would silently mis-centre the caller
// rather than let it fall back to the advance box.
func (ctx *Context) LayoutInkBounds(layout *Layout) (Rect, bool) {
	if ctx == nil || layout == nil || len(layout.Items) == 0 {
		return Rect{}, false
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	painted := false

	for _, item := range layout.Items {
		cf := loadCachedFace(item.FontPath)
		if cf == nil || cf.face == nil {
			return Rect{}, false
		}
		// Font units → logical pixels. Shaping ran at the device size
		// (style size × DPI scale × the fallback cap-height fit), and the
		// layout's coordinates were divided back by the DPI scale, so the
		// logical em is the style size times the fit factor.
		_, sizePx, _, _ := resolveFTFontParams(item.Style, ctx.scaleFactor)
		em := sizePx * itemFontScale(item) / float64(ctx.scaleFactor)
		unit := em / float64(nonZeroUpem(cf.upem))

		penX, penY := item.X, item.Y
		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			ext, ok := glyphInkExtents(cf, layout.Text, g)
			if !ok {
				return Rect{}, false
			}
			// A blank cluster (space, ZWJ) paints nothing; it must not
			// stretch the box to the pen position.
			if ext.Width != 0 && ext.Height != 0 {
				x := penX + g.XOffset + float64(ext.XBearing)*unit
				// YBearing is the top of the ink measured up from the
				// baseline, Height is negative downward (see capEm).
				y := penY - g.YOffset - float64(ext.YBearing)*unit
				minX = math.Min(minX, x)
				minY = math.Min(minY, y)
				maxX = math.Max(maxX, x+float64(ext.Width)*unit)
				maxY = math.Max(maxY, y-float64(ext.Height)*unit)
				painted = true
			}
			penX += g.XAdvance
			penY -= g.YAdvance
		}
	}

	if !painted {
		return Rect{}, false
	}
	return Rect{
		X:      float32(minX),
		Y:      float32(minY),
		Width:  float32(maxX - minX),
		Height: float32(maxY - minY),
	}, true
}

// glyphInkExtents reads one glyph's outline extents in font units.
//
// The lock and the recover mirror capEm: GlyphExtents fills the face's
// extent cache (a mutation of the shared face) and can panic on defective
// font tables, which must degrade to "unmeasurable" rather than crash the
// render thread.
func glyphInkExtents(cf *cachedFace, text string, g Glyph) (
	ext font.GlyphExtents, ok bool,
) {
	gid, ok := glyphGID(cf, text, g)
	if !ok {
		return font.GlyphExtents{}, false
	}
	cf.mu.Lock()
	defer cf.mu.Unlock()
	defer func() {
		if recover() != nil {
			ext, ok = font.GlyphExtents{}, false
		}
	}()
	return cf.face.GlyphExtents(gid)
}

// glyphGID resolves the glyph id to measure. Layout records one when it
// shaped the cluster itself; otherwise the cluster is re-resolved through
// the face's cmap, which only single-rune clusters can satisfy.
func glyphGID(cf *cachedFace, text string, g Glyph) (font.GID, bool) {
	if g.Shaped || g.GlyphID != 0 {
		return font.GID(g.GlyphID), true
	}
	ch := glyphText(text, g)
	r, n := utf8.DecodeRuneInString(ch)
	if n == 0 || n != len(ch) || r == utf8.RuneError {
		return 0, false
	}
	return cf.face.NominalGlyph(r)
}
