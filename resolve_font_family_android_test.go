//go:build android

package glyph

import (
	"testing"

	"github.com/go-text/typesetting/font"
)

func TestParseSizeFromFontName(t *testing.T) {
	tests := []struct {
		name string
		want float32
	}{
		{"Sans Bold 18", 18},
		{"Monospace 12", 12},
		{"Sans", 0},
		{"", 0},
		{"Sans Bold", 0},
		{"Serif 0", 0},
		{"Mono 100", 100},
		{"Font 12.5", 12},
	}
	for _, tt := range tests {
		got := parseSizeFromFontName(tt.name)
		if got != tt.want {
			t.Errorf("parseSizeFromFontName(%q) = %v, want %v",
				tt.name, got, tt.want)
		}
	}
}

func TestParseFamilyFromFontName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Sans Bold 18", "Sans"},
		{"Noto Sans Bold Italic 14", "Noto Sans"},
		{"Monospace 12", "Monospace"},
		{"Liberation Mono Bold", "Liberation Mono"},
		{"Sans", "Sans"},
		{"Bold", "Bold"},
		{"", ""},
		{"Fira Code Light 11", "Fira Code"},
		{"Serif Regular 16", "Serif"},
	}
	for _, tt := range tests {
		got := parseFamilyFromFontName(tt.name)
		if got != tt.want {
			t.Errorf("parseFamilyFromFontName(%q) = %q, want %q",
				tt.name, got, tt.want)
		}
	}
}

func TestResolveFontFamily(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Sans 12", "Roboto"},
		{"sans-serif Bold 14", "Roboto"},
		{"Serif 11", "Noto Serif"},
		{"Monospace 10", "Roboto Mono"},
		{"mono Bold 12", "Roboto Mono"},
		{"system 16", "Roboto"},
		{"Fira Code 12", "Fira Code"},
		{"", "Roboto"},
	}
	for _, tt := range tests {
		got := resolveFontFamily(tt.name)
		if got != tt.want {
			t.Errorf("resolveFontFamily(%q) = %q, want %q",
				tt.name, got, tt.want)
		}
	}
}

func TestAndroidGenericAlias(t *testing.T) {
	tests := []struct {
		family string
		want   string
	}{
		{"roboto", "sans-serif"},
		{"roboto condensed", "sans-serif"},
		{"noto sans kr", "sans-serif"},
		{"droid sans", "sans-serif"},
		{"roboto mono", "monospace"},
		{"noto sans mono", "monospace"},
		{"droid sans mono", "monospace"},
		{"noto mono", "monospace"},
		{"cutive mono", "monospace"},
		{"noto serif", "serif"},
		{"droid serif", "serif"},
		{"some unknown font", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := androidGenericAlias(tt.family)
		if got != tt.want {
			t.Errorf("androidGenericAlias(%q) = %q, want %q",
				tt.family, got, tt.want)
		}
	}
}

func TestEnsureAndroidDefaults(t *testing.T) {
	// Empty maps: all three keys should be filled.
	ctx := &Context{
		fontPaths:   make(map[string]string),
		fontWeights: make(map[string]font.Weight),
	}
	ctx.ensureAndroidDefaults()
	const roboto = "/system/fonts/Roboto-Regular.ttf"
	if v := ctx.fontPaths["Roboto"]; v != roboto {
		t.Errorf("Roboto = %q, want %q", v, roboto)
	}
	if v := ctx.fontPaths["Roboto-Regular"]; v != roboto {
		t.Errorf("Roboto-Regular = %q, want %q", v, roboto)
	}
	if v := ctx.fontPaths["sans-serif"]; v != roboto {
		t.Errorf("sans-serif = %q, want %q", v, roboto)
	}

	// Pre-existing keys must not be overwritten.
	ctx2 := &Context{
		fontPaths: map[string]string{
			"Roboto": "/custom/Roboto.ttf",
		},
	}
	ctx2.ensureAndroidDefaults()
	if v := ctx2.fontPaths["Roboto"]; v != "/custom/Roboto.ttf" {
		t.Error("ensureAndroidDefaults overwrote existing Roboto key")
	}
}
