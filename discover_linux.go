//go:build linux && !android

package glyph

import (
	"os"
	"path/filepath"
	"strings"
)

// discoverSystemFonts walks standard Linux font directories and populates
// ctx.fontPaths using go-text to read family names. No fontconfig dependency.
func (ctx *Context) discoverSystemFonts() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".local", "share", "fonts"),
		filepath.Join(home, ".fonts"),
		"/usr/local/share/fonts",
		"/usr/share/fonts",
	}

	scan := newFontScan(ctx)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			scan.consider(path, linuxGenericAlias)
			return nil
		})
	}
	scan.finish()
}

// linuxGenericAlias maps a lower-cased family name to a generic alias key,
// matching the common Linux font packages (DejaVu / Liberation / Noto). The
// monospace case is tested first because those family names also contain the
// sans-serif substring.
func linuxGenericAlias(lowerFam string) string {
	switch {
	case strings.Contains(lowerFam, "dejavu sans mono") ||
		strings.Contains(lowerFam, "liberation mono") ||
		strings.Contains(lowerFam, "noto mono"):
		return "monospace"
	case strings.Contains(lowerFam, "dejavu serif") ||
		strings.Contains(lowerFam, "liberation serif") ||
		strings.Contains(lowerFam, "noto serif"):
		return "serif"
	case strings.Contains(lowerFam, "dejavu sans") ||
		strings.Contains(lowerFam, "liberation sans") ||
		strings.Contains(lowerFam, "noto sans"):
		return "sans-serif"
	}
	return ""
}

// resolveFontFamily maps generic font names to Linux font families
// (DejaVu / Liberation / Noto).
func resolveFontFamily(fontName string) string {
	family := parseFamilyFromFontName(fontName)
	if family == "" {
		return "DejaVu Sans"
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return "DejaVu Sans"
	case "serif":
		return "DejaVu Serif"
	case "monospace", "mono":
		return "DejaVu Sans Mono"
	default:
		return family
	}
}
