//go:build linux || darwin || windows

package glyph

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/go-text/typesetting/harfbuzz"
)

// TestIsDefaultIgnorable checks the boundary of every default-ignorable
// range faceCovers relies on: ignorables must be reported so a cluster of
// (base + joiner/variation-selector) is not forced to a fallback, and
// adjacent non-members must not be, or real characters would silently skip
// coverage checks. Set from Unicode 17.0 Default_Ignorable_Code_Point.
func TestIsDefaultIgnorable(t *testing.T) {
	ignorable := []rune{
		0x00AD, 0xFEFF, // soft hyphen, ZWNBSP/BOM
		0x034F,         // combining grapheme joiner
		0x061C,         // Arabic letter mark
		0x115F, 0x1160, // Hangul choseong/jungseong fillers
		0x17B4, 0x17B5, // Khmer vowel inherent AQ/AA
		0x180B, 0x180E, 0x180F, // Mongolian FVS1..3, MVS, FVS4 (range ends)
		0x200B, 0x200D, 0x200F, // ZW space, ZWJ, RLM
		0x202A, 0x202E, // bidi embeddings/overrides (range ends)
		0x2060, 0x206F, // word joiner .. bidi ops (range ends)
		0x3164,         // Hangul filler
		0xFE00, 0xFE0F, // variation selectors (range ends)
		0xFFA0,         // halfwidth Hangul filler
		0xFFF0, 0xFFF8, // reserved format characters (range ends)
		0x1BCA0, 0x1BCA3, // shorthand format controls (range ends)
		0x1D173, 0x1D17A, // musical beam/tie controls (range ends)
		0xE0000, 0xE0FFF, // tags + VS supplement (range ends)
	}
	for _, r := range ignorable {
		if !isDefaultIgnorable(r) {
			t.Errorf("isDefaultIgnorable(%#U) = false, want true", r)
		}
	}
	// Just-outside-range and ordinary code points must not be ignorable.
	notIgnorable := []rune{
		'a', '0', 0x00AC, 0x0350, 0x061B,
		0x115E, 0x1161, 0x17B3, 0x17B6, 0x180A, 0x1810,
		0x200A, 0x2010, 0x2029, 0x202F, 0x2070,
		0x3163, 0x3165, 0xFDFF, 0xFE10, 0xFF9F, 0xFFA1,
		0xFFEF, 0xFFF9, // interlinear annotation anchor is excluded
		0x1BC9F, 0x1BCA4, 0x1D172, 0x1D17B, 0xE1000,
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

// TestPooledShapeBufferReset guards the pooled-shaping-buffer invariant:
// releaseShapeBuffer must fully reset a buffer (glyph content AND the segment
// properties GuessSegmentProperties filled in) before it is reused. A leak of
// either would carry stale glyphs into the next shape's Info/Pos or freeze the
// first shape's script/direction onto unrelated text. The pool may hand back a
// fresh buffer instead of the recycled one — a fresh buffer passes trivially,
// a recycled one passes only if Clear worked, so the test is valid either way.
func TestPooledShapeBufferReset(t *testing.T) {
	releaseShapeBuffer(nil) // failure paths release unconditionally; must not panic

	path, _, _ := resolveTestGlyph(t, "H")
	cf := loadCachedFace(path)
	if cf == nil {
		t.Skipf("could not load face %q", path)
	}

	ltr := shapeWith(cf, 20, "HHH")
	if ltr == nil || len(ltr.Info) != 3 {
		t.Skip("HHH did not shape to three glyphs in the default font")
	}
	if ltr.Props.Direction != harfbuzz.LeftToRight {
		t.Fatalf("Latin shape direction = %v, want LeftToRight", ltr.Props.Direction)
	}
	releaseShapeBuffer(ltr)

	// Arabic through the (likely recycled) buffer: stale content would leave
	// more than one glyph; stale segment properties would keep LeftToRight,
	// since GuessSegmentProperties only fills unset fields.
	rtl := shapeWith(cf, 20, "\u0628") // ARABIC LETTER BEH
	if rtl == nil {
		t.Fatal("shapeWith(Arabic) = nil, want buffer")
	}
	defer releaseShapeBuffer(rtl)
	if len(rtl.Info) != 1 {
		t.Errorf("len(Info) after reuse = %d, want 1 (stale glyphs leaked)",
			len(rtl.Info))
	}
	if rtl.Props.Direction != harfbuzz.RightToLeft {
		t.Errorf("Arabic shape direction = %v, want RightToLeft "+
			"(segment properties leaked across pool reuse)", rtl.Props.Direction)
	}
}

// TestReleaseShapeBufferDropsOversized guards the pool's memory cap: a buffer
// grown past shapeBufMaxRetain glyphs must be dropped on release (observable
// as its content surviving untouched, since the drop path skips Clear), while
// a normal-sized buffer is cleared for reuse. Without the cap, one
// multi-megabyte run would pin its full Info/Pos growth inside the pool.
func TestReleaseShapeBufferDropsOversized(t *testing.T) {
	big := harfbuzz.NewBuffer()
	for i := 0; i <= shapeBufMaxRetain; i++ {
		big.AddRune('a', i)
	}
	releaseShapeBuffer(big)
	if len(big.Info) != shapeBufMaxRetain+1 {
		t.Errorf("oversized buffer was cleared (len=%d): it entered the pool "+
			"instead of being dropped", len(big.Info))
	}

	small := harfbuzz.NewBuffer()
	small.AddRune('a', 0)
	releaseShapeBuffer(small)
	if len(small.Info) != 0 {
		t.Errorf("normal buffer not cleared on release: len=%d, want 0",
			len(small.Info))
	}
}

// BenchmarkMeasureString tracks the steady-state cost of shaping-based
// measurement, the hot path of relayout (scroll/resize). Guards the pooled
// shape-buffer optimization: a regression back to per-shape NewBuffer +
// []rune(text) shows up as a jump in allocs/op.
func BenchmarkMeasureString(b *testing.B) {
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Skipf("NewContext: %v", err)
	}
	defer ctx.Free()
	var item Item
	family, size, bold, italic := resolveFTFontParams(item.Style, 1.0)
	paths := fontFallbackPaths(ftFontPathsSingleton, family, bold, italic)
	if len(paths) == 0 {
		b.Skip("no font installed")
	}
	f := openFTFont(paths[0], size)
	if f.face == nil {
		b.Skipf("could not open face %q", paths[0])
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = f.measureString("The quick brown fox jumps over the lazy dog")
	}
}

// BenchmarkLayoutText tracks the steady-state cost of a full layout pass.
// Guards the Context-scoped scratch buffers (layoutScratch) and the presized
// index maps: a regression back to per-call slice/map allocation shows up as
// a jump in allocs/op.
func BenchmarkLayoutText(b *testing.B) {
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Skipf("NewContext: %v", err)
	}
	defer ctx.Free()
	cfg := TextConfig{}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ctx.LayoutText(
			"The quick brown fox jumps over the lazy dog", cfg); err != nil {
			b.Fatal(err)
		}
	}
}

