//go:build linux || darwin || windows

package glyph

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/harfbuzz"
	"golang.org/x/image/vector"
)

// This file is the pure-Go replacement for the cgo rasterizer on Linux,
// Android, macOS, and Windows. Shaping/rasterization are shared; only font
// discovery (see discover_*.go) differs by platform.
// Shaping comes from go-text/typesetting/harfbuzz; monochrome glyphs are
// rasterized with golang.org/x/image/vector; color-emoji glyphs either
// decode their embedded bitmaps (CBDT/sbix PNG) or, for COLR v0 layered
// fonts such as Windows' Segoe UI Emoji, composite palette-colored outline
// layers (see renderCOLRGlyph); stroked text uses the pure-Go path stroker
// (see loadStrokedGlyphFT and addStrokedOutline).

// glyphBitmapPad matches the 2px border the cgo path reserved around the
// ink bounds so anti-aliased edges are not clipped in the atlas.
const glyphBitmapPad = 2

// maxEmbeddedBitmapBytes caps the compressed size of an embedded CBDT/sbix
// bitmap accepted for decoding. Real strikes are tens to hundreds of KB;
// beyond this the entry is hostile or corrupt, and decoding it risks a
// decompression bomb.
const maxEmbeddedBitmapBytes = 16 << 20

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

// renderFontPaths returns the font map a glyph load resolves names in:
// the Renderer's own Context map when it has one, else the process-wide
// map of the most recent Context. The per-Renderer map matters when a
// process has more than one Context (one TextSystem per window): the
// global map points at the newest Context only, so a font added to an
// older Context resolved during layout but not during rendering.
func renderFontPaths(own map[string]string) map[string]string {
	if own != nil {
		return own
	}
	return ftFontPathsSingleton
}

// rasterResult holds a rendered RGBA bitmap plus its atlas placement
// offsets (left = x bearing from the pen origin, top = distance from the
// baseline up to the top row).
type rasterResult struct {
	data      []byte
	w, h      int
	left, top int
}

// loadGlyphFT rasterizes a grapheme cluster using the pure-Go pipeline.
// When runText differs from ch it provides the surrounding text context
// for correct Arabic shaping (initial/medial/final forms).
//
// fontPaths is the font map of the Context that laid the text out (see
// Renderer.useContextFonts). nil selects the process-wide map of the most
// recent Context (ftFontPathsSingleton).
func loadGlyphFT(atlas *GlyphAtlas, fontPaths map[string]string,
	ch string, runText string, targetRuneIdx int, item Item, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	family, fontSize, bold, italic := resolveFTFontParams(item.Style, scaleFactor)
	paths := fontFallbackPaths(renderFontPaths(fontPaths), family, bold, italic)
	subpixelShift := float64(subpixelBin) / 4.0

	var res *rasterResult

	if item.UseOriginalColor {
		for _, fp := range ftColorFallbacksSingleton {
			if r, ok := renderColorGlyph(atlas, fp, fontSize, ch); ok {
				r.top = max(0, int(float64(r.h)-float64(item.Descent)*float64(scaleFactor)/2))
				res = r
				break
			}
			fg := color.NRGBA{R: item.Color.R, G: item.Color.G,
				B: item.Color.B, A: item.Color.A}
			if r, ok := renderCOLRGlyph(atlas, fp, fontSize, ch, fg); ok {
				res = r
				break
			}
		}
	}

	covered := false
	if res == nil {
		for _, fontPath := range paths {
			r, rejected := renderMonoRun(atlas, fontPath, fontSize,
				subpixelShift, ch, runText, targetRuneIdx, true)
			if !rejected {
				res = r
				covered = true
				break
			}
		}
	}

	if !covered && res == nil {
		for _, fp := range orderedTextFallbackPaths(ch) {
			// Same cap-height match the layout path applies, so a cluster
			// rasterized here is the size it would be if it had been shaped
			// into its own item.
			r, rejected := renderMonoRun(atlas, fp,
				fontSize*textFallbackFitScale(paths, fp), subpixelShift,
				ch, runText, targetRuneIdx, true)
			if !rejected {
				res = r
				covered = true
				break
			}
		}
	}

	if !covered && res == nil && len(paths) > 0 {
		res, _ = renderMonoRun(atlas, paths[0], fontSize, subpixelShift,
			ch, runText, targetRuneIdx, false)
	}

	return insertRaster(atlas, res)
}

