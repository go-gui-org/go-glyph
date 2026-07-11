//go:build windows

package glyph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Context holds GDI state for text shaping on Windows.
//
// Not safe for concurrent use.
type Context struct {
	gdi             *gdiContext
	scaleFactor     float32
	scaleInv        float32
	metrics         metricsCache
	registeredFonts []string
}

// NewContext creates a Windows text context using GDI.
func NewContext(scaleFactor float32) (*Context, error) {
	if scaleFactor <= 0 {
		scaleFactor = 1.0
	}
	gdi := getGDI()
	if gdi.hdc == 0 {
		return nil, fmt.Errorf("glyph: failed to create GDI device context")
	}
	return &Context{
		gdi:         gdi,
		scaleFactor: scaleFactor,
		scaleInv:    1.0 / scaleFactor,
		metrics:     newMetricsCache(256),
	}, nil
}

// Free releases resources.
func (ctx *Context) Free() {
	const frPrivate = 0x10
	for _, path := range ctx.registeredFonts {
		if p, err := syscall.UTF16PtrFromString(path); err == nil {
			procRemoveFontResourceExW.Call(
				uintptr(unsafe.Pointer(p)), frPrivate, 0)
		}
	}
	ctx.registeredFonts = nil
	ctx.metrics = metricsCache{}
}

// ScaleFactor returns the DPI scale factor.
func (ctx *Context) ScaleFactor() float32 { return ctx.scaleFactor }

// AddFontFile registers a font file with GDI for the current process.
func (ctx *Context) AddFontFile(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("glyph: invalid font path: %w", err)
	}
	const frPrivate = 0x10
	ret, _, callErr := procAddFontResourceExW.Call(
		uintptr(unsafe.Pointer(p)), frPrivate, 0)
	if ret == 0 {
		return fmt.Errorf(
			"glyph: AddFontResourceExW failed for %q: %v", path, callErr)
	}
	ctx.registeredFonts = append(ctx.registeredFonts, path)
	return nil
}

// FontHeight returns ascent + descent in logical pixels.
func (ctx *Context) FontHeight(cfg TextConfig) (float32, error) {
	ctx.selectFont(cfg.Style)
	tm := ctx.gdi.getTextMetrics()
	h := float32(tm.TmAscent+tm.TmDescent) * ctx.scaleInv
	return h, nil
}

// FontMetrics returns detailed metrics in logical pixels.
func (ctx *Context) FontMetrics(cfg TextConfig) (TextMetrics, error) {
	ctx.selectFont(cfg.Style)
	tm := ctx.gdi.getTextMetrics()
	inv := ctx.scaleInv
	asc := float32(tm.TmAscent) * inv
	dsc := float32(tm.TmDescent) * inv
	return TextMetrics{
		Ascender:  asc,
		Descender: dsc,
		Height:    asc + dsc,
		LineGap:   float32(tm.TmExternalLeading) * inv,
	}, nil
}

// ResolveFontName returns the input name unchanged on Windows.
func (ctx *Context) ResolveFontName(name string) (string, error) {
	return name, nil
}

// selectFont configures the GDI context for the given style.
func (ctx *Context) selectFont(style TextStyle) {
	ctx.gdi.selectFont(winFontParams(style, ctx.scaleFactor))
}

// parseFontDesc extracts family name and point size from a Pango-style
// font description string like "Sans Bold 14".
func parseFontDesc(style TextStyle) (string, float32) {
	size := style.Size
	name := style.FontName
	if name == "" {
		name = "Segoe UI"
	}

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "Segoe UI", max(size, 12)
	}

	// Strip trailing number (size).
	end := len(parts)
	if end > 0 {
		var sz float32
		if _, err := fmt.Sscanf(parts[end-1], "%f", &sz); err == nil && sz > 0 {
			if size <= 0 {
				size = sz
			}
			end--
		}
	}

	// Strip style keywords.
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

	family := strings.Join(parts[:end], " ")
	family = mapWindowsFamily(family)

	if size <= 0 {
		size = 12
	}
	return family, size
}

// mapWindowsFamily maps generic font families to Windows equivalents.
func mapWindowsFamily(family string) string {
	switch strings.ToLower(family) {
	case "sans", "sans-serif":
		return "Segoe UI"
	case "serif":
		return "Times New Roman"
	case "monospace", "mono":
		return "Consolas"
	default:
		return family
	}
}

// registerWindowsFonts is a no-op — GDI discovers system fonts.
func (ctx *Context) registerWindowsFonts() {}

// windowsFontDirs returns standard Windows font directories.
func windowsFontDirs() []string {
	dirs := []string{filepath.Join(os.Getenv("WINDIR"), "Fonts")}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	return dirs
}
