//go:build linux || darwin || windows

package glyph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Context holds font state for text shaping on Linux, Android, macOS,
// and Windows, backed by the pure-Go go-text/typesetting stack (no cgo,
// no system font libraries). Only font discovery differs per platform
// (discoverSystemFonts, defined in discover_linux.go / discover_android.go /
// discover_darwin.go / discover_windows.go).
//
// Not safe for concurrent use.
type Context struct {
	ftLib         FTLibrary // retained for API/shared-code compatibility
	scaleFactor   float32
	scaleInv      float32
	metrics       metricsCache
	fontPaths     map[string]string
	fontWeights   map[string]font.Weight // weight backing each fontPaths key
	fontItalics   map[string]bool        // italic flag backing each fontPaths key
	fallbackPaths []string               // script fallback fonts (CJK, Arabic, etc.)
	colorPaths    []string               // color-emoji fonts (CBDT/CBLC), render side

	// families is a case-folded set of collected family names
	// (lower(displayName) → first-seen display case). Built at
	// registerFontPath; excludes generic aliases and leading-"." names.
	families map[string]string

	// lang is the session locale (from LC_ALL/LC_CTYPE/LANG), used only to
	// order the CJK fallback tier so Han-unified ideographs render in the
	// reader's expected regional shapes (zh-Hans/zh-Hant/ja/ko). Empty means
	// no preference — discovery order is kept. It is a whole-session hint, not
	// per-run: a terminal cell carries no language, so this mirrors what native
	// terminals do. Note the render-side fallback singletons are process-global,
	// so multiple Contexts with different langs share one ordering (last writer
	// wins); acceptable for the single-Context terminal case.
	lang string

	// scratch holds buildLayout's reusable working buffers; safe because the
	// Context is documented single-goroutine. See layoutScratch.
	scratch layoutScratch

	// fallbackResolve memoizes script-fallback selection per grapheme cluster.
	// The result (winning font path, or "" when no fallback covers the cluster)
	// depends only on the fixed fallback set, so it is stable across the base
	// font, size, and re-layouts. Caching the negative ("no font, render tofu")
	// case is what removes the per-scroll rescan of every fallback font for
	// unsupported scripts (Chakma, Javanese, …). resolveOrder is the FIFO key
	// queue for bounded eviction (see fallbackResolveCap). It grows by append
	// until it holds fallbackResolveCap keys, then works as a ring buffer:
	// resolveHead is the index of the oldest key, which the next insert
	// overwrites. A ring keeps the backing array fixed at the cap. Popping
	// the head with a reslice would shrink the capacity by one on each
	// eviction and force append to copy the whole queue (about 4 MB of
	// string headers) again and again.
	fallbackResolve map[string]fbResolution
	resolveOrder    []string
	resolveHead     int
}

// fbResolution is a cached script-fallback decision for one cluster.
type fbResolution struct {
	path    string // "" means no fallback covers the cluster (renders tofu)
	isColor bool
}

// fallbackResolveCap bounds the resolution cache. Overflow evicts the
// oldest entries (FIFO) rather than clearing the whole map: a resolution
// depends only on the fixed fallback set, so a full clear turns the next
// scroll through a large buffer into a re-probe of every cluster it covered
// (each probe shaping color candidates — see probeFallback). Sized to hold a
// full Unicode sweep (~150k distinct clusters) with headroom; at ~24 bytes
// per entry the bound costs a few tens of MB worst-case.
const fallbackResolveCap = 1 << 18

