//go:build linux && !android

package glyph

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/harfbuzz"
	"golang.org/x/image/vector"
)

// This file is the pure-Go replacement for bitmap_ft.go on Linux.
// Shaping comes from go-text/typesetting/harfbuzz; monochrome glyphs are
// rasterized with golang.org/x/image/vector; color-emoji glyphs decode
// their embedded bitmaps (CBDT/sbix PNG). Stroked text is degraded to
// the unstroked glyph for the spike (see loadStrokedGlyphFT).

// glyphBitmapPad matches the 2px border the cgo path reserved around the
// ink bounds so anti-aliased edges are not clipped in the atlas.
const glyphBitmapPad = 2

// Render-side singletons, populated by NewContext. They mirror the
// singletons the cgo backend exposed so the shared renderer code is
// unchanged.
var (
	ftFontPathsSingleton       map[string]string
	ftScriptFallbacksSingleton []string
	ftColorFallbacksSingleton  []string
)

// setFTLib is a no-op: the pure-Go backend keeps no library handle. It
// exists so shared setup code (NewContext) can call it uniformly.
func setFTLib(_ FTLibrary)                   {}
func setFTFontPaths(paths map[string]string) { ftFontPathsSingleton = paths }
func setFTScriptFallbacks(paths []string)    { ftScriptFallbacksSingleton = paths }
func setFTColorFallbacks(paths []string)     { ftColorFallbacksSingleton = paths }

// rasterResult holds a rendered RGBA bitmap plus its atlas placement
// offsets (left = x bearing from the pen origin, top = distance from the
// baseline up to the top row).
type rasterResult struct {
	data      []byte
	w, h      int
	left, top int
}

// loadGlyphFT rasterizes a grapheme cluster using the pure-Go pipeline.
func loadGlyphFT(atlas *GlyphAtlas, ch string, item Item,
	subpixelBin int, scaleFactor float32) (LoadGlyphResult, error) {

	family, fontSize, bold, italic := resolveFTFontParams(item.Style, scaleFactor)
	paths := fontFallbackPaths(ftFontPathsSingleton, family, bold, italic)
	subpixelShift := float64(subpixelBin) / 4.0

	var res *rasterResult

	// Color-emoji items resolve against the color fonts first: the
	// primary text font often carries a monochrome glyph for the same
	// codepoint, which would otherwise win and render tinted/blank.
	if item.UseOriginalColor {
		for _, fp := range ftColorFallbacksSingleton {
			if r, ok := renderColorGlyph(fp, fontSize, ch); ok {
				res = r
				break
			}
		}
	}

	// Primary font paths (reject .notdef so fallbacks get a turn).
	if res == nil {
		for _, fontPath := range paths {
			r, _ := renderMonoRun(fontPath, fontSize, subpixelShift, ch, true)
			if r != nil {
				res = r
				break
			}
		}
	}

	// Script fallback fonts if the primary font lacked the glyph.
	if res == nil {
		for _, fp := range ftScriptFallbacksSingleton {
			r, _ := renderMonoRun(fp, fontSize, subpixelShift, ch, true)
			if r != nil {
				res = r
				break
			}
		}
	}

	// Last resort: render with the primary font even if it is tofu.
	if res == nil && len(paths) > 0 {
		res, _ = renderMonoRun(paths[0], fontSize, subpixelShift, ch, false)
	}

	return insertRaster(atlas, res)
}

// loadStrokedGlyphFT renders a stroked cluster. The pure-Go stack has no
// path stroker yet, so this degrades to the unstroked glyph. Tracked as
// a follow-up (see issue #31 plan): stroked text on Linux is a known gap
// during the spike.
func loadStrokedGlyphFT(atlas *GlyphAtlas, ch string, item Item,
	strokeWidth float32, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	_ = strokeWidth
	return loadGlyphFT(atlas, ch, item, subpixelBin, scaleFactor)
}

