//go:build darwin && !glyph_pango

package glyph

import (
	"fmt"
	"testing"
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
		{"Font 12.5", 12}, // fractional truncated at dot
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
		{"Bold", "Bold"}, // lone style word preserved (end==1)
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

func BenchmarkResolveCTFontParams(b *testing.B) {
	style := TextStyle{FontName: "Fira Code Bold 14"}
	// Warm parse + resolve cache.
	resolveCTFontParams(style, 2.0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = resolveCTFontParams(style, 2.0)
	}
}

func BenchmarkLookupParsedFontName(b *testing.B) {
	name := "Fira Code Bold 14"
	lookupParsedFontName(name)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = lookupParsedFontName(name)
	}
}

func TestResolveFontFamilyDarwin(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Sans 12", ".AppleSystemUIFont"},
		{"sans-serif Bold 14", ".AppleSystemUIFont"},
		{"Serif 11", "New York"},
		{"Monospace 10", "SF Mono"},
		{"mono Bold 12", "SF Mono"},
		{"system 16", ".AppleSystemUIFont"},
		{"Fira Code 12", "Fira Code"},
		{"", ".AppleSystemUIFont"},
	}
	for _, tt := range tests {
		got := resolveFontFamilyDarwin(tt.name)
		if got != tt.want {
			t.Errorf("resolveFontFamilyDarwin(%q) = %q, want %q",
				tt.name, got, tt.want)
		}
	}
}

func TestComputeParsedFontName_BoldItalicFlags(t *testing.T) {
	tests := []struct {
		name       string
		wantBold   bool
		wantItalic bool
	}{
		{"Sans Bold 14", true, false},
		{"Serif Italic 12", false, true},
		{"Mono Bold Italic 16", true, true},
		{"Fira Code 12", false, false},
		{"", false, false},
		{"BOLD 14", false, false},           // no leading space → not detected
		{"Noto Bold Italic 12", true, true}, // both embedded style words present
	}
	for _, tt := range tests {
		p := computeParsedFontName(tt.name)
		if p.hasBold != tt.wantBold || p.hasItalic != tt.wantItalic {
			t.Errorf("computeParsedFontName(%q): bold=%v italic=%v, want bold=%v italic=%v",
				tt.name, p.hasBold, p.hasItalic, tt.wantBold, tt.wantItalic)
		}
	}
}

func TestLookupParsedFontName_CacheOverflow(t *testing.T) {
	// Replace global cache with one already at capacity so the overflow
	// path (entry silently dropped) is exercised without affecting other tests.
	fontNameParseMu.Lock()
	saved := fontNameParseCache
	fontNameParseCache = make(map[string]parsedFontName, fontNameParseCacheMax+1)
	for i := 0; i < fontNameParseCacheMax; i++ {
		fontNameParseCache[fmt.Sprintf("TestOverflowFont%d 12", i)] = parsedFontName{}
	}
	fontNameParseMu.Unlock()
	t.Cleanup(func() {
		fontNameParseMu.Lock()
		fontNameParseCache = saved
		fontNameParseMu.Unlock()
	})

	overflow := fmt.Sprintf("TestOverflowFont%d 12", fontNameParseCacheMax)
	p := lookupParsedFontName(overflow)
	if p.size != 12 {
		t.Errorf("overflow lookup size = %v, want 12", p.size)
	}

	fontNameParseMu.RLock()
	_, stored := fontNameParseCache[overflow]
	fontNameParseMu.RUnlock()
	if stored {
		t.Error("overflow entry must not be stored when cache is at capacity")
	}
}

func TestCTFont_MetricsNullRef(t *testing.T) {
	f := ctFont{} // ref is nil/0 — exercises the null-guard added to metrics()
	a, d, l := f.metrics()
	if a != 0 || d != 0 || l != 0 {
		t.Errorf("null ref metrics = (%v, %v, %v), want (0, 0, 0)", a, d, l)
	}
}
