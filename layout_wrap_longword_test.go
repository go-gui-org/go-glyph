package glyph

import (
	"strings"
	"testing"
)

// longWordLayout wraps text at a width narrower than an unbreakable run
// of 200 characters, the shape a caret-navigation test needs.
func longWordLayout(t *testing.T, text string) Layout {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	l, err := ctx.LayoutText(text, TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: 300, Wrap: WrapWord},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	return l
}

// TestWrapLongWordNoEmptyLine guards the wrap pass against a break
// opportunity that sits at the line's own start. uniseg reports one
// directly after a hard newline, and taking it emitted a zero-length
// line whose StartIndex duplicated the following line's.
func TestWrapLongWordNoEmptyLine(t *testing.T) {
	text := "abc\n" + strings.Repeat("x", 200)
	l := longWordLayout(t, text)
	if len(l.Lines) != 2 {
		for i, ln := range l.Lines {
			t.Logf("line %d: start=%d len=%d", i, ln.StartIndex, ln.Length)
		}
		t.Fatalf("Lines = %d, want 2 (short line + overlong word)",
			len(l.Lines))
	}
	if got := l.Lines[1]; got.StartIndex != 4 || got.Length != 200 {
		t.Errorf("Lines[1] = {start %d, len %d}, want {4, 200}",
			got.StartIndex, got.Length)
	}
}

// TestMoveCursorUpPastLongWord walks the caret up from the line below an
// overlong unbreakable word to the top line. A duplicated zero-length
// line used to trap it: MoveCursorUp skipped that line on the way down
// the search loop, then landed back on it, so every further press
// returned the same index.
func TestMoveCursorUpPastLongWord(t *testing.T) {
	long := strings.Repeat("x", 200)
	text := "alpha beta\n\n" + long + "\ntail"
	l := longWordLayout(t, text)

	idx := len(text) // end of "tail"
	seen := []int{idx}
	for range 6 {
		next := l.MoveCursorUp(idx, -1)
		if next == idx {
			break
		}
		idx = next
		seen = append(seen, idx)
	}
	// alpha beta / empty / long word / tail = four lines, so the caret
	// must reach the first line rather than stalling below the word.
	if idx > len("alpha beta") {
		t.Errorf("MoveCursorUp stalled at %d (path %v); want a byte on the "+
			"first line (0..%d)", idx, seen, len("alpha beta"))
	}
}

// TestCursorPosAtSoftWrapLineEnd pins that the caret geometry exists for
// the byte a soft wrap consumed. That byte carries no log attr — no line
// contains it — but it is the index findClosestIndexInLine hands back
// for a click past the line's right edge and for vertical motion with a
// preferred x beyond the line, so refusing it left a caller with a caret
// index it could not draw.
func TestCursorPosAtSoftWrapLineEnd(t *testing.T) {
	text := strings.Repeat("word ", 30)
	l := longWordLayout(t, text)
	if len(l.Lines) < 3 {
		t.Fatalf("Lines = %d, want a wrapped run of at least 3", len(l.Lines))
	}
	line := l.Lines[1]
	end := line.StartIndex + line.Length
	if end >= l.Lines[2].StartIndex {
		t.Fatalf("line 1 end %d is not a wrap-consumed byte (line 2 starts %d)",
			end, l.Lines[2].StartIndex)
	}
	cp, ok := l.GetCursorPos(end)
	if !ok {
		t.Fatalf("GetCursorPos(%d) = ok=false, want the line's end geometry",
			end)
	}
	if want := line.Rect.X + line.Rect.Width; cp.X != want {
		t.Errorf("GetCursorPos(%d).X = %v, want %v (end of line 1)",
			end, cp.X, want)
	}
	if cp.Y != line.Rect.Y {
		t.Errorf("GetCursorPos(%d).Y = %v, want %v (line 1)",
			end, cp.Y, line.Rect.Y)
	}
}

// TestMoveCursorUpKeepsDrawableIndex walks the caret up out of an
// overlong unbreakable line, which pins the preferred x far to the
// right of every wrapped line, and pins that every index it stops on
// has caret geometry. Those stops are line ends, and a line end a soft
// wrap broke at is a byte no line contains: the caret vanished on the
// middle lines of a wrapped paragraph whenever the long line was
// present to drag the preferred x out there.
func TestMoveCursorUpKeepsDrawableIndex(t *testing.T) {
	para := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
		"Sed do eiusmod tempor incididunt ut labore et dolore magna " +
		"aliqua. Ut enim ad minim veniam, quis nostrud exercitation."
	text := para + "\n" + strings.Repeat("x", 200) + "\ntail line"
	l := longWordLayout(t, text)

	// The caret starts at the end of the overlong line, so the
	// preferred x it carries upward is that line's width.
	idx := len(para) + 1 + 200
	preferredX := float32(-1)
	if cp, ok := l.GetCursorPos(idx); ok {
		preferredX = cp.X
	}
	stops := 0
	for range len(l.Lines) + 1 {
		next := l.MoveCursorUp(idx, preferredX)
		if next == idx {
			break
		}
		idx = next
		stops++
		if _, ok := l.GetCursorPos(idx); !ok {
			t.Fatalf("MoveCursorUp stopped at %d, which has no caret "+
				"geometry", idx)
		}
	}
	if stops < 3 {
		t.Fatalf("walked %d lines, want the whole wrapped paragraph", stops)
	}
}