// insertRaster uploads a rasterized bitmap to the atlas.
func insertRaster(atlas *GlyphAtlas, res *rasterResult) (LoadGlyphResult, error) {
	if res == nil || res.w == 0 || res.h == 0 {
		return LoadGlyphResult{}, nil
	}
	if _, err := checkAllocationSize(res.w, res.h, 4); err != nil {
		return LoadGlyphResult{}, err
	}
	bmp := Bitmap{
		Width:    res.w,
		Height:   res.h,
		Channels: 4,
		Data:     res.data,
	}
	cached, resetOccurred, resetPage, err := atlas.InsertBitmap(
		bmp, res.left, res.top)
	if err != nil {
		return LoadGlyphResult{}, err
	}
	return LoadGlyphResult{
		Cached:        cached,
		ResetOccurred: resetOccurred,
		ResetPage:     resetPage,
	}, nil
}

// placedGlyph is a shaped outline glyph with its pen position (in px).
type placedGlyph struct {
	segs       []font.Segment
	penX       float64
	xOff, yOff float64
}

// renderMonoRun shapes text with the given font and rasterizes the
// resulting outline glyphs into a white+alpha RGBA bitmap. Returns
// (nil, true) when rejectNotdef is set and any glyph is missing.
func renderMonoRun(path string, size, subpixelShift float64,
	text string, rejectNotdef bool) (*rasterResult, bool) {

	cf := loadCachedFace(path)
	if cf == nil {
		return nil, false
	}
	buf := shapeWith(cf, size, text)
	if buf == nil || len(buf.Info) == 0 {
		return nil, false
	}
	if rejectNotdef {
		for i := range buf.Info {
			if buf.Info[i].Glyph == 0 {
				return nil, true
			}
		}
	}

	scale := size / float64(nonZeroUpem(cf.upem))

	var glyphs []placedGlyph
	penX := subpixelShift
	for i := range buf.Info {
		pos := buf.Pos[i]
		gd := cf.face.GlyphData(buf.Info[i].Glyph)
		if out, ok := gd.(font.GlyphOutline); ok && len(out.Segments) > 0 {
			glyphs = append(glyphs, placedGlyph{
				segs: out.Segments,
				penX: penX,
				xOff: float64(pos.XOffset) / 64.0,
				yOff: float64(pos.YOffset) / 64.0,
			})
		}
		penX += float64(pos.XAdvance) / 64.0
	}
	if len(glyphs) == 0 {
		return nil, false // whitespace / no ink
	}

	// Map a font-unit point (Y up) to baseline-relative device px (Y down).
	mapPt := func(g placedGlyph, fx, fy float32) (dx, dy float64) {
		dx = g.penX + g.xOff + float64(fx)*scale
		dy = -float64(fy)*scale - g.yOff
		return dx, dy
	}

	minDx, minDy := math.Inf(1), math.Inf(1)
	maxDx, maxDy := math.Inf(-1), math.Inf(-1)
	for _, g := range glyphs {
		for si := range g.segs {
			for _, pt := range g.segs[si].ArgsSlice() {
				dx, dy := mapPt(g, pt.X, pt.Y)
				minDx, maxDx = math.Min(minDx, dx), math.Max(maxDx, dx)
				minDy, maxDy = math.Min(minDy, dy), math.Max(maxDy, dy)
			}
		}
	}
	if minDx > maxDx || minDy > maxDy {
		return nil, false
	}

	left := int(math.Floor(minDx)) - glyphBitmapPad
	topEdge := int(math.Floor(minDy)) - glyphBitmapPad
	w := int(math.Ceil(maxDx)) - left + glyphBitmapPad
	h := int(math.Ceil(maxDy)) - topEdge + glyphBitmapPad
	w, h = clampBitmapDim(w), clampBitmapDim(h)

	rz := vector.NewRasterizer(w, h)
	rz.DrawOp = draw.Src
	offX, offY := float64(-left), float64(-topEdge)
	for _, g := range glyphs {
		addOutline(rz, g, mapPt, offX, offY)
	}

	alpha := image.NewAlpha(image.Rect(0, 0, w, h))
	rz.Draw(alpha, alpha.Bounds(), image.Opaque, image.Point{})

	data := make([]byte, w*h*4)
	for i, a := range alpha.Pix {
		o := i * 4
		data[o+0] = 255
		data[o+1] = 255
		data[o+2] = 255
		data[o+3] = a
	}
	return &rasterResult{data: data, w: w, h: h, left: left, top: -topEdge}, false
}