// cacheFallback stores a resolution, lazily allocating and bounding the map.
// Eviction is FIFO by first-probe order, preserving the recently probed hot
// set that scrolling re-layouts actually consult.
//
// Invariant: resolveOrder holds each key of fallbackResolve exactly once,
// so len(resolveOrder) == len(fallbackResolve).
func (ctx *Context) cacheFallback(text string, res fbResolution) {
	if ctx.fallbackResolve == nil {
		ctx.fallbackResolve = make(map[string]fbResolution)
	}
	if _, ok := ctx.fallbackResolve[text]; ok {
		ctx.fallbackResolve[text] = res
		return
	}
	// text is a cluster sliced out of a caller's layout text. Stored as is,
	// each entry would keep that whole text (up to MaxRichTextLength bytes)
	// alive for as long as the entry lives. Copy only the cluster's bytes.
	text = strings.Clone(text)
	if len(ctx.resolveOrder) >= fallbackResolveCap {
		// Full: overwrite the oldest slot in place and advance the head.
		delete(ctx.fallbackResolve, ctx.resolveOrder[ctx.resolveHead])
		ctx.resolveOrder[ctx.resolveHead] = text
		ctx.resolveHead = (ctx.resolveHead + 1) % len(ctx.resolveOrder)
	} else {
		ctx.resolveOrder = append(ctx.resolveOrder, text)
	}
	ctx.fallbackResolve[text] = res
}

// NewContext creates a text context backed by go-text/typesetting.
func NewContext(scaleFactor float32) (*Context, error) {
	scaleFactor = sanitizeScale(scaleFactor)

	ctx := &Context{
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
		lang:        detectLang(),
	}
	setFTLib(ctx.ftLib)
	// Fills fontPaths, fontWeights, fontItalics, families, colorPaths and
	// fallbackPaths from the once-per-process system font scan.
	ctx.discoverSystemFonts()
	setFTFontPaths(ctx.fontPaths)
	setFTScriptFallbacks(ctx.fallbackPaths)
	setFTColorFallbacks(ctx.colorPaths)
	// Idle eviction decays the face cache after a burst session (the render
	// thread alone could never trigger it: an idle terminal performs no
	// inserts). One goroutine per process, gated by sync.Once.
	startFaceSweeper()
	// Warm the fallback coverage cache off the layout path, once per
	// process (see warmFallbackCoverageOnce). fallbackPaths is never
	// mutated after discovery, so the goroutine reads a stable slice.
	warmFallbackCoverageOnce(ctx.fallbackPaths)
	return ctx, nil
}

// Free releases resources. Every map, cache, and scratch buffer is
// dropped, so a freed Context holds no font tables or cluster keys. The
// render-side singletons keep their own references (see setFTFontPaths),
// so a Renderer that shares this Context's font map still works.
// AddFontFile after Free returns an error; it does not panic.
func (ctx *Context) Free() {
	ctx.metrics = metricsCache{}
	ctx.fontPaths = nil
	ctx.fontWeights = nil
	ctx.fontItalics = nil
	ctx.families = nil
	ctx.fallbackPaths = nil
	ctx.colorPaths = nil
	ctx.scratch = layoutScratch{}
	ctx.fallbackResolve = nil
	ctx.resolveOrder = nil
	ctx.resolveHead = 0
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile registers a font file, extracting its family name and
// aspect (bold/italic) so it resolves through the normal path lookup.
// Every face of a collection (.ttc) is registered.
func (ctx *Context) AddFontFile(path string) error {
	if ctx.fontPaths == nil {
		// Free dropped the maps; writing to a nil map panics.
		return fmt.Errorf("glyph: AddFontFile on freed Context")
	}
	faces := describeFontFaces(path)
	if len(faces) == 0 {
		return fmt.Errorf("glyph: failed to parse font %q", path)
	}
	for _, fc := range faces {
		registerFontPath(ctx.fontPaths, ctx.fontWeights, ctx.fontItalics,
			ctx.families, fc.desc.Family, fc.desc.Aspect, fc.path)
	}
	return nil
}

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	font := newFTFont(ctx.ftLib, ctx.fontPaths, cfg.Style, ctx.scaleFactor)
	if font.face == nil {
		return 0, fmt.Errorf("glyph: no font resolves for %q", cfg.Style.FontName)
	}
	defer font.close()

	ascent, descent, _ := font.metrics()
	return float32(ascent+descent) / ctx.scaleFactor, nil
}

