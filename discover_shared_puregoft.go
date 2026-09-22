//go:build linux || darwin || windows

package glyph

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
)

// This file holds the platform-independent half of system font discovery.
// Each discover_<os>.go file supplies only the data that differs per
// platform:
//
//   - systemFontDirs: the directories to walk
//   - platformAliases: the families that may fill each generic alias key
//   - platformGenerics: the families the generic names resolve to
//   - ensurePlatformDefaults: fixed keys to fill when discovery found none

// aliasTable maps a generic alias key ("sans-serif", "serif", "monospace")
// to the lower-cased family names that may fill it, most preferred first.
// Names match whole, never as substrings: a substring such as "noto sans"
// also matches "Noto Sans Adlam" and every other Noto script font, and a
// script font with no Latin letters must never become the fallback for
// unresolved families.
type aliasTable map[string][]string

// lookup returns the alias key that lowerFam fills and its rank in that
// key's preference list (0 = most preferred). ok is false when lowerFam
// fills no alias.
func (t aliasTable) lookup(lowerFam string) (alias string, rank int, ok bool) {
	for key, fams := range t {
		if i := slices.Index(fams, lowerFam); i >= 0 {
			return key, i, true
		}
	}
	return "", 0, false
}

// genericNames holds the concrete families the generic Pango/CSS names
// resolve to on one platform.
type genericNames struct {
	sans, serif, mono string
}

// resolveFontFamily maps generic font names ("sans", "serif", "monospace",
// …) to concrete platform families. Other names pass through with the size
// and style words removed.
func resolveFontFamily(fontName string) string {
	return platformGenerics.resolve(fontName)
}

// resolve implements resolveFontFamily for one set of generic names.
func (g genericNames) resolve(fontName string) string {
	family := parseFamilyFromFontName(fontName)
	if family == "" {
		return g.sans
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return g.sans
	case "serif":
		return g.serif
	case "monospace", "mono":
		return g.mono
	default:
		return family
	}
}

// systemFonts is the result of one walk of the system font directories.
// It is computed once per process (see cachedSystemFonts) and never
// changed after that: each Context copies what it may change.
type systemFonts struct {
	fontPaths   map[string]string
	fontWeights map[string]font.Weight
	fontItalics map[string]bool
	families    map[string]string

	// Fallback tiers in discovery order. The CJK tier is ordered per
	// Context, because the order depends on the Context's locale.
	color, emoji, cjk, cjkFams, script, general []string
}

var (
	systemFontsOnce sync.Once
	systemFontsVal  *systemFonts
)

// cachedSystemFonts returns the system font scan, running it on the first
// call only. Discovery opens and parses the header of every system font
// file (hundreds on macOS and Windows), and without this cache each
// NewContext paid that cost again. The trade-off: fonts installed after
// the first NewContext are not seen by later Contexts in the same process;
// AddFontFile registers them.
func cachedSystemFonts() *systemFonts {
	systemFontsOnce.Do(func() { systemFontsVal = scanSystemFonts() })
	return systemFontsVal
}

// scanSystemFonts walks the platform font directories and returns the
// result. It has no cache; see cachedSystemFonts.
func scanSystemFonts() *systemFonts {
	scratch := &Context{
		fontPaths:   map[string]string{},
		fontWeights: map[string]font.Weight{},
		fontItalics: map[string]bool{},
		families:    map[string]string{},
	}
	s := newFontScan(scratch)
	s.walkDirs(systemFontDirs(), platformAliases)
	ensurePlatformDefaults(scratch.fontPaths)
	return &systemFonts{
		fontPaths:   scratch.fontPaths,
		fontWeights: scratch.fontWeights,
		fontItalics: scratch.fontItalics,
		families:    scratch.families,
		color:       s.colorPaths,
		emoji:       s.emojiPaths,
		cjk:         s.cjkPaths,
		cjkFams:     s.cjkFams,
		script:      s.scriptPaths,
		general:     s.generalPaths,
	}
}

// discoverSystemFonts fills the Context's font maps and fallback lists from
// the process-wide scan. The maps are copied because AddFontFile changes
// them per Context. The fallback order puts color emoji first, so colored
// glyphs win over monochrome coverage; colorPaths also drives the
// render-side color path. Script fonts (Arabic, Hebrew, etc.) come after
// CJK. General fonts (symbol/icon/Nerd Fonts, etc.) are the last tier.
func (ctx *Context) discoverSystemFonts() {
	sf := cachedSystemFonts()
	ctx.fontPaths = maps.Clone(sf.fontPaths)
	ctx.fontWeights = maps.Clone(sf.fontWeights)
	ctx.fontItalics = maps.Clone(sf.fontItalics)
	ctx.families = maps.Clone(sf.families)
	ctx.colorPaths = slices.Clone(sf.color)
	// assembleFallbacks builds a new slice, so the shared tiers (and
	// orderCJKForLang's result, which can be sf.cjk itself) are not aliased.
	cjk := orderCJKForLang(sf.cjk, sf.cjkFams, ctx.lang)
	ctx.fallbackPaths = assembleFallbacks(sf.color, sf.emoji, cjk,
		sf.script, sf.general)
}

// fillDefaultKeys sets each key to path when discovery did not set it, so
// fonts found on disk keep precedence over the fixed defaults.
func fillDefaultKeys(fontPaths map[string]string, path string, keys ...string) {
	for _, k := range keys {
		if _, ok := fontPaths[k]; !ok {
			fontPaths[k] = path
		}
	}
}
