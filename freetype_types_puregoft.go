//go:build linux || darwin || windows

package glyph

import (
	"bytes"
	"container/list"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/harfbuzz"
)

// Color-glyph table tags. FreeType sets FT_FACE_FLAG_COLOR when a face
// carries any of these; the pure-Go path mirrors that by inspecting the
// table directory.
var colorTableTags = []ot.Tag{
	ot.MustNewTag("CBDT"), // color bitmap data (Noto Color Emoji)
	ot.MustNewTag("sbix"), // Apple color bitmaps
	ot.MustNewTag("COLR"), // layered color vector glyphs
	ot.MustNewTag("SVG "), // OpenType-SVG color glyphs
}

// cachedFace holds a parsed go-text face plus the cheap-to-read
// attributes the backend queries repeatedly. The parsed face is shared
// across ftFont values; its only mutable state is the bitmap ppem, set
// under mu by the color-glyph path (renderColorGlyph), so concurrent
// color renders across Contexts cannot corrupt each other's strike
// selection.
type cachedFace struct {
	mu    sync.Mutex // guards ppem-dependent access to face (SetPpem+GlyphData)
	face  *font.Face
	upem  uint16
	color bool
	// hbPool recycles HarfBuzz shaping fonts. NewFont builds the face's
	// shaping accelerators and is the single largest source of layout
	// allocations, so rebuilding one per shape (per line) is wasteful. A Font
	// carries lazy mutable state that Shape writes, so it cannot be shared
	// concurrently; the pool hands each shape an exclusively-owned font that
	// it returns when done, amortizing NewFont across shapes without a lock.
	hbPool sync.Pool
}

// getHB borrows a HarfBuzz font scaled to scale (26.6 px) from the pool,
// building one on a miss. The caller owns it until putHB and may Shape with
// it freely. scale is (re)applied on every borrow since a pooled font may
// have last been used at another size.
func (cf *cachedFace) getHB(scale int32) *harfbuzz.Font {
	f, _ := cf.hbPool.Get().(*harfbuzz.Font)
	if f == nil {
		f = harfbuzz.NewFont(cf.face)
	}
	f.XScale, f.YScale = scale, scale
	return f
}

// putHB returns a font borrowed from getHB. Safe to call after Shape: the
// results live on the Buffer, not the font.
func (cf *cachedFace) putHB(f *harfbuzz.Font) { cf.hbPool.Put(f) }

// faceCacheCap bounds the number of parsed faces kept resident. It must
// exceed the number of distinct fonts a session actually touches, or the LRU
// thrashes: evicting a still-needed face forces a full re-parse on its next
// glyph, and a Unicode-wide workload (e.g. ucs-detect, which pulls in ~180
// distinct system fallback fonts across every script) then re-parses fonts
// dozens of times, stalling the render thread for seconds. Sized well above
// that working set so every touched font is parsed once and retained. Resident
// memory is bounded by the fonts a session references, not by this cap —
// normal use touches a handful; the pathological Unicode sweep tops out around
// ~1.5GB (each parsed face holds its font's tables; see parseFace).
const faceCacheCap = 512

var faceCache = newFaceLRU(faceCacheCap)

// faceLRU is a bounded, concurrency-safe path→cachedFace cache. It caches
// misses (nil) too, so a bad path is not re-read on every glyph.
type faceLRU struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List // front = most recently used
	items map[string]*list.Element
}

type faceEntry struct {
	path string
	face *cachedFace // may be nil (negative cache)
}

func newFaceLRU(capacity int) *faceLRU {
	return &faceLRU{
		cap:   capacity,
		ll:    list.New(),
		items: make(map[string]*list.Element, capacity),
	}
}

// loadCachedFace parses (once) the font at path and caches it. Returns
// nil if the file cannot be read or parsed. Handles single fonts and
// collections (.ttc), using the first face of a collection to match the
// old FT_New_Face(path, 0) behavior.
func loadCachedFace(path string) *cachedFace {
	if path == "" {
		return nil
	}
	if cf, ok := faceCache.get(path); ok {
		return cf // may be nil: negative cache
	}
	cf := parseFace(path)
	faceCache.add(path, cf)
	return cf
}

