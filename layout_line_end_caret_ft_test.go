//go:build android || linux || darwin || windows

package glyph

import "testing"

// lastCharEdge returns the trailing edge of the last char of line (in logical
// order): the left edge for a right-to-left char, the right edge for a
// left-to-right one. A hard line break at the end of the line is skipped.
func lastCharEdge(t *testing.T, l Layout, line Line) float32 {
	t.Helper()
	lo, hi := l.lineRectRange(line)
	best := -1
	for ri := lo; ri < hi; ri++ {
		idx := l.CharRects[ri].Index
		if hardLineBreakLenAt(l.Text, idx) > 0 {
			continue
		}
		if best < 0 || idx > l.CharRects[best].Index {
			best = ri
		}
	}
	if best < 0 {
		t.Fatalf("line %+v has no char rect", line)
	}
	r := l.CharRects[best].Rect
	if l.rectIsRTL(best) {
		return r.X
	}
	return r.X + r.Width
}

// The caret at the end of a line sits on the trailing edge of the last
// char of the line (issue 206). For a right-to-left line that is the left
// edge, not the right edge where the text starts.
func TestGetCursorPos_LineEndUsesTrailingEdge(t *testing.T) {
	ctx := newReviewContext(t)
	tests := []struct {
		text string
		// pureRTL: the whole line is right-to-left, so the end is X 0.
		// pureLTR: the end is the right edge of the line.
		pureRTL, pureLTR bool
	}{
		{text: "abc", pureLTR: true},
		{text: "אבג", pureRTL: true},
		{text: "אבג\nדהו", pureRTL: true},
		{text: "אבג abc"},
		{text: "abc אבג"},
	}
	for _, tt := range tests {
		l, err := ctx.LayoutText(tt.text, reviewCfg)
		if err != nil {
			t.Fatal(err)
		}
		line := l.Lines[0]
		start, ok := l.GetCursorPos(line.StartIndex)
		if !ok {
			t.Fatalf("%q: no caret at line start", tt.text)
		}
		end, ok := l.GetCursorPos(line.StartIndex + line.Length)
		if !ok {
			t.Fatalf("%q: no caret at line end", tt.text)
		}
		want := lastCharEdge(t, l, line)
		switch {
		case tt.pureRTL:
			want = line.Rect.X
		case tt.pureLTR:
			want = line.Rect.X + line.Rect.Width
		}
		if !near(end.X, want) {
			t.Errorf("%q: end X = %.2f, want %.2f", tt.text, end.X, want)
		}
		if near(start.X, end.X) {
			t.Errorf("%q: start and end caret both at X %.2f", tt.text, start.X)
		}
	}
}

// A soft wrap consumes the space it breaks at. The caret at that byte
// (answered by lineEndPos) must also use the trailing edge on an RTL line.
func TestGetCursorPos_RTLSoftWrapEndUsesLeftEdge(t *testing.T) {
	ctx := newReviewContext(t)
	first, err := ctx.LayoutText("אבג", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg := reviewCfg
	cfg.Block = BlockStyle{Width: first.Width + 2, Wrap: WrapWord}
	l, err := ctx.LayoutText("אבג דהו", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Lines) < 2 {
		t.Fatalf("got %d lines, want a soft wrap", len(l.Lines))
	}
	line := l.Lines[0]
	end, ok := l.GetCursorPos(line.StartIndex + line.Length)
	if !ok {
		t.Fatal("no caret at end of wrapped line")
	}
	if want := lastCharEdge(t, l, line); !near(end.X, want) {
		t.Errorf("end X = %.2f, want left edge of last char %.2f", end.X, want)
	}
	if near(end.X, line.Rect.X+line.Rect.Width) {
		t.Errorf("end X = %.2f is the right edge (line start of RTL text)", end.X)
	}
}

// LineEndCursorPos answers each line of a layout, and false for a line
// index out of range.
func TestLineEndCursorPos(t *testing.T) {
	ctx := newReviewContext(t)
	l, err := ctx.LayoutText("אבג\nדהו", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(l.Lines))
	}
	for i, line := range l.Lines {
		cp, ok := l.LineEndCursorPos(i)
		if !ok {
			t.Fatalf("line %d: not ok", i)
		}
		if !near(cp.X, line.Rect.X) {
			t.Errorf("line %d: X = %.2f, want left edge %.2f", i, cp.X, line.Rect.X)
		}
		if cp.Y != line.Rect.Y || cp.Height != line.Rect.Height {
			t.Errorf("line %d: Y/Height = %v/%v, want %v/%v",
				i, cp.Y, cp.Height, line.Rect.Y, line.Rect.Height)
		}
	}
	for _, i := range []int{-1, len(l.Lines)} {
		if _, ok := l.LineEndCursorPos(i); ok {
			t.Errorf("LineEndCursorPos(%d) ok, want false", i)
		}
	}
}

// Edge cases of LineEndCursorPos: an empty line, a CRLF break, a vertical
// layout, and a zero Layout.
func TestLineEndCursorPos_EdgeCases(t *testing.T) {
	ctx := newReviewContext(t)

	// The empty middle line has no char rect: the caret is at its rect.
	l, err := ctx.LayoutText("אבג\n\nדהו", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(l.Lines))
	}
	mid := l.Lines[1]
	if cp, ok := l.LineEndCursorPos(1); !ok || !near(cp.X, mid.Rect.X+mid.Rect.Width) {
		t.Errorf("empty line: X = %.2f ok=%v, want %.2f",
			cp.X, ok, mid.Rect.X+mid.Rect.Width)
	}

	// A CRLF break is not a visible char: the RTL line still ends left.
	l, err = ctx.LayoutText("אבג\r\nדהו", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	if cp, ok := l.LineEndCursorPos(0); !ok || !near(cp.X, l.Lines[0].Rect.X) {
		t.Errorf("CRLF line: X = %.2f ok=%v, want %.2f", cp.X, ok, l.Lines[0].Rect.X)
	}

	// Vertical: the caret is a horizontal bar, the same as GetCursorPos.
	cfg := reviewCfg
	cfg.Orientation = OrientationVertical
	l, err = ctx.LayoutText("abc", cfg)
	if err != nil {
		t.Fatal(err)
	}
	cp, ok := l.LineEndCursorPos(0)
	want, wantOK := l.GetCursorPos(len("abc"))
	if !ok || !wantOK || cp != want {
		t.Errorf("vertical: got %+v ok=%v, want %+v ok=%v", cp, ok, want, wantOK)
	}
	if cp.Height != 0 || cp.Width == 0 {
		t.Errorf("vertical: caret %+v is not a horizontal bar", cp)
	}

	// A zero Layout has no lines.
	var zero Layout
	if _, ok := zero.LineEndCursorPos(0); ok {
		t.Error("zero Layout: ok, want false")
	}
}