// orderedTextFallbackPaths returns the fallback fonts covering ch in the
// order the layout-time selector (probeFallback) would try them: monochrome
// fonts first, color fonts last. Sharing orderTextFallbacks keeps the
// render-side text path and layout selection picking the same font for the
// same cluster, instead of the raw tier order that let a color/emoji font
// shadow a monochrome one here.
func orderedTextFallbackPaths(ch string) []string {
	mono, color := orderTextFallbacks(ftScriptFallbacksSingleton, ch)
	// mono is freshly allocated by orderTextFallbacks (not shared), so
	// appending color onto it in place saves a third slice allocation.
	return append(mono, color...)
}

// loadStrokedGlyphFT renders a stroked cluster using the pure-Go stroker.
// fontPaths is as for loadGlyphFT.
func loadStrokedGlyphFT(atlas *GlyphAtlas, fontPaths map[string]string,
	ch string, runText string, targetRuneIdx int, item Item,
	strokeWidth float32, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	if strokeWidth <= 0 {
		return loadGlyphFT(atlas, fontPaths, ch, runText, targetRuneIdx, item,
			subpixelBin, scaleFactor)
	}

	family, fontSize, bold, italic := resolveFTFontParams(item.Style, scaleFactor)
	paths := fontFallbackPaths(renderFontPaths(fontPaths), family, bold, italic)
	subpixelShift := float64(subpixelBin) / 4.0
	sw := float64(strokeWidth)

	var res *rasterResult

	covered := false
	for _, fontPath := range paths {
		r, rejected := renderStrokedRun(atlas, fontPath, fontSize, sw,
			subpixelShift, ch, runText, targetRuneIdx, true)
		if !rejected {
			res = r
			covered = true
			break
		}
	}

	if !covered && res == nil {
		for _, fp := range orderedTextFallbackPaths(ch) {
			r, rejected := renderStrokedRun(atlas, fp,
				fontSize*textFallbackFitScale(paths, fp), sw,
				subpixelShift, ch, runText, targetRuneIdx, true)
			if !rejected {
				res = r
				covered = true
				break
			}
		}
	}

	if !covered && res == nil && len(paths) > 0 {
		res, _ = renderStrokedRun(atlas, paths[0], fontSize, sw,
			subpixelShift, ch, runText, targetRuneIdx, false)
	}

	return insertRaster(atlas, res)
}

// loadGlyphByIDFT rasterizes a single shaped glyph by its id using the exact
// font (path) the layout shaped with. A glyph id is font-specific, so there
// is no fallback iteration and no notdef rejection: the layout already picked
// the covering font when it produced the id. strokeWidth > 0 strokes the
// outline instead of filling it.
func loadGlyphByIDFT(atlas *GlyphAtlas, path string, gid uint32, item Item,
	strokeWidth float32, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	_, fontSize, _, _ := resolveFTFontParams(item.Style, scaleFactor)
	fontSize *= itemFontScale(item)
	subpixelShift := float64(subpixelBin) / 4.0
	res := renderGlyphByID(atlas, path, fontSize, float64(strokeWidth),
		subpixelShift, gid)
	return insertRaster(atlas, res)
}

// insertRaster uploads a rasterized bitmap to the atlas.
// res.data may alias atlas scratch storage. InsertBitmap copies the
// pixels at once, so the alias is safe as long as res goes straight
// to InsertBitmap with no other render in between.
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

// renderMonoRun rasterizes filled outline glyphs (no stroke).
func renderMonoRun(atlas *GlyphAtlas, path string, size, subpixelShift float64,
	text string, runText string, targetRuneIdx int,
	rejectNotdef bool) (*rasterResult, bool) {
	return renderRun(atlas, path, size, 0, subpixelShift, text, runText,
		targetRuneIdx, rejectNotdef)
}

// renderStrokedRun rasterizes the stroke border of the outline glyphs at
// the given stroke radius, using the pure-Go stroker.
func renderStrokedRun(atlas *GlyphAtlas, path string, size, strokeWidth, subpixelShift float64,
	text string, runText string, targetRuneIdx int,
	rejectNotdef bool) (*rasterResult, bool) {
	return renderRun(atlas, path, size, strokeWidth, subpixelShift, text, runText,
		targetRuneIdx, rejectNotdef)
}

