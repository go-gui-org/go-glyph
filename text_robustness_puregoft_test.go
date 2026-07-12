//go:build linux || darwin || windows

package glyph

import (
	"strconv"
	"testing"
)

// TestIsDefaultIgnorable checks the boundary of every default-ignorable
// range faceCovers relies on: ignorables must be reported so a cluster of
// (base + joiner/variation-selector) is not forced to a fallback.
func TestIsDefaultIgnorable(t *testing.T) {
	ignorable := []rune{
		0x00AD, 0xFEFF, // soft hyphen, ZWNBSP/BOM
		0x200B, 0x200D, 0x200F, // ZW space, ZWJ, RLM
		0x2060, 0x206F, // word joiner .. bidi ops (range ends)
		0xFE00, 0xFE0F, // variation selectors (range ends)
		0xE0000, 0xE0FFF, // tags + VS supplement (range ends)
	}
	for _, r := range ignorable {
		if !isDefaultIgnorable(r) {
			t.Errorf("isDefaultIgnorable(%#U) = false, want true", r)
		}
	}
	// Just-outside-range and ordinary code points must not be ignorable.
	notIgnorable := []rune{
		'a', '0', 0x00AC, 0x200A, 0x2010, 0x2070, 0xFDFF, 0xFE10, 0xE1000,
	}
	for _, r := range notIgnorable {
		if isDefaultIgnorable(r) {
			t.Errorf("isDefaultIgnorable(%#U) = true, want false", r)
		}
	}
}

// TestLooksMonospace verifies the fixed-width name heuristic that steers an
// unresolved lookup to the "monospace" generic alias instead of proportional
// "sans-serif".
func TestLooksMonospace(t *testing.T) {
	mono := []string{
		"DejaVu Sans Mono", "Consolas", "Courier New", "Menlo", "Fixedsys",
		"JetBrainsMono Nerd Font",
	}
	for _, f := range mono {
		if !looksMonospace(f) {
			t.Errorf("looksMonospace(%q) = false, want true", f)
		}
	}
	proportional := []string{"Arial", "DejaVu Serif", "Noto Sans", "Georgia", ""}
	for _, f := range proportional {
		if looksMonospace(f) {
			t.Errorf("looksMonospace(%q) = true, want false", f)
		}
	}
}

// TestGenericFallback verifies that a monospace-looking family prefers the
// "monospace" alias when registered, and that both cases degrade to
// "sans-serif" (or empty) as documented.
func TestGenericFallback(t *testing.T) {
	full := map[string]string{
		"monospace":  "/mono.ttf",
		"sans-serif": "/sans.ttf",
	}
	if got := genericFallback(full, "Courier New"); got != "/mono.ttf" {
		t.Errorf("genericFallback(mono family) = %q, want /mono.ttf", got)
	}
	if got := genericFallback(full, "Arial"); got != "/sans.ttf" {
		t.Errorf("genericFallback(prop family) = %q, want /sans.ttf", got)
	}

	// A monospace request with no monospace alias must not degrade to a
	// missing key; it falls through to sans-serif.
	sansOnly := map[string]string{"sans-serif": "/sans.ttf"}
	if got := genericFallback(sansOnly, "Menlo"); got != "/sans.ttf" {
		t.Errorf("genericFallback(mono, no mono alias) = %q, want /sans.ttf", got)
	}

	if got := genericFallback(map[string]string{}, "Menlo"); got != "" {
		t.Errorf("genericFallback(empty) = %q, want empty", got)
	}
}

// TestCacheFallbackBounded verifies the resolution cache lazily allocates,
// stores results, and clears itself at the cap so a pathological stream of
// distinct clusters cannot grow it without bound.
func TestCacheFallbackBounded(t *testing.T) {
	ctx := &Context{}
	if ctx.fallbackResolve != nil {
		t.Fatal("fallbackResolve should start nil")
	}
	ctx.cacheFallback("x", fbResolution{path: "/a.ttf", isColor: true})
	if ctx.fallbackResolve == nil {
		t.Fatal("cacheFallback did not lazily allocate the map")
	}
	if got := ctx.fallbackResolve["x"]; got.path != "/a.ttf" || !got.isColor {
		t.Fatalf("stored resolution = %+v, want {/a.ttf true}", got)
	}

	// Fill exactly to the cap: no clear happens until an insert sees len==cap.
	ctx2 := &Context{}
	for i := 0; i < fallbackResolveCap; i++ {
		ctx2.cacheFallback(strconv.Itoa(i), fbResolution{})
	}
	if len(ctx2.fallbackResolve) != fallbackResolveCap {
		t.Fatalf("len at cap = %d, want %d",
			len(ctx2.fallbackResolve), fallbackResolveCap)
	}
	// The next distinct insert must clear first, leaving just the new entry.
	ctx2.cacheFallback("overflow", fbResolution{})
	if got := len(ctx2.fallbackResolve); got != 1 {
		t.Fatalf("len after overflow = %d, want 1 (map cleared)", got)
	}
}

// TestFaceCovers exercises the cmap-coverage predicate that now decides every
// fallback: a covered letter is true, a nil face is false, and a cluster of
// only default-ignorable code points counts as covered (shapers drop them).
func TestFaceCovers(t *testing.T) {
	path, size, _ := resolveTestGlyph(t, "H") // installed font path
	f := openFTFont(path, size)
	if f.face == nil {
		t.Skipf("could not open face %q", path)
	}
	if !f.covers("H") {
		t.Error("covers(\"H\") = false, want true for the default font")
	}
	if !faceCovers(f.face, "\u200d") { // ZWJ only
		t.Error("faceCovers(ZWJ-only) = false, want true (ignorable)")
	}
	if !faceCovers(f.face, "") {
		t.Error("faceCovers(empty) = false, want true")
	}
	var empty ftFont
	if empty.covers("H") {
		t.Error("covers on nil face = true, want false")
	}
}

// TestPooledFontRescales guards the pooled-shaping-font invariant: getHB
// reapplies the requested 26.6 scale on every borrow, so shaping the same
// glyph at a larger size through the recycled font yields a larger advance
// (a regression that dropped the rescale would return equal advances).
func TestPooledFontRescales(t *testing.T) {
	path, _, _ := resolveTestGlyph(t, "H")
	cf := loadCachedFace(path)
	if cf == nil {
		t.Skipf("could not load face %q", path)
	}
	small := shapeWith(cf, 20, "H")
	big := shapeWith(cf, 40, "H") // reuses the just-returned pooled font
	if small == nil || big == nil ||
		len(small.Pos) != 1 || len(big.Pos) != 1 {
		t.Skip("H did not shape to a single glyph in the default font")
	}
	if big.Pos[0].XAdvance <= small.Pos[0].XAdvance {
		t.Errorf("advance at 40px (%d) not greater than at 20px (%d): "+
			"pooled font scale not reapplied",
			big.Pos[0].XAdvance, small.Pos[0].XAdvance)
	}
}
