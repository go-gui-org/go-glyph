//go:build linux || darwin || windows

package glyph

import (
	"math"
	"testing"

	"github.com/go-text/typesetting/font"
)

func TestIsPUA(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'A', false},
		{0x2713, false},  // check mark: a real Unicode symbol, not PUA
		{0xDFFF, false},  // just below the BMP PUA
		{0xE000, true},   // BMP PUA start
		{0xF608, true},   // the Nerd Font slot that picked up Athelas
		{0xF8FF, true},   // BMP PUA end
		{0xF900, false},  // CJK compatibility ideographs, just past the PUA
		{0xF0004, true},  // plane 15 (Material Design icons)
		{0x100001, true}, // plane 16
	}
	for _, tt := range tests {
		if got := isPUA(tt.r); got != tt.want {
			t.Errorf("isPUA(U+%04X) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

// textIsPUA must look only at the base rune: a variation selector or combining
// mark after an ordinary character must not make the cluster look private-use,
// and an icon followed by one must still rank as an icon.
func TestTextIsPUA(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"", false},
		{"A", false},
		{"", true},
		{"️", true},
		{"A", false},
	}
	for _, tt := range tests {
		if got := textIsPUA(tt.text); got != tt.want {
			t.Errorf("textIsPUA(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

// A PUA cluster must resolve to the font with real icon coverage, whatever the
// fallback tier order says: the first covering font is routinely a text face
// that maps the slot to an unrelated glyph.
func TestRankIconFallbacksPrefersIconFont(t *testing.T) {
	seedCoverage(t, "text.ttf", &coverage{iconScore: 0})
	seedCoverage(t, "partial-icons.ttf", &coverage{iconScore: 3})
	seedCoverage(t, "nerd.ttf", &coverage{iconScore: 18})

	got := rankIconFallbacks(
		[]string{"text.ttf", "partial-icons.ttf", "nerd.ttf"}, "\uF608")
	want := []string{"nerd.ttf", "partial-icons.ttf"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// With no icon font covering the codepoint the candidate list must come back
// empty so the base font's .notdef renders. A serif "D" standing in for a
// folder icon is worse than a visible tofu box: it hides the missing font. An
// unparseable candidate (nil coverage) is dropped the same way, without
// panicking.
func TestRankIconFallbacksDropsNonIconFonts(t *testing.T) {
	seedCoverage(t, "athelas.ttc", &coverage{iconScore: 0})
	seedCoverage(t, "broken.ttf", nil)
	if got := rankIconFallbacks(
		[]string{"athelas.ttc", "broken.ttf"}, "\uF608"); len(got) != 0 {
		t.Fatalf("got %v, want no candidates", got)
	}
}

// Ordinary text must be untouched by icon ranking — order there is the
// fallback tier order, which encodes script and locale preference.
func TestRankIconFallbacksLeavesTextAlone(t *testing.T) {
	seedCoverage(t, "cjk.ttc", &coverage{iconScore: 0})
	seedCoverage(t, "nerd.ttf", &coverage{iconScore: 18})

	got := rankIconFallbacks([]string{"cjk.ttc", "nerd.ttf"}, "✓")
	if len(got) != 2 || got[0] != "cjk.ttc" || got[1] != "nerd.ttf" {
		t.Fatalf("got %v, want tier order preserved", got)
	}
}

// TestScoreIconCoverage counts mapped icon probes: a full Nerd Font layout
// scores every probe, a partial icon font its subset, and a plain text face 0
// — the signal rankIconFallbacks sorts and filters on.
func TestScoreIconCoverage(t *testing.T) {
	all := fakeCmap{}
	for i, r := range iconProbeRunes {
		all[r] = font.GID(i + 1)
	}
	if got := scoreIconCoverage(&coverage{cmap: all}); got != int8(len(iconProbeRunes)) {
		t.Errorf("full coverage: got %d, want %d", got, len(iconProbeRunes))
	}
	partial := fakeCmap{iconProbeRunes[0]: 1, iconProbeRunes[7]: 2}
	if got := scoreIconCoverage(&coverage{cmap: partial}); got != 2 {
		t.Errorf("partial coverage: got %d, want 2", got)
	}
	if got := scoreIconCoverage(&coverage{cmap: fakeCmap{}}); got != 0 {
		t.Errorf("no coverage: got %d, want 0", got)
	}
}

func TestFallbackFitScale(t *testing.T) {
	face := func(cap float64) *cachedFace {
		return &cachedFace{cap: cap, capMeasured: true}
	}
	tests := []struct {
		name        string
		base, fb    *cachedFace
		want        float64
		wantExactly bool
	}{
		{name: "nil base", base: nil, fb: face(0.7), want: 1, wantExactly: true},
		{name: "unmeasurable fallback (emoji/ideographic only)",
			base: face(0.73), fb: face(0), want: 1, wantExactly: true},
		{name: "within deadband",
			base: face(0.73), fb: face(0.72), want: 1, wantExactly: true},
		{name: "short fallback scales up",
			base: face(0.73), fb: face(0.671), want: 0.73 / 0.671},
		{name: "tall fallback scales down",
			base: face(0.66), fb: face(0.93), want: minFitScale, wantExactly: true},
		{name: "clamped at the top",
			base: face(0.9), fb: face(0.3), want: maxFitScale, wantExactly: true},
	}
	for _, tt := range tests {
		got := fallbackFitScale(tt.base, tt.fb)
		if tt.wantExactly {
			if got != tt.want {
				t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
			}
			continue
		}
		if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// The zero value must read as "unscaled" so every non-fallback item, and any
// Item built by a caller that predates the field, rasterizes unchanged.
func TestItemFontScaleZeroMeansUnscaled(t *testing.T) {
	if got := itemFontScale(Item{}); got != 1 {
		t.Errorf("itemFontScale(zero Item) = %v, want 1", got)
	}
	if got := itemFontScale(Item{FontScale: 1.088}); got != 1.088 {
		t.Errorf("itemFontScale(1.088) = %v, want 1.088", got)
	}
}

// TestCapEmSystemFonts exercises the lazy cap-height measurement against real
// host fonts: the first face with a measurable cap must measure, memoize
// (second call returns the same value and marks the face measured), satisfy
// the identity fit (fallbackFitScale(cf, cf) == 1 within the deadband), and
// the render-side plumbing (textFallbackFitScale([]string{p}, p) == 1).
// Skipped when no host font yields a measurable cap (icon-only font sets).
func TestCapEmSystemFonts(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	// Sampling a handful of the fallback set bounds the test's face parses:
	// capEm needs a parsed face, and the first text font found is enough.
	for _, p := range ctx.fallbackPaths {
		cf := loadCachedFace(p)
		if cf == nil {
			continue
		}
		c0 := cf.capEm()
		if !cf.capMeasured {
			t.Errorf("%s: capEm did not mark the face measured", p)
		}
		if c1 := cf.capEm(); c1 != c0 {
			t.Errorf("%s: capEm not memoized: %v then %v", p, c0, c1)
		}
		if c0 == 0 {
			continue
		}
		if fit := fallbackFitScale(cf, cf); fit != 1 {
			t.Errorf("%s: identity fit = %v, want 1", p, fit)
		}
		if fit := textFallbackFitScale([]string{p}, p); fit != 1 {
			t.Errorf("%s: text-path identity fit = %v, want 1", p, fit)
		}
		return // one measurable face is enough
	}
	t.Skip("no host font with a measurable cap height; skipping")
}

// TestFallbackItemFontScaleMatchesAdvance checks the renderer-side contract
// of FontScale end to end: a fallback item's glyph advances were shaped at
// the cap-height-matched size (base size × FontScale), so re-measuring the
// cluster at that same derived size must reproduce the advance. This keeps
// the by-id raster (loadGlyphByIDFT re-derives the size from the item's
// style and multiplies by FontScale) aligned with the advances the layout
// recorded. Skipped when no host font set triggers a scaled fallback.
func TestFallbackItemFontScaleMatchesAdvance(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no fallback fonts installed; skipping")
	}

	layout, err := ctx.LayoutText("a\u2713b", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}

	for _, item := range layout.Items {
		if item.FontScale == 0 || item.UseOriginalColor {
			continue // base-font item, or an emoji cell (width is a cell, not an advance)
		}
		_, baseSize, _, _ := resolveFTFontParams(item.Style, 1.0)
		f := newFTFontFromPath(ctx.ftLib, item.FontPath,
			baseSize*itemFontScale(item))
		if f.face == nil {
			t.Fatalf("fallback item font %q did not open", item.FontPath)
		}
		for i := item.GlyphStart; i < item.GlyphStart+item.GlyphCount; i++ {
			if i < 0 || i >= len(layout.Glyphs) {
				continue
			}
			g := layout.Glyphs[i]
			ch := glyphText(layout.Text, g)
			if ch == "" {
				continue
			}
			adv := f.measureString(ch)
			if diff := math.Abs(adv - float64(g.XAdvance)); diff > 0.01 {
				t.Errorf("item %q at %gpx (fit %g): layout advance %g, re-measured %g (diff %g)",
					ch, baseSize*itemFontScale(item), item.FontScale,
					g.XAdvance, adv, diff)
			}
		}
		f.close()
	}
}

// TestHashGlyphStyleFontScale: items that differ only in FontScale must key
// apart in the glyph cache, so a cap-matched fallback raster never collides
// with the unscaled raster of the same glyph id, font path and style.
func TestHashGlyphStyleFontScale(t *testing.T) {
	k0 := hashGlyphStyle(fnvOffsetBasis, Item{}, 0, 1, 0)
	k1 := hashGlyphStyle(fnvOffsetBasis, Item{FontScale: 1.1}, 0, 1, 0)
	if k0 == k1 {
		t.Error("different FontScale values produced the same glyph-cache key")
	}
}
