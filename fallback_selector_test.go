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
	mono, color := orderTextFallbacks(paths, string(star))
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
	mono, color := orderTextFallbacks(nil, "X")
	if len(mono) != 0 || len(color) != 0 {
		t.Errorf("nil paths: mono=%v color=%v, want empty", mono, color)
	}
	seedCoverage(t, "fake:empty", &coverage{cmap: fakeCmap{}, color: false})
	mono, color = orderTextFallbacks([]string{"fake:empty"}, "X")
	if len(mono) != 0 || len(color) != 0 {
		t.Errorf("no coverage: mono=%v color=%v, want empty", mono, color)
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