// get returns the cached face for path (moving it to most-recently-used)
// and whether it was present.
func (c *faceLRU) get(path string) (*cachedFace, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[path]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*faceEntry).face, true
	}
	return nil, false
}

// add inserts path→cf, evicting the least-recently-used entry when over
// capacity. A concurrent add of the same path keeps the existing entry.
func (c *faceLRU) add(path string, cf *cachedFace) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[path]; ok {
		c.ll.MoveToFront(el)
		return
	}
	c.items[path] = c.ll.PushFront(&faceEntry{path: path, face: cf})
	for c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*faceEntry).path)
	}
}

func parseFace(path string) (cf *cachedFace) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Use the first loader so single fonts and collections (.ttc) both
	// resolve to face index 0, matching FT_New_Face(path, 0).
	loaders, err := ot.NewLoaders(bytes.NewReader(data))
	if err != nil || len(loaders) == 0 {
		return nil
	}
	ld := loaders[0]

	defer func() {
		if r := recover(); r != nil {
			cf = nil // font parser panicked (e.g. malformed CFF2)
		}
	}()

	ft, err := font.NewFont(ld)
	if err != nil {
		return nil
	}
	face := font.NewFace(ft)
	return &cachedFace{
		face:  face,
		upem:  face.Upem(),
		color: hasColorTable(ld),
	}
}

// hasColorTable reports whether the font's table directory contains a
// color-glyph table (the pure-Go FT_FACE_FLAG_COLOR equivalent).
func hasColorTable(ld *ot.Loader) bool {
	tables := ld.Tables()
	for _, want := range colorTableTags {
		for _, have := range tables {
			if have == want {
				return true
			}
		}
	}
	return false
}

// ftFont wraps a go-text face + a harfbuzz shaping font scaled to a
// pixel size. It mirrors the method set of the cgo ftFont so the shared
// layout code (layout_ft.go) is platform-agnostic.
type ftFont struct {
	face  *font.Face // parsed face; nil means "failed to open"
	upem  uint16
	size  float64 // physical pixel size (already includes scaleFactor)
	scale int32   // 26.6 shaping scale = round(size*64)
	path  string  // file path the face was loaded from (by-glyph-id render)
	cf    *cachedFace
}

// newFTFont creates a shaping font from a TextStyle, trying the
// requested style then falling back (regular → sans-serif).
func newFTFont(_ FTLibrary, fontPaths map[string]string,
	style TextStyle, scaleFactor float32) ftFont {

	family, size, bold, italic := resolveFTFontParams(style, scaleFactor)
	for _, path := range fontFallbackPaths(fontPaths, family, bold, italic) {
		if f := openFTFont(path, size); f.face != nil {
			return f
		}
	}
	return ftFont{}
}

// newFTFontFromPath creates a shaping font from a file path and size
// (in physical pixels).
func newFTFontFromPath(_ FTLibrary, path string, fontSize float64) ftFont {
	return openFTFont(path, fontSize)
}

// openFTFont builds an ftFont from a cached face at the given pixel size.
func openFTFont(path string, size float64) ftFont {
	cf := loadCachedFace(path)
	if cf == nil {
		return ftFont{}
	}
	// Scale so shaped positions come out in 26.6 fixed-point pixels
	// (output = fontUnit * scale / upem); dividing by 64 yields pixels,
	// matching the old hb_ft advance convention.
	return ftFont{
		face:  cf.face,
		upem:  cf.upem,
		size:  size,
		scale: int32(math.Round(size * 64)),
		path:  path,
		cf:    cf,
	}
}

// close drops the face references. The parsed face and its shaping-font pool
// stay cached.
func (f *ftFont) close() {
	f.face = nil
	f.cf = nil
}

// shapeRunes shapes text and returns the harfbuzz buffer. Caller reads
// buf.Info / buf.Pos. Returns nil if the font is not loaded. The shaping
// font is borrowed from the face pool for the duration of the shape only.
func (f ftFont) shape(text string) *harfbuzz.Buffer {
	return shapeBuffer(f.cf, f.scale, text)
}