// FontMetrics returns detailed metrics in logical pixels.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	font := newFTFont(ctx.ftLib, ctx.fontPaths, cfg.Style, ctx.scaleFactor)
	if font.face == nil {
		return TextMetrics{}, fmt.Errorf("glyph: no font resolves for %q",
			cfg.Style.FontName)
	}
	defer font.close()

	ascent, descent, leading := font.metrics()
	sf := float64(ctx.scaleFactor)
	asc := float32(ascent / sf)
	dsc := float32(descent / sf)
	lineHeight := recommendedLineHeight(ascent, descent, leading, font.size)
	return TextMetrics{
		Ascender:   asc,
		Descender:  dsc,
		Height:     asc + dsc,
		LineGap:    float32(leading / sf),
		LineHeight: float32(lineHeight / sf),
	}, nil
}

// ResolveFontName returns the resolved platform font family name.
func (ctx *Context) ResolveFontName(fontDescStr string) (string, error) {
	family := resolveFontFamily(fontDescStr)
	return family, nil
}

// createFTFont builds an ftFont from TextStyle. Caller must call close().
func (ctx *Context) createFTFont(style TextStyle) ftFont {
	return newFTFont(ctx.ftLib, ctx.fontPaths, style, ctx.scaleFactor)
}

// faceInfo describes one face of a font file.
type faceInfo struct {
	path  string // face path (see facePath); the plain path for face 0
	index int    // face index within the file
	desc  font.Description
	color bool // has a color-glyph table
}

// describeFontFaces reads the family/aspect and color-glyph flag of every
// face in a font file, without building full faces. A plain font gives one
// face; a collection (.ttc) gives one per member. It opens the file lazily
// and lets each loader read only the table directory and the few metadata
// tables it needs (via ReadAt), rather than slurping the whole file —
// discovery walks every system font, so this keeps startup I/O small.
// Returns nil when the file cannot be opened or parsed.
func describeFontFaces(path string) []faceInfo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	loaders, err := ot.NewLoaders(f)
	if err != nil || len(loaders) == 0 {
		return nil
	}
	faces := make([]faceInfo, 0, len(loaders))
	for i, ld := range loaders {
		desc, _ := font.Describe(ld, nil)
		faces = append(faces, faceInfo{
			path:  facePath(path, i),
			index: i,
			desc:  desc,
			color: hasColorTable(ld),
		})
	}
	return faces
}

// aspectBoldItalic maps a go-text Aspect to the bold/italic booleans the
// path-resolution scheme keys on.
func aspectBoldItalic(a font.Aspect) (bold, italic bool) {
	bold = a.Weight >= font.WeightBold
	italic = a.Style == font.StyleItalic
	return bold, italic
}

// registerFontPath stores a font path under its style-specific key
// (e.g. "DejaVu Sans-Bold") and bare family key.
//
// The bold/italic style scheme buckets several weights into one key
// (e.g. Book, Regular, and Medium all map to "-Regular"). To keep a
// family with multiple weights from resolving to the wrong one, the
// entry whose weight is closest to the bucket's canonical weight wins;
// on a weight tie the upright face wins over an italic one (Regular and
// Italic both carry WeightNormal, so without this the bare family key
// could fall to an italic face that sorts first in the walk). Ties
// otherwise keep the first (preserving the user-fonts-first discovery
// order). keyWeights/keyItalics track the stored weight and italic flag
// per key; both may be nil (no tie resolution — first-wins) for callers
// that do not track them.
func registerFontPath(fontPaths map[string]string,
	keyWeights map[string]font.Weight, keyItalics map[string]bool,
	families map[string]string, family string, aspect font.Aspect,
	path string) {
	if family == "" {
		return
	}
	// Collect the clean family name into the case-folded set. Exclude
	// leading-"." names (e.g. ".LastResort") — they are private Apple
	// fallback fonts useful for resolution but never a user-facing family.
	// Generic aliases are inserted outside registerFontPath and never
	// reach this set. families may be nil (as keyWeights may) — callers
	// that don't collect names pass nil; writing to a nil map panics.
	if families != nil && !strings.HasPrefix(family, ".") {
		lc := strings.ToLower(family)
		if _, ok := families[lc]; !ok {
			families[lc] = family // first-seen display case
		}
	}
	bold, italic := aspectBoldItalic(aspect)
	styleKey := family + styleSuffix(bold, italic)
	considerFontKey(fontPaths, keyWeights, keyItalics, styleKey,
		aspect.Weight, italic, canonicalWeight(bold), path)
	// The bare family key is the last-resort fallback in resolveFontPath,
	// so it should hold the Regular-weight face when several weights exist.
	considerFontKey(fontPaths, keyWeights, keyItalics, family,
		aspect.Weight, italic, font.WeightNormal, path)
}

