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

// TestCursorPosAtCharWrapBoundaryUsesNextLine guards the caret position at a
// soft wrap that consumes no space (WrapChar, CJK breaks). Line 0 ends at
// byte 3 and line 1 starts at byte 3. The movement functions treat that byte
// as the start of line 1, so GetCursorPos must report line 1's Y and Height
// instead of drawing the caret at the left edge of line 0.
func TestCursorPosAtCharWrapBoundaryUsesNextLine(t *testing.T) {
	l := Layout{
		Text: "abcdef",
		Lines: []Line{
			{StartIndex: 0, Length: 3, Rect: Rect{X: 0, Y: 0, Width: 30, Height: 20}},
			{StartIndex: 3, Length: 3, Rect: Rect{X: 0, Y: 20, Width: 30, Height: 20}},
		},
		CharRectByIndex: map[int]int{},
		LogAttrByIndex:  map[int]int{},
		Width:           30,
		Height:          40,
	}
	for i := 0; i < 6; i++ {
		y := float32(0)
		if i >= 3 {
			y = 20
		}
		l.CharRects = append(l.CharRects, CharRect{
			Rect:  Rect{X: float32(i%3) * 10, Y: y, Width: 10, Height: 20},
			Index: i,
		})
		l.CharRectByIndex[i] = i
	}
	for i := 0; i <= 6; i++ {
		l.LogAttrs = append(l.LogAttrs, LogAttr{IsCursorPosition: true})
		l.LogAttrByIndex[i] = i
	}

	cp, ok := l.GetCursorPos(3)
	if !ok {
		t.Fatal("GetCursorPos(3) not ok")
	}
	if cp.X != 0 || cp.Y != 20 || cp.Height != 20 {
		t.Errorf("GetCursorPos(3) = %+v, want X=0 Y=20 Height=20", cp)
	}
	cp, ok = l.GetCursorPos(6)
	if !ok || cp.X != 30 || cp.Y != 20 {
		t.Errorf("GetCursorPos(6) = %+v ok=%v, want X=30 Y=20", cp, ok)
	}
	cp, ok = l.GetCursorPos(0)
	if !ok || cp.Y != 0 {
		t.Errorf("GetCursorPos(0) = %+v ok=%v, want Y=0", cp, ok)
	}
}

// TestCursorPosAtCharWrapBoundaryRealLayout runs the same check against a
// real WrapChar layout: every line that starts at the byte where the
// previous one ended must put the caret on its own row.
func TestCursorPosAtCharWrapBoundaryRealLayout(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	l, err := ctx.LayoutText(strings.Repeat("x", 60), TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: 120, Wrap: WrapChar},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Lines) < 2 {
		t.Fatalf("got %d lines, want >= 2", len(l.Lines))
	}
	checked := 0
	for k := 1; k < len(l.Lines); k++ {
		prev, cur := l.Lines[k-1], l.Lines[k]
		t.Logf("line %d: start=%d len=%d y=%v", k, cur.StartIndex, cur.Length, cur.Rect.Y)
		if cur.StartIndex != prev.StartIndex+prev.Length {
			continue
		}
		checked++
		cp, ok := l.GetCursorPos(cur.StartIndex)
		if !ok {
			t.Fatalf("GetCursorPos(%d) not ok", cur.StartIndex)
		}
		if cp.Y != cur.Rect.Y {
			t.Errorf("line %d: caret Y=%v, want %v", k, cp.Y, cur.Rect.Y)
		}
	}
	if checked == 0 {
		t.Fatal("no line pair shares a boundary byte")
	}
}
