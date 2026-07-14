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
