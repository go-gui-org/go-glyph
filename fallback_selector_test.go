//go:build linux || darwin || windows

package glyph

import "testing"

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
