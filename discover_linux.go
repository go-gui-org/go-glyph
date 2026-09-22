//go:build linux && !android

package glyph

import (
	"os"
	"path/filepath"
	"strings"
)

// systemFontDirs lists the Linux font directories. go-text reads the
// family names; there is no fontconfig dependency.
func systemFontDirs() []string {
	home, _ := os.UserHomeDir()
	return linuxFontDirs(home, os.Getenv("XDG_DATA_HOME"),
		os.Getenv("XDG_DATA_DIRS"))
}

// linuxFontDirs builds the font directory list from the XDG Base Directory
// variables, user dirs first so user fonts win on ties:
//
//  1. $XDG_DATA_HOME/fonts (default ~/.local/share/fonts)
//  2. ~/.fonts (legacy)
//  3. <dir>/fonts for each dir in $XDG_DATA_DIRS
//  4. /usr/local/share/fonts and /usr/share/fonts
//
// Step 3 finds fonts that Flatpak, Guix, and NixOS profiles put outside
// /usr/share. Step 4 always runs, because a custom $XDG_DATA_DIRS often
// omits the standard dirs. Relative entries are dropped (the XDG spec
// says to ignore them), home-based entries are dropped when home is
// unknown, and duplicates are removed so no dir is walked twice. Pure, so
// it is unit-testable.
func linuxFontDirs(home, dataHome, dataDirs string) []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(dir string) {
		if !filepath.IsAbs(dir) {
			return
		}
		dir = filepath.Clean(dir)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	if !filepath.IsAbs(dataHome) && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	add(filepath.Join(dataHome, "fonts"))
	if home != "" {
		add(filepath.Join(home, ".fonts"))
	}
	for d := range strings.SplitSeq(dataDirs, ":") {
		if d != "" {
			add(filepath.Join(d, "fonts"))
		}
	}
	add("/usr/local/share/fonts")
	add("/usr/share/fonts")
	return dirs
}

// platformAliases lists the Linux families that may fill each generic
// alias, most preferred first, across the common packages (DejaVu,
// Liberation, Noto). See aliasTable. Whole-name matching keeps Noto Sans
// script fonts (Noto Sans Arabic, Noto Sans CJK, …) out of sans-serif.
var platformAliases = aliasTable{
	"monospace": {
		"dejavu sans mono", "liberation mono", "noto sans mono", "noto mono",
	},
	"serif":      {"dejavu serif", "liberation serif", "noto serif"},
	"sans-serif": {"dejavu sans", "liberation sans", "noto sans"},
}

// platformGenerics are the families the generic names resolve to on
// Linux. When DejaVu is missing, resolveFontPath degrades to the alias,
// which then holds the most preferred family that is installed.
var platformGenerics = genericNames{
	sans:  "DejaVu Sans",
	serif: "DejaVu Serif",
	mono:  "DejaVu Sans Mono",
}

// ensurePlatformDefaults does nothing on Linux: font paths differ per
// distribution, so there is no fixed path that is safe to assume.
func ensurePlatformDefaults(map[string]string) {}