// renderRun shapes text with the given font and rasterizes the resulting
// outline glyphs into a white+alpha RGBA bitmap. When strokeWidth > 0 the
// glyph contours are stroked (round joins/caps, radius = strokeWidth)
// instead of filled. Returns (nil, true) when rejectNotdef is set and a
// glyph belonging to the target cluster is missing.
//
// When runText differs from text it provides surrounding context for
// correct Arabic shaping. targetRuneIdx identifies which rune(s) in
// runText to rasterize; glyphs from other clusters advance penX but are
// not rasterized.
func renderRun(atlas *GlyphAtlas, path string, size, strokeWidth, subpixelShift float64,
	text string, runText string, targetRuneIdx int,
	rejectNotdef bool) (*rasterResult, bool) {

	cf := loadCachedFace(path)
	if cf == nil {
		return nil, false
	}

	shapeText := runText
	if shapeText == "" || shapeText == text {
		shapeText = text
	}
	buf := shapeWith(cf, size, shapeText)
	defer releaseShapeBuffer(buf) // nil-safe
	if buf == nil || len(buf.Info) == 0 {
		return nil, false
	}
	// collectAll is set when the whole shaped buffer belongs to the target
	// (no surrounding context); otherwise only the target cluster's glyphs
	// participate in notdef rejection and rasterization.
	collectAll := shapeText == text
	if rejectNotdef {
		for i := range buf.Info {
			if !collectAll && buf.Info[i].Cluster != targetRuneIdx {
				continue
			}
			if buf.Info[i].Glyph == 0 {
				return nil, true
			}
		}
	}

	var glyphs []placedGlyph
	penX := subpixelShift
	for i := range buf.Info {
		pos := buf.Pos[i]
		if !collectAll && buf.Info[i].Cluster != targetRuneIdx {
			continue
		}
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
	return rasterizeGlyphs(atlas, cf, size, strokeWidth, glyphs)
}

// renderGlyphByID rasterizes a single glyph by its font-specific glyph id,
// with no shaping. gid must come from the same font (path) the layout shaped
// with — a glyph id is meaningless in any other font, so this path does no
// fallback iteration. HarfBuzz x/y offsets are NOT baked into the cell; the
// draw loop applies them via Glyph.XOffset/YOffset. Returns (nil, false) when
// the glyph has no outline (a space, or a bitmap/color glyph handled by the
// color paths).
func renderGlyphByID(atlas *GlyphAtlas, path string, size, strokeWidth, subpixelShift float64,
	gid uint32) *rasterResult {

	cf := loadCachedFace(path)
	if cf == nil {
		return nil
	}
	gd := cf.face.GlyphData(font.GID(gid))
	out, ok := gd.(font.GlyphOutline)
	if !ok || len(out.Segments) == 0 {
		return nil
	}
	glyphs := []placedGlyph{{segs: out.Segments, penX: subpixelShift}}
	res, _ := rasterizeGlyphs(atlas, cf, size, strokeWidth, glyphs)
	return res
}

// rasterizeGlyphs renders pre-positioned outline glyphs into a white+alpha
// RGBA bitmap. When strokeWidth > 0 the contours are stroked (round
// joins/caps, radius = strokeWidth) instead of filled. Each glyph's penX and
// xOff/yOff place it; glyph coordinates are scaled by size/upem. Returns
// (nil, false) when the glyphs carry no ink.
//
// When atlas is non-nil, scratch buffers from the atlas are reused to reduce
// per-glyph allocations. When nil, buffers are freshly allocated (test path).
func rasterizeGlyphs(atlas *GlyphAtlas, cf *cachedFace, size, strokeWidth float64,
	glyphs []placedGlyph) (*rasterResult, bool) {

	if len(glyphs) == 0 {
		return nil, false
	}
	if cf == nil || !validRenderSize(size) {
		return nil, false
	}
	scale := size / float64(nonZeroUpem(cf.upem))
	// A negative or NaN stroke radius is meaningless; render filled.
	// A huge or +Inf radius is refused by the span check below.
	strokeR := strokeWidth
	if !(strokeR >= 0) {
		strokeR = 0
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
	// Stroking grows the ink by the stroke radius on every side.
	if strokeR > 0 {
		minDx -= strokeR
		minDy -= strokeR
		maxDx += strokeR
		maxDy += strokeR
	}
	// Refuse in float space, before the int conversions below. Go gives an
	// implementation-defined result when it converts an out-of-range float
	// (such as the bounds of an +Inf or 1e300 stroke) to int. The
	// resulting w0/h0 can wrap to a small positive size and pass the
	// maxNaturalGlyphDim check. The negated <= also catches NaN.
	if !(maxDx-minDx <= maxNaturalGlyphDim && maxDy-minDy <= maxNaturalGlyphDim) {
		return nil, false
	}

	left := int(math.Floor(minDx)) - glyphBitmapPad
	topEdge := int(math.Floor(minDy)) - glyphBitmapPad
	w0 := int(math.Ceil(maxDx)) - left + glyphBitmapPad
	h0 := int(math.Ceil(maxDy)) - topEdge + glyphBitmapPad

	// One emit closure serves both paths below so the outline logic
	// cannot drift between the fast and oversize renders.
	emit := func(rz *vector.Rasterizer, offX, offY float64) {
		for _, g := range glyphs {
			if strokeR > 0 {
				addStrokedOutline(rz, g, mapPt, offX, offY, strokeR)
			} else {
				addOutline(rz, g, mapPt, offX, offY)
			}
		}
	}

	// Oversized ink renders at natural size into local buffers and
	// downscales to MaxGlyphSize, preserving the glyph instead of
	// cropping to the top-left 256px.
	if w0 > MaxGlyphSize || h0 > MaxGlyphSize {
		return rasterizeGlyphsOversize(emit, left, topEdge, w0, h0)
	}
	w, h := clampBitmapDim(w0), clampBitmapDim(h0)

	var rz *vector.Rasterizer
	if atlas != nil {
		rz = atlas.ensureRasterizer(w, h)
		if rz == nil {
			return nil, false
		}
	} else {
		rz = vector.NewRasterizer(w, h)
	}
	rz.DrawOp = draw.Src
	offX, offY := float64(-left), float64(-topEdge)
	emit(rz, offX, offY)

	if atlas != nil {
		alpha := atlas.ensureAlpha(w, h)
		if alpha == nil {
			return nil, false
		}
		rz.Draw(alpha, alpha.Bounds(), image.Opaque, image.Point{})
		rgba := atlas.ensureRGBA(w, h)
		if rgba == nil {
			return nil, false
		}
		data := rgba.Pix
		for i, a := range alpha.Pix {
			o := i * 4
			data[o+0] = 255
			data[o+1] = 255
			data[o+2] = 255
			data[o+3] = a
		}
		return &rasterResult{data: data, w: w, h: h, left: left, top: -topEdge}, false
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

// rasterizeGlyphsOversize renders ink larger than MaxGlyphSize at its
// natural size into local buffers (never atlas scratch, which is
// grow-only and must not absorb a one-off giant), then downscales to
// fit. Bearings scale with the bitmap so the shrunken cell stays
// aligned to its pen origin. Beyond maxNaturalGlyphDim it refuses with
// (nil, false) — the caller caches a blank glyph instead of
// allocating tens of MB for one glyph.
func rasterizeGlyphsOversize(emit func(rz *vector.Rasterizer, offX, offY float64),
	left, topEdge, w0, h0 int) (*rasterResult, bool) {

	if w0 > maxNaturalGlyphDim || h0 > maxNaturalGlyphDim {
		return nil, false
	}
	if _, err := checkAllocationSize(w0, h0, 4); err != nil {
		return nil, false
	}
	rz := vector.NewRasterizer(w0, h0)
	rz.DrawOp = draw.Src
	offX, offY := float64(-left), float64(-topEdge)
	emit(rz, offX, offY)

	alpha := image.NewAlpha(image.Rect(0, 0, w0, h0))
	rz.Draw(alpha, alpha.Bounds(), image.Opaque, image.Point{})

	pix := make([]byte, w0*h0*4)
	for i, a := range alpha.Pix {
		o := i * 4
		pix[o+0] = 255
		pix[o+1] = 255
		pix[o+2] = 255
		pix[o+3] = a
	}
	out, dstW, dstH, s := downscaleGlyph(pix, w0, h0)
	if out == nil {
		return nil, false
	}
	return &rasterResult{data: out, w: dstW, h: dstH,
		left: scaleOffset(left, s), top: scaleOffset(-topEdge, s)}, false
}

// downscaleGlyph shrinks a w0 x h0 RGBA buffer to fit MaxGlyphSize and
// returns the new pixels, their size, and the applied scale (for the
// bearings). The result is a fresh buffer, not atlas scratch: copying it
// into scratch would only add a copy, because insertRaster copies it
// again. A nil out means the scale failed.
func downscaleGlyph(pix []byte, w0, h0 int) (out []byte, w, h int, s float64) {
	w, h, s = fitGlyphDims(w0, h0)
	out = ScaleBitmapBicubic(pix, w0, h0, w, h)
	return out, w, h, s
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
		// A contour without an opening MoveTo (malformed font) has no
		// defined start point; dropping its segments beats drawing
		// from the rasterizer origin.
		if seg.Op != ot.SegmentOpMoveTo && !started {
			continue
		}
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
func renderColorGlyph(atlas *GlyphAtlas, path string, size float64, text string) (*rasterResult, bool) {
	if !validRenderSize(size) {
		return nil, false
	}
	cf := loadCachedFace(path)
	if cf == nil {
		return nil, false
	}

	buf := shapeWith(cf, size, text)
	defer releaseShapeBuffer(buf) // nil-safe
	if buf == nil || len(buf.Info) == 0 {
		return nil, false
	}
	gid := buf.Info[0].Glyph
	if gid == 0 {
		return nil, false
	}

	// SetPpem selects the bitmap strike and mutates the shared face, so
	// the SetPpem+GlyphData pair runs under mu against concurrent
	// renders of the same cached face from another Context. The bitmap
	// bytes are copied under the same lock: GlyphData may alias face
	// internals that the next SetPpem invalidates, so decoding must use
	// the copy, outside the lock. ppem is clamped to the uint16 range
	// instead of wrapping on absurd sizes.
	ppem := uint16(min(size+0.5, 65535))
	cf.mu.Lock()
	cf.face.SetPpem(ppem, ppem)
	gd := cf.face.GlyphData(gid)
	var pngData []byte
	if gb, ok := gd.(font.GlyphBitmap); ok && gb.Format == font.PNG &&
		len(gb.Data) > 0 && len(gb.Data) <= maxEmbeddedBitmapBytes {
		pngData = append([]byte(nil), gb.Data...)
	}
	cf.mu.Unlock()
	if pngData == nil {
		return nil, false
	}

	// Read the header first. png.Decode allocates the full pixel buffer
	// from the header dimensions, so a small hostile PNG that claims
	// 65535x65535 would allocate ~16GB before the size check below runs.
	cfg, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > maxNaturalGlyphDim || cfg.Height > maxNaturalGlyphDim {
		return nil, false
	}
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, false
	}
	// Oversized strikes downscale to MaxGlyphSize instead of cropping
	// to the top-left 256px.
	if w > MaxGlyphSize || h > MaxGlyphSize {
		// DecodeConfig above already capped w and h at maxNaturalGlyphDim.
		full := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(full, full.Bounds(), img, b.Min, draw.Src)
		out, dstW, dstH, _ := downscaleGlyph(full.Pix, w, h)
		if out == nil {
			return nil, false
		}
		return &rasterResult{data: out, w: dstW, h: dstH, left: 0, top: dstH}, true
	}
	var rgba *image.RGBA
	if atlas != nil {
		rgba = atlas.ensureRGBA(w, h)
		if rgba == nil {
			return nil, false
		}
	} else {
		rgba = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	// data aliases atlas scratch storage when atlas is set. The
	// caller must pass it to insertRaster before the next render.
	return &rasterResult{data: rgba.Pix, w: w, h: h, left: 0, top: h}, true
}

// renderCOLRGlyph rasterizes a COLR v0 (layered) color glyph — the format
// Windows' Segoe UI Emoji uses — for the first shaped glyph of text. Each
// layer is an outline glyph filled with a solid color drawn from palette 0
// of the CPAL table; layers paint bottom-to-top with source-over
// compositing, yielding a premultiplied-alpha RGBA cell. fg is the run's
// text color, used for layers referencing the 0xFFFF "foreground" palette
// index. Returns false when the font has no COLR v0 entry for the glyph
// (bitmap or COLR v1 emoji, or a plain text font), so the caller can try
// the next fallback.
func renderCOLRGlyph(atlas *GlyphAtlas, path string, size float64, text string,
	fg color.NRGBA) (*rasterResult, bool) {
	if !validRenderSize(size) {
		return nil, false
	}
	cf := loadCachedFace(path)
	if cf == nil || cf.face.COLR == nil || len(cf.face.CPAL) == 0 {
		return nil, false
	}
	buf := shapeWith(cf, size, text)
	defer releaseShapeBuffer(buf) // nil-safe
	if buf == nil || len(buf.Info) == 0 {
		return nil, false
	}
	gid := buf.Info[0].Glyph
	if gid == 0 {
		return nil, false
	}
	paint, ok := cf.face.COLR.Search(tables.GlyphID(gid))
	if !ok {
		return nil, false
	}
	// COLR v1 paint graphs (gradients/transforms) resolve to other paint
	// types; only the v0 layered form is handled here.
	layers, ok := paint.(tables.PaintColrLayersResolved)
	if !ok || len(layers) == 0 {
		return nil, false
	}

	palette := cf.face.CPAL[0]
	scale := size / float64(nonZeroUpem(cf.upem))

	type coloredLayer struct {
		g   placedGlyph
		col color.NRGBA
	}
	var cls []coloredLayer
	for _, layer := range layers {
		gd := cf.face.GlyphData(font.GID(layer.GlyphID))
		out, ok := gd.(font.GlyphOutline)
		if !ok || len(out.Segments) == 0 {
			continue // empty/space layer contributes no ink
		}
		cls = append(cls, coloredLayer{
			g:   placedGlyph{segs: out.Segments},
			col: paletteColor(palette, layer.PaletteIndex, fg),
		})
	}
	if len(cls) == 0 {
		return nil, false
	}

	// Map a font-unit point (Y up) to baseline-relative device px (Y down).
	mapPt := func(g placedGlyph, fx, fy float32) (dx, dy float64) {
		dx = g.penX + g.xOff + float64(fx)*scale
		dy = -float64(fy)*scale - g.yOff
		return dx, dy
	}

	minDx, minDy := math.Inf(1), math.Inf(1)
	maxDx, maxDy := math.Inf(-1), math.Inf(-1)
	for _, cl := range cls {
		for si := range cl.g.segs {
			for _, pt := range cl.g.segs[si].ArgsSlice() {
				dx, dy := mapPt(cl.g, pt.X, pt.Y)
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
	w0 := int(math.Ceil(maxDx)) - left + glyphBitmapPad
	h0 := int(math.Ceil(maxDy)) - topEdge + glyphBitmapPad
	w, h := clampBitmapDim(w0), clampBitmapDim(h0)
	// Oversized output renders at natural size into local buffers and
	// downscales below, so ink is preserved instead of cropped. Beyond
	// maxNaturalGlyphDim the glyph refuses (blank cell) rather than
	// allocating tens of MB for one glyph.
	oversize := w0 > MaxGlyphSize || h0 > MaxGlyphSize
	if oversize {
		if w0 > maxNaturalGlyphDim || h0 > maxNaturalGlyphDim {
			return nil, false
		}
		w, h = w0, h0
	}
	local := atlas == nil || oversize

	var acc *image.RGBA
	if !local {
		acc = atlas.ensureRGBA(w, h)
		if acc == nil {
			return nil, false
		}
		clear(acc.Pix)
	} else {
		acc = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	var rz *vector.Rasterizer
	if !local {
		rz = atlas.ensureRasterizer(w, h)
		if rz == nil {
			return nil, false
		}
	}
	offX, offY := float64(-left), float64(-topEdge)
	for _, cl := range cls {
		if !local {
			rz.Reset(w, h)
		} else {
			rz = vector.NewRasterizer(w, h)
		}
		rz.DrawOp = draw.Src
		addOutline(rz, cl.g, mapPt, offX, offY)
		var mask *image.Alpha
		if !local {
			mask = atlas.ensureAlpha(w, h)
			if mask == nil {
				return nil, false
			}
		} else {
			mask = image.NewAlpha(image.Rect(0, 0, w, h))
		}
		rz.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
		draw.DrawMask(acc, acc.Bounds(), image.NewUniform(cl.col),
			image.Point{}, mask, image.Point{}, draw.Over)
	}
	if oversize {
		out, dstW, dstH, s := downscaleGlyph(acc.Pix, w0, h0)
		if out == nil {
			return nil, false
		}
		return &rasterResult{data: out, w: dstW, h: dstH,
			left: scaleOffset(left, s), top: scaleOffset(-topEdge, s)}, true
	}
	return &rasterResult{data: acc.Pix, w: w, h: h, left: left, top: -topEdge}, true
}

// paletteColor resolves a CPAL entry to a straight-alpha color. The special
// index 0xFFFF ("use text foreground") resolves to fg, the run's text
// color; a fully transparent fg means no color was set, so it falls back
// to opaque black to keep the layer visible.
func paletteColor(palette []tables.ColorRecord, idx uint16, fg color.NRGBA) color.NRGBA {
	if idx == 0xFFFF {
		if fg.A == 0 {
			return color.NRGBA{A: 255}
		}
		return fg
	}
	if int(idx) >= len(palette) {
		return color.NRGBA{A: 255}
	}
	rec := palette[idx]
	return color.NRGBA{R: rec.Red, G: rec.Green, B: rec.Blue, A: rec.Alpha}
}

// shapeWith shapes text with a harfbuzz font borrowed from the face pool,
// scaled so positions are in 26.6 fixed-point pixels. The returned buffer is
// pooled; callers hand it back with releaseShapeBuffer when done reading
// Info/Pos (skipping the release is safe but forgoes reuse). A nil return
// means the size is unusable (see validRenderSize) or shaping produced
// nothing; callers already nil-check.
func shapeWith(cf *cachedFace, size float64, text string) *harfbuzz.Buffer {
	if cf == nil || !validRenderSize(size) {
		return nil
	}
	return shapeBuffer(cf, int32(math.Round(size*64)), text)
}

// clampBitmapDim keeps a bitmap dimension within the 1..MaxGlyphSize
// range. It is a backstop; the raster paths branch to downscaling
// before it could clip.
func clampBitmapDim(v int) int {
	if v < 1 {
		return 1
	}
	if v > MaxGlyphSize {
		return MaxGlyphSize
	}
	return v
}

// --- Pure-Go path stroker -------------------------------------------------
//
// FreeType's FT_Stroker is replaced here by flattening each glyph contour to
// a polyline and covering it with offset quads (the segment bodies) plus
// discs at every vertex (round joins/caps). All primitives are emitted with
// a single orientation so the vector rasterizer's nonzero winding rule
// unions them into one solid stroke band, radius = strokeR.

type vec2 struct{ x, y float64 }

// flattenTol is the maximum curve-to-chord deviation (device px) tolerated
// before a Bézier segment is subdivided. discSegs sets the round join/cap
// smoothness. flattenDepth caps subdivision recursion at 2^12 points per
// curve, which bounds the stroke primitive count for any single contour.
const (
	flattenTol   = 0.2
	discSegs     = 16
	flattenDepth = 12
)

// addStrokedOutline flattens one glyph's contours and strokes them with the
// given radius, emitting fill primitives into the rasterizer.
func addStrokedOutline(rz *vector.Rasterizer, g placedGlyph,
	mapPt func(placedGlyph, float32, float32) (float64, float64),
	offX, offY, radius float64) {

	pt := func(sp ot.SegmentPoint) vec2 {
		dx, dy := mapPt(g, sp.X, sp.Y)
		return vec2{dx + offX, dy + offY}
	}
	var contour []vec2
	var cur vec2
	flush := func() {
		emitStroke(rz, contour, radius)
		contour = contour[:0]
	}
	for si := range g.segs {
		seg := g.segs[si]
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			flush()
			cur = pt(seg.Args[0])
			contour = append(contour, cur)
		case ot.SegmentOpLineTo:
			cur = pt(seg.Args[0])
			contour = append(contour, cur)
		case ot.SegmentOpQuadTo:
			c := pt(seg.Args[0])
			e := pt(seg.Args[1])
			flattenQuad(cur, c, e, flattenDepth, &contour)
			cur = e
		case ot.SegmentOpCubeTo:
			c1 := pt(seg.Args[0])
			c2 := pt(seg.Args[1])
			e := pt(seg.Args[2])
			flattenCube(cur, c1, c2, e, flattenDepth, &contour)
			cur = e
		}
	}
	flush()
}

// emitStroke covers a closed polyline contour with segment quads and vertex
// discs at the given radius.
func emitStroke(rz *vector.Rasterizer, c []vec2, radius float64) {
	n := len(c)
	// Font contours are implicitly closed; drop a duplicated closing point.
	if n >= 2 && c[n-1] == c[0] {
		c = c[:n-1]
		n--
	}
	if n == 0 {
		return
	}
	if n == 1 {
		emitDisc(rz, c[0], radius)
		return
	}
	for i := 0; i < n; i++ {
		emitSeg(rz, c[i], c[(i+1)%n], radius)
	}
	for i := 0; i < n; i++ {
		emitDisc(rz, c[i], radius)
	}
}

// emitSeg emits the offset quad for one edge (a rectangle of half-thickness
// radius centered on the a->b segment).
func emitSeg(rz *vector.Rasterizer, a, b vec2, radius float64) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l < 1e-6 {
		return
	}
	nx, ny := -dy/l*radius, dx/l*radius
	emitPolygon(rz, []vec2{
		{a.x + nx, a.y + ny},
		{b.x + nx, b.y + ny},
		{b.x - nx, b.y - ny},
		{a.x - nx, a.y - ny},
	})
}

// emitDisc emits a regular polygon approximating a filled circle, used for
// round joins and caps. The polygon lives on the stack: stroking emits one
// disc per vertex, and a heap allocation per vertex showed up in profiles.
func emitDisc(rz *vector.Rasterizer, c vec2, radius float64) {
	var poly [discSegs]vec2
	for i := range poly {
		t := 2 * math.Pi * float64(i) / float64(discSegs)
		poly[i] = vec2{c.x + radius*math.Cos(t), c.y + radius*math.Sin(t)}
	}
	emitPolygon(rz, poly[:])
}

// emitPolygon feeds one closed polygon to the rasterizer, forcing a positive
// signed area so every primitive shares the same winding direction (required
// for the nonzero union of overlapping fills).
func emitPolygon(rz *vector.Rasterizer, p []vec2) {
	if len(p) < 3 {
		return
	}
	if signedArea(p) < 0 {
		for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
			p[i], p[j] = p[j], p[i]
		}
	}
	rz.MoveTo(float32(p[0].x), float32(p[0].y))
	for i := 1; i < len(p); i++ {
		rz.LineTo(float32(p[i].x), float32(p[i].y))
	}
	rz.ClosePath()
}

func signedArea(p []vec2) float64 {
	var a float64
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].x*p[j].y - p[j].x*p[i].y
	}
	return a / 2
}

// flattenQuad appends the flattened points (excluding the start point) of a
// quadratic Bézier to out.
func flattenQuad(p0, p1, p2 vec2, depth int, out *[]vec2) {
	if depth <= 0 || distToLine(p1, p0, p2) <= flattenTol {
		*out = append(*out, p2)
		return
	}
	p01 := mid(p0, p1)
	p12 := mid(p1, p2)
	p012 := mid(p01, p12)
	flattenQuad(p0, p01, p012, depth-1, out)
	flattenQuad(p012, p12, p2, depth-1, out)
}

// flattenCube appends the flattened points (excluding the start point) of a
// cubic Bézier to out.
func flattenCube(p0, p1, p2, p3 vec2, depth int, out *[]vec2) {
	if depth <= 0 ||
		(distToLine(p1, p0, p3) <= flattenTol && distToLine(p2, p0, p3) <= flattenTol) {
		*out = append(*out, p3)
		return
	}
	p01 := mid(p0, p1)
	p12 := mid(p1, p2)
	p23 := mid(p2, p3)
	p012 := mid(p01, p12)
	p123 := mid(p12, p23)
	p0123 := mid(p012, p123)
	flattenCube(p0, p01, p012, p0123, depth-1, out)
	flattenCube(p0123, p123, p23, p3, depth-1, out)
}

func mid(a, b vec2) vec2 { return vec2{(a.x + b.x) / 2, (a.y + b.y) / 2} }

// distToLine returns the perpendicular distance from p to the line a-b.
func distToLine(p, a, b vec2) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		return math.Hypot(p.x-a.x, p.y-a.y)
	}
	return math.Abs((p.x-a.x)*dy-(p.y-a.y)*dx) / l
}
