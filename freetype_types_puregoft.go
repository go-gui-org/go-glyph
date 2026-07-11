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
}

// faceCacheCap bounds the number of parsed faces kept resident. Real
// workloads touch far fewer fonts than this; the cap only guards against
// unbounded growth (e.g. many AddFontFile paths), evicting least-recently
// used entries. A shared global cache mirrors the render-side singletons.
const faceCacheCap = 128

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

func parseFace(path string) *cachedFace {
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
	face *font.Face     // parsed face; nil means "failed to open"
	hb   *harfbuzz.Font // shaping font, scaled to size in 26.6 px
	upem uint16
	size float64 // physical pixel size (already includes scaleFactor)
	path string  // file path the face was loaded from (by-glyph-id render)
	cf   *cachedFace
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
	hb := harfbuzz.NewFont(cf.face)
	// Scale so shaped positions come out in 26.6 fixed-point pixels
	// (output = fontUnit * scale / upem); dividing by 64 yields pixels,
	// matching the old hb_ft advance convention.
	s := int32(math.Round(size * 64))
	hb.XScale, hb.YScale = s, s
	return ftFont{
		face: cf.face,
		hb:   hb,
		upem: cf.upem,
		size: size,
		path: path,
		cf:   cf,
	}
}

// close releases the shaping font. The parsed face stays cached.
func (f *ftFont) close() {
	f.face = nil
	f.hb = nil
	f.cf = nil
}

// shapeRunes shapes text and returns the harfbuzz buffer. Caller reads
// buf.Info / buf.Pos. Returns nil if the font is not loaded.
func (f ftFont) shape(text string) *harfbuzz.Buffer {
	if f.hb == nil || len(text) == 0 {
		return nil
	}
	buf := harfbuzz.NewBuffer()
	runes := []rune(text)
	buf.AddRunes(runes, 0, len(runes))
	buf.GuessSegmentProperties()
	buf.Shape(f.hb, nil)
	return buf
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

	if fallback, ok := fontPaths["sans-serif"]; ok {
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
	// Last resort: whatever sans-serif resolved to.
	if p, ok := fontPaths["sans-serif"]; ok {
		return p
	}
	return ""
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
