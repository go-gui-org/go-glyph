//go:build android || linux || darwin || windows

package glyph

import (
	"sort"
	"testing"
)

// TestClusterIsEmoji_MiscTechnical pins the Misc Technical (U+2300–U+23FF)
// classification. Only default-emoji-presentation codepoints take the color
// path. Default-text emoji (Emoji but not Emoji_Presentation, e.g. ⌨ ⏏ ⏸)
// and plain symbols (the media triangles ⏴⏵⏶⏷) must render as their monochrome
// text glyph, not the base font's .notdef box.
func TestClusterIsEmoji_MiscTechnical(t *testing.T) {
	emoji := []struct {
		name string
		s    string
	}{
		{"watch U+231A", "⌚"},
		{"hourglass U+231B", "⌛"},
		{"fast-forward U+23E9", "⏩"},
		{"alarm clock U+23F0", "⏰"},
		{"hourglass-flowing U+23F3", "⏳"},
		{"grinning face U+1F600", "😀"},
	}
	for _, c := range emoji {
		if !clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=false, want true", c.name)
		}
	}

	notEmoji := []struct {
		name string
		s    string
	}{
		// Default-text emoji: bare, they must stay monochrome text.
		{"keyboard U+2328", "⌨"},
		{"eject U+23CF", "⏏"},
		{"pause U+23F8", "⏸"},
		{"record U+23FA", "⏺"},
		// Plain non-emoji symbols.
		{"reverse triangle U+23F4", "⏴"},
		{"play triangle U+23F5", "⏵"},
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

// TestClusterIsEmoji_MiscSymbolsDingbats pins the Miscellaneous Symbols +
// Dingbats (U+2600–U+27BF) classification and the default-text-emoji rule.
// Codepoints that default to emoji presentation take the color path; the many
// default-text emoji in these blocks (☀ ✂ ✳ ❄ ❤ ➡ — Emoji but not
// Emoji_Presentation) must render as the monochrome text glyph unless an
// explicit VS16 requests color. U+2733 ✳ is the reported case: bare it is a
// plain asterisk (as Ghostty shows), not the green squared emoji.
func TestClusterIsEmoji_MiscSymbolsDingbats(t *testing.T) {
	emoji := []struct {
		name string
		s    string
	}{
		{"umbrella with rain U+2614", "☔"},
		{"soccer ball U+26BD", "⚽"},
		{"snowman without snow U+26C4", "⛄"},
		{"sparkles U+2728", "✨"},
		{"check button U+2705", "✅"},
		{"cross mark U+274C", "❌"},
		{"question mark U+2753", "❓"},
		{"double curly loop U+27BF", "➿"},
	}
	for _, c := range emoji {
		if !clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=false, want true", c.name)
		}
	}

	notEmoji := []struct {
		name string
		s    string
	}{
		// Default-text emoji: monochrome unless followed by VS16.
		{"sun U+2600", "☀"},
		{"scissors U+2702", "✂"},
		{"airplane U+2708", "✈"},
		{"eight-spoked asterisk U+2733", "✳"}, // the reported bug (green tofu)
		{"snowflake U+2744", "❄"},
		{"heavy heart U+2764", "❤"},
		{"black rightwards arrow U+27A1", "➡"},
		{"heavy check mark U+2714", "✔"},
		// Plain non-emoji symbols.
		{"heavy asterisk U+2731", "✱"},
		{"open centre asterisk U+2732", "✲"},
		{"lower-right pencil U+270E", "✎"},
		{"check mark U+2713", "✓"},
		{"curved stem paragraph U+2761", "❡"},
	}
	for _, c := range notEmoji {
		if clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=true, want false", c.name)
		}
	}

	// VS16 flips a default-text emoji to the color path.
	if !clusterIsEmoji("✳️") {
		t.Error("clusterIsEmoji(U+2733 U+FE0F)=false, want true (VS16 forces emoji)")
	}
}

