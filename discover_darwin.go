//go:build darwin

package glyph

import (
	"os"
	"path/filepath"
)

// systemFontDirs lists the macOS font directories: system, local, and the
// user's own (only when the home directory is known).
func systemFontDirs() []string {
	dirs := []string{
		"/System/Library/Fonts",
		"/Library/Fonts",
	}
	if home, _ := os.UserHomeDir(); home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
	}
	return dirs
}

// platformAliases lists the macOS families that may fill each generic
// alias, most preferred first. See aliasTable.
var platformAliases = aliasTable{
	"monospace":  {"menlo", "sf mono", "monaco"},
	"serif":      {"times new roman", "times"},
	"sans-serif": {"helvetica", "helvetica neue"},
}

// platformGenerics are the families the generic names resolve to on macOS.
var platformGenerics = genericNames{
	sans:  "Helvetica",
	serif: "Times New Roman",
	mono:  "Menlo",
}

// ensurePlatformDefaults makes sure a sans-serif face exists even when the
// walk found none. Helvetica ships with every macOS release.
func ensurePlatformDefaults(fontPaths map[string]string) {
	fillDefaultKeys(fontPaths, "/System/Library/Fonts/Helvetica.ttc",
		"Helvetica", "sans-serif")
}
