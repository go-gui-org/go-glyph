//go:build linux || darwin || windows

package glyph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-text/typesetting/font"
)

// newTestScan builds a fontScan with no ctx, exercising only the disk-free
// tiering core (record / assembleFallbacks).
func newTestScan() *fontScan {
	return &fontScan{
		seenFallback: map[string]bool{},
		seenColor:    map[string]bool{},
		slots:        map[string]*fallbackSlot{},
	}
}

func TestRecordBucketsByTier(t *testing.T) {
	s := newTestScan()
	s.record("Noto Color Emoji", font.Aspect{}, true, "/emoji-color.ttf")
	s.record("Noto Emoji", font.Aspect{}, false, "/emoji-mono.ttf")
	s.record("Noto Sans CJK JP", font.Aspect{}, false, "/cjk.otf")
	s.record("Noto Sans Arabic", font.Aspect{}, false, "/arabic.ttf")
	s.record("DejaVu Sans", font.Aspect{}, false, "/general.ttf")

	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"color", s.colorPaths, []string{"/emoji-color.ttf"}},
		{"emoji", s.emojiPaths, []string{"/emoji-mono.ttf"}},
		{"cjk", s.cjkPaths, []string{"/cjk.otf"}},
		{"script", s.scriptPaths, []string{"/arabic.ttf"}},
		{"general", s.generalPaths, []string{"/general.ttf"}},
	}
	for _, c := range cases {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s tier = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestRecordColorLeadsAndSkipsOtherTiers(t *testing.T) {
	s := newTestScan()
	// A color emoji family must land only in colorPaths, never re-added to
	// the monochrome emoji tier below it.
	s.record("Apple Color Emoji", font.Aspect{}, true, "/apple-emoji.ttc")
	if want := []string{"/apple-emoji.ttc"}; !reflect.DeepEqual(s.colorPaths, want) {
		t.Errorf("colorPaths = %v, want %v", s.colorPaths, want)
	}
	if len(s.emojiPaths) != 0 {
		t.Errorf("emojiPaths = %v, want empty", s.emojiPaths)
	}
}

func TestRecordDedupesByFamily(t *testing.T) {
	s := newTestScan()
	s.record("DejaVu Sans", font.Aspect{}, false, "/first.ttf")
	s.record("DejaVu Sans", font.Aspect{}, false, "/second.ttf")
	if want := []string{"/first.ttf"}; !reflect.DeepEqual(s.generalPaths, want) {
		t.Errorf("generalPaths = %v, want %v (first-wins dedupe)",
			s.generalPaths, want)
	}
}

func TestRecordSkipsLastResort(t *testing.T) {
	s := newTestScan()
	s.record(".LastResort", font.Aspect{}, false, "/lastresort.ttf")
	if len(s.generalPaths)+len(s.cjkPaths)+len(s.scriptPaths)+
		len(s.emojiPaths)+len(s.colorPaths) != 0 {
		t.Error(".LastResort must not enter any fallback tier")
	}
}

func TestAssembleFallbacksOrder(t *testing.T) {
	got := assembleFallbacks(
		[]string{"c"}, []string{"e"}, []string{"j"},
		[]string{"s"}, []string{"g"})
	want := []string{"c", "e", "j", "s", "g"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assembleFallbacks order = %v, want %v", got, want)
	}
}

func aspect(w font.Weight, italic bool) font.Aspect {
	a := font.Aspect{Weight: w, Style: font.StyleNormal}
	if italic {
		a.Style = font.StyleItalic
	}
	return a
}

// TestRecordPrefersRegularOverBold is the #3 regression: a family whose Bold
// file sorts ahead of its Regular file in the walk must still fall back in
// Regular weight, not bold.
func TestRecordPrefersRegularOverBold(t *testing.T) {
	s := newTestScan()
	s.record("Noto Sans CJK JP", aspect(font.WeightBold, false), false, "/cjk-bold.otf")
	s.record("Noto Sans CJK JP", aspect(font.WeightNormal, false), false, "/cjk-reg.otf")
	if want := []string{"/cjk-reg.otf"}; !reflect.DeepEqual(s.cjkPaths, want) {
		t.Errorf("cjkPaths = %v, want %v (Regular must replace Bold)",
			s.cjkPaths, want)
	}
}

func TestRecordKeepsRegularWhenBoldFollows(t *testing.T) {
	s := newTestScan()
	s.record("Noto Sans CJK JP", aspect(font.WeightNormal, false), false, "/cjk-reg.otf")
	s.record("Noto Sans CJK JP", aspect(font.WeightBold, false), false, "/cjk-bold.otf")
	if want := []string{"/cjk-reg.otf"}; !reflect.DeepEqual(s.cjkPaths, want) {
		t.Errorf("cjkPaths = %v, want %v (Bold must not replace Regular)",
			s.cjkPaths, want)
	}
}

// TestRecordClosestWeightWins checks Medium loses to Regular even though both
// arrive after Bold: the closest-to-Normal face must end up stored.
func TestRecordClosestWeightWins(t *testing.T) {
	s := newTestScan()
	s.record("Fallback Sans", aspect(font.WeightBold, false), false, "/b.ttf")
	s.record("Fallback Sans", aspect(font.WeightMedium, false), false, "/m.ttf")
	s.record("Fallback Sans", aspect(font.WeightNormal, false), false, "/r.ttf")
	s.record("Fallback Sans", aspect(font.WeightBlack, false), false, "/x.ttf")
	if want := []string{"/r.ttf"}; !reflect.DeepEqual(s.generalPaths, want) {
		t.Errorf("generalPaths = %v, want %v", s.generalPaths, want)
	}
}

// TestRecordPrefersUprightOverItalic: equal weight, upright beats italic.
func TestRecordPrefersUprightOverItalic(t *testing.T) {
	s := newTestScan()
	s.record("Fallback Sans", aspect(font.WeightNormal, true), false, "/italic.ttf")
	s.record("Fallback Sans", aspect(font.WeightNormal, false), false, "/upright.ttf")
	if want := []string{"/upright.ttf"}; !reflect.DeepEqual(s.generalPaths, want) {
		t.Errorf("generalPaths = %v, want %v (upright must replace italic)",
			s.generalPaths, want)
	}
}

// TestRecordColorFaceNeverSwapsIntoMonoTier: when a family already sits in a
// monochrome tier, a later color face of the same family must join colorPaths
// but never replace the mono-tier entry — even when it looks "plainer" (the
// stored face is italic, the color face upright at equal weight).
func TestRecordColorFaceNeverSwapsIntoMonoTier(t *testing.T) {
	s := newTestScan()
	s.record("Symbols Font", aspect(font.WeightNormal, true), false, "/mono-italic.ttf")
	s.record("Symbols Font", aspect(font.WeightNormal, false), true, "/color.ttf")
	if want := []string{"/mono-italic.ttf"}; !reflect.DeepEqual(s.generalPaths, want) {
		t.Errorf("generalPaths = %v, want %v (color face must not enter a mono tier)",
			s.generalPaths, want)
	}
	if want := []string{"/color.ttf"}; !reflect.DeepEqual(s.colorPaths, want) {
		t.Errorf("colorPaths = %v, want %v", s.colorPaths, want)
	}
}

// TestRecordReplacesInPlace: a replacement must hit the family's own slot and
// leave other families in the same tier untouched.
func TestRecordReplacesInPlace(t *testing.T) {
	s := newTestScan()
	s.record("Alpha", aspect(font.WeightBold, false), false, "/alpha-bold.ttf")
	s.record("Beta", aspect(font.WeightNormal, false), false, "/beta.ttf")
	s.record("Alpha", aspect(font.WeightNormal, false), false, "/alpha-reg.ttf")
	want := []string{"/alpha-reg.ttf", "/beta.ttf"}
	if !reflect.DeepEqual(s.generalPaths, want) {
		t.Errorf("generalPaths = %v, want %v", s.generalPaths, want)
	}
}

// TestRecordCJKFamsStayAlignedAfterWeightSwap: a same-family weight swap in
// the CJK tier replaces the path in place, so cjkFams must stay index-aligned
// with cjkPaths and the locale reorder must still match the right family.
func TestRecordCJKFamsStayAlignedAfterWeightSwap(t *testing.T) {
	s := newTestScan()
	s.record("Hiragino Sans", aspect(font.WeightBold, false), false, "/hiragino-bold.ttc")
	s.record("PingFang SC", aspect(font.WeightNormal, false), false, "/pingfang.ttc")
	s.record("Hiragino Sans", aspect(font.WeightNormal, false), false, "/hiragino-reg.ttc")

	if want := []string{"/hiragino-reg.ttc", "/pingfang.ttc"}; !reflect.DeepEqual(s.cjkPaths, want) {
		t.Fatalf("cjkPaths = %v, want %v", s.cjkPaths, want)
	}
	if want := []string{"Hiragino Sans", "PingFang SC"}; !reflect.DeepEqual(s.cjkFams, want) {
		t.Fatalf("cjkFams = %v, want %v (misalignment breaks locale reorder)",
			s.cjkFams, want)
	}
	got := orderCJKForLang(s.cjkPaths, s.cjkFams, "ja_JP.UTF-8")
	want := []string{"/hiragino-reg.ttc", "/pingfang.ttc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ja reorder after swap = %v, want %v", got, want)
	}
}

