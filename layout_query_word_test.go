package glyph

import (
	"slices"
	"testing"
)

// wordLayout builds a Layout whose LogAttrs are the real ones
// applyWordAttrs produces for text, with one caret stop per byte. It
// exercises the public word API directly rather than through a backend, so
// the same expectations hold on FreeType and wasm.
func wordLayout(text string) Layout {
	logAttrs, logAttrByIndex := syntheticAttrs(text)
	return Layout{
		Text:           text,
		LogAttrs:       logAttrs,
		LogAttrByIndex: logAttrByIndex,
	}
}

func TestGetWordAtIndexClassRuns(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		index      int
		start, end int
	}{
		{"inside first component", "foo.bar.baz", 1, 0, 3},
		{"on the separator", "foo.bar.baz", 3, 3, 4},
		{"inside middle component", "foo.bar.baz", 5, 4, 7},
		{"inside last component", "foo.bar.baz", 9, 8, 11},
		{"at end of text", "foo.bar.baz", 11, 8, 11},
		{"lone punctuation", "a+b", 1, 1, 2},
		{"start of text", "hello world", 0, 0, 5},
		// The last word used to have no IsWordEnd at all, so this fell
		// into the old snap-to-nearest branch.
		{"final word has a real end", "hello world", 8, 6, 11},
		{"inside whitespace run", "hello   world", 6, 5, 8},
		{"leading whitespace", "   hi", 1, 0, 3},
		{"trailing whitespace", "hi   ", 4, 2, 5},
		{"newline is a gap", "hello\nfoo", 5, 5, 6},
		{"han run", "日本語のテキスト", 3, 0, 9},
		// Byte 5 is inside the second kanji (bytes 3-8); a mid-rune
		// index must still resolve to the enclosing word.
		{"mid-rune index stays in word", "日本語のテキスト", 5, 0, 9},
		{"katakana run", "日本語のテキスト", 15, 12, 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := wordLayout(tt.text)
			start, end := l.GetWordAtIndex(tt.index)
			if start != tt.start || end != tt.end {
				t.Errorf("GetWordAtIndex(%d) on %q = (%d, %d), want (%d, %d): got %q",
					tt.index, tt.text, start, end, tt.start, tt.end,
					tt.text[max(0, start):min(len(tt.text), end)])
			}
		})
	}
}

// GetWordAtIndex must return a sane range for every index in the text, not
// just the ones inside words.
func TestGetWordAtIndexTotal(t *testing.T) {
	for _, text := range []string{
		"foo.bar.baz", "  a  b  ", "日本語のテキスト", "hello\nworld", "   ", "",
	} {
		l := wordLayout(text)
		for i := 0; i <= len(text); i++ {
			start, end := l.GetWordAtIndex(i)
			if start < 0 || end > len(text) || start > end {
				t.Errorf("%q: GetWordAtIndex(%d) = (%d, %d), out of range",
					text, i, start, end)
			}
		}
	}
}

func TestMoveCursorWordLeftRight(t *testing.T) {
	// Word-left from the end must stop at every component of a dotted
	// identifier — the behaviour the whitespace test could not produce.
	l := wordLayout("foo.bar.baz")
	var left []int
	for pos := len(l.Text); pos > 0; {
		pos = l.MoveCursorWordLeft(pos)
		left = append(left, pos)
	}
	if want := []int{8, 7, 4, 3, 0}; !slices.Equal(left, want) {
		t.Errorf("word-left walk = %v, want %v", left, want)
	}

	var right []int
	for pos := 0; ; {
		next := l.MoveCursorWordRight(pos)
		if next <= pos {
			break
		}
		pos = next
		right = append(right, pos)
	}
	// The final hop is MoveCursorWordRight's end-of-text fallback: past
	// the last word start there is nowhere left to go but the end.
	if want := []int{3, 4, 7, 8, 11}; !slices.Equal(right, want) {
		t.Errorf("word-right walk = %v, want %v", right, want)
	}
}

// Word motion skips whitespace rather than stopping in it, even though
// GetWordAtIndex reports whitespace runs as selectable.
func TestMoveCursorWordSkipsWhitespace(t *testing.T) {
	l := wordLayout("hello   world")
	if got := l.MoveCursorWordRight(0); got != 8 {
		t.Errorf("MoveCursorWordRight(0) = %d, want 8 (start of \"world\")", got)
	}
	if got := l.MoveCursorWordLeft(13); got != 8 {
		t.Errorf("MoveCursorWordLeft(13) = %d, want 8", got)
	}
	if got := l.MoveCursorWordLeft(8); got != 0 {
		t.Errorf("MoveCursorWordLeft(8) = %d, want 0", got)
	}
}

func TestMoveCursorWordJapanese(t *testing.T) {
	// 日本語 | の | テキスト at byte offsets 0, 9, 12.
	l := wordLayout("日本語のテキスト")
	if got := l.MoveCursorWordRight(0); got != 9 {
		t.Errorf("MoveCursorWordRight(0) = %d, want 9", got)
	}
	if got := l.MoveCursorWordRight(9); got != 12 {
		t.Errorf("MoveCursorWordRight(9) = %d, want 12", got)
	}
	if got := l.MoveCursorWordLeft(24); got != 12 {
		t.Errorf("MoveCursorWordLeft(24) = %d, want 12", got)
	}
}