// TestClusterIsEmoji_SupplementalBlocks pins the supplemental-plane blocks that
// the former coarse ranges (U+1F000–U+1F0FF, U+1F300–U+1FAFF) swept in wholesale.
// Those blocks are mostly non-emoji symbols — mahjong tiles, playing cards,
// alchemical and chess symbols, Supplemental Arrows-C — that no color font
// covers, so the whole-block match rendered them as tofu.
func TestClusterIsEmoji_SupplementalBlocks(t *testing.T) {
	emoji := []struct {
		name string
		s    string
	}{
		{"grinning face U+1F600", "😀"},
		{"mahjong red dragon U+1F004", "🀄"},
		{"joker U+1F0CF", "🃏"},
		{"large red square U+1F7E5", "🟥"},
		{"orange circle U+1F7E0", "🟠"},
		{"rocket U+1F680", "🚀"},
	}
	for _, c := range emoji {
		if !clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=false, want true", c.name)
		}
	}

	notEmoji := []struct {
		name string
		s    string
	}{
		{"mahjong tile east wind U+1F000", "🀀"},
		{"domino tile U+1F031", "🀱"},
		{"playing card ace of spades U+1F0A1", "🂡"},
		{"alchemical quintessence U+1F700", "🜀"},
		{"neutral chess king U+1FA00", "🨀"},
		{"leftwards arrow small triangle U+1F800", "🠀"},
		{"ornamental leaf U+1F650", "🙐"},
		{"black isosceles triangle U+1F780", "🞀"},
	}
	for _, c := range notEmoji {
		if clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=true, want false", c.name)
		}
	}
}

// TestClusterIsEmoji_TextAndCombiners guards the two edges the base table must
// not cross: ASCII / common text symbols never demand a color font on their
// own, but the emoji combiners (VS16, ZWJ, keycap) still force one.
func TestClusterIsEmoji_TextAndCombiners(t *testing.T) {
	notEmoji := []struct {
		name string
		s    string
	}{
		{"hash U+0023", "#"},
		{"asterisk U+002A", "*"},
		{"digit five U+0035", "5"},
		{"letter A", "A"},
		{"copyright U+00A9", "©"},
		{"registered U+00AE", "®"},
		{"trademark U+2122", "™"},
	}
	for _, c := range notEmoji {
		if clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=true, want false", c.name)
		}
	}

	emoji := []struct {
		name string
		s    string
	}{
		{"keycap digit 5", "5️⃣"},
		{"copyright + VS16", "©️"},
		{"flag US regional indicators", "🇺🇸"},
		{"ZWJ family sequence", "👨‍👩‍👧"},
	}
	for _, c := range emoji {
		if !clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=false, want true", c.name)
		}
	}
}

// TestClusterIsEmoji_PresentationSelectors pins VS15 / VS16 precedence. VS16
// (U+FE0F) forces the color path; VS15 (U+FE0E) forces the monochrome text
// path and must win even over an emoji-base codepoint (the base is iterated
// first, so the override is a separate pre-pass). The reported case is
// starship's record dot U+23FA+VS15, colored via SGR — it must stay text so
// the foreground color applies instead of a color-font glyph.
func TestClusterIsEmoji_PresentationSelectors(t *testing.T) {
	text := []struct {
		name string
		s    string
	}{
		{"record + VS15 (starship dot)", "⏺︎"}, // U+23FA U+FE0E
		{"default-text emoji ✳ + VS15", "✳︎"},  // U+2733 U+FE0E
		{"emoji-base hourglass + VS15", "⏳︎"},  // U+23F3 U+FE0E, base is emoji
		{"emoji-base watch + VS15", "⌚︎"},      // U+231A U+FE0E, base is emoji
	}
	for _, c := range text {
		if clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=true, want false (VS15 forces text)", c.name)
		}
	}

	color := []struct {
		name string
		s    string
	}{
		{"record + VS16", "⏺️"},               // U+23FA U+FE0F
		{"default-text emoji ✳ + VS16", "✳️"}, // U+2733 U+FE0F
	}
	for _, c := range color {
		if !clusterIsEmoji(c.s) {
			t.Errorf("clusterIsEmoji(%s)=false, want true (VS16 forces emoji)", c.name)
		}
	}

	// VS1–VS14 carry no presentation meaning: a bare text symbol stays text.
	if clusterIsEmoji("A︀") {
		t.Error("clusterIsEmoji(A U+FE00)=true, want false (VS1 is not an emoji signal)")
	}
}