// listFontFamilies backs (*TextSystem).ListFontFamilies on native
// builds. families is keyed by the case-folded name, so sorting the keys
// gives a case-insensitive order without re-lowercasing per comparison;
// each key maps back to its display-case value.
func (ctx *Context) listFontFamilies() []string {
	if ctx.families == nil {
		return nil
	}
	keys := make([]string, 0, len(ctx.families))
	for lc := range ctx.families {
		keys = append(keys, lc)
	}
	sort.Strings(keys)
	names := make([]string, len(keys))
	for i, lc := range keys {
		names[i] = ctx.families[lc]
	}
	return names
}

// considerFontKey records path under key when the key is unset, or when
// weight is strictly closer to target than the weight already stored. On a
// weight tie the upright face wins over the stored italic one (mirroring
// betterRegular, which the fallback tiers use): Regular and Italic both
// carry WeightNormal, so without this rule the bare family key could end up
// holding an italic face that merely sorted first in the directory walk.
// keyItalics tracks the stored italic flag per key and must accompany
// keyWeights (both nil disables tie resolution — first-wins). It may be
// nil when keyWeights is non-nil, in which case only the weight-distance
// comparison applies.
func considerFontKey(fontPaths map[string]string,
	keyWeights map[string]font.Weight, keyItalics map[string]bool,
	key string, weight font.Weight, italic bool,
	target font.Weight, path string) {
	if _, exists := fontPaths[key]; !exists {
		fontPaths[key] = path
		if keyWeights != nil {
			keyWeights[key] = weight
			if keyItalics != nil {
				keyItalics[key] = italic
			}
		}
		return
	}
	if keyWeights == nil {
		return // no tie resolution requested: keep first
	}
	prev, ok := keyWeights[key]
	if !ok {
		return
	}
	prevDist := weightDist(prev, target)
	newDist := weightDist(weight, target)
	if newDist < prevDist ||
		// Weight ties: upright beats the stored italic face.
		(newDist == prevDist && keyItalics != nil && keyItalics[key] && !italic) {
		fontPaths[key] = path
		keyWeights[key] = weight
		if keyItalics != nil {
			keyItalics[key] = italic
		}
	}
}

// canonicalWeight is the reference weight for a style bucket: Bold for
// bold buckets, Normal otherwise.
func canonicalWeight(bold bool) font.Weight {
	if bold {
		return font.WeightBold
	}
	return font.WeightNormal
}

func weightDist(w, target font.Weight) font.Weight {
	if w < target {
		return target - w
	}
	return w - target
}

// styleSuffix returns the resolveFontPath key suffix for a style.
func styleSuffix(bold, italic bool) string {
	switch {
	case bold && italic:
		return "-BoldItalic"
	case bold:
		return "-Bold"
	case italic:
		return "-Italic"
	default:
		return "-Regular"
	}
}

// isEmojiFamily reports whether a lower-cased family name is a color
// emoji font (e.g. "Noto Color Emoji").
func isEmojiFamily(lowerFamily string) bool {
	return strings.Contains(lowerFamily, "emoji")
}

// isCJKFamily reports whether a lower-cased family name covers CJK
// scripts, matching the common Linux CJK font packages.
func isCJKFamily(lowerFamily string) bool {
	for _, n := range cjkFamilyNeedles {
		if strings.Contains(lowerFamily, n) {
			return true
		}
	}
	return false
}

