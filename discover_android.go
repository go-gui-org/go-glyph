//go:build android

package glyph

import (
	"os"
	"path/filepath"
	"strings"
)

// discoverSystemFonts walks the Android system font directories and
// populates ctx.fontPaths using go-text to read family names — replacing
// the old /system/etc/fonts.xml parse. Family/style, generic aliases, and
// color/CJK/emoji fallbacks are read directly from each font's tables.
func (ctx *Context) discoverSystemFonts() {
	dirs := []string{
		"/system/fonts",
		"/product/fonts", // vendor/product-partition fonts (Android 10+)
		"/system/font",   // some vendor images use the singular form
		"/data/fonts",    // runtime-installed downloadable fonts
	}

	scan := newFontScan(ctx)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			scan.consider(path, androidGenericAlias)
			return nil
		})
	}
	scan.finish()
	ctx.ensureAndroidDefaults()
}

// androidGenericAlias maps a lower-cased family name to a generic alias key,
// matching the Android system fonts (Roboto / Noto / Droid). The monospace
// case is tested first because those family names also contain "roboto".
func androidGenericAlias(lowerFam string) string {
	switch {
	case strings.Contains(lowerFam, "roboto mono") ||
		strings.Contains(lowerFam, "droid sans mono") ||
		strings.Contains(lowerFam, "noto sans mono") ||
		strings.Contains(lowerFam, "cutive mono") ||
		strings.Contains(lowerFam, "noto mono"):
		return "monospace"
	case strings.Contains(lowerFam, "noto serif") ||
		strings.Contains(lowerFam, "droid serif"):
		return "serif"
	case strings.Contains(lowerFam, "roboto") ||
		strings.Contains(lowerFam, "noto sans") ||
		strings.Contains(lowerFam, "droid sans"):
		return "sans-serif"
	}
	return ""
}

// ensureAndroidDefaults guarantees a usable sans-serif fallback exists even
// if the directory walk found nothing (e.g. restricted image). Only fills
// keys that discovery did not already set, so real fonts take precedence.
// Missing serif/monospace resolve through resolveFontPath's sans-serif
// last resort.
func (ctx *Context) ensureAndroidDefaults() {
	const roboto = "/system/fonts/Roboto-Regular.ttf"
	for _, k := range []string{"Roboto", "Roboto-Regular", "sans-serif"} {
		if _, ok := ctx.fontPaths[k]; !ok {
			ctx.fontPaths[k] = roboto
		}
	}
}

// resolveFontFamily maps generic Pango-style font names to Android system
// font families. Concrete names match go-text's parsed family strings; if a
// requested family is absent, resolveFontPath degrades to the sans-serif
// alias.
func resolveFontFamily(fontName string) string {
	family := parseFamilyFromFontName(fontName)
	if family == "" {
		return "Roboto"
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return "Roboto"
	case "serif":
		return "Noto Serif"
	case "monospace", "mono":
		return "Roboto Mono"
	default:
		return family
	}
}
