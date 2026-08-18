package glyph

import (
	"slices"
	"testing"
)

// syntheticAttrs builds the attribute table every layout builder produces
// for text without ligatures — one cursor-position attr per byte plus the
// end-of-text attr — and runs applyWordAttrs over it. Shared by the word
// tests so the reference segmentation and the Layout-driven word API tests
// exercise the exact same table shape.
func syntheticAttrs(text string) (logAttrs []LogAttr, logAttrByIndex map[int]int) {
	logAttrs = make([]LogAttr, len(text)+1)
	logAttrByIndex = make(map[int]int, len(text)+1)
	for i := range logAttrs {
		logAttrs[i].IsCursorPosition = true
		logAttrByIndex[i] = i
	}
	applyWordAttrs(text, logAttrs, logAttrByIndex)
	return logAttrs, logAttrByIndex
}

// wordAttrsFor returns the word start and end byte offsets that
// applyWordAttrs produces for text.
func wordAttrsFor(text string) (starts, ends []int) {
	logAttrs, _ := syntheticAttrs(text)
	for i, a := range logAttrs {
		if a.IsWordStart {
			starts = append(starts, i)
		}
		if a.IsWordEnd {
			ends = append(ends, i)
		}
	}
	return starts, ends
}

func TestApplyWordAttrs(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		starts []int
		ends   []int
	}{{
		// The motivating case: dotted identifiers and paths must walk
		// component by component, so each '.' is a word of its own.
		name:   "dotted identifier splits at punctuation",
		text:   "foo.bar.baz",
		starts: []int{0, 3, 4, 7, 8},
		ends:   []int{3, 4, 7, 8, 11},
	}, {
		name:   "underscores stay inside one word",
		text:   "snake_case_name",
		starts: []int{0},
		ends:   []int{15},
	}, {
		name:   "single punctuation is its own word",
		text:   "a+b",
		starts: []int{0, 1, 2},
		ends:   []int{1, 2, 3},
	}, {
		// 日本語 (Han) の (Hiragana) テキスト (Katakana); every rune is
		// 3 bytes.
		name:   "japanese breaks at script transitions",
		text:   "日本語のテキスト",
		starts: []int{0, 9, 12},
		ends:   []int{9, 12, 24},
	}, {
		name:   "newline ends a word",
		text:   "hello world\nfoo",
		starts: []int{0, 6, 12},
		ends:   []int{5, 11, 15},
	}, {
		// e + U+0301 combining acute: the mark is classWord, so it stays
		// attached to its base rather than starting a new word.
		name:   "combining mark stays with its base",
		text:   "café",
		starts: []int{0},
		ends:   []int{6},
	}, {
		// Plain ASCII prose is the one case the old whitespace test got
		// right; it must not move.
		name:   "ascii prose unchanged",
		text:   "hello world",
		starts: []int{0, 6},
		ends:   []int{5, 11},
	}, {
		name:   "leading and trailing whitespace are not words",
		text:   "  hi  ",
		starts: []int{2},
		ends:   []int{4},
	}, {
		name:   "tabs separate words",
		text:   "a\tb",
		starts: []int{0, 2},
		ends:   []int{1, 3},
	}, {
		name:   "empty text has no words",
		text:   "",
		starts: nil,
		ends:   nil,
	}, {
		name:   "whitespace only has no words",
		text:   "   ",
		starts: nil,
		ends:   nil,
	}, {
		name:   "digits and letters are one word",
		text:   "utf8",
		starts: []int{0},
		ends:   []int{4},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starts, ends := wordAttrsFor(tt.text)
			if !slices.Equal(starts, tt.starts) {
				t.Errorf("starts = %v, want %v", starts, tt.starts)
			}
			if !slices.Equal(ends, tt.ends) {
				t.Errorf("ends = %v, want %v", ends, tt.ends)
			}
		})
	}
}

// Word starts and ends must strictly alternate — GetWordAtIndex pairs them
// by index and would return nonsense ranges otherwise.
func TestApplyWordAttrsAlternates(t *testing.T) {
	texts := []string{
		"foo.bar.baz", "  a  b  ", "日本語のテキスト", "hello world\nfoo",
		"a", ".", " . ", "a+b*c/d", "café naïve", "\n\n\n", "x\ty\nz",
	}
	for _, text := range texts {
		starts, ends := wordAttrsFor(text)
		if len(starts) != len(ends) {
			t.Fatalf("%q: %d starts but %d ends", text, len(starts), len(ends))
		}
		for i := range starts {
			if starts[i] >= ends[i] {
				t.Errorf("%q: word %d is empty or inverted: [%d,%d)",
					text, i, starts[i], ends[i])
			}
			if i > 0 && ends[i-1] > starts[i] {
				t.Errorf("%q: word %d starts at %d before word %d ends at %d",
					text, i, starts[i], i-1, ends[i-1])
			}
		}
	}
}

// A boundary that falls inside a grapheme cluster must be dropped: the ZWJ
// in an emoji sequence classifies as punctuation, but the cluster is one
// caret stop and one word.
func TestApplyWordAttrsSkipsNonCursorPositions(t *testing.T) {
	// Woman + ZWJ + laptop — a single grapheme cluster.
	text := "\U0001F469\u200d\U0001F4BB"
	logAttrs := []LogAttr{
		{IsCursorPosition: true},  // cluster start, byte 0
		{IsCursorPosition: false}, // absorbed continuation
		{IsCursorPosition: true},  // end of text
	}
	logAttrByIndex := map[int]int{0: 0, 4: 1, len(text): 2}
	applyWordAttrs(text, logAttrs, logAttrByIndex)

	if logAttrs[1].IsWordStart || logAttrs[1].IsWordEnd {
		t.Errorf("word boundary set on a non-cursor position: %+v", logAttrs[1])
	}
}

func TestRuneClass(t *testing.T) {
	tests := []struct {
		r    rune
		want int
	}{
		{' ', classSpace}, {'\t', classSpace}, {'\n', classSpace},
		{' ', classSpace}, // NBSP
		{'.', classPunct}, {'+', classPunct}, {'-', classPunct},
		{'a', classWord}, {'Z', classWord}, {'7', classWord},
		{'_', classWord}, {'́', classWord}, // combining acute
		{'日', classHan}, {'の', classHiragana}, {'テ', classKatakana},
	}
	for _, tt := range tests {
		if got := runeClass(tt.r); got != tt.want {
			t.Errorf("runeClass(%q) = %d, want %d", tt.r, got, tt.want)
		}
	}
}
