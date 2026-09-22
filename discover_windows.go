//go:build windows

package glyph

import (
	"os"
	"path/filepath"
)

// windowsDir returns the Windows directory. %WINDIR% can be missing (for
// example in a process started with an empty environment); without a
// fallback, the font dir would become the relative path "Fonts" and resolve
// against the working directory.
func windowsDir() string {
	for _, k := range []string{"WINDIR", "SystemRoot"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return `C:\Windows`
}

// systemFontDirs lists the system font directory and the per-user font
// directory (fonts installed "for me only").
func systemFontDirs() []string {
	dirs := []string{filepath.Join(windowsDir(), "Fonts")}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs,
			filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	return dirs
}

// platformAliases lists the Windows families that may fill each generic
// alias, most preferred first. See aliasTable. Rank 0 matches
// platformGenerics, so the alias and resolveFontFamily agree. Whole-name
// matching keeps Segoe UI Emoji, Segoe UI Symbol, and Arial Black out.
var platformAliases = aliasTable{
	"monospace":  {"consolas", "courier new", "lucida console"},
	"serif":      {"times new roman", "georgia"},
	"sans-serif": {"segoe ui", "arial"},
}

// platformGenerics are the families the generic names resolve to on
// Windows.
var platformGenerics = genericNames{
	sans:  "Segoe UI",
	serif: "Times New Roman",
	mono:  "Consolas",
}

// ensurePlatformDefaults makes sure a sans-serif face exists even when the
// walk found none. Segoe UI ships with every supported Windows release.
func ensurePlatformDefaults(fontPaths map[string]string) {
	fillDefaultKeys(fontPaths,
		filepath.Join(windowsDir(), "Fonts", "segoeui.ttf"),
		"Segoe UI", "sans-serif")
}
