//go:build linux || darwin || windows

package glyph

import (
	"strconv"
	"testing"

	"github.com/go-text/typesetting/font"
)

// newScanCtx returns a Context with empty discovery maps, for driving
// fontScan without disk access.
func newScanCtx() *Context {
	return &Context{
		fontPaths:   map[string]string{},
		fontWeights: map[string]font.Weight{},
		fontItalics: map[string]bool{},
		families:    map[string]string{},
	}
}

// TestGenericAliasPrefersRegular is the regression test for the generic
// alias taking the first face in walk order. On Linux "DejaVuSans-Bold.ttf"
// sorts before "DejaVuSans.ttf", so "sans-serif" held the Bold file and an
// unresolved family fell back to Bold text.
func TestGenericAliasPrefersRegular(t *testing.T) {
	ctx := newScanCtx()
	s := newFontScan(ctx)
	alias := func(string) string { return "sans-serif" }
	faces := []faceInfo{
		{path: "/f/DejaVuSans-Bold.ttf", desc: font.Description{
			Family: "DejaVu Sans", Aspect: font.Aspect{Weight: font.WeightBold}}},
		{path: "/f/DejaVuSans-Oblique.ttf", desc: font.Description{
			Family: "DejaVu Sans", Aspect: font.Aspect{
				Weight: font.WeightNormal, Style: font.StyleItalic}}},
		{path: "/f/DejaVuSans.ttf", desc: font.Description{
			Family: "DejaVu Sans", Aspect: font.Aspect{Weight: font.WeightNormal}}},
		{path: "/f/DejaVuSans-ExtraLight.ttf", desc: font.Description{
			Family: "DejaVu Sans", Aspect: font.Aspect{Weight: font.WeightExtraLight}}},
	}
	for _, fc := range faces {
		s.considerFace(fc, alias)
	}
	if got := ctx.fontPaths["sans-serif"]; got != "/f/DejaVuSans.ttf" {
		t.Errorf("sans-serif alias = %q, want /f/DejaVuSans.ttf", got)
	}
}

// TestCollectionMembersInFallbackTiers checks which collection members
// join the fallback tiers: every member of a CJK collection (so the
// locale reorder can pick the regional face), but only face 0 of any
// other collection.
func TestCollectionMembersInFallbackTiers(t *testing.T) {
	ctx := newScanCtx()
	s := newFontScan(ctx)
	none := func(string) string { return "" }
	reg := font.Aspect{Weight: font.WeightNormal}
	for i, fam := range []string{"Noto Sans CJK JP", "Noto Sans CJK SC"} {
		s.considerFace(faceInfo{path: facePath("/cjk.ttc", i), index: i,
			desc: font.Description{Family: fam, Aspect: reg}}, none)
	}
	for i, fam := range []string{"Helvetica", "Helvetica Light"} {
		s.considerFace(faceInfo{path: facePath("/helv.ttc", i), index: i,
			desc: font.Description{Family: fam, Aspect: reg}}, none)
	}
	if len(s.cjkPaths) != 2 {
		t.Errorf("cjkPaths = %q, want both CJK members", s.cjkPaths)
	}
	if len(s.generalPaths) != 1 || s.generalPaths[0] != "/helv.ttc" {
		t.Errorf("generalPaths = %q, want only face 0 of /helv.ttc",
			s.generalPaths)
	}
	// Every member still resolves by name.
	if got := ctx.fontPaths["Helvetica Light"]; got != facePath("/helv.ttc", 1) {
		t.Errorf("fontPaths[Helvetica Light] = %q", got)
	}
}

// TestIsCJKFamilyNoFalsePositives is the regression test for the loose
// "han" and "ipa" needles, which put Devanagari and Khmer fonts in the
// CJK tier.
func TestIsCJKFamilyNoFalsePositives(t *testing.T) {
	for _, fam := range []string{"khand", "chandas", "hanuman", "shantell sans"} {
		if isCJKFamily(fam) {
			t.Errorf("isCJKFamily(%q) = true, want false", fam)
		}
	}
	for _, fam := range []string{
		"source han sans sc", "noto sans cjk jp", "ipagothic",
		"ipaexmincho", "ipapgothic", "hiragino sans",
	} {
		if !isCJKFamily(fam) {
			t.Errorf("isCJKFamily(%q) = false, want true", fam)
		}
	}
}

// TestFallbackResolveRingKeepsCapacity is the regression test for the
// FIFO queue popping its head with a reslice. Each eviction shrank the
// capacity by one, so append kept copying the whole 262k-entry queue.
// The ring must keep one backing array once full.
func TestFallbackResolveRingKeepsCapacity(t *testing.T) {
	ctx := &Context{}
	for i := range fallbackResolveCap {
		ctx.cacheFallback(strconv.Itoa(i), fbResolution{})
	}
	base := &ctx.resolveOrder[0]
	const extra = 1000
	for i := range extra {
		ctx.cacheFallback("x"+strconv.Itoa(i), fbResolution{})
	}
	if &ctx.resolveOrder[0] != base {
		t.Error("resolveOrder was reallocated after reaching the cap")
	}
	if len(ctx.fallbackResolve) != fallbackResolveCap ||
		len(ctx.resolveOrder) != fallbackResolveCap {
		t.Fatalf("sizes = map %d / queue %d, want %d",
			len(ctx.fallbackResolve), len(ctx.resolveOrder), fallbackResolveCap)
	}
	// FIFO: the first `extra` keys are gone, the next one survives.
	if _, ok := ctx.fallbackResolve[strconv.Itoa(extra-1)]; ok {
		t.Error("key inserted before the evicted range still present")
	}
	if _, ok := ctx.fallbackResolve[strconv.Itoa(extra)]; !ok {
		t.Error("oldest surviving key was evicted early")
	}
	if ctx.resolveHead != extra {
		t.Errorf("resolveHead = %d, want %d", ctx.resolveHead, extra)
	}
}

// TestFreedContext is the regression test for use after Free: AddFontFile
// wrote into the nil fontPaths map and panicked, and Free left the
// fallback cache and discovery maps alive.
func TestFreedContext(t *testing.T) {
	// A real, parseable font: a missing file fails before the map write
	// and would hide the panic.
	fontFile, _, _, _ := twoSystemTTFs(t)

	ctx := newScanCtx()
	ctx.cacheFallback("x", fbResolution{})
	ctx.Free()
	if ctx.fallbackResolve != nil || ctx.resolveOrder != nil ||
		ctx.families != nil || ctx.fontWeights != nil || ctx.fontItalics != nil {
		t.Error("Free left caches or discovery maps allocated")
	}
	if err := ctx.AddFontFile(fontFile); err == nil {
		t.Error("AddFontFile after Free returned nil error")
	}
}

// TestRendererUsesContextFonts is the regression test for the renderer
// resolving font names in the process-wide map of the newest Context. A
// font added to an older TextSystem's Context was found by layout but not
// by rendering.
func TestRendererUsesContextFonts(t *testing.T) {
	ts, err := NewTextSystem(newRecordingBackend())
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()
	// A second Context takes over the process-wide map.
	other, err := NewContext(1)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Free()

	ts.ctx.fontPaths["Only In First"] = "/first.ttf"
	got := renderFontPaths(ts.renderer.fontPaths)["Only In First"]
	if got != "/first.ttf" {
		t.Errorf("renderer resolved %q, want /first.ttf from its own Context", got)
	}
	// A Renderer with no Context keeps the process-wide map.
	if m := renderFontPaths(nil); m == nil {
		t.Error("renderFontPaths(nil) returned nil")
	}
}
