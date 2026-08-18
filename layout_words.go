package glyph

import (
	"unicode"
	"unicode/utf8"
)

// Word segmentation: class-run, no dictionary.
//
// A word is one maximal run of runes that share a class. Whitespace runs
// are not words — they are the gaps between them. Punctuation runs *are*
// words of their own, which is what makes a word motion walk `foo.bar.baz`
// component by component instead of jumping the whole token. Han,
// Hiragana, and Katakana are separate classes so script transitions end a
// word in Japanese, which is the standard approximation when no dictionary
// is available.
//
// Ported from go-shirei (widgets/editcore.go, zlib licensed) so that
// go-glyph, go-gui, and go-shirei all segment identically.
//
// UAX #29 word rules are deliberately not used here. They keep `3.14`,
// `can't`, and `foo.bar` together, which is the opposite of what an editor
// caret should do, and the browser's Intl.Segmenter applies locale-specific
// rules that the native path could never reproduce byte for byte.

const (
	classSpace = iota
	classPunct
	classWord
	classHan
	classHiragana
	classKatakana
)

// runeClass reports which word class r belongs to.
func runeClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return classSpace
	case unicode.Is(unicode.Han, r):
		return classHan
	case unicode.Is(unicode.Hiragana, r):
		return classHiragana
	case unicode.Is(unicode.Katakana, r):
		return classKatakana
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) ||
		unicode.IsMark(r):
		// unicode.IsMark keeps a combining accent attached to its base,
		// and '_' keeps snake_case_name a single word.
		return classWord
	default:
		return classPunct
	}
}

// wordRuns calls fn once for each maximal run of same-class runes in
// text, passing the run's byte range [start, end) and whether the run is
// whitespace. fn returns false to stop the walk early.
//
// This is the single traversal behind both the layout attribute pass and
// the exported string helpers, so the two cannot disagree about where a
// word begins or ends.
func wordRuns(text string, fn func(start, end int, space bool) bool) {
	for off := 0; off < len(text); {
		r, size := utf8.DecodeRuneInString(text[off:])
		c := runeClass(r)
		end := off + size
		for end < len(text) {
			r2, size2 := utf8.DecodeRuneInString(text[end:])
			if runeClass(r2) != c {
				break
			}
			end += size2
		}
		if !fn(off, end, c == classSpace) {
			return
		}
		off = end
	}
}

// hasWordRun reports whether text contains at least one non-whitespace
// run. Text made only of whitespace has no words, which the Layout word
// API signals by returning the query index unchanged.
func hasWordRun(text string) bool {
	found := false
	wordRuns(text, func(_, _ int, space bool) bool {
		if !space {
			found = true
			return false
		}
		return true
	})
	return found
}

// WordBoundsInString returns the [start, end) byte range of the class run
// containing byteIdx. It is the string-only twin of
// (*Layout).GetWordAtIndex: a punctuation run is a word of its own, and an
// index inside a whitespace run returns that whole run.
//
// The one deliberate difference is grapheme clusters. GetWordAtIndex
// consults LogAttrs and refuses to mark a boundary inside a cluster, so a
// ZWJ stays attached to its base; this function has only the text, so a
// ZWJ between word runes classifies as punctuation and splits them.
// Callers with a Layout in hand should prefer the method.
func WordBoundsInString(s string, byteIdx int) (int, int) {
	// Whitespace-only (or empty) text has no words at all, and the
	// Layout method reports that by returning an empty range at the
	// query index. Mirror it before clamping, as the method does.
	if !hasWordRun(s) {
		return byteIdx, byteIdx
	}
	if byteIdx >= len(s) {
		// The caret one past the end belongs to the last run, so a
		// double-click there selects the final word. Run boundaries are
		// rune-aligned, so a mid-rune index still resolves correctly.
		byteIdx = len(s) - 1
	}
	byteIdx = max(byteIdx, 0)

	start, end := byteIdx, byteIdx
	wordRuns(s, func(rs, re int, _ bool) bool {
		if byteIdx < re {
			start, end = rs, re
			return false
		}
		return true
	})
	return start, end
}

// WordStartLeft returns the byte index of the word start before byteIdx,
// or 0 when there is none. It is the string-only twin of
// (*Layout).MoveCursorWordLeft: the caret lands on word starts only,
// never inside whitespace and never on a word end.
func WordStartLeft(s string, byteIdx int) int {
	if byteIdx <= 0 {
		return 0
	}
	result := 0
	wordRuns(s, func(rs, _ int, space bool) bool {
		if space {
			return true
		}
		if rs < byteIdx {
			result = rs
			return true
		}
		return false
	})
	return result
}

// WordStartRight returns the byte index of the word start after byteIdx,
// or len(s) when there is none. It is the string-only twin of
// (*Layout).MoveCursorWordRight.
func WordStartRight(s string, byteIdx int) int {
	result := -1
	wordRuns(s, func(rs, _ int, space bool) bool {
		if space {
			return true
		}
		if rs > byteIdx {
			result = rs
			return false
		}
		return true
	})
	if result >= 0 {
		return result
	}
	return len(s)
}

// applyWordAttrs sets IsWordStart and IsWordEnd on logAttrs from the class
// runs of text. It runs as a post-pass over the finished attribute table
// rather than inline in a layout loop, for three reasons: the per-char
// layout loops visit chars in visual (bidi) order while word boundaries are
// a property of logical order; both backends can then share one
// implementation and agree byte for byte by construction; and the vertical
// layout paths, which build no word flags of their own, get them for free.
//
// IsWordEnd marks the byte index one past the last rune of a word — the
// exclusive end — which is what GetWordAtIndex and the go-gui selection
// code already expect.
func applyWordAttrs(text string, logAttrs []LogAttr, logAttrByIndex map[int]int) {
	if len(logAttrs) == 0 {
		return
	}

	// mark sets a flag at a byte offset, but only where the offset is a
	// real caret stop. An offset with no map entry, or one whose attr is
	// not a cursor position, lies inside a grapheme cluster — a ZWJ in an
	// emoji sequence classifies as punctuation, and must not be allowed to
	// split the cluster into two words.
	mark := func(off int, start bool) {
		idx, ok := logAttrByIndex[off]
		if !ok || idx < 0 || idx >= len(logAttrs) {
			return
		}
		if !logAttrs[idx].IsCursorPosition {
			return
		}
		if start {
			logAttrs[idx].IsWordStart = true
		} else {
			logAttrs[idx].IsWordEnd = true
		}
	}

	// Every non-whitespace run is a word: it opens at its first byte and
	// ends at the byte one past its last, which for a run reaching the
	// end of the text is len(text) — the end-of-text attr every layout
	// builder appends.
	wordRuns(text, func(start, end int, space bool) bool {
		if !space {
			mark(start, true)
			mark(end, false)
		}
		return true
	})
}
