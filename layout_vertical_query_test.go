//go:build android || linux || darwin || windows

package glyph

import "testing"

// verticalLayout lays out text as one top-to-bottom column.
func verticalLayout(t *testing.T, text string) Layout {
	t.Helper()
	cfg := reviewCfg
	cfg.Orientation = OrientationVertical
	l, err := newReviewContext(t).LayoutText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// The caret is a horizontal bar across the column at the top of the char.
func TestVerticalCursorPosIsHorizontalBar(t *testing.T) {
	text := "abcde"
	l := verticalLayout(t, text)
	col := l.Lines[0].Rect
	for _, cr := range l.CharRects {
		cp, ok := l.GetCursorPos(cr.Index)
		if !ok {
			t.Fatalf("GetCursorPos(%d) not ok", cr.Index)
		}
		if cp.X != col.X || cp.Y != cr.Rect.Y || cp.Width != col.Width ||
			cp.Height != 0 {
			t.Errorf("GetCursorPos(%d) = %+v, want X=%v Y=%v W=%v H=0",
				cr.Index, cp, col.X, cr.Rect.Y, col.Width)
		}
	}
	last := l.CharRects[len(l.CharRects)-1].Rect
	cp, ok := l.GetCursorPos(len(text))
	if !ok || cp.Y != last.Y+last.Height {
		t.Errorf("end caret = %+v ok=%v, want Y=%v", cp, ok, last.Y+last.Height)
	}
}

// A hard break has no CharRect but is still a caret position.
func TestVerticalCursorPosAtHardBreak(t *testing.T) {
	l := verticalLayout(t, "ab\ncd")
	cp, ok := l.GetCursorPos(2)
	if !ok {
		t.Fatal("GetCursorPos(2) not ok")
	}
	ri, found := l.CharRectByIndex[3]
	if !found {
		t.Fatal("no CharRect for index 3")
	}
	if cp.Y != l.CharRects[ri].Rect.Y {
		t.Errorf("Y = %v, want %v", cp.Y, l.CharRects[ri].Rect.Y)
	}
}

func TestVerticalSelectionRects(t *testing.T) {
	text := "abcde"
	l := verticalLayout(t, text)
	col := l.Lines[0].Rect
	rects := l.GetSelectionRects(1, 3)
	if len(rects) != 1 {
		t.Fatalf("got %d rects, want 1", len(rects))
	}
	r := rects[0]
	wantBottom := l.CharRects[2].Rect.Y + l.CharRects[2].Rect.Height
	if r.X != col.X || r.Width != col.Width || r.Y != l.CharRects[1].Rect.Y ||
		r.Y+r.Height != wantBottom {
		t.Errorf("rect = %+v, want Y=%v bottom=%v", r, l.CharRects[1].Rect.Y,
			wantBottom)
	}
	rects = l.GetSelectionRects(0, len(text))
	if len(rects) != 1 {
		t.Fatalf("full: got %d rects, want 1", len(rects))
	}
	last := l.CharRects[len(l.CharRects)-1].Rect
	if rects[0].Y != l.CharRects[0].Rect.Y ||
		rects[0].Y+rects[0].Height != last.Y+last.Height {
		t.Errorf("full rect = %+v", rects[0])
	}
}

func TestVerticalClosestOffsetUsesY(t *testing.T) {
	text := "abcde"
	l := verticalLayout(t, text)
	col := l.Lines[0].Rect
	for i, cr := range l.CharRects {
		if got := l.GetClosestOffset(col.X+1, cr.Rect.Y+1); got != cr.Index {
			t.Errorf("top of char %d: got %d, want %d", i, got, cr.Index)
		}
		want := len(text)
		if i+1 < len(l.CharRects) {
			want = l.CharRects[i+1].Index
		}
		got := l.GetClosestOffset(col.X+1, cr.Rect.Y+cr.Rect.Height-1)
		if got != want {
			t.Errorf("bottom of char %d: got %d, want %d", i, got, want)
		}
	}
	if got := l.GetClosestOffset(col.X+1, -50); got != 0 {
		t.Errorf("above: got %d, want 0", got)
	}
	if got := l.GetClosestOffset(col.X+1, l.Height+50); got != len(text) {
		t.Errorf("below: got %d, want %d", got, len(text))
	}
}

func TestHorizontalCursorPosWidthZero(t *testing.T) {
	l, err := newReviewContext(t).LayoutText("abc", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	cp, ok := l.GetCursorPos(1)
	if !ok || cp.Width != 0 || cp.Height <= 0 {
		t.Errorf("cp = %+v ok=%v", cp, ok)
	}
}

func TestDrawCompositionVerticalCaret(t *testing.T) {
	l := verticalLayout(t, "abcde")
	col := l.Lines[0].Rect
	backend := newRecordingBackend()
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("ab", 1)
	r.DrawComposition(l, 0, 0, &cs, Color{0, 0, 0, 255})
	want := Rect{X: col.X, Y: l.CharRects[1].Rect.Y, Width: col.Width, Height: 2}
	found := false
	for _, f := range backend.filledRects {
		if f.Dst == want {
			found = true
		}
		if f.Dst.Height == l.Height {
			t.Errorf("full-height rect drawn: %+v", f.Dst)
		}
	}
	if !found {
		t.Errorf("no caret rect %+v in %+v", want, backend.filledRects)
	}
}
