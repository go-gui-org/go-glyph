//go:build linux || darwin || windows

package glyph

import (
	"container/list"
	"math"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

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
// across ftFont values; its mutable state is the bitmap ppem (set under
// mu by the color-glyph path) and the extent cache GlyphExtents fills
// (read under mu by capEm), so concurrent renders/measurements across
// Contexts cannot corrupt each other's strike selection or measurements.
type cachedFace struct {
	mu    sync.Mutex // guards ppem-dependent access to face (SetPpem+GlyphData)
	face  *font.Face
	upem  uint16
	color bool
	// cap is the face's cap height in em units, measured lazily by capEm for
	// the fallback fit scale; capMeasured distinguishes "not yet measured"
	// from "measured, this face has no cap height".
	cap         float64
	capMeasured bool
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
// that working set so every touched font is parsed once and retained.
const faceCacheCap = 512

// faceCacheBudget bounds the resident face bytes instead. Each parsed face
// holds its font's tables (see parseFace), so the count cap alone lets a
// Unicode-wide session retain ~1.5GB; the budget evicts by bytes once the
// realistic hot set is resident. That hot set is larger than it looks: the
// macOS Apple Color Emoji TTC alone is 192MB, a CJK TTC ~56MB, and the base
// face plus a few script faces add tens of MB — roughly 260-320MB total. A
// budget below that thrashes the render path: every emoji glyph in a
// scrolled buffer then re-parses the 192MB TTC (tens of ms each, seconds
// per frame), so the budget must hold the emoji+CJK hot set or the terminal
// freezes while scrolling through mixed-script output. Count cap and budget
// both apply; the budget tames the pathological sweep. It is intentionally
// not sized to hold the full ucs-detect working set — evicted faces re-parse
// on re-touch, which the 8192-entry layout cache in front of the text path
// mostly absorbs. Post-session RSS is bounded separately by the embedder's
// soft memory limit, and the idle sweeper (faceIdleAge) releases the whole
// retained set once the terminal has been quiet for minutes, so a large
// budget does not keep the long-term number high.
const faceCacheBudget = 384 << 20

var faceCache = newFaceLRU(faceCacheCap, faceCacheBudget)

// faceIdleAge is how long a face may go untouched before the background
// sweeper evicts it, and faceSweepInterval how often the sweeper runs. The
// byte budget alone bounds retention only while new faces keep arriving; a
// session that loaded a big working set (e.g. a full ucs-detect sweep) would
// otherwise hold the budget's worth of faces forever, keeping RSS high long
// after the work is done. Idle eviction returns that memory to the OS once
// the terminal sits quiet; a re-touched face re-parses in tens of ms, so the
// cost is a single frame after minutes of inactivity. Vars, not consts, so
// tests can shorten them; the sweeper re-reads both every cycle.
var (
	faceIdleAge       = 5 * time.Minute
	faceSweepInterval = time.Minute
)

// faceSweeperOnce starts the idle sweeper exactly once per process.
var faceSweeperOnce sync.Once

// startFaceSweeper launches a background goroutine that evicts faces the
// render thread has not touched for faceIdleAge. The render thread alone
// cannot drive this: an idle terminal performs no inserts, so a sweep
// throttled to inserts would never fire. The LRU mutex makes the sweep safe
// against concurrent loads and rasterization; an in-flight render holds its
// own *cachedFace pointer, which the GC keeps alive regardless of eviction.
//
// After an eviction it forces a collection with debug.FreeOSMemory. Without
// it the freed table buffers would sit in the heap until the next
// allocation-driven GC — minutes away at idle — and the pooled HarfBuzz
// fonts each face carried survive one extra GC via sync.Pool's victim list,
// pinning the tables another cycle. FreeOSMemory makes the RSS decay
// deterministic. It only runs when something was actually evicted, which by
// definition means the terminal has been quiet for faceIdleAge, so the
// stop-the-world pause cannot hit an active render.
func startFaceSweeper() {
	faceSweeperOnce.Do(func() {
		go func() {
			for {
				time.Sleep(faceSweepInterval)
				if faceCache.evictIdle() > 0 {
					// One collection is not enough: the pooled HarfBuzz fonts
					// each face carried survive a single GC via sync.Pool's
					// victim list, pinning the face's table buffers one extra
					// cycle (measured: ~200MB still inuse after one GC, ~6MB
					// after two). FreeOSMemory forces a GC each call, so two
					// calls release the tables and return the pages to the OS.
					debug.FreeOSMemory()
					debug.FreeOSMemory()
				}
			}
		}()
	})
}

// faceLRU is a bounded, concurrency-safe path→cachedFace cache. It caches
// misses (nil) too, so a bad path is not re-read on every glyph.
type faceLRU struct {
	mu     sync.Mutex
	cap    int
	budget int64      // max sum of resident entry sizes; 0 disables the byte bound
	used   int64      // sum of entry sizes currently resident
	ll     *list.List // front = most recently used
	items  map[string]*list.Element
}

type faceEntry struct {
	path      string
	face      *cachedFace // may be nil (negative cache)
	size      int64       // retained table bytes, for the budget
	lastTouch int64       // UnixMilli of last get/add; drives idle eviction
}

func newFaceLRU(capacity int, budget int64) *faceLRU {
	return &faceLRU{
		cap:    capacity,
		budget: budget,
		ll:     list.New(),
		items:  make(map[string]*list.Element, capacity),
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
	cf, size := parseFace(path)
	faceCache.add(path, cf, size)
	return cf
}

// get returns the cached face for path (moving it to most-recently-used)
// and whether it was present.
func (c *faceLRU) get(path string) (*cachedFace, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[path]; ok {
		c.touch(el)
		return el.Value.(*faceEntry).face, true
	}
	return nil, false
}

// touch marks an entry as used now, moving it to the front of the recency
// list. Caller holds mu.
func (c *faceLRU) touch(el *list.Element) {
	c.ll.MoveToFront(el)
	el.Value.(*faceEntry).lastTouch = time.Now().UnixMilli()
}

// add inserts path→cf, evicting the least-recently-used entry when over
// capacity or over the byte budget. A concurrent add of the same path keeps
// the existing entry. The newly added entry is never evicted by its own size:
// a single face bigger than the whole budget (e.g. a multi-hundred-MB color
// emoji TTC) must stay resident or every glyph would re-parse it.
func (c *faceLRU) add(path string, cf *cachedFace, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[path]; ok {
		c.touch(el)
		return
	}
	c.items[path] = c.ll.PushFront(&faceEntry{
		path:      path,
		face:      cf,
		size:      size,
		lastTouch: time.Now().UnixMilli(),
	})
	c.used += size
	// Over-budget eviction keeps the freshest entry even when it alone
	// exceeds the budget; capacity eviction always applies.
	for c.ll.Len() > c.cap ||
		(c.budget > 0 && c.used > c.budget && c.ll.Len() > 1) {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		ent := oldest.Value.(*faceEntry)
		delete(c.items, ent.path)
		c.used -= ent.size
	}
}

// evictIdle removes every entry untouched for faceIdleAge and returns the
// bytes freed. Unlike the budget-based eviction, this one is driven by
// wall-clock time, so the retention a burst session built up decays once the
// terminal sits quiet — the OS reclaims the freed face tables and RSS returns
// to the pre-session baseline. A face still in active use is untouched by
// definition.
func (c *faceLRU) evictIdle() (freed int64) {
	cutoff := time.Now().UnixMilli() - int64(faceIdleAge/time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	for el := c.ll.Front(); el != nil; {
		next := el.Next()
		if el.Value.(*faceEntry).lastTouch < cutoff {
			c.ll.Remove(el)
			ent := el.Value.(*faceEntry)
			delete(c.items, ent.path)
			c.used -= ent.size
			freed += ent.size
		}
		el = next
	}
	return freed
}

func parseFace(path string) (cf *cachedFace, size int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	// The face retains its font's tables (the loader's table buffers and the
	// lazy per-table parses), so the file size is the closest cheap proxy for
	// resident bytes. Slight overestimate for .ttc collections (face 0 only),
	// which evicts earlier — the safe direction. On parse failure the entry
	// is a negative cache hit; it costs nothing, so it gets size 0.
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	// Use the first loader so single fonts and collections (.ttc) both
	// resolve to face index 0, matching FT_New_Face(path, 0).
	loaders, err := ot.NewLoaders(f)
	if err != nil || len(loaders) == 0 {
		return nil, 0
	}
	ld := loaders[0]

	defer func() {
		if r := recover(); r != nil {
			cf = nil // font parser panicked (e.g. malformed CFF2)
			size = 0
		}
	}()

	ft, err := font.NewFont(ld)
	if err != nil {
		return nil, 0
	}
	face := font.NewFace(ft)
	return &cachedFace{
		face:  face,
		upem:  face.Upem(),
		color: hasColorTable(ld),
	}, size
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
	face *font.Face // parsed face; nil means "failed to open"
	cf   *cachedFace
	path string  // file path the face was loaded from (by-glyph-id render)
	size float64 // physical pixel size (already includes scaleFactor)
	// fit is the cap-height match factor already folded into size for a
	// fallback face (see fallbackFitScale). 0 means none was applied. The
	// renderer needs it to rasterize at the size layout shaped with, since it
	// re-derives the size from the item's style rather than from size here.
	fit   float64
	scale int32 // 26.6 shaping scale = round(size*64)
	upem  uint16
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

// shape shapes text and returns a pooled harfbuzz buffer. Caller reads
// buf.Info / buf.Pos and hands the buffer back with releaseShapeBuffer when
// done (nil is safe to release). Returns nil if the font is not loaded. The
// shaping font is borrowed from the face pool for the duration of the shape
// only.
func (f ftFont) shape(text string) *harfbuzz.Buffer {
	return shapeBuffer(f.cf, f.scale, text)
}

// shapeBufPool recycles HarfBuzz buffers across shapes. NewBuffer starts with
// empty Info/Pos slices that regrow from zero on every shape, and each fresh
// buffer also starts with an empty shape-plan cache, so a plan is rebuilt per
// shape. Reusing buffers keeps the grown slices and the plan cache (keyed by
// face, so correct across fonts), turning a steady per-layout allocation
// stream into a handful of long-lived buffers. The pool is shared across
// Contexts; sync.Pool guarantees exclusive ownership between Get and Put, so
// a borrowed buffer is never touched by another goroutine.
//
// Caveat: the per-buffer plan cache matches on segment properties and user
// features but NOT variation coords. That is safe only while this path shapes
// with Shape(hb, nil) and never applies variation coords to the pooled
// harfbuzz.Fonts; if variable-font support is added here, pooled buffers must
// stop caching plans (or the fork's plan.equal must learn about coords).
var shapeBufPool = sync.Pool{
	New: func() any { return harfbuzz.NewBuffer() },
}

// shapeBufMaxRetain caps the glyph capacity a buffer may keep when returned
// to the pool. A pathological single run (multi-megabyte paste shaped as one
// span) grows Info/Pos to millions of entries; pooling that buffer would pin
// the growth until the GC drains the pool. Oversized buffers are dropped and
// reallocated small on the next borrow. 4096 glyphs ≈ a few hundred KB per
// pooled buffer, far above any real layout run.
const shapeBufMaxRetain = 4096

// releaseShapeBuffer returns a buffer obtained from shape / shapeBuffer /
// shapeWith to the pool. Safe on nil (shaping failure paths release
// unconditionally). The caller must not touch buf.Info / buf.Pos afterwards:
// the next borrower reuses the same backing arrays.
func releaseShapeBuffer(buf *harfbuzz.Buffer) {
	if buf == nil {
		return
	}
	if cap(buf.Info) > shapeBufMaxRetain {
		return // drop oversized buffers; see shapeBufMaxRetain
	}
	// Clear resets content and segment properties (set per shape by
	// GuessSegmentProperties) while keeping Info/Pos capacity and the
	// shape-plan cache — exactly the state worth recycling. maxOps needs no
	// reset here: the shaper restores it at the end of every clean shape, and
	// a panicked shape never reaches this release (the buffer is discarded).
	buf.Clear()
	shapeBufPool.Put(buf)
}

// shapeBuffer shapes text with a HarfBuzz font borrowed from cf's pool at the
// given 26.6 scale, returning a pooled buffer (nil when cf is unset or text
// empty) that the caller releases via releaseShapeBuffer. The font is returned
// to its pool after a clean shape; after a recovered panic both the font and
// the buffer are discarded, since the panic may have left either corrupt
// mid-shape.
func shapeBuffer(cf *cachedFace, scale int32, text string) *harfbuzz.Buffer {
	if cf == nil || len(text) == 0 {
		return nil
	}
	buf := shapeBufPool.Get().(*harfbuzz.Buffer)
	// Feeding runes straight off the string is equivalent to the former
	// AddRunes([]rune(text), 0, n) — same appends, same cluster values (rune
	// index) — without materializing a throwaway []rune conversion slice
	// (AddRunes copies into the buffer, so the slice was garbage immediately).
	i := 0
	for _, r := range text {
		buf.AddRune(r, i)
		i++
	}
	buf.GuessSegmentProperties()
	hb := cf.getHB(scale)
	if !safeShape(buf, hb) {
		return nil // discard hb AND buf: neither returns to its pool
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
	if buf == nil {
		return false
	}
	defer releaseShapeBuffer(buf)
	if len(buf.Info) == 0 {
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
	// iconScore counts the iconProbeRunes this font maps (see
	// scoreIconCoverage). It ranks Private Use Area candidates, where plain
	// coverage cannot tell a real icon font from a text face squatting a few
	// PUA slots with unrelated shapes.
	iconScore int8
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
//
// The file is opened lazily (like describeFontFile) rather than slurped:
// NewLoaders on an *os.File reads tables via ReadAt on demand, and RawTable
// copies each requested table into a fresh heap buffer, so only the cmap and
// OS/2 tables are ever read — not the whole file, which for system .ttc
// collections (Apple Color Emoji, Hiragino, Noto CJK) runs to hundreds of MB.
// Nothing retains the loader or file: coverage holds a parsed font.Cmap over
// those heap copies, so closing on return is safe.
func parseCoverage(path string) (cov *coverage) {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	loaders, err := ot.NewLoaders(f)
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
	cov = &coverage{cmap: cm, color: hasColorTable(ld)}
	cov.iconScore = scoreIconCoverage(cov)
	return cov
}

// warmFallbackOnce gates the background warm to once per process: the
// fallback set is derived from the system font dirs, so every Context in a
// process sees the same set and re-warming is pure waste (goroutine spawn +
// full-slice loop contending coverageCache.mu against live layout probes).
// A Context whose set somehow differed loses nothing: orderTextFallbacks
// loads any un-warmed path on demand, exactly as before.
var warmFallbackOnce sync.Once

// warmFallbackCoverageOnce fires warmFallbackCoverage in a background
// goroutine, once per process (warmFallbackOnce). The recover backstop
// keeps a defective font file from ever crashing the process from a
// best-effort warm: parseCoverage recovers parser panics itself, this
// covers anything outside that window, and a failed warm only means the
// layout path parses coverage on demand, as it always has. The slice is a
// parameter rather than a closure capture so the caller's Context stays
// collectable while the warm runs.
func warmFallbackCoverageOnce(fallbackPaths []string) {
	warmFallbackOnce.Do(func() {
		go func() {
			defer func() { _ = recover() }()
			warmFallbackCoverage(fallbackPaths)
		}()
	})
}

// warmFallbackCoverage pre-populates the resident coverage cache for every
// font in the fallback set. Without it, the first cluster the base font does
// not cover (and that is not emoji — the color path exits at the first color
// font) forces orderTextFallbacks to read and cmap-parse the entire fallback
// set on the layout path: file I/O surfacing as a one-time UI freeze at an
// arbitrary moment (first ⌘, →, box-drawing char, …). NewContext runs this
// in a background goroutine (via warmFallbackCoverageOnce) so the cost is
// paid off the layout path, during startup. Safe concurrently with layout-time
// probes: loadCoverage is designed for concurrent callers, and a probe
// racing the warm either finds the entry cached or parses it first itself.
func warmFallbackCoverage(fallbackPaths []string) {
	// Fallback order is priority order, so the fonts most likely to be
	// needed first are warmed first.
	for _, path := range fallbackPaths {
		loadCoverage(path)
	}
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
//
// Within the monochrome group a Private Use Area cluster is re-ranked by icon
// coverage (rankIconFallbacks): tier order is meaningless for PUA, where the
// first covering font is routinely a text face that maps the slot to an
// unrelated glyph.
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
	return rankIconFallbacks(mono, text), color
}

// isDefaultIgnorable reports whether r is a Unicode default-ignorable code
// point, which shapers omit from output. Derived from Default_Ignorable_
// Code_Point in Unicode 17.0 DerivedCoreProperties.txt. A codepoint missing
// here is treated as needing real cmap coverage, which can force a spurious
// fallback (or tofu) for text that would shape fine — so keep this complete.
func isDefaultIgnorable(r rune) bool {
	switch {
	case r == 0x00AD, r == 0x034F: // soft hyphen, combining grapheme joiner
		return true
	case r == 0x061C: // Arabic letter mark
		return true
	case r == 0x115F || r == 0x1160: // Hangul choseong/jungseong fillers
		return true
	case r == 0x17B4 || r == 0x17B5: // Khmer vowel inherent AQ/AA
		return true
	case r >= 0x180B && r <= 0x180F: // Mongolian FVS1-3, MVS, FVS4
		return true
	case r >= 0x200B && r <= 0x200F: // ZW space, ZWNJ/ZWJ, LRM/RLM
		return true
	case r >= 0x202A && r <= 0x202E: // bidi embeddings/overrides (LRE..RLO)
		return true
	case r >= 0x2060 && r <= 0x206F: // word joiner, invisible ops, bidi
		return true
	case r == 0x3164: // Hangul filler
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r == 0xFEFF: // ZWNBSP/BOM
		return true
	case r == 0xFFA0: // halfwidth Hangul filler
		return true
	case r >= 0xFFF0 && r <= 0xFFF8: // reserved format characters
		return true
	case r >= 0x1BCA0 && r <= 0x1BCA3: // shorthand format controls
		return true
	case r >= 0x1D173 && r <= 0x1D17A: // musical beam/tie controls
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
	releaseShapeBuffer(buf)
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
