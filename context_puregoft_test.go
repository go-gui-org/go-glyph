//go:build linux && !android

package glyph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	"golang.org/x/image/vector"
)

func TestLinuxGenericAlias(t *testing.T) {
	tests := []struct {
		family string
		want   string
	}{
		{"dejavu sans", "sans-serif"},
		{"dejavu sans mono", "monospace"},
		{"dejavu serif", "serif"},
		{"liberation sans", "sans-serif"},
		{"liberation mono", "monospace"},
		{"liberation serif", "serif"},
		{"noto sans", "sans-serif"},
		{"noto mono", "monospace"},
		{"noto serif", "serif"},
		{"some unknown font", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := linuxGenericAlias(tt.family)
		if got != tt.want {
			t.Errorf("linuxGenericAlias(%q) = %q, want %q",
				tt.family, got, tt.want)
		}
	}
}

func TestResolveFontFamilyLinux(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Sans 12", "DejaVu Sans"},
		{"sans-serif Bold 14", "DejaVu Sans"},
		{"Serif 11", "DejaVu Serif"},
		{"Monospace 10", "DejaVu Sans Mono"},
		{"mono Bold 12", "DejaVu Sans Mono"},
		{"system 16", "DejaVu Sans"},
		{"Fira Code 12", "Fira Code"},
		{"", "DejaVu Sans"},
	}
	for _, tt := range tests {
		got := resolveFontFamily(tt.name)
		if got != tt.want {
			t.Errorf("resolveFontFamily(%q) = %q, want %q",
				tt.name, got, tt.want)
		}
	}
}

func TestSignedArea(t *testing.T) {
	ccw := []vec2{{0, 0}, {10, 0}, {0, 10}}
	if got := signedArea(ccw); got <= 0 {
		t.Errorf("signedArea(CCW) = %v, want >0", got)
	}
	cw := []vec2{{0, 0}, {0, 10}, {10, 0}}
	if got := signedArea(cw); got >= 0 {
		t.Errorf("signedArea(CW) = %v, want <0", got)
	}
	if got := signedArea([]vec2{{0, 0}, {0, 0}, {0, 0}}); got != 0 {
		t.Errorf("signedArea(degenerate) = %v, want 0", got)
	}
	if got := signedArea([]vec2{}); got != 0 {
		t.Errorf("signedArea(empty) = %v, want 0", got)
	}
}

func TestMid(t *testing.T) {
	got := mid(vec2{0, 0}, vec2{10, 10})
	if got.x != 5 || got.y != 5 {
		t.Errorf("mid({0,0},{10,10}) = %v, want {5,5}", got)
	}
}

func TestDistToLine(t *testing.T) {
	d := distToLine(vec2{1, 1}, vec2{0, 0}, vec2{2, 0})
	if d != 1 {
		t.Errorf("distToLine((1,1), (0,0)->(2,0)) = %v, want 1", d)
	}
	if got := distToLine(vec2{5, 0}, vec2{0, 0}, vec2{10, 0}); got != 0 {
		t.Errorf("distToLine on-line = %v, want 0", got)
	}
	d = distToLine(vec2{0, 0}, vec2{0, 0}, vec2{0, 0})
	if d != 0 {
		t.Errorf("distToLine(degenerate segment) = %v, want 0", d)
	}
}

func TestEmitDiscPoints(t *testing.T) {
	rz := vector.NewRasterizer(100, 100)
	emitDisc(rz, vec2{50, 50}, 10)
}

func TestEmitPolygonWinding(t *testing.T) {
	cw := []vec2{{0, 0}, {0, 10}, {10, 0}}
	cpy := make([]vec2, len(cw))
	copy(cpy, cw)
	rz := vector.NewRasterizer(50, 50)
	emitPolygon(rz, cpy)
	if got := signedArea(cpy); got <= 0 {
		t.Errorf("signedArea after emitPolygon(CW) = %v, want >0", got)
	}

	ccw := []vec2{{0, 0}, {10, 0}, {0, 10}}
	cpy2 := make([]vec2, len(ccw))
	copy(cpy2, ccw)
	emitPolygon(rz, cpy2)
	if got := signedArea(cpy2); got <= 0 {
		t.Errorf("signedArea after emitPolygon(CCW) = %v, want >0", got)
	}
}