// shapeBuffer shapes text with a HarfBuzz font borrowed from cf's pool at the
// given 26.6 scale, returning the buffer (nil when cf is unset or text empty).
// The font is returned to the pool after a clean shape and discarded after a
// recovered panic.
func shapeBuffer(cf *cachedFace, scale int32, text string) *harfbuzz.Buffer {
	if cf == nil || len(text) == 0 {
		return nil
	}
	buf := harfbuzz.NewBuffer()
	runes := []rune(text)
	buf.AddRunes(runes, 0, len(runes))
	buf.GuessSegmentProperties()
	hb := cf.getHB(scale)
	if !safeShape(buf, hb) {
		return nil // discard hb: a recovered panic may have left it corrupt
	}
	cf.putHB(hb)
	return buf
}

// safeShape runs HarfBuzz shaping, recovering from panics inside go-text's
// shaper. typesetting v0.3.4 indexes out of range in its AAT morx
// glyph-deletion path for some macOS system fonts (e.g. GeezaPro with an
// Arabic combining mark, Raanana/NewPeninimMT with a Hebrew presentation
// form). A panicking font is reported as unable to shape the text so callers
// fall through to the next fallback font or to text-based rendering, rather
// than crashing the host process. Returns false on panic.
func safeShape(buf *harfbuzz.Buffer, hb *harfbuzz.Font) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	buf.Shape(hb, nil)
	return true
}

// hasGlyphs reports whether the font can shape all of text with no
// missing (.notdef, GID 0) glyphs.
func (f ftFont) hasGlyphs(text string) bool {
	buf := f.shape(text)
	if buf == nil || len(buf.Info) == 0 {
		return false
	}
	for i := range buf.Info {
		if buf.Info[i].Glyph == 0 {
			return false
		}
	}
	return true
}

// covers reports whether the face has a glyph for every rune in text, using
// a cmap lookup rather than shaping. Script-fallback selection asks this
// question for every cluster against many fonts; a full HarfBuzz shape per
// probe is the dominant cost when laying out text the base font lacks (mixed
// scripts, symbols, box-drawing). cmap coverage is the standard signal for
// "does this font support this codepoint" and is orders of magnitude cheaper.
func (f ftFont) covers(text string) bool {
	if f.face == nil {
		return false
	}
	return faceCovers(f.face, text)
}

// faceCovers reports whether face's cmap maps every non-ignorable rune in
// text. Default-ignorable codepoints (variation selectors, joiners, bidi
// marks) are treated as covered: shapers drop them, so they must not force a
// fallback. A cluster of only ignorables counts as covered.
func faceCovers(face *font.Face, text string) bool {
	for _, r := range text {
		if isDefaultIgnorable(r) {
			continue
		}
		if _, ok := face.NominalGlyph(r); !ok {
			return false
		}
	}
	return true
}

// coverage holds the cheap-to-build subset of a font that script-fallback
// probing needs: its cmap (rune→glyph mapping) and whether it is a color
// font. Building it skips font.NewFont's shaping accelerators (GSUB/GPOS/morx)
// and, because ProcessCmap copies, retains none of the font-file bytes — so
// the entire fallback set stays resident for a few tens of KB each. The
// heavyweight cachedFace is loaded only for fonts actually selected to shape.
type coverage struct {
	cmap  font.Cmap
	color bool
}

// covers reports whether the cmap maps every non-ignorable rune in text,
// mirroring faceCovers against a bare cmap (no parsed face).
func (c *coverage) covers(text string) bool {
	for _, r := range text {
		if isDefaultIgnorable(r) {
			continue
		}
		if _, ok := c.cmap.Lookup(r); !ok {
			return false
		}
	}
	return true
}

// coverageMap is a resident path→coverage cache. Unlike faceCache it does not
// evict: the fallback set is fixed at font discovery and a single probe scans
// all of it, so a bounded cache would thrash — re-reading fonts from disk on
// every uncovered cluster, which is what stalled ucs-detect once the general
// fallback tier pushed the set past the face-cache cap. Each entry is a bare
// cmap, so holding the whole set is cheap. nil is cached for unparseable fonts.
type coverageMap struct {
	mu    sync.Mutex
	items map[string]*coverage
}

var coverageCache = &coverageMap{items: make(map[string]*coverage)}

