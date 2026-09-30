//go:build linux || darwin || windows

package glyph

import (
	"reflect"
	"testing"

	"github.com/go-text/typesetting/font"
)

// fakeCmap is a map-backed font.Cmap so orderTextFallbacks can be unit-tested
// deterministically by seeding coverageCache, independent of host fonts.
type fakeCmap map[rune]font.GID

func (f fakeCmap) Lookup(r rune) (font.GID, bool) {
	g, ok := f[r]
	return g, ok
}

func (f fakeCmap) Iter() font.CmapIter { return &fakeCmapIter{} }

// fakeCmapIter yields nothing; orderTextFallbacks only uses Lookup.
type fakeCmapIter struct{}

func (it *fakeCmapIter) Next() bool             { return false }
func (it *fakeCmapIter) Char() (rune, font.GID) { return 0, 0 }

// seedCoverage installs a fake coverage entry for path and removes it when
// the test ends, so seeded fakes never leak into other tests' probes.
func seedCoverage(t *testing.T, path string, cov *coverage) {
	t.Helper()
	coverageCache.mu.Lock()
	coverageCache.items[path] = cov
	coverageCache.mu.Unlock()
	t.Cleanup(func() {
		coverageCache.mu.Lock()
		delete(coverageCache.items, path)
		coverageCache.mu.Unlock()
	})
}

// TestOrderTextFallbacks exercises the shared selection policy directly with
// seeded coverage: covering monochrome fonts partition before covering color
// fonts, each preserving input order; non-covering and unparseable (nil
// coverage) paths are dropped.
func TestOrderTextFallbacks(t *testing.T) {
	star := 'X'
	cm := fakeCmap{star: 1}
	seedCoverage(t, "fake:color1", &coverage{cmap: cm, color: true})
	seedCoverage(t, "fake:mono1", &coverage{cmap: cm, color: false})
	seedCoverage(t, "fake:color2", &coverage{cmap: cm, color: true})
	seedCoverage(t, "fake:mono2", &coverage{cmap: cm, color: false})
	seedCoverage(t, "fake:miss", &coverage{cmap: fakeCmap{}, color: false})
	seedCoverage(t, "fake:bad", nil)

	paths := []string{
		"fake:color1", "fake:miss", "fake:mono1",
		"fake:bad", "fake:color2", "fake:mono2",
	}
	mono, color := orderTextFallbacks(paths, "", string(star))
	if want := []string{"fake:mono1", "fake:mono2"}; !reflect.DeepEqual(mono, want) {
		t.Errorf("mono = %v, want %v", mono, want)
	}
	if want := []string{"fake:color1", "fake:color2"}; !reflect.DeepEqual(color, want) {
		t.Errorf("color = %v, want %v", color, want)
	}
}

// TestOrderTextFallbacksEmpty: nil/empty fallback list yields empty partitions
// (no panic), and text covered by nothing yields empty partitions.
func TestOrderTextFallbacksEmpty(t *testing.T) {
	mono, color := orderTextFallbacks(nil, "", "X")
	if len(mono) != 0 || len(color) != 0 {
		t.Errorf("nil paths: mono=%v color=%v, want empty", mono, color)
	}
	seedCoverage(t, "fake:empty", &coverage{cmap: fakeCmap{}, color: false})
	mono, color = orderTextFallbacks([]string{"fake:empty"}, "", "X")
	if len(mono) != 0 || len(color) != 0 {
		t.Errorf("no coverage: mono=%v color=%v, want empty", mono, color)
	}
}

// TestProbeFallbackLatinPrefersBase is the #146 regression: a Latin letter
// the primary face lacks must resolve to the default sans (fallbackBase), not
// to a CJK face that also covers it and sorts first in the tier list. Before
// the fix the CJK collection won and its ~30 MB of tables stayed in
// faceCache. The text path decides by coverage alone, so ensureFont (the face
// load) must not run during the probe.
func TestProbeFallbackLatinPrefersBase(t *testing.T) {
	cm := fakeCmap{'H': 1}
	seedCoverage(t, "fake:sans", &coverage{cmap: cm})
	seedCoverage(t, "fake:cjk", &coverage{cmap: cm})
	ctx := &Context{
		fallbackPaths: []string{"fake:cjk"},
		fallbackBase:  "fake:sans",
	}

	loads := 0
	ensure := func(string) ftFont { loads++; return ftFont{} }
	path, isColor := ctx.probeFallback("H", false, ensure)
	if path != "fake:sans" || isColor {
		t.Errorf("probeFallback(H) = %q (color=%v), want fake:sans", path, isColor)
	}
	if loads != 0 {
		t.Errorf("probe loaded %d faces, want 0", loads)
	}
}

