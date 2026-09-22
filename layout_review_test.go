package glyph

import "testing"

// Backspace at the end of the text reports its range in old-text byte
// positions. Undo used to check that range against the new, shorter text
// and refused to undo.
func TestUndo_BackspaceAtEndOfText(t *testing.T) {
	um := NewUndoManager(10)
	r := MutationResult{
		NewText: "ab", DeletedText: "c", CursorPos: 2,
		RangeStart: 2, RangeEnd: 3,
	}
	um.RecordMutation(r, "", 3, 3)
	u := um.Undo(r.NewText)
	if u == nil {
		t.Fatal("Undo returned nil")
	}
	if u.Text != "abc" {
		t.Errorf("Undo text = %q, want %q", u.Text, "abc")
	}
}

// DeleteSelection must report the deleted range, so that Redo can delete it
// again. It used to report an empty range, and Redo did nothing.
func TestRedo_DeleteSelection(t *testing.T) {
	um := NewUndoManager(10)
	r := DeleteSelection("Hello World", 5, 11)
	if r.RangeStart != 5 || r.RangeEnd != 11 {
		t.Fatalf("range = [%d,%d), want [5,11)", r.RangeStart, r.RangeEnd)
	}
	um.RecordMutation(r, "", 5, 11)
	u := um.Undo(r.NewText)
	if u == nil || u.Text != "Hello World" {
		t.Fatalf("Undo = %+v", u)
	}
	rd := um.Redo(u.Text)
	if rd == nil || rd.Text != "Hello" {
		t.Fatalf("Redo = %+v, want text %q", rd, "Hello")
	}
}

// bidiSelectionLayout is a one-line layout for "abcd" whose visual order is
// a, b, d, c (c and d form a right-to-left run).
func bidiSelectionLayout() Layout {
	rect := func(x float32) Rect { return Rect{X: x, Y: 0, Width: 10, Height: 20} }
	l := Layout{
		Text: "abcd",
		CharRects: []CharRect{
			{Rect: rect(0), Index: 0},
			{Rect: rect(10), Index: 1},
			{Rect: rect(20), Index: 3},
			{Rect: rect(30), Index: 2},
		},
		CharRectByIndex: map[int]int{0: 0, 1: 1, 3: 2, 2: 3},
		Lines: []Line{{StartIndex: 0, Length: 4,
			Rect: Rect{Width: 40, Height: 20}, IsParagraphStart: true}},
	}
	return l
}

// A logical selection that is not visually contiguous must give one
// rectangle for each visual piece, not one rectangle over the gap.
func TestSelectionRects_BidiSplitsVisualPieces(t *testing.T) {
	l := bidiSelectionLayout()
	rects := l.GetSelectionRects(1, 3) // b (x 10) and c (x 30)
	if len(rects) != 2 {
		t.Fatalf("got %d rects %+v, want 2", len(rects), rects)
	}
	if rects[0].X != 10 || rects[0].Width != 10 ||
		rects[1].X != 30 || rects[1].Width != 10 {
		t.Errorf("rects = %+v, want x=10 w=10 and x=30 w=10", rects)
	}
}

func TestSelectionRects_ContiguousMerges(t *testing.T) {
	l := bidiSelectionLayout()
	rects := l.GetSelectionRects(0, 4)
	if len(rects) != 1 || rects[0].X != 0 || rects[0].Width != 40 {
		t.Errorf("rects = %+v, want one rect x=0 w=40", rects)
	}
}

// A combining mark after a space joins the space's grapheme cluster, so the
// word run that starts at the mark starts inside a cluster. The word start
// must move to the next caret stop. It used to be dropped while its word end
// was kept, so the start and end lists had different lengths and every word
// query returned an empty range.
func TestWordAttrs_StartInsideClusterSnapsForward(t *testing.T) {
	text := "foo \u0301bar" // f o o [space+U+0301] b a r
	// Caret stops: 0 1 2 3 6 7 8 and the end of text 9.
	stops := []int{0, 1, 2, 3, 6, 7, 8, 9}
	l := Layout{Text: text, LogAttrByIndex: map[int]int{}}
	for i, off := range stops {
		l.LogAttrs = append(l.LogAttrs, LogAttr{IsCursorPosition: true})
		l.LogAttrByIndex[off] = i
	}
	applyWordAttrs(text, l.LogAttrs, l.LogAttrByIndex)
	l.buildPositionCaches()

	if s, e := l.GetWordAtIndex(7); s != 6 || e != 9 {
		t.Errorf("GetWordAtIndex(7) = (%d,%d), want (6,9)", s, e)
	}
	if s, e := l.GetWordAtIndex(1); s != 0 || e != 3 {
		t.Errorf("GetWordAtIndex(1) = (%d,%d), want (0,3)", s, e)
	}
}

// A run that lies wholly inside one cluster (a ZWJ glued to a letter)
// collapses to nothing and must not add a start or an end of its own.
func TestWordAttrs_RunInsideClusterDropped(t *testing.T) {
	text := "ab\u200dcd" // a [b+ZWJ] c d
	stops := []int{0, 1, 5, 6, 7}
	l := Layout{Text: text, LogAttrByIndex: map[int]int{}}
	for i, off := range stops {
		l.LogAttrs = append(l.LogAttrs, LogAttr{IsCursorPosition: true})
		l.LogAttrByIndex[off] = i
	}
	applyWordAttrs(text, l.LogAttrs, l.LogAttrByIndex)
	l.buildPositionCaches()
	if len(l.wordStarts) != len(l.wordEnds) {
		t.Fatalf("starts %v ends %v differ in length", l.wordStarts, l.wordEnds)
	}
	for k := range l.wordStarts {
		if l.wordStarts[k] >= l.wordEnds[k] {
			t.Errorf("word %d = [%d,%d) is empty", k, l.wordStarts[k], l.wordEnds[k])
		}
	}
}
