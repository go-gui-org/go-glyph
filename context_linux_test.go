//go:build linux && !android

package glyph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
)

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
	registerFontPath(paths, weights, "Foo", norm(font.WeightMedium), "medium.ttf")
	registerFontPath(paths, weights, "Foo", norm(font.WeightNormal), "regular.ttf")
	// A later Book (380) is farther from 400 than Regular, so Regular stays.
	registerFontPath(paths, weights, "Foo", norm(font.WeightNormal-20), "book.ttf")

	if got := paths["Foo-Regular"]; got != "regular.ttf" {
		t.Errorf("Foo-Regular = %q, want regular.ttf", got)
	}
	if got := paths["Foo"]; got != "regular.ttf" {
		t.Errorf("Foo (bare) = %q, want regular.ttf", got)
	}

	// Bold bucket: Bold(700) is canonical; a heavier Black(900) must not win.
	registerFontPath(paths, weights, "Foo", font.Aspect{
		Style: font.StyleNormal, Weight: font.WeightBlack}, "black.ttf")
	registerFontPath(paths, weights, "Foo", font.Aspect{
		Style: font.StyleNormal, Weight: font.WeightBold}, "bold.ttf")
	if got := paths["Foo-Bold"]; got != "bold.ttf" {
		t.Errorf("Foo-Bold = %q, want bold.ttf", got)
	}

	// Exact-weight tie keeps the first registration (user-fonts-first order).
	registerFontPath(paths, weights, "Bar", norm(font.WeightNormal), "user.ttf")
	registerFontPath(paths, weights, "Bar", norm(font.WeightNormal), "system.ttf")
	if got := paths["Bar-Regular"]; got != "user.ttf" {
		t.Errorf("Bar-Regular = %q, want user.ttf (first wins on tie)", got)
	}

	// nil keyWeights disables tie resolution (first-wins), as in AddFontFile.
	p2 := map[string]string{}
	registerFontPath(p2, nil, "Baz", norm(font.WeightMedium), "medium.ttf")
	registerFontPath(p2, nil, "Baz", norm(font.WeightNormal), "regular.ttf")
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

	covers := func(runes string) bool {
		for _, p := range ctx.fallbackPaths {
			f := newFTFontFromPath(ctx.ftLib, p, 16)
			if f.face == nil {
				continue
			}
			ok := f.hasGlyphs(runes)
			f.close()
			if ok {
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

	// Confirm an emoji font is actually present before asserting.
	hasEmoji := false
	for _, p := range ctx.fallbackPaths {
		f := newFTFontFromPath(ctx.ftLib, p, 16)
		if f.isColorFont() && f.hasGlyphs("\U0001F600") {
			hasEmoji = true
		}
		f.close()
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