// TestEmojiBaseRangesInvariant guards the generated table: strictly ascending,
// non-overlapping, non-adjacent (adjacent ranges should have been coalesced),
// and entirely at or above the U+2300 floor isEmojiBase relies on.
func TestEmojiBaseRangesInvariant(t *testing.T) {
	prevHi := rune(-1)
	for i, r := range emojiBaseRanges {
		lo, hi := r[0], r[1]
		if lo > hi {
			t.Errorf("range %d: lo U+%04X > hi U+%04X", i, lo, hi)
		}
		if lo < 0x2300 {
			t.Errorf("range %d: lo U+%04X below the U+2300 floor", i, lo)
		}
		if lo <= prevHi+1 {
			t.Errorf("range %d: lo U+%04X not strictly above prev hi U+%04X (uncoalesced or overlapping)",
				i, lo, prevHi)
		}
		prevHi = hi
	}
}

// TestClusterAssignmentEquivalence verifies the binary-search cluster assignment
// in buildLayout produces the same results as the old linear scan for a variety of
// cluster sequences: monotonically-increasing (LTR), non-increasing (RTL), and
// merged ligature clusters.
func TestClusterAssignmentEquivalence(t *testing.T) {
	linearScan := func(startRune []int, c int) int {
		k := 0
		for k+1 < len(startRune) && startRune[k+1] <= c {
			k++
		}
		return k
	}

	binarySearch := func(startRune []int, c int) int {
		k := sort.Search(len(startRune), func(i int) bool {
			return startRune[i] > c
		}) - 1
		if k < 0 {
			k = 0
		}
		return k
	}

	tests := []struct {
		name      string
		startRune []int // rune-index of each cluster start
		queries   []int // cluster values from HarfBuzz
		desc      string
	}{
		{
			name:      "LTR monotonic ASCII",
			startRune: []int{0, 2, 4, 6, 8, 10},
			queries:   []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			desc:      "monotonically increasing cluster values, no gaps",
		},
		{
			name:      "RTL non-increasing",
			startRune: []int{0, 3, 6, 9},
			queries:   []int{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
			desc:      "HarfBuzz emits RTL glyphs in non-increasing cluster order",
		},
		{
			name:      "merged ligature clusters",
			startRune: []int{0, 1, 2, 4, 5, 6},
			queries:   []int{0, 1, 2, 3, 4, 5, 6},
			desc:      "cluster 3 belongs to rune 2 (f-i ligature merges 2..3)",
		},
		{
			name:      "single cluster run",
			startRune: []int{0},
			queries:   []int{0, 0, 0, 0},
			desc:      "all glyphs land on the sole cluster",
		},
		{
			name:      "cluster values at exact boundaries",
			startRune: []int{0, 5, 10, 15},
			queries:   []int{0, 5, 10, 15},
			desc:      "each query lands exactly on a boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, c := range tt.queries {
				want := linearScan(tt.startRune, c)
				got := binarySearch(tt.startRune, c)
				if got != want {
					t.Errorf("startRune=%v c=%d: got k=%d, want k=%d (%s)",
						tt.startRune, c, got, want, tt.desc)
					return
				}
			}
		})
	}
}

// TestIsEmojiBase exercises the binary-search function directly, including
// the r < 0x2300 fast-path and boundary cases at range edges.
func TestIsEmojiBase(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"below floor U+22FF", 0x22FF, false},
		{"floor boundary U+2300 (gap)", 0x2300, false},
		{"first range lo U+231A", 0x231A, true},
		{"first range hi U+231B", 0x231B, true},
		{"gap U+231C", 0x231C, false},
		{"grinning face U+1F600", 0x1F600, true},
		{"last range hi U+1FAF8", 0x1FAF8, true},
		{"beyond last U+1FAF9", 0x1FAF9, false},
	}
	for _, tt := range tests {
		if got := isEmojiBase(tt.r); got != tt.want {
			t.Errorf("isEmojiBase(U+%04X)=%v, want %v (%s)",
				tt.r, got, tt.want, tt.name)
		}
	}
}