// loadCoverage builds (once) the cmap coverage for the font at path. Returns
// nil if the file cannot be read or parsed; the nil is cached too so a bad
// path is not re-read on every probe. The file read + parse happens outside
// the lock so concurrent probes of distinct fonts do not serialize.
func loadCoverage(path string) *coverage {
	if path == "" {
		return nil
	}
	coverageCache.mu.Lock()
	cov, ok := coverageCache.items[path]
	coverageCache.mu.Unlock()
	if ok {
		return cov // may be nil: negative cache
	}
	cov = parseCoverage(path)
	coverageCache.mu.Lock()
	coverageCache.items[path] = cov
	coverageCache.mu.Unlock()
	return cov
}

// parseCoverage reads the font at path and extracts its cmap and color flag
// without building the full shaping face. It mirrors the cmap handling in
// font.NewFont (OS/2 FontPage steers legacy-Arabic subtable selection) and
// recovers from parser panics like parseFace, so a defective font is skipped.
func parseCoverage(path string) (cov *coverage) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	loaders, err := ot.NewLoaders(bytes.NewReader(data))
	if err != nil || len(loaders) == 0 {
		return nil
	}
	ld := loaders[0]

	defer func() {
		if r := recover(); r != nil {
			cov = nil // font parser panicked (e.g. malformed cmap/CFF2)
		}
	}()

	raw, err := ld.RawTable(ot.MustNewTag("cmap"))
	if err != nil {
		return nil
	}
	tb, _, err := tables.ParseCmap(raw)
	if err != nil {
		return nil
	}
	os2raw, _ := ld.RawTable(ot.MustNewTag("OS/2"))
	os2, _, _ := tables.ParseOs2(os2raw)
	cm, _, err := font.ProcessCmap(tb, os2.FontPage())
	if err != nil {
		return nil
	}
	return &coverage{cmap: cm, color: hasColorTable(ld)}
}

// orderTextFallbacks partitions fallbackPaths into the fonts that cover text,
// split into monochrome and color fonts, each kept in fallback order. It is the
// shared text-presentation selection policy used by both layout-time selection
// (probeFallback) and the render-side text path (loadGlyphFT/loadStrokedGlyphFT):
// prefer a monochrome glyph and fall back to a color font only when nothing
// monochrome covers the text — so a default-text symbol does not resolve to a
// color-emoji font that merely sorts earlier. Coverage is a cmap lookup via the
// resident coverage cache (no shaping, no face parse), matching how the base
// coverage check decides.
func orderTextFallbacks(fallbackPaths []string, text string) (mono, color []string) {
	for _, path := range fallbackPaths {
		cov := loadCoverage(path)
		if cov == nil || !cov.covers(text) {
			continue
		}
		if cov.color {
			color = append(color, path)
		} else {
			mono = append(mono, path)
		}
	}
	return mono, color
}

// isDefaultIgnorable reports whether r is a Unicode default-ignorable code
// point (the common ranges), which shapers omit from output.
func isDefaultIgnorable(r rune) bool {
	switch {
	case r == 0x00AD, r == 0xFEFF: // soft hyphen, ZWNBSP/BOM
		return true
	case r >= 0x200B && r <= 0x200F: // ZW space, ZWNJ/ZWJ, LRM/RLM
		return true
	case r >= 0x2060 && r <= 0x206F: // word joiner, invisible ops, bidi
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r >= 0xE0000 && r <= 0xE0FFF: // tags + variation-selector supplement
		return true
	}
	return false
}

// isColorFont reports whether the face carries color bitmap glyphs
// (CBDT/CBLC/sbix emoji fonts).
func (f ftFont) isColorFont() bool {
	return f.cf != nil && f.cf.color
}

// metrics returns ascent, descent, leading in physical pixels.
func (f ftFont) metrics() (ascent, descent, leading float64) {
	if f.face == nil {
		return 0, 0, 0
	}
	scale := f.size / float64(nonZeroUpem(f.upem))
	ext, ok := f.face.FontHExtents()
	if !ok {
		// Fonts lacking hhea/OS-2 typo metrics: approximate.
		return f.size * 0.8, f.size * 0.2, 0
	}
	ascent = float64(ext.Ascender) * scale
	descent = -float64(ext.Descender) * scale
	leading = float64(ext.LineGap) * scale
	if leading < 0 {
		leading = 0
	}
	return ascent, descent, leading
}

// measureString returns the shaped advance width of text in pixels.
func (f ftFont) measureString(text string) float64 {
	buf := f.shape(text)
	if buf == nil {
		return 0
	}
	var w float64
	for i := range buf.Pos {
		w += float64(buf.Pos[i].XAdvance) / 64.0
	}
	return w
}

