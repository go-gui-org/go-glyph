//go:build linux || darwin || windows

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

// This file is the pure-Go replacement for the cgo rasterizer on Linux,
// Android, macOS, and Windows. Shaping/rasterization are shared; only font
// discovery (see discover_*.go) differs by platform.
// Shaping comes from go-text/typesetting/harfbuzz; monochrome glyphs are
// rasterized with golang.org/x/image/vector; color-emoji glyphs decode
// their embedded bitmaps (CBDT/sbix PNG); stroked text uses the pure-Go
// path stroker (see loadStrokedGlyphFT and addStrokedOutline).

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

// loadStrokedGlyphFT renders a stroked cluster using the pure-Go stroker
// (see addStrokedOutline). It mirrors the cgo FT_Stroker path: the stroke
// radius equals strokeWidth and joins/caps are round. Color-emoji items
// are not stroked; they fall back to the plain color bitmap.
func loadStrokedGlyphFT(atlas *GlyphAtlas, ch string, item Item,
	strokeWidth float32, subpixelBin int,
	scaleFactor float32) (LoadGlyphResult, error) {

	if strokeWidth <= 0 {
		return loadGlyphFT(atlas, ch, item, subpixelBin, scaleFactor)
	}

	family, fontSize, bold, italic := resolveFTFontParams(item.Style, scaleFactor)
	paths := fontFallbackPaths(ftFontPathsSingleton, family, bold, italic)
	subpixelShift := float64(subpixelBin) / 4.0
	sw := float64(strokeWidth)

	var res *rasterResult

	// Primary font paths (reject .notdef so fallbacks get a turn).
	for _, fontPath := range paths {
		r, _ := renderStrokedRun(fontPath, fontSize, sw, subpixelShift, ch, true)
		if r != nil {
			res = r
			break
		}
	}

	// Script fallback fonts if the primary font lacked the glyph.
	if res == nil {
		for _, fp := range ftScriptFallbacksSingleton {
			r, _ := renderStrokedRun(fp, fontSize, sw, subpixelShift, ch, true)
			if r != nil {
				res = r
				break
			}
		}
	}

	// Last resort: stroke with the primary font even if it is tofu.
	if res == nil && len(paths) > 0 {
		res, _ = renderStrokedRun(paths[0], fontSize, sw, subpixelShift, ch, false)
	}

	return insertRaster(atlas, res)
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

// renderMonoRun rasterizes filled outline glyphs (no stroke).
func renderMonoRun(path string, size, subpixelShift float64,
	text string, rejectNotdef bool) (*rasterResult, bool) {
	return renderRun(path, size, 0, subpixelShift, text, rejectNotdef)
}

// renderStrokedRun rasterizes the stroke border of the outline glyphs at
// the given stroke radius, using the pure-Go stroker.
func renderStrokedRun(path string, size, strokeWidth, subpixelShift float64,
	text string, rejectNotdef bool) (*rasterResult, bool) {
	return renderRun(path, size, strokeWidth, subpixelShift, text, rejectNotdef)
}

// renderRun shapes text with the given font and rasterizes the resulting
// outline glyphs into a white+alpha RGBA bitmap. When strokeWidth > 0 the
// glyph contours are stroked (round joins/caps, radius = strokeWidth)
// instead of filled. Returns (nil, true) when rejectNotdef is set and any
// glyph is missing.
func renderRun(path string, size, strokeWidth, subpixelShift float64,
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
	strokeR := strokeWidth

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
	// Stroking grows the ink by the stroke radius on every side.
	if strokeR > 0 {
		minDx -= strokeR
		minDy -= strokeR
		maxDx += strokeR
		maxDy += strokeR
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
		if strokeR > 0 {
			addStrokedOutline(rz, g, mapPt, offX, offY, strokeR)
		} else {
			addOutline(rz, g, mapPt, offX, offY)
		}
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

	buf := shapeWith(cf, size, text)
	if buf == nil || len(buf.Info) == 0 {
		return nil, false
	}
	gid := buf.Info[0].Glyph
	if gid == 0 {
		return nil, false
	}

	// SetPpem selects the bitmap strike and mutates the shared face, so the
	// SetPpem+GlyphData pair must be atomic against other color renders of
	// the same cached face (possibly on another Context/goroutine).
	ppem := uint16(size + 0.5)
	if ppem == 0 {
		ppem = 1
	}
	cf.mu.Lock()
	cf.face.SetPpem(ppem, ppem)
	gd := cf.face.GlyphData(gid)
	cf.mu.Unlock()

	gb, ok := gd.(font.GlyphBitmap)
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
	w, h = clampBitmapDim(w), clampBitmapDim(h)
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
// smoothness. flattenDepth caps subdivision recursion.
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
// round joins and caps.
func emitDisc(rz *vector.Rasterizer, c vec2, radius float64) {
	poly := make([]vec2, discSegs)
	for i := 0; i < discSegs; i++ {
		t := 2 * math.Pi * float64(i) / float64(discSegs)
		poly[i] = vec2{c.x + radius*math.Cos(t), c.y + radius*math.Sin(t)}
	}
	emitPolygon(rz, poly)
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