func TestEmitStroke(t *testing.T) {
	rz := vector.NewRasterizer(50, 50)
	emitStroke(rz, []vec2{{25, 25}}, 5)

	rz2 := vector.NewRasterizer(50, 50)
	emitStroke(rz2, []vec2{{5, 5}, {45, 45}}, 3)

	rz3 := vector.NewRasterizer(50, 50)
	emitStroke(rz3, []vec2{}, 3)
	emitStroke(rz3, []vec2{{0, 0}, {0, 0}}, 3)
}

func TestEmitSeg(t *testing.T) {
	rz := vector.NewRasterizer(50, 50)
	emitSeg(rz, vec2{5, 5}, vec2{5, 5}, 3)
	emitSeg(rz, vec2{5, 5}, vec2{45, 45}, 3)
}

func TestFlattenQuad(t *testing.T) {
	var out []vec2
	flattenQuad(vec2{0, 0}, vec2{5, 5}, vec2{10, 0}, 12, &out)
	if len(out) == 0 {
		t.Error("flattenQuad produced no points")
	}
}

func TestFlattenCube(t *testing.T) {
	var out []vec2
	flattenCube(vec2{0, 0}, vec2{3, 5}, vec2{7, 5}, vec2{10, 0}, 12, &out)
	if len(out) == 0 {
		t.Error("flattenCube produced no points")
	}
}

// TestRegisterFontPathWeightSelection verifies that when several weights
// of one family collapse to the same style key, the weight closest to the
// bucket's canonical value wins — regardless of discovery order — while
// exact-weight ties keep the first (user-fonts-first) registration.
func TestRegisterFontPathWeightSelection(t *testing.T) {
	norm := func(w font.Weight) font.Aspect {
		return font.Aspect{Style: font.StyleNormal, Weight: w}
	}

	paths := map[string]string{}
	weights := map[string]font.Weight{}

	// Medium walked before Regular must not keep the "-Regular"/bare slots.
	registerFontPath(paths, weights, nil, "Foo", norm(font.WeightMedium), "medium.ttf")
	registerFontPath(paths, weights, nil, "Foo", norm(font.WeightNormal), "regular.ttf")
	// A later Book (380) is farther from 400 than Regular, so Regular stays.
	registerFontPath(paths, weights, nil, "Foo", norm(font.WeightNormal-20), "book.ttf")

	if got := paths["Foo-Regular"]; got != "regular.ttf" {
		t.Errorf("Foo-Regular = %q, want regular.ttf", got)
	}
	if got := paths["Foo"]; got != "regular.ttf" {
		t.Errorf("Foo (bare) = %q, want regular.ttf", got)
	}

	// Bold bucket: Bold(700) is canonical; a heavier Black(900) must not win.
	registerFontPath(paths, weights, nil, "Foo", font.Aspect{
		Style: font.StyleNormal, Weight: font.WeightBlack}, "black.ttf")
	registerFontPath(paths, weights, nil, "Foo", font.Aspect{
		Style: font.StyleNormal, Weight: font.WeightBold}, "bold.ttf")
	if got := paths["Foo-Bold"]; got != "bold.ttf" {
		t.Errorf("Foo-Bold = %q, want bold.ttf", got)
	}

	// Exact-weight tie keeps the first registration (user-fonts-first order).
	registerFontPath(paths, weights, nil, "Bar", norm(font.WeightNormal), "user.ttf")
	registerFontPath(paths, weights, nil, "Bar", norm(font.WeightNormal), "system.ttf")
	if got := paths["Bar-Regular"]; got != "user.ttf" {
		t.Errorf("Bar-Regular = %q, want user.ttf (first wins on tie)", got)
	}

	// nil keyWeights disables tie resolution (first-wins), as in AddFontFile.
	p2 := map[string]string{}
	registerFontPath(p2, nil, nil, "Baz", norm(font.WeightMedium), "medium.ttf")
	registerFontPath(p2, nil, nil, "Baz", norm(font.WeightNormal), "regular.ttf")
	if got := p2["Baz-Regular"]; got != "medium.ttf" {
		t.Errorf("Baz-Regular = %q, want medium.ttf (nil weights = first-wins)", got)
	}
}