// TestRecycleScratch guards the scratch-recycling contract buildLayout relies
// on: a normal-sized buffer comes back emptied (len 0, capacity kept) with its
// elements cleared so pointer fields cannot pin the previous layout's text or
// faces, an oversized buffer is dropped entirely (maxLayoutScratch cap), and
// nil is safe.
func TestRecycleScratch(t *testing.T) {
	s := make([]graphemeCluster, 3, 8)
	s[0] = graphemeCluster{text: "x", byteI: 1, byteL: 1}
	r := recycleScratch(s)
	if len(r) != 0 || cap(r) != 8 {
		t.Fatalf("recycleScratch len/cap = %d/%d, want 0/8", len(r), cap(r))
	}
	// The cleared elements are still reachable through the shared backing
	// array; any surviving field would pin the old layout's strings.
	if got := r[:3][0]; got != (graphemeCluster{}) {
		t.Errorf("element not cleared on recycle: %+v", got)
	}

	big := make([]charInfo, maxLayoutScratch+1)
	if got := recycleScratch(big); got != nil {
		t.Errorf("oversized scratch retained (cap %d), want nil", cap(got))
	}

	if got := recycleScratch[charInfo](nil); len(got) != 0 {
		t.Errorf("recycleScratch(nil) len = %d, want 0", len(got))
	}
}

