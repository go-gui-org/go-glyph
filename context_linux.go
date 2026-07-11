//go:build linux && !android

package glyph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Context holds font state for text shaping on Linux, backed by the
// pure-Go go-text/typesetting stack (no cgo, no FreeType/HarfBuzz).
//
// Not safe for concurrent use.
type Context struct {
	ftLib         FTLibrary // retained for API/shared-code compatibility
	scaleFactor   float32
	scaleInv      float32
	metrics       metricsCache
	fontPaths     map[string]string
	fontWeights   map[string]font.Weight // weight backing each fontPaths key
	fallbackPaths []string               // script fallback fonts (CJK, Arabic, etc.)
	colorPaths    []string               // color-emoji fonts (CBDT/CBLC), render side
}

// NewContext creates a Linux text context backed by go-text/typesetting.
func NewContext(scaleFactor float32) (*Context, error) {
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}

	ctx := &Context{
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
		fontPaths:   make(map[string]string),
		fontWeights: make(map[string]font.Weight),
	}
	setFTLib(ctx.ftLib)
	ctx.discoverSystemFonts()
	setFTFontPaths(ctx.fontPaths)
	setFTScriptFallbacks(ctx.fallbackPaths)
	setFTColorFallbacks(ctx.colorPaths)
	return ctx, nil
}

// Free releases resources.
func (ctx *Context) Free() {
	ctx.metrics = metricsCache{}
	ctx.fontPaths = nil
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile registers a font file, extracting its family name and
// aspect (bold/italic) so it resolves through the normal path lookup.
func (ctx *Context) AddFontFile(path string) error {
	desc, _, ok := describeFontFile(path)
	if !ok {
		return fmt.Errorf("failed to parse font %q", path)
	}
	registerFontPath(ctx.fontPaths, ctx.fontWeights,
		desc.Family, desc.Aspect, path)
	return nil
}

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	font := newFTFont(ctx.ftLib, ctx.fontPaths, cfg.Style, ctx.scaleFactor)
	if font.face == nil {
		return 0, fmt.Errorf("failed to create font")
	}
	defer font.close()

	ascent, descent, _ := font.metrics()
	return float32(ascent+descent) / ctx.scaleFactor, nil
}

// FontMetrics returns detailed metrics in logical pixels.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	font := newFTFont(ctx.ftLib, ctx.fontPaths, cfg.Style, ctx.scaleFactor)
	if font.face == nil {
		return TextMetrics{}, fmt.Errorf("failed to create font")
	}
	defer font.close()

	ascent, descent, leading := font.metrics()
	sf := float64(ctx.scaleFactor)
	asc := float32(ascent / sf)
	dsc := float32(descent / sf)
	return TextMetrics{
		Ascender:  asc,
		Descender: dsc,
		Height:    asc + dsc,
		LineGap:   float32(leading / sf),
	}, nil
}

// ResolveFontName returns the resolved Linux font family name.
func (ctx *Context) ResolveFontName(fontDescStr string) (string, error) {
	family := resolveFontFamilyLinux(fontDescStr)
	return family, nil
}

// createFTFont builds an ftFont from TextStyle. Caller must call close().
func (ctx *Context) createFTFont(style TextStyle) ftFont {
	return newFTFont(ctx.ftLib, ctx.fontPaths, style, ctx.scaleFactor)
}

// describeFontFile reads a font's family/aspect and color-glyph flag
// without building a full face. It opens the file lazily and lets the
// loader read only the table directory and the few metadata tables it
// needs (via ReadAt), rather than slurping the whole file — discovery
// walks every system font, so this keeps startup I/O small.
func describeFontFile(path string) (desc font.Description, color, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return desc, false, false
	}
	defer f.Close()
	loaders, err := ot.NewLoaders(f)
	if err != nil || len(loaders) == 0 {
		return desc, false, false
	}
	ld := loaders[0]
	desc, _ = font.Describe(ld, nil)
	return desc, hasColorTable(ld), true
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
// ties keep the first (preserving the user-fonts-first discovery order).
// keyWeights tracks the stored weight per key and may be nil (no tie
// resolution — first-wins), as when registering a single AddFontFile.
func registerFontPath(fontPaths map[string]string,
	keyWeights map[string]font.Weight, family string,
	aspect font.Aspect, path string) {
	if family == "" {
		return
	}
	bold, italic := aspectBoldItalic(aspect)
	styleKey := family + styleSuffix(bold, italic)
	considerFontKey(fontPaths, keyWeights, styleKey,
		aspect.Weight, canonicalWeight(bold), path)
	// The bare family key is the last-resort fallback in resolveFontPath,
	// so it should hold the Regular-weight face when several weights exist.
	considerFontKey(fontPaths, keyWeights, family,
		aspect.Weight, font.WeightNormal, path)
}

