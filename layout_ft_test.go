//go:build android || linux || darwin || windows

package glyph

import "testing"

// TestClusterIsEmoji_MiscTechnical pins the Misc Technical (U+2300–U+23FF)
// classification. The block is mostly non-emoji symbols; matching the whole
// range forced a color-only fallback on text symbols such as the media
// triangles ⏴⏵⏶⏷ (U+23F4–U+23F7), which no color font covers — so they lost
// their text fallback (STIX) and rendered as the base font's .notdef box.
// Only the codepoints Unicode marks Emoji (emoji-data.txt) must match.
func TestClusterIsEmoji_MiscTechnical(t *testing.T) {
	emoji := []struct {
		name string
		s    string
	}{
		{"watch U+231A", "⌚"},
		{"hourglass U+231B", "⌛"},
		{"keyboard U+2328", "⌨"},
		{"eject U+23CF", "⏏"},
		{"fast-forward U+23E9", "⏩"},
		{"alarm clock U+23F0", "⏰"},
		{"hourglass-flowing U+23F3", "⏳"},
		{"pause U+23F8", "⏸"},
		{"record U+23FA", "⏺"},
		{"grinning face U+1F600", "😀"},
	}
	for _, c := range emoji {
		if !clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=false, want true", c.name)
		}
	}

	// Non-emoji symbols in/around the block must NOT demand a color font,
	// or they lose their monochrome/text fallback and render as tofu.
	notEmoji := []struct {
		name string
		s    string
	}{
		{"reverse triangle U+23F4", "⏴"},
		{"play triangle U+23F5", "⏵"}, // the reported bug (Claude Code prompt)
		{"up triangle U+23F6", "⏶"},
		{"down triangle U+23F7", "⏷"},
		{"house U+2302", "⌂"},
		{"command U+2318", "⌘"},
		{"ascii letter", "A"},
	}
	for _, c := range notEmoji {
		if clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=true, want false", c.name)
		}
	}
}
