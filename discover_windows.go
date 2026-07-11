//go:build windows

package glyph

import (
	"os"
	"path/filepath"
	"strings"
)

func (ctx *Context) discoverSystemFonts() {
	var dirs []string
	if windir := os.Getenv("WINDIR"); windir != "" {
		dirs = append(dirs, filepath.Join(windir, "Fonts"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs,
			filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}

	scan := newFontScan(ctx)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			scan.consider(path, windowsGenericAlias)
			return nil
		})
	}
	scan.finish()
	ctx.ensureWindowsDefaults()
}

func windowsGenericAlias(lowerFam string) string {
	switch {
	case strings.Contains(lowerFam, "consolas") ||
		strings.Contains(lowerFam, "courier new") ||
		strings.Contains(lowerFam, "lucida console"):
		return "monospace"
	case strings.Contains(lowerFam, "times new roman") ||
		strings.Contains(lowerFam, "georgia"):
		return "serif"
	case strings.Contains(lowerFam, "segoe ui") ||
		strings.Contains(lowerFam, "arial"):
		return "sans-serif"
	}
	return ""
}

func (ctx *Context) ensureWindowsDefaults() {
	const segoeName = "Segoe UI"
	segoePath := filepath.Join(os.Getenv("WINDIR"), "Fonts", "segoeui.ttf")
	for _, k := range []string{segoeName, "sans-serif"} {
		if _, ok := ctx.fontPaths[k]; !ok {
			ctx.fontPaths[k] = segoePath
		}
	}
}

func resolveFontFamily(fontName string) string {
	family := parseFamilyFromFontName(fontName)
	if family == "" {
		return "Segoe UI"
	}
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "system":
		return "Segoe UI"
	case "serif":
		return "Times New Roman"
	case "monospace", "mono":
		return "Consolas"
	default:
		return family
	}
}
