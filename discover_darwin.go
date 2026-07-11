//go:build darwin

package glyph

import (
	"os"
	"path/filepath"
	"strings"
)

func (ctx *Context) discoverSystemFonts() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		"/System/Library/Fonts",
		"/Library/Fonts",
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
	}

	scan := newFontScan(ctx)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			scan.consider(path, darwinGenericAlias)
			return nil
		})
	}
	scan.finish()
	ctx.ensureDarwinDefaults()
}

func darwinGenericAlias(lowerFam string) string {
	switch {
	case strings.Contains(lowerFam, "menlo") ||
		strings.Contains(lowerFam, "monaco") ||
		strings.Contains(lowerFam, "sf mono"):
		return "monospace"
	case strings.Contains(lowerFam, "times new roman") ||
		strings.Contains(lowerFam, "times"):
		return "serif"
	case strings.Contains(lowerFam, "helvetica") ||
		strings.Contains(lowerFam, "helvetica neue"):
		return "sans-serif"
	}
	return ""
}

func (ctx *Context) ensureDarwinDefaults() {
	const helvetica = "/System/Library/Fonts/Helvetica.ttc"
	for _, k := range []string{"Helvetica", "sans-serif"} {
		if _, ok := ctx.fontPaths[k]; !ok {
			ctx.fontPaths[k] = helvetica
		}
	}
}

func resolveFontFamily(fontName string) string {
	family := parseFamilyFromFontName(fontName)
	if family == "" {
		return "Helvetica"
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return "Helvetica"
	case "serif":
		return "Times New Roman"
	case "monospace", "mono":
		return "Menlo"
	default:
		return family
	}
}