// fontFileInstalled reports whether any font under the standard Linux
// font directories has a filename containing one of the substrings.
// Used to distinguish "font not installed" (skip) from "font present
// but discovery/fallback broke" (fail).
func fontFileInstalled(substrs ...string) bool {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".local", "share", "fonts"),
		filepath.Join(home, ".fonts"),
		"/usr/local/share/fonts",
		"/usr/share/fonts",
	}
	found := false
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			lp := strings.ToLower(p)
			for _, s := range substrs {
				if strings.Contains(lp, s) {
					found = true
					return filepath.SkipAll
				}
			}
			return nil
		})
	}
	return found
}

// TestLinuxScriptFallbacks verifies that font discovery populates
// fallbackPaths and that the discovered fallbacks actually cover CJK
// and color-emoji runes — the regression that made those scripts
// render as tofu on the pango-free backend. Coverage is only asserted
// for scripts whose fonts are actually installed, so the test is a
// no-op (not a failure) on hosts without CJK/emoji fonts.
func TestLinuxScriptFallbacks(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	// Coverage uses the resident cmap cache (loadCoverage), matching how
	// production probeFallback decides coverage — a cheap cmap lookup, not a
	// full face parse per candidate. Scanning the whole fallback set with
	// newFTFontFromPath parsed every system font (100s of large .ttc files),
	// spiking peak RSS into multi-GB and OOM-killing the macOS CI runner.
	covers := func(runes string) bool {
		for _, p := range ctx.fallbackPaths {
			if cov := loadCoverage(p); cov != nil && cov.covers(runes) {
				return true
			}
		}
		return false
	}

	cjkInstalled := fontFileInstalled("notosanscjk", "notoserifcjk",
		"droidsansfallback", "wenquanyi", "wqy", "uming", "ukai",
		"nanum", "vlgothic", "takao", "ipafont", "ipagp")
	if cjkInstalled {
		if !covers("中") {
			t.Error("CJK font installed but no fallback covers 中")
		}
		if !covers("日") {
			t.Error("CJK font installed but no fallback covers 日")
		}
	} else {
		t.Log("no CJK font installed; skipping CJK coverage assertions")
	}

	if fontFileInstalled("emoji") {
		if !covers("\U0001F600") { // 😀
			t.Error("emoji font installed but no fallback covers 😀")
		}
	} else {
		t.Log("no emoji font installed; skipping emoji coverage assertion")
	}
}

// TestEmojiItemUseOriginalColor verifies that laying out a color-emoji
// grapheme yields an item flagged UseOriginalColor, so the renderer
// draws the CBDT bitmap in its own colors (and scaled) instead of
// tinting it with the text color.
func TestEmojiItemUseOriginalColor(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no emoji fallback font installed; skipping")
	}

	// Confirm an emoji font is actually present before asserting. Filter with
	// the cheap cmap coverage cache first (mirroring production probeFallback)
	// and only full-parse a color candidate that maps the emoji — otherwise this
	// guard parsed every system fallback font, spiking RSS and OOM-killing CI.
	hasEmoji := false
	for _, p := range ctx.fallbackPaths {
		cov := loadCoverage(p)
		if cov == nil || !cov.color || !cov.covers("\U0001F600") {
			continue
		}
		f := newFTFontFromPath(ctx.ftLib, p, 16)
		ok := f.isColorFont() && f.hasGlyphs("\U0001F600")
		f.close()
		if ok {
			hasEmoji = true
			break
		}
	}
	if !hasEmoji {
		t.Skip("no color-emoji font covers 😀; skipping")
	}

	layout, err := ctx.LayoutText("a\U0001F600b", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	found := false
	for _, it := range layout.Items {
		if it.UseOriginalColor {
			found = true
		}
	}
	if !found {
		t.Error("emoji layout produced no UseOriginalColor item")
	}
}

