package glyph

import (
	"slices"
	"testing"
)

// wordCorpus exercises every class transition the segmenter knows about:
// dotted identifiers, underscores, punctuation runs, Japanese script
// changes, combining marks, newlines, and text that is only whitespace.
var wordCorpus = []string{
	"foo.bar.baz",
	"hello   world",
	"日本語のテキスト",
	"hello\nworld",
	"   ",
	"",
	"snake_case_name",
	"a+b",
	"café",
	"  a  b  ",
	"path/to/file.go:42",
}

// The string helpers and the Layout methods must agree at every index of
// every corpus text. They share wordRuns, so a divergence means one of
// them grew a special case the other did not — which is exactly the drift
// this test exists to catch.
func TestStringHelpersMatchLayoutMethods(t *testing.T) {
	for _, text := range wordCorpus {
		l := wordLayout(text)
		for i := 0; i <= len(text); i++ {
			wantBeg, wantEnd := l.GetWordAtIndex(i)
			gotBeg, gotEnd := WordBoundsInString(text, i)
			if gotBeg != wantBeg || gotEnd != wantEnd {
				t.Errorf("WordBoundsInString(%q, %d) = (%d, %d), "+
					"GetWordAtIndex = (%d, %d)",
					text, i, gotBeg, gotEnd, wantBeg, wantEnd)
			}
			if got, want := WordStartLeft(text, i),
				l.MoveCursorWordLeft(i); got != want {
				t.Errorf("WordStartLeft(%q, %d) = %d, "+
					"MoveCursorWordLeft = %d", text, i, got, want)
			}
			if got, want := WordStartRight(text, i),
				l.MoveCursorWordRight(i); got != want {
				t.Errorf("WordStartRight(%q, %d) = %d, "+
					"MoveCursorWordRight = %d", text, i, got, want)
			}
		}
	}
}

// The behaviours issue go-gui-org/go-gui#329 asks for, asserted directly
// on the string API that go-gui's no-layout fallback calls.
func TestWordBoundsInStringCases(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		index      int
		start, end int
	}{
		{"dotted component", "foo.bar.baz", 5, 4, 7},
		{"separator is its own word", "foo.bar.baz", 3, 3, 4},
		{"underscore joins", "snake_case_name", 3, 0, 15},
		{"lone punctuation", "a+b", 1, 1, 2},
		{"whitespace run", "hello   world", 6, 5, 8},
		{"combining mark stays", "café", 3, 0, 6},
		{"han run", "日本語のテキスト", 3, 0, 9},
		{"katakana run", "日本語のテキスト", 15, 12, 24},
		{"end of text picks last word", "foo.bar.baz", 11, 8, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := WordBoundsInString(tt.text, tt.index)
			if start != tt.start || end != tt.end {
				t.Errorf("WordBoundsInString(%q, %d) = (%d, %d), want (%d, %d)",
					tt.text, tt.index, start, end, tt.start, tt.end)
			}
		})
	}
}

func TestWordStartWalk(t *testing.T) {
	// Word-left from the end walks every dotted component and separator.
	var left []int
	for pos := len("foo.bar.baz"); pos > 0; {
		pos = WordStartLeft("foo.bar.baz", pos)
		left = append(left, pos)
	}
	if want := []int{8, 7, 4, 3, 0}; !slices.Equal(left, want) {
		t.Errorf("word-left walk = %v, want %v", left, want)
	}

	// Japanese: 日本語 | の | テキスト at byte offsets 0, 9, 12.
	if got := WordStartRight("日本語のテキスト", 0); got != 9 {
		t.Errorf("WordStartRight(japanese, 0) = %d, want 9", got)
	}
	if got := WordStartRight("日本語のテキスト", 9); got != 12 {
		t.Errorf("WordStartRight(japanese, 9) = %d, want 12", got)
	}

	// Past the last word start there is nowhere to go but the end.
	if got := WordStartRight("hi", 1); got != 2 {
		t.Errorf("WordStartRight(%q, 1) = %d, want 2", "hi", got)
	}
}

// The one deliberate divergence from GetWordAtIndex: with no LogAttrs
// there is no cluster guard, so a ZWJ between word runes classifies as
// punctuation and splits the sequence. The Layout keeps a+ZWJ as one word.
func TestWordBoundsInStringSplitsZWJ(t *testing.T) {
	text := "a\u200db"
	start, end := WordBoundsInString(text, 1)
	if start != 1 || end != 4 {
		t.Errorf("WordBoundsInString(%q, 1) = (%d, %d), want (1, 4)",
			text, start, end)
	}
	if got := WordStartLeft(text, 5); got != 4 {
		t.Errorf("WordStartLeft(%q, 5) = %d, want 4", text, got)
	}
	if got := WordStartRight(text, 1); got != 4 {
		t.Errorf("WordStartRight(%q, 1) = %d, want 4", text, got)
	}
}