// addOutline feeds one glyph's contours to the rasterizer, closing each
// subpath (font outlines are implicitly closed).
func addOutline(rz *vector.Rasterizer, g placedGlyph,
	mapPt func(placedGlyph, float32, float32) (float64, float64),
	offX, offY float64) {

	pt := func(sp ot.SegmentPoint) (float32, float32) {
		dx, dy := mapPt(g, sp.X, sp.Y)
		return float32(dx + offX), float32(dy + offY)
	}
	started := false
	for si := range g.segs {
		seg := g.segs[si]
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			if started {
				rz.ClosePath()
			}
			x, y := pt(seg.Args[0])
			rz.MoveTo(x, y)
			started = true
		case ot.SegmentOpLineTo:
			x, y := pt(seg.Args[0])
			rz.LineTo(x, y)
		case ot.SegmentOpQuadTo:
			bx, by := pt(seg.Args[0])
			cx, cy := pt(seg.Args[1])
			rz.QuadTo(bx, by, cx, cy)
		case ot.SegmentOpCubeTo:
			bx, by := pt(seg.Args[0])
			cx, cy := pt(seg.Args[1])
			dx, dy := pt(seg.Args[2])
			rz.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	if started {
		rz.ClosePath()
	}
}

// renderColorGlyph decodes the embedded color bitmap (CBDT/sbix PNG) for
// the first shaped glyph of text. Positioning is approximate for the
// spike: the glyph is placed as a full-ascent cell (left=0, top=height);
// the renderer scales it into the emoji cell.
func renderColorGlyph(path string, size float64, text string) (*rasterResult, bool) {
	cf := loadCachedFace(path)
	if cf == nil {
		return nil, false
	}
	// Select the bitmap strike nearest the requested size.
	ppem := uint16(size + 0.5)
	if ppem == 0 {
		ppem = 1
	}
	cf.face.SetPpem(ppem, ppem)

	buf := shapeWith(cf, size, text)
	if buf == nil || len(buf.Info) == 0 {
		return nil, false
	}
	gid := buf.Info[0].Glyph
	if gid == 0 {
		return nil, false
	}
	gb, ok := cf.face.GlyphData(gid).(font.GlyphBitmap)
	if !ok || len(gb.Data) == 0 || gb.Format != font.PNG {
		return nil, false
	}
	img, err := png.Decode(bytes.NewReader(gb.Data))
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, false
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	return &rasterResult{data: rgba.Pix, w: w, h: h, left: 0, top: h}, true
}

// shapeWith shapes text with a fresh harfbuzz font scaled so positions
// are in 26.6 fixed-point pixels.
func shapeWith(cf *cachedFace, size float64, text string) *harfbuzz.Buffer {
	if len(text) == 0 {
		return nil
	}
	hb := harfbuzz.NewFont(cf.face)
	s := int32(math.Round(size * 64))
	hb.XScale, hb.YScale = s, s
	buf := harfbuzz.NewBuffer()
	runes := []rune(text)
	buf.AddRunes(runes, 0, len(runes))
	buf.GuessSegmentProperties()
	buf.Shape(hb, nil)
	return buf
}

// clampBitmapDim keeps a bitmap dimension within the same 1..256 range
// the cgo path enforced.
func clampBitmapDim(v int) int {
	if v < 1 {
		return 1
	}
	if v > 256 {
		return 256
	}
	return v
}