func TestListFontFamiliesForwardReverse(t *testing.T) {
	// White-box test in package glyph: every family reported by
	// ListFontFamilies has a real resolution key in fontPaths
	// (bare family or a style key like "Foo-Regular") mapping
	// to a non-empty path.
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()

	ts := &TextSystem{ctx: ctx}
	fams := ts.ListFontFamilies()

	// If the test environment has no fonts, skip the forward check.
	if len(fams) == 0 {
		t.Log("no system fonts — skipping forward check")
		return
	}

	for _, fam := range fams {
		// Check bare family key.
		path, ok := ctx.fontPaths[fam]
		if !ok {
			// Check style keys (Regular, Bold, Italic, BoldItalic).
			ok = false
			for _, suffix := range []string{"-Regular", "-Bold", "-Italic", "-BoldItalic"} {
				if path, ok = ctx.fontPaths[fam+suffix]; ok && path != "" {
					break
				}
			}
		}
		if !ok || path == "" {
			t.Errorf("ListFontFamilies reports %q but no fontPaths key "+
				"(bare or style-suffixed) maps to a non-empty path", fam)
		}
	}

	// Reverse: every family registered via registerFontPath appears in the
	// list exactly once. Use a fresh context with a known fixture.
	ctx2, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx2.Free()

	// Add a font file we know is in the test fixtures.
	registerFontPath(ctx2.fontPaths, ctx2.fontWeights, ctx2.families,
		"TestForward", font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal},
		"/nonexistent.ttf")
	// Same family with a different style should NOT duplicate.
	registerFontPath(ctx2.fontPaths, ctx2.fontWeights, ctx2.families,
		"TestForward", font.Aspect{Style: font.StyleNormal, Weight: font.WeightBold},
		"/nonexistent-bold.ttf")

	ts2 := &TextSystem{ctx: ctx2}
	fams2 := ts2.ListFontFamilies()

	count := 0
	for _, f := range fams2 {
		if f == "TestForward" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("TestForward appears %d times in ListFontFamilies, want 1", count)
	}

	// Leading-"." names should not appear.
	registerFontPath(ctx2.fontPaths, ctx2.fontWeights, ctx2.families,
		".PrivateFont", font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal},
		"/private.ttf")
	fams2 = ts2.ListFontFamilies()
	for _, f := range fams2 {
		if strings.HasPrefix(f, ".") {
			t.Errorf("ListFontFamilies includes dot-prefixed name %q", f)
		}
	}

	// Case-fold dedup: two faces reporting the same family in different
	// case list once, keeping the first-seen display case.
	ctx3, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx3.Free()
	registerFontPath(ctx3.fontPaths, ctx3.fontWeights, ctx3.families,
		"Helvetica", font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal},
		"/helvetica.ttf")
	registerFontPath(ctx3.fontPaths, ctx3.fontWeights, ctx3.families,
		"HELVETICA", font.Aspect{Style: font.StyleNormal, Weight: font.WeightBold},
		"/helvetica-caps.ttf")

	ts3 := &TextSystem{ctx: ctx3}
	fams3 := ts3.ListFontFamilies()
	got := 0
	for _, f := range fams3 {
		if strings.EqualFold(f, "helvetica") {
			got++
			if f != "Helvetica" {
				t.Errorf("case-fold dedup kept %q, want first-seen %q", f, "Helvetica")
			}
		}
	}
	if got != 1 {
		t.Errorf("Helvetica/HELVETICA folded to %d entries, want 1", got)
	}
}