func nonZeroUpem(upem uint16) uint16 {
	if upem == 0 {
		return 1000
	}
	return upem
}

func resolveFTFontParams(style TextStyle, scaleFactor float32) (
	family string, size float64, bold, italic bool,
) {
	family = resolveFontFamily(style.FontName)

	rawSize := style.Size
	if rawSize <= 0 {
		rawSize = parseSizeFromFontName(style.FontName)
	}
	if rawSize <= 0 {
		rawSize = 16
	}
	size = float64(rawSize) * float64(scaleFactor)

	bold = style.Typeface == TypefaceBold ||
		style.Typeface == TypefaceBoldItalic
	italic = style.Typeface == TypefaceItalic ||
		style.Typeface == TypefaceBoldItalic

	lower := strings.ToLower(style.FontName)
	if !bold && strings.Contains(lower, " bold") {
		bold = true
	}
	if !italic && strings.Contains(lower, " italic") {
		italic = true
	}

	return family, size, bold, italic
}

// fontFallbackPaths returns a list of font paths to try in order:
// requested style → regular variant → sans-serif fallback.
func fontFallbackPaths(fontPaths map[string]string,
	family string, bold, italic bool) []string {

	primary := resolveFontPath(fontPaths, family, bold, italic)
	paths := []string{primary}

	if bold || italic {
		regular := resolveFontPath(fontPaths, family, false, false)
		if regular != primary {
			paths = append(paths, regular)
		}
	}

	if fallback := genericFallback(fontPaths, family); fallback != "" {
		if paths[len(paths)-1] != fallback {
			paths = append(paths, fallback)
		}
	}
	return paths
}

// resolveFontFamily maps generic font names to concrete platform font
// families; it is defined per-OS (see discover_linux.go / discover_android.go).

// resolveFontPath finds the .ttf/.otf path for a family+style combo.
func resolveFontPath(fontPaths map[string]string,
	family string, bold, italic bool) string {

	// Try style-specific keys first.
	suffix := ""
	if bold && italic {
		suffix = "-BoldItalic"
	} else if bold {
		suffix = "-Bold"
	} else if italic {
		suffix = "-Italic"
	} else {
		suffix = "-Regular"
	}

	if p, ok := fontPaths[family+suffix]; ok {
		return p
	}
	// Fall back to regular variant.
	if p, ok := fontPaths[family+"-Regular"]; ok {
		return p
	}
	// Fall back to bare family name.
	if p, ok := fontPaths[family]; ok {
		return p
	}
	// Last resort: the matching generic alias. A monospace request must not
	// degrade to a proportional font, so prefer "monospace" over "sans-serif"
	// when the family name denotes a fixed-width face (e.g. a Nerd Font whose
	// go-text family name differs from the requested spelling).
	return genericFallback(fontPaths, family)
}

// genericFallback returns the generic-alias path to use when no concrete
// family match was found: "monospace" for monospace-looking names, otherwise
// "sans-serif". Empty if neither alias is registered.
func genericFallback(fontPaths map[string]string, family string) string {
	if looksMonospace(family) {
		if p, ok := fontPaths["monospace"]; ok {
			return p
		}
	}
	if p, ok := fontPaths["sans-serif"]; ok {
		return p
	}
	return ""
}

// looksMonospace reports whether a requested family name denotes a
// fixed-width font, so an unresolved lookup falls back to the "monospace"
// generic alias rather than proportional "sans-serif".
func looksMonospace(family string) bool {
	l := strings.ToLower(family)
	for _, s := range []string{"mono", "consol", "courier", "menlo", "fixed"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
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
	for _, c := range last {
		if c >= '0' && c <= '9' {
			sz = sz*10 + float32(c-'0')
		} else if c == '.' {
			break
		} else {
			return 0
		}
	}
	return sz
}

// parseFamilyFromFontName extracts the family portion from a Pango
// font name, stripping trailing size and style keywords.
func parseFamilyFromFontName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}

	end := len(parts)
	if sz := parseSizeFromFontName(name); sz > 0 {
		end--
	}

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
		end = 1
	}
	return strings.Join(parts[:end], " ")
}