// considerFontKey records path under key when the key is unset, or when
// weight is strictly closer to target than the weight already stored.
func considerFontKey(fontPaths map[string]string,
	keyWeights map[string]font.Weight, key string,
	weight, target font.Weight, path string) {
	if _, exists := fontPaths[key]; !exists {
		fontPaths[key] = path
		if keyWeights != nil {
			keyWeights[key] = weight
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
	if weightDist(weight, target) < weightDist(prev, target) {
		fontPaths[key] = path
		keyWeights[key] = weight
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

// discoverSystemFonts walks standard Linux font directories and
// populates ctx.fontPaths using go-text to read family names. No
// fontconfig dependency.
func (ctx *Context) discoverSystemFonts() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".local", "share", "fonts"),
		filepath.Join(home, ".fonts"),
		"/usr/local/share/fonts",
		"/usr/share/fonts",
	}

	// Script-fallback candidates, collected during the walk and
	// assembled in priority order afterwards. Color emoji is tried
	// before CJK so colored glyphs win over monochrome coverage.
	var emojiPaths, cjkPaths, colorPaths []string
	seenFallback := map[string]bool{}
	seenColor := map[string]bool{}

	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			lower := strings.ToLower(path)
			// .ttc covers OpenType collections (Noto CJK ships this way).
			if !strings.HasSuffix(lower, ".ttf") &&
				!strings.HasSuffix(lower, ".otf") &&
				!strings.HasSuffix(lower, ".ttc") {
				return nil
			}

			desc, isColorFace, ok := describeFontFile(path)
			if !ok || desc.Family == "" {
				return nil
			}
			family := desc.Family

			registerFontPath(ctx.fontPaths, ctx.fontWeights,
				family, desc.Aspect, path)

			// Register generic aliases on first match.
			lowerFam := strings.ToLower(family)
			switch {
			case strings.Contains(lowerFam, "dejavu sans mono") ||
				strings.Contains(lowerFam, "liberation mono") ||
				strings.Contains(lowerFam, "noto mono"):
				if _, exists := ctx.fontPaths["monospace"]; !exists {
					ctx.fontPaths["monospace"] = path
				}
			case strings.Contains(lowerFam, "dejavu serif") ||
				strings.Contains(lowerFam, "liberation serif") ||
				strings.Contains(lowerFam, "noto serif"):
				if _, exists := ctx.fontPaths["serif"]; !exists {
					ctx.fontPaths["serif"] = path
				}
			case strings.Contains(lowerFam, "dejavu sans") ||
				strings.Contains(lowerFam, "liberation sans") ||
				strings.Contains(lowerFam, "noto sans"):
				if _, exists := ctx.fontPaths["sans-serif"]; !exists {
					ctx.fontPaths["sans-serif"] = path
				}
			}

			// Color-emoji fonts (CBDT/CBLC) are tracked separately so
			// the renderer can pick a true color font over a monochrome
			// emoji font (e.g. Noto Emoji) that also covers the glyph.
			if isColorFace && !seenColor[family] {
				colorPaths = append(colorPaths, path)
				seenColor[family] = true
				// Already leads the general fallback list; don't re-add.
				seenFallback[family] = true
			}

			// Collect script-fallback fonts (one path per family).
			if !seenFallback[family] {
				switch {
				case isEmojiFamily(lowerFam):
					emojiPaths = append(emojiPaths, path)
					seenFallback[family] = true
				case isCJKFamily(lowerFam):
					cjkPaths = append(cjkPaths, path)
					seenFallback[family] = true
				}
			}
			return nil
		})
	}

	// Color emoji leads the general fallback list so colored glyphs win
	// over monochrome coverage; colorPaths drives the render color path.
	ctx.colorPaths = colorPaths
	ctx.fallbackPaths = append(ctx.fallbackPaths, colorPaths...)
	ctx.fallbackPaths = append(ctx.fallbackPaths, emojiPaths...)
	ctx.fallbackPaths = append(ctx.fallbackPaths, cjkPaths...)
}

// isEmojiFamily reports whether a lower-cased family name is a color
// emoji font (e.g. "Noto Color Emoji").
func isEmojiFamily(lowerFamily string) bool {
	return strings.Contains(lowerFamily, "emoji")
}

// isCJKFamily reports whether a lower-cased family name covers CJK
// scripts, matching the common Linux CJK font packages.
func isCJKFamily(lowerFamily string) bool {
	needles := []string{
		"cjk", "han",
		"wenquanyi", "wqy", "uming", "ukai", "ar pl",
		"noto sans jp", "noto serif jp",
		"noto sans sc", "noto sans tc", "noto sans kr",
		"droid sans fallback", "nanum", "ipa", "vl gothic", "takao",
	}
	for _, n := range needles {
		if strings.Contains(lowerFamily, n) {
			return true
		}
	}
	return false
}