// TestSegmentGraphemesScratchReuse guards the dst contract of
// segmentGraphemes: sufficient capacity is reused rather than reallocated, a
// dirty (non-truncated) dst never leaks stale clusters into the result, empty
// text hands the backing array back instead of dropping it, and segmentation
// output stays correct through reuse.
func TestSegmentGraphemesScratchReuse(t *testing.T) {
	first := segmentGraphemes(nil, strings.Repeat("x", 64))
	if len(first) != 64 {
		t.Fatalf("len(first) = %d, want 64", len(first))
	}

	// A dirty dst (len > 0) must be truncated, not appended onto.
	out := segmentGraphemes(first, "aé\U0001F600")
	if len(out) != 3 {
		t.Fatalf("dirty dst: len = %d, want 3 (stale clusters leaked)", len(out))
	}
	if cap(out) < 64 {
		t.Errorf("dst backing dropped: cap = %d, want >= 64", cap(out))
	}
	want := []graphemeCluster{
		{text: "a", byteI: 0, byteL: 1},
		{text: "é", byteI: 1, byteL: 2},
		{text: "\U0001F600", byteI: 3, byteL: 4},
	}
	for i, w := range want {
		if out[i] != w {
			t.Errorf("cluster[%d] = %+v, want %+v", i, out[i], w)
		}
	}

	// Empty text must preserve the scratch's backing array for the next call.
	empty := segmentGraphemes(out[:0], "")
	if len(empty) != 0 || cap(empty) < 64 {
		t.Errorf("empty text: len/cap = %d/%d, want 0 with cap >= 64",
			len(empty), cap(empty))
	}

	if got := segmentGraphemes(nil, ""); got != nil {
		t.Errorf("segmentGraphemes(nil, \"\") = %v, want nil", got)
	}
}

// TestLayoutScratchReuseDeterminism is the end-to-end regression guard for
// layoutScratch: a Context that has laid out many different texts (growing,
// shrinking, and dirtying every scratch buffer) must produce layouts
// byte-identical to a fresh Context's for the same input. A stale element
// surviving buffer reuse — a break flag, a bidi range, a leftover glyph
// stream — shows up here as a diff. The sequence crosses the paths that
// stress each buffer: word wrap (canBreak), RTL + ligatures (bidiChars,
// absorbed clusters, multi-glyph streams), emoji fallback, newlines, empty
// text, and a text large enough to trip the maxLayoutScratch drop.
func TestLayoutScratchReuseDeterminism(t *testing.T) {
	reused, err := NewContext(1.0)
	if err != nil {
		t.Skipf("NewContext: %v", err)
	}
	defer reused.Free()

	wrapCfg := TextConfig{
		Block: BlockStyle{Width: 60, Wrap: WrapWordChar, Align: AlignLeft},
	}
	cases := []struct {
		name string
		text string
		cfg  TextConfig
	}{
		{"wrapped", strings.Repeat("The quick brown fox ", 8), wrapCfg},
		{"short", "short", TextConfig{}},
		{"rtl-ligature", "مرحبا الاتحاد hello", TextConfig{}},
		{"emoji-combining", "a\u0301 \U0001F600 b", TextConfig{}},
		{"newlines", "line one\nline two\n", TextConfig{}},
		{"empty", "", TextConfig{}},
		{"oversized", strings.Repeat("a", maxLayoutScratch+8), TextConfig{}},
	}
	for _, tc := range cases {
		got, err := reused.LayoutText(tc.text, tc.cfg)
		if err != nil {
			t.Skipf("%s: LayoutText(reused): %v", tc.name, err)
		}
		fresh, err := NewContext(1.0)
		if err != nil {
			t.Skipf("%s: NewContext(fresh): %v", tc.name, err)
		}
		wantL, err := fresh.LayoutText(tc.text, tc.cfg)
		fresh.Free()
		if err != nil {
			t.Skipf("%s: LayoutText(fresh): %v", tc.name, err)
		}
		if !reflect.DeepEqual(got, wantL) {
			t.Errorf("%s: reused-Context layout differs from fresh-Context "+
				"layout (scratch reuse leaked state)", tc.name)
		}
	}

	// Retention policy: after the oversized layout above, every grown scratch
	// buffer must have been dropped, not pinned on the Context…
	if got := cap(reused.scratch.chars); got != 0 {
		t.Errorf("oversized chars scratch retained: cap = %d, want 0", got)
	}
	if got := cap(reused.scratch.clusters); got != 0 {
		t.Errorf("oversized clusters scratch retained: cap = %d, want 0", got)
	}
	// …while a normal-sized layout leaves its buffers behind for reuse.
	if _, err := reused.LayoutText("retain me", TextConfig{}); err != nil {
		t.Fatalf("LayoutText(retain me): %v", err)
	}
	if cap(reused.scratch.chars) == 0 || cap(reused.scratch.clusters) == 0 {
		t.Error("small-layout scratch not retained: chars/clusters cap = 0")
	}
}