// TestOrderTextFallbacksBaseScopedToLatin: the default sans goes first only
// for Latin, Greek, Cyrillic and script-neutral text. For Arabic it keeps
// tier order, even when the sans covers Arabic (DejaVu Sans, Segoe UI do), so
// the script-tier face made for Arabic still wins.
func TestOrderTextFallbacksBaseScopedToLatin(t *testing.T) {
	cm := fakeCmap{'H': 1, 'Ж': 2, '→': 3, 'ا': 4}
	seedCoverage(t, "fake:sans", &coverage{cmap: cm})
	seedCoverage(t, "fake:cjk", &coverage{cmap: fakeCmap{'H': 1, 'Ж': 2, '→': 3}})
	seedCoverage(t, "fake:arabic", &coverage{cmap: fakeCmap{'ا': 1}})
	// fake:sans is also in the general tier, as a host sans can be.
	paths := []string{"fake:cjk", "fake:arabic", "fake:sans"}

	tests := []struct {
		text string
		want []string
	}{
		{"H", []string{"fake:sans", "fake:cjk"}},
		{"Ж", []string{"fake:sans", "fake:cjk"}},
		{"→", []string{"fake:sans", "fake:cjk"}},
		{"H́", nil}, // combining mark: Inherited, but no font covers U+0301
		{"ا", []string{"fake:arabic", "fake:sans"}},
	}
	for _, tt := range tests {
		mono, _ := orderTextFallbacks(paths, "fake:sans", tt.text)
		if !reflect.DeepEqual(mono, tt.want) {
			t.Errorf("%q: mono = %v, want %v", tt.text, mono, tt.want)
		}
	}
}

// TestOrderTextFallbacksBaseMissesKeepsTierOrder: a base that does not cover
// the text, or has no coverage entry, leaves the tier order unchanged.
func TestOrderTextFallbacksBaseMissesKeepsTierOrder(t *testing.T) {
	seedCoverage(t, "fake:sans", &coverage{cmap: fakeCmap{}})
	seedCoverage(t, "fake:bad", nil)
	seedCoverage(t, "fake:cjk", &coverage{cmap: fakeCmap{'H': 1}})
	for _, base := range []string{"fake:sans", "fake:bad"} {
		mono, _ := orderTextFallbacks([]string{"fake:cjk"}, base, "H")
		if want := []string{"fake:cjk"}; !reflect.DeepEqual(mono, want) {
			t.Errorf("base %q: mono = %v, want %v", base, mono, want)
		}
	}
}

func TestUsesBaseFallback(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"H", true},
		{"Ω", true},  // Greek
		{"Ж", true},  // Cyrillic
		{"7", true},  // Common
		{"é", true}, // Inherited combining mark
		{"中", false},
		{"ا", false},
		{"", false}, // PUA: script Unknown
		{"H中", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := usesBaseFallback(tt.text); got != tt.want {
			t.Errorf("usesBaseFallback(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

// TestRenderTextPathMatchesProbeFallback guards the #5 unification: the
// render-side text path (orderedTextFallbackPaths) must try fonts in the
// order the layout-time selector (probeFallback) picks them, so a cluster
// resolved at layout and one rasterized by the text path choose the same
// font. Host-tolerant: symbols nothing covers are skipped.
func TestRenderTextPathMatchesProbeFallback(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no fallback fonts discovered")
	}

	ensure := func(p string) ftFont {
		return newFTFontFromPath(ctx.ftLib, p, 16)
	}

	for _, sym := range []string{"✓", "⏵", "中", "∑", "☀"} {
		gotPath, gotColor := ctx.probeFallback(sym, false, ensure)
		cands := orderedTextFallbackPaths(sym)

		if gotPath == "" {
			if len(cands) != 0 {
				t.Errorf("%q: probeFallback found nothing but render path has candidates %v",
					sym, cands)
			}
			continue
		}
		if len(cands) == 0 || cands[0] != gotPath {
			t.Errorf("%q: render path first candidate != layout choice %q (isColor=%v); candidates %v",
				sym, gotPath, gotColor, cands)
		}
	}
}

// TestOrderedTextFallbackPathsMonoFirst verifies the text-presentation policy
// in the render path: every monochrome candidate precedes every color
// candidate, regardless of tier order (color/emoji tiers sort first in the
// raw fallback list).
func TestOrderedTextFallbackPathsMonoFirst(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	for _, sym := range []string{"✳", "✂", "❄", "⏵"} {
		seenColor := false
		for _, p := range orderedTextFallbackPaths(sym) {
			cov := loadCoverage(p)
			if cov == nil {
				t.Fatalf("%q: candidate %q has no coverage", sym, p)
			}
			if cov.color {
				seenColor = true
			} else if seenColor {
				t.Errorf("%q: monochrome font %q sorted after a color font", sym, p)
			}
		}
	}
}