// cjkFamilyNeedles are the family-name substrings isCJKFamily matches. They
// live at package level so each call does not rebuild the slice. Needles are
// specific enough not to hit unrelated families: a bare "han" matched Khand
// and Chandas (Devanagari) and Hanuman (Khmer), and a bare "ipa" matched any
// name holding those letters, so those fonts landed in the CJK tier ahead of
// the script tier.
var cjkFamilyNeedles = []string{
	"cjk", "source han", "han sans", "han serif",
	"wenquanyi", "wqy", "uming", "ukai", "ar pl",
	"noto sans jp", "noto serif jp",
	"noto sans sc", "noto sans tc", "noto sans kr",
	"droid sans fallback", "nanum",
	"ipagothic", "ipamincho", "ipapgothic", "ipapmincho",
	"ipaexgothic", "ipaexmincho", "ipa gothic", "ipa mincho",
	"vl gothic", "takao",
	"pingfang", "heiti", "hiragino",
	"apple sd gothic neo", "applegothic",
	"microsoft yahei", "microsoft jhenghei",
	"malgun gothic", "meiryo", "yu gothic",
	"batang", "simsun", "mingliu",
	"ms gothic", "ms mincho",
}

// isScriptFamily reports whether a lower-cased family name belongs to a
// font that covers one or more non-CJK, non-Latin scripts (Arabic, Hebrew,
// Thai, Indic, etc.). These are collected as the lowest-priority fallback
// tier so the layout engine can find glyphs the primary Latin font lacks.
func isScriptFamily(lowerFamily string) bool {
	for _, n := range scriptFamilyNeedles {
		if strings.Contains(lowerFamily, n) {
			return true
		}
	}
	return false
}

// scriptFamilyNeedles are the family-name substrings isScriptFamily
// matches, kept at package level so each call does not rebuild the slice.
var scriptFamilyNeedles = []string{
	"arabic", "naskh", "geeza", "al nile",
	"baghdad", "damascus", "kufi", "nadeem",
	"hebrew", "corsiva", "peninim", "raanana",
	"thai", "thonburi", "krungthep", "sathu", "silom",
	"devanagari", "kohinoor", "sangam",
	"gujarati", "gurmukhi", "kannada",
	"malayalam", "oriya", "tamil", "telugu", "bengali",
	"georgian", "armenian", "khmer", "lao",
	"myanmar", "sinhala", "tibetan", "burmese",
	"noto naskh", "noto sans arabic", "noto sans hebrew",
	"noto sans thai", "noto sans devanagari", "noto sans tamil",
}

// fontScan accumulates discovery results across a directory walk. The
// discover files drive it; only the directory list and the generic-alias
// mapping differ per platform. Color emoji is collected ahead of CJK so
// colored glyphs win over monochrome coverage in the fallback order.
// Script fonts (Arabic, Hebrew, Thai, Indic, etc.) are collected as the
// next tier. General fonts (all remaining, including symbol/icon/Nerd Fonts
// with PUA coverage) trail as the lowest-priority fallback.
type fontScan struct {
	ctx          *Context
	emojiPaths   []string
	cjkPaths     []string
	cjkFams      []string // family per cjkPaths entry, index-aligned; drives locale reorder
	scriptPaths  []string
	generalPaths []string
	colorPaths   []string
	seenFallback map[string]bool
	seenColor    map[string]bool
	// slots records where each family landed in a weight-preferring tier so a
	// later, closer-to-Regular face of the same family can replace it in place.
	slots map[string]*fallbackSlot
	// aliasRank holds, per generic alias key, the aliasTable rank of the
	// family now stored under that key. A family with a lower rank (more
	// preferred) replaces it, whatever the walk order. See considerAlias.
	aliasRank map[string]int
}

// fallbackSlot points at a family's entry in one fallback tier and remembers
// the aspect of the face currently stored there, so preferRegular can swap in
// a plainer face without disturbing tier order.
type fallbackSlot struct {
	tier   *[]string // the tier slice this family sits in
	index  int       // its index within that tier
	weight font.Weight
	italic bool
}

func newFontScan(ctx *Context) *fontScan {
	return &fontScan{
		ctx:          ctx,
		seenFallback: map[string]bool{},
		seenColor:    map[string]bool{},
		slots:        map[string]*fallbackSlot{},
		aliasRank:    map[string]int{},
	}
}

// walkDirs feeds every file under each dir to consider. Relative dirs are
// skipped: an empty $HOME or %WINDIR% turns a dir such as "$HOME/.fonts"
// into ".fonts", which resolves against the working directory, and discovery
// would then parse whatever font files an attacker put there. Unreadable
// dirs and entries are skipped silently; a missing font dir is normal.
func (s *fontScan) walkDirs(dirs []string, aliases aliasTable) {
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			s.consider(path, aliases)
			return nil
		})
	}
}

