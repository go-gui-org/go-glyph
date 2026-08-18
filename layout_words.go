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

	prevClass := classSpace
	for off := 0; off < len(text); {
		r, size := utf8.DecodeRuneInString(text[off:])
		c := runeClass(r)
		if c != prevClass {
			// Close the run that just ended, unless it was whitespace.
			if prevClass != classSpace {
				mark(off, false)
			}
			// Open the run starting here, unless it is whitespace.
			if c != classSpace {
				mark(off, true)
			}
			prevClass = c
		}
		off += size
	}

	// A word that runs to the end of the text ends at len(text), which is
	// the end-of-text attr appended by every layout builder.
	if prevClass != classSpace {
		mark(len(text), false)
	}
}