// TestCoverageCovers exercises the cmap-only coverage predicate used by
// probeFallback: a covered letter is true, an ignorable-only cluster and the
// empty string count as covered, an empty path yields no coverage, and an
// unreadable path is negative-cached rather than re-read on every probe.
func TestCoverageCovers(t *testing.T) {
	path, _, _ := resolveTestGlyph(t, "H") // installed font path
	cov := loadCoverage(path)
	if cov == nil {
		t.Fatalf("loadCoverage(%q) = nil, want coverage", path)
	}
	if !cov.covers("H") {
		t.Error("covers(\"H\") = false, want true for the default font")
	}
	if !cov.covers("\u200d") { // ZWJ only
		t.Error("covers(ZWJ-only) = false, want true (ignorable)")
	}
	if !cov.covers("") {
		t.Error("covers(empty) = false, want true")
	}
	if loadCoverage("") != nil {
		t.Error("loadCoverage(\"\") = non-nil, want nil")
	}
	const bad = "/nonexistent/does-not-exist.ttf"
	if loadCoverage(bad) != nil {
		t.Error("loadCoverage(bad path) = non-nil, want nil")
	}
	// The nil must be cached so a bad path is not re-read on every probe.
	coverageCache.mu.Lock()
	cachedNil, ok := coverageCache.items[bad]
	coverageCache.mu.Unlock()
	if !ok || cachedNil != nil {
		t.Errorf("bad path not negative-cached: ok=%v val=%v", ok, cachedNil)
	}
}

// TestCoverageMatchesFace pins the invariant that the cheap cmap-only probe
// agrees with the parsed-face coverage the render path uses. Probing and
// rendering must resolve the same font, so coverage.covers must equal
// faceCovers and the color flag must match, for the same font — a divergence
// would let a font pass the probe yet render tofu (or vice versa).
func TestCoverageMatchesFace(t *testing.T) {
	path, _, _ := resolveTestGlyph(t, "H")
	cov := loadCoverage(path)
	cf := loadCachedFace(path)
	if cov == nil || cf == nil || cf.face == nil {
		t.Skipf("could not load coverage/face for %q", path)
	}
	if cov.color != cf.color {
		t.Errorf("color mismatch: coverage=%v face=%v", cov.color, cf.color)
	}
	samples := []string{
		"H", "z", "0", "é", // Latin (likely covered)
		"中", "\U0001F600", "क", // CJK, emoji, Devanagari (likely not)
		"\u200d", "a\ufe0f", "", // ignorable-only, base+VS16, empty
	}
	for _, s := range samples {
		if got, want := cov.covers(s), faceCovers(cf.face, s); got != want {
			t.Errorf("covers(%q): coverage=%v faceCovers=%v (must agree)",
				s, got, want)
		}
	}
}

// TestLoadCoverageCaches verifies loadCoverage builds coverage once and returns
// the cached instance on subsequent calls, so a probe never re-reads the font.
func TestLoadCoverageCaches(t *testing.T) {
	path, _, _ := resolveTestGlyph(t, "H")
	a := loadCoverage(path)
	if a == nil {
		t.Fatalf("loadCoverage(%q) = nil", path)
	}
	if b := loadCoverage(path); a != b {
		t.Error("loadCoverage returned a different instance on second call; not cached")
	}
}