// consider examines one candidate path and records it: its family/style
// path, a generic alias (via aliases, which maps a lower-cased family name
// to "sans-serif"/"serif"/"monospace" or none), and its color/CJK/emoji
// fallback role. Non-font files are ignored.
func (s *fontScan) consider(path string, aliases aliasTable) {
	lower := strings.ToLower(path)
	// .ttc covers OpenType collections (Noto CJK ships this way).
	if !strings.HasSuffix(lower, ".ttf") &&
		!strings.HasSuffix(lower, ".otf") &&
		!strings.HasSuffix(lower, ".ttc") {
		return
	}
	for _, fc := range describeFontFaces(path) {
		s.considerFace(fc, aliases)
	}
}

// considerFace records one face of a font file. See consider.
func (s *fontScan) considerFace(fc faceInfo, aliases aliasTable) {

	desc, path := fc.desc, fc.path
	if desc.Family == "" {
		return
	}
	family := desc.Family
	registerFontPath(s.ctx.fontPaths, s.ctx.fontWeights, s.ctx.fontItalics,
		s.ctx.families, family, desc.Aspect, path)

	lowerFam := strings.ToLower(family)

	// Apple's ".LastResort" font maps every codepoint to a placeholder
	// "tofu" glyph. Left in any fallback tier it shadows real glyphs that
	// sort after it (STIX math symbols such as U+23F5 ⏵, etc.): every
	// codepoint missing from the base font resolves to LastResort's box
	// instead of the font that actually draws it. It is never a useful
	// fallback — go-glyph already renders the base font's own tofu when
	// nothing covers a cluster — so keep it out of the fallback lists.
	// It stays in fontPaths (registered above), which is harmless since
	// no caller requests the dot-prefixed private family by name.
	if lowerFam == ".lastresort" {
		return
	}

	// Register the generic alias ("sans-serif", "monospace", …). The alias
	// is the last-resort face for an unresolved family (genericFallback).
	if alias, rank, ok := aliases.lookup(lowerFam); ok {
		s.considerAlias(alias, rank, desc.Aspect, path)
	}

	// Collection members after the first join the fallback tiers only for
	// CJK families, where several regional faces share one file (Noto Sans
	// CJK, PingFang) and the locale reorder must be able to pick the reader's
	// face. Other collections (Helvetica.ttc, emoji .ttc files) keep one
	// fallback entry per file, as before: extra members add coverage probes
	// and face-cache entries but no glyphs the first face lacks.
	if fc.index > 0 && !isCJKFamily(lowerFam) {
		return
	}
	s.record(family, desc.Aspect, fc.color, path)
}

// considerAlias stores path under a generic alias key. Two rules pick the
// face:
//
//  1. Family rank. The most preferred family in the aliasTable wins, in any
//     walk order. First-match-wins let whatever sorted first take the key:
//     on Android "NotoSansAdlam-VF.ttf" sorts before "Roboto-Regular.ttf",
//     so a loose "noto sans" match made an Adlam font the sans-serif
//     fallback for every unresolved family.
//  2. Weight, inside one family. The alias must hold the plain Regular
//     face: on Linux "DejaVuSans-Bold.ttf" sorts before "DejaVuSans.ttf"
//     ('-' < '.'). considerFontKey keeps the face closest to Regular
//     weight, and upright on a tie.
func (s *fontScan) considerAlias(alias string, rank int,
	aspect font.Aspect, path string) {

	cur, seen := s.aliasRank[alias]
	if seen && rank > cur {
		return // a more preferred family already holds the key
	}
	if !seen || rank < cur {
		// A more preferred family: drop the stored face so considerFontKey
		// records this one without comparing weights across families.
		delete(s.ctx.fontPaths, alias)
		delete(s.ctx.fontWeights, alias)
		delete(s.ctx.fontItalics, alias)
		s.aliasRank[alias] = rank
	}
	considerFontKey(s.ctx.fontPaths, s.ctx.fontWeights, s.ctx.fontItalics,
		alias, aspect.Weight, aspect.Style == font.StyleItalic,
		font.WeightNormal, path)
}