func TestNormalizeCJKLang(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"en_US.UTF-8":     "",
		"C":               "",
		"ja_JP.UTF-8":     "ja",
		"ja":              "ja",
		"ko_KR":           "ko",
		"ko":              "ko",
		"zh":              "zh-hans",
		"zh_CN.UTF-8":     "zh-hans",
		"zh_SG":           "zh-hans",
		"zh-Hans":         "zh-hans",
		"zh_TW.UTF-8":     "zh-hant",
		"zh_HK":           "zh-hant",
		"zh-Hant":         "zh-hant",
		"zh-Hant-TW":      "zh-hant",
		"ZH_tw":           "zh-hant",
		"ja_JP.eucJP@cjk": "ja",
	}
	for in, want := range cases {
		if got := normalizeCJKLang(in); got != want {
			t.Errorf("normalizeCJKLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestOrderCJKForLang floats the locale-matching family to the front in
// preference order, keeps non-matching fonts in discovery order after, and
// no-ops on empty/non-CJK locales.
func TestOrderCJKForLang(t *testing.T) {
	paths := []string{"/yahei.ttc", "/hiragino.ttc", "/pingfangsc.ttc", "/dejavu.ttf"}
	fams := []string{"Microsoft YaHei", "Hiragino Sans", "PingFang SC", "DejaVu Sans"}

	t.Run("ja floats Hiragino first", func(t *testing.T) {
		got := orderCJKForLang(paths, fams, "ja_JP.UTF-8")
		want := []string{"/hiragino.ttc", "/yahei.ttc", "/pingfangsc.ttc", "/dejavu.ttf"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("zh-Hans floats PingFang SC then YaHei", func(t *testing.T) {
		got := orderCJKForLang(paths, fams, "zh_CN.UTF-8")
		want := []string{"/pingfangsc.ttc", "/yahei.ttc", "/hiragino.ttc", "/dejavu.ttf"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("empty locale is a no-op", func(t *testing.T) {
		got := orderCJKForLang(paths, fams, "")
		if !reflect.DeepEqual(got, paths) {
			t.Errorf("got %v, want unchanged %v", got, paths)
		}
	})

	t.Run("non-CJK locale is a no-op", func(t *testing.T) {
		got := orderCJKForLang(paths, fams, "en_US.UTF-8")
		if !reflect.DeepEqual(got, paths) {
			t.Errorf("got %v, want unchanged %v", got, paths)
		}
	})

	t.Run("misaligned fams is a no-op", func(t *testing.T) {
		got := orderCJKForLang(paths, fams[:2], "ja")
		if !reflect.DeepEqual(got, paths) {
			t.Errorf("got %v, want unchanged %v", got, paths)
		}
	})

	t.Run("single entry is a no-op", func(t *testing.T) {
		got := orderCJKForLang(paths[:1], fams[:1], "ja")
		if !reflect.DeepEqual(got, paths[:1]) {
			t.Errorf("got %v, want unchanged %v", got, paths[:1])
		}
	})
}

// TestOrderCJKForLangStable: two families matching different preferences sort
// by preference; discovery order breaks ties.
func TestOrderCJKForLangStable(t *testing.T) {
	paths := []string{"/a-noto.otf", "/b-noto.otf", "/yahei.ttc"}
	fams := []string{"Noto Sans SC", "Noto Serif SC", "Microsoft YaHei"}
	got := orderCJKForLang(paths, fams, "zh_CN")
	want := []string{"/a-noto.otf", "/b-noto.otf", "/yahei.ttc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectLangPrecedence(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "ja_JP.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := detectLang(); got != "ja_JP.UTF-8" {
		t.Errorf("LC_CTYPE should win over LANG: got %q", got)
	}
	t.Setenv("LC_ALL", "ko_KR.UTF-8")
	if got := detectLang(); got != "ko_KR.UTF-8" {
		t.Errorf("LC_ALL should win: got %q", got)
	}
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	if got := detectLang(); got != "" {
		t.Errorf("all unset should yield empty: got %q", got)
	}
}

func TestBetterRegular(t *testing.T) {
	cases := []struct {
		name      string
		w         font.Weight
		italic    bool
		curW      font.Weight
		curItalic bool
		want      bool
	}{
		{"regular beats bold", font.WeightNormal, false, font.WeightBold, false, true},
		{"bold loses to regular", font.WeightBold, false, font.WeightNormal, false, false},
		{"upright beats italic same weight", font.WeightNormal, false, font.WeightNormal, true, true},
		{"italic loses same weight", font.WeightNormal, true, font.WeightNormal, false, false},
		{"equal upright no change", font.WeightNormal, false, font.WeightNormal, false, false},
		{"medium beats bold", font.WeightMedium, false, font.WeightBold, false, true},
	}
	for _, c := range cases {
		if got := betterRegular(c.w, c.italic, c.curW, c.curItalic); got != c.want {
			t.Errorf("%s: betterRegular = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestWarmFallbackCoverage verifies the warm pass leaves a coverageCache
// entry for every fallback font (see warmFallbackCoverage for why).
// Entries may be nil (negative cache for unparseable fonts) — presence is
// what matters.
func TestWarmFallbackCoverage(t *testing.T) {
	ctx, err := NewContext(1)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	if len(ctx.fallbackPaths) == 0 {
		t.Skip("no system fallback fonts discovered")
	}

	// Synchronous call: idempotent, so this deterministically completes
	// the population even if the NewContext goroutine is still running.
	warmFallbackCoverage(ctx.fallbackPaths)

	coverageCache.mu.Lock()
	defer coverageCache.mu.Unlock()
	for _, p := range ctx.fallbackPaths {
		if p == "" {
			continue // loadCoverage skips "" without caching it
		}
		if _, ok := coverageCache.items[p]; !ok {
			t.Errorf("fallback font %q not in coverage cache after warm", p)
		}
	}
}

// TestWarmFallbackCoverageBadPaths verifies the warm loop survives
// degenerate fallback entries: empty strings are skipped without caching,
// while nonexistent and corrupt font files are negative-cached as nil
// instead of panicking or aborting the loop.
func TestWarmFallbackCoverageBadPaths(t *testing.T) {
	corrupt := filepath.Join(t.TempDir(), "corrupt.ttf")
	if err := os.WriteFile(corrupt, []byte("not a font"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing.ttf")

	// Bad entries are negative-cached in the process-global map; remove
	// them afterward so they never leak into other tests' probes.
	t.Cleanup(func() {
		coverageCache.mu.Lock()
		delete(coverageCache.items, missing)
		delete(coverageCache.items, corrupt)
		coverageCache.mu.Unlock()
	})

	warmFallbackCoverage([]string{"", missing, corrupt})

	coverageCache.mu.Lock()
	defer coverageCache.mu.Unlock()
	if _, ok := coverageCache.items[""]; ok {
		t.Error("empty path must not be cached")
	}
	for _, p := range []string{missing, corrupt} {
		cov, ok := coverageCache.items[p]
		if !ok {
			t.Errorf("bad path %q not negative-cached", p)
		} else if cov != nil {
			t.Errorf("bad path %q cached non-nil coverage", p)
		}
	}
}
