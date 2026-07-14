//go:build linux || darwin || windows

package glyph

import (
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