// record files one already-parsed font into the fallback tiers. It is the
// disk-free, ctx-free core of consider — tier bucketing and per-family
// dedupe only — so the bucketing policy is unit-testable without font
// fixtures. Callers pass the family name, its aspect (weight/style), whether
// it carries color-glyph tables, and its path. When a family recurs, a face
// closer to Regular replaces the stored one (see preferRegular) so a bold or
// italic file sorting ahead of Regular in the walk does not become the
// fallback face.
func (s *fontScan) record(family string, aspect font.Aspect,
	color bool, path string) {

	lowerFam := strings.ToLower(family)
	// LastResort never belongs in a fallback tier (see consider); guard here
	// too so record is correct when called directly.
	if lowerFam == ".lastresort" {
		return
	}

	// Color-emoji fonts (CBDT/CBLC/sbix) are tracked separately so the
	// renderer can pick a true color font over a monochrome emoji font
	// (e.g. Noto Emoji) that also covers the glyph.
	if color && !s.seenColor[family] {
		s.colorPaths = append(s.colorPaths, path)
		s.seenColor[family] = true
		// Already leads the general fallback list; don't re-add below.
		s.seenFallback[family] = true
	}

	if s.seenFallback[family] {
		// Already placed. If it sits in a weight-preferring tier, swap in a
		// face closer to Regular — but never a color face: the mono tiers
		// must stay monochrome (the color face is already in colorPaths),
		// and color families with no slot are a no-op anyway.
		if !color {
			s.preferRegular(family, aspect, path)
		}
		return
	}

	var tier *[]string
	switch {
	case isEmojiFamily(lowerFam):
		tier = &s.emojiPaths
	case isCJKFamily(lowerFam):
		tier = &s.cjkPaths
	case isScriptFamily(lowerFam):
		tier = &s.scriptPaths
	default:
		tier = &s.generalPaths
	}
	*tier = append(*tier, path)
	if tier == &s.cjkPaths {
		// Track the family index-aligned with cjkPaths so the locale reorder
		// can match on family name. A later same-family weight swap keeps the
		// index (family unchanged), so this stays aligned.
		s.cjkFams = append(s.cjkFams, family)
	}
	s.seenFallback[family] = true
	s.slots[family] = &fallbackSlot{
		tier:   tier,
		index:  len(*tier) - 1,
		weight: aspect.Weight,
		italic: aspect.Style == font.StyleItalic,
	}
}

// preferRegular replaces a family's stored fallback face with path when the
// new face is a plainer choice — closer to Regular weight, or equally close
// but upright where the stored face is italic. A no-op for families with no
// tier slot (color-emoji fonts, which lead by first-seen).
func (s *fontScan) preferRegular(family string, aspect font.Aspect, path string) {
	slot, ok := s.slots[family]
	if !ok {
		return
	}
	newItalic := aspect.Style == font.StyleItalic
	if betterRegular(aspect.Weight, newItalic, slot.weight, slot.italic) {
		(*slot.tier)[slot.index] = path
		slot.weight = aspect.Weight
		slot.italic = newItalic
	}
}

// betterRegular reports whether a candidate face (weight w, italic) is a
// better plain fallback than the stored one: strictly closer to Regular
// weight, or equally close but upright where the stored face is italic.
func betterRegular(w font.Weight, italic bool,
	curW font.Weight, curItalic bool) bool {

	nd := weightDist(w, font.WeightNormal)
	cd := weightDist(curW, font.WeightNormal)
	if nd != cd {
		return nd < cd
	}
	return curItalic && !italic
}

// assembleFallbacks concatenates the tier slices into the final fallback
// priority order: color emoji, monochrome emoji, CJK, other scripts, then
// general. Pure — no ctx or disk — so the ordering is unit-testable.
func assembleFallbacks(color, emoji, cjk, script, general []string) []string {
	out := make([]string, 0,
		len(color)+len(emoji)+len(cjk)+len(script)+len(general))
	out = append(out, color...)
	out = append(out, emoji...)
	out = append(out, cjk...)
	out = append(out, script...)
	out = append(out, general...)
	return out
}

// detectLang reads the session locale from the environment, matching the
// POSIX precedence LC_ALL > LC_CTYPE > LANG. Returns "" when none is set.
func detectLang() string {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// cjkLangPrefs maps a normalized CJK locale to the lower-cased family-name
// substrings whose fonts should lead the CJK fallback tier, so Han-unified
// ideographs render in that locale's regional shapes. List order is
// preference order (most-preferred first). Substrings are matched against the
// discovered family name; the vocabulary mirrors isCJKFamily.
var cjkLangPrefs = map[string][]string{
	"zh-hans": {
		"pingfang sc", "noto sans sc", "noto serif sc",
		"source han sans sc", "source han serif sc",
		"microsoft yahei", "heiti sc", "songti sc", "simhei", "simsun",
	},
	"zh-hant": {
		"pingfang tc", "pingfang hk", "noto sans tc", "noto serif tc",
		"noto sans hk", "source han sans tc", "source han serif tc",
		"microsoft jhenghei", "heiti tc", "pmingliu", "mingliu",
	},
	"ja": {
		"hiragino", "yu gothic", "yugothic", "yu mincho", "meiryo",
		"noto sans jp", "noto serif jp", "source han sans jp",
		"source han serif jp", "ms gothic", "ms mincho", "ms pgothic",
		"ipagothic", "ipaexgothic", "takao", "vl gothic",
	},
	"ko": {
		"apple sd gothic neo", "applegothic", "malgun gothic",
		"noto sans kr", "noto serif kr", "source han sans kr",
		"source han serif kr", "nanum", "batang", "gulim", "dotum",
	},
}

// normalizeCJKLang reduces a POSIX/BCP-47 locale string to one of the keys in
// cjkLangPrefs ("zh-hans", "zh-hant", "ja", "ko"), or "" when the locale is
// not a CJK language needing regional Han shaping. It strips the encoding
// (".UTF-8") and modifier ("@euro") suffixes, lower-cases, and resolves the
// Chinese script from an explicit Hans/Hant subtag or the region (CN/SG/MY →
// Hans; TW/HK/MO → Hant; bare zh defaults to Hans).
func normalizeCJKLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if l == "" {
		return ""
	}
	if i := strings.IndexAny(l, ".@"); i >= 0 {
		l = l[:i]
	}
	l = strings.ReplaceAll(l, "_", "-")
	parts := strings.Split(l, "-")
	switch parts[0] {
	case "ja":
		return "ja"
	case "ko":
		return "ko"
	case "zh":
		for _, p := range parts[1:] {
			switch p {
			case "hans":
				return "zh-hans"
			case "hant":
				return "zh-hant"
			case "cn", "sg", "my":
				return "zh-hans"
			case "tw", "hk", "mo":
				return "zh-hant"
			}
		}
		return "zh-hans" // bare "zh": default to Simplified
	}
	return ""
}

// orderCJKForLang stable-sorts the CJK fallback paths so families matching the
// session locale lead, in preference order, with all other fonts keeping their
// discovery order after. fams is index-aligned with paths. A no-op when the
// locale needs no CJK reorder, the tier has fewer than two entries, or the
// family list is not aligned.
func orderCJKForLang(paths, fams []string, lang string) []string {
	key := normalizeCJKLang(lang)
	if key == "" || len(paths) < 2 || len(fams) != len(paths) {
		return paths
	}
	prefs := cjkLangPrefs[key]
	// Precompute each path's preference rank once (substring scans are the
	// costly part), so the sort comparator is a plain int compare instead of
	// re-running strings.Contains O(n log n) times.
	ranks := make([]int, len(paths))
	for i := range ranks {
		fam := strings.ToLower(fams[i])
		ranks[i] = len(prefs) // unmatched: after all matches, order preserved
		for r, sub := range prefs {
			if strings.Contains(fam, sub) {
				ranks[i] = r
				break
			}
		}
	}
	idx := make([]int, len(paths))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return ranks[idx[a]] < ranks[idx[b]]
	})
	out := make([]string, len(paths))
	for i, j := range idx {
		out[i] = paths[j]
	}
	return out
}
