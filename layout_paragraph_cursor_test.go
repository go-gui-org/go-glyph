package glyph

import "testing"

// paragraphLayout builds a layout with two short paragraphs separated by
// an empty line ("\n\n"), returning the layout and the byte indices of
// the interesting caret positions.
func paragraphLayout(t *testing.T) (Layout, paragraphPos) {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	// "alpha beta"(0-9) \n(10) "gamma delta"(11-21) \n(22) \n(23)
	// "epsilon zeta"(24-35) \n(36) "eta theta"(37-45)
	text := "alpha beta\ngamma delta\n\nepsilon zeta\neta theta"
	l, err := ctx.LayoutText(text, TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	return l, paragraphPos{
		lastCharPara:  21, // "a" of "delta"
		paraEnd:       22, // first \n after "delta"
		emptyLine:     23, // second \n (the empty line)
		nextParaStart: 24,
	}
}

type paragraphPos struct {
	lastCharPara, paraEnd, emptyLine, nextParaStart int
}

// TestNewlineBytesAreCaretStops verifies that hard line breaks are valid
// cursor positions: the end of a paragraph and the empty line between
// paragraphs must both be reachable.
func TestNewlineBytesAreCaretStops(t *testing.T) {
	l, p := paragraphLayout(t)
	valid := make(map[int]bool)
	for _, pos := range l.GetValidCursorPositions() {
		valid[pos] = true
	}
	for _, want := range []int{p.paraEnd, p.emptyLine} {
		if !valid[want] {
			t.Errorf("byte %d (newline) is not a valid cursor position", want)
		}
	}
	// Cursor geometry must exist for both newline positions: end of the
	// paragraph's last line, and the start of the empty line.
	for byteIdx, wantX := range map[int]float32{
		p.paraEnd:   l.Lines[1].Rect.X + l.Lines[1].Rect.Width,
		p.emptyLine: l.Lines[2].Rect.X,
	} {
		cp, ok := l.GetCursorPos(byteIdx)
		if !ok {
			t.Errorf("GetCursorPos(%d) = ok=false, want geometry", byteIdx)
			continue
		}
		if cp.X != wantX {
			t.Errorf("GetCursorPos(%d).X = %v, want %v", byteIdx, cp.X, wantX)
		}
	}
}

// TestParagraphRightArrowStepsLines verifies that moving right from the
// last character of a paragraph visits the end-of-paragraph position and
// the empty line before the next paragraph, instead of skipping straight
// to the next paragraph.
func TestParagraphRightArrowStepsLines(t *testing.T) {
	l, p := paragraphLayout(t)

	got1 := l.MoveCursorRight(p.lastCharPara)
	if got1 != p.paraEnd {
		t.Errorf("MoveCursorRight(last char) = %d, want %d (end of paragraph)",
			got1, p.paraEnd)
	}
	got2 := l.MoveCursorRight(got1)
	if got2 != p.emptyLine {
		t.Errorf("MoveCursorRight(end of paragraph) = %d, want %d (empty line)",
			got2, p.emptyLine)
	}
	got3 := l.MoveCursorRight(got2)
	if got3 != p.nextParaStart {
		t.Errorf("MoveCursorRight(empty line) = %d, want %d (next paragraph)",
			got3, p.nextParaStart)
	}

	// Mirror back: left arrow steps the same positions in reverse.
	back := l.MoveCursorLeft(got3)
	if back != p.emptyLine {
		t.Errorf("MoveCursorLeft(next paragraph) = %d, want %d (empty line)",
			back, p.emptyLine)
	}
}

// TestParagraphDownArrowLandsOnEmptyLine verifies that pressing Down from
// the last line of a paragraph moves to the empty line (the next visual
// line), and Down again reaches the next paragraph. Up mirrors this.
func TestParagraphDownArrowLandsOnEmptyLine(t *testing.T) {
	l, p := paragraphLayout(t)

	down := l.MoveCursorDown(p.lastCharPara, -1)
	if down != p.emptyLine {
		t.Errorf("MoveCursorDown(last line of paragraph) = %d, want %d (empty line)",
			down, p.emptyLine)
	}
	down2 := l.MoveCursorDown(down, -1)
	if down2 != p.nextParaStart {
		t.Errorf("MoveCursorDown(empty line) = %d, want %d (next paragraph)",
			down2, p.nextParaStart)
	}

	up := l.MoveCursorUp(down2, -1)
	if up != p.emptyLine {
		t.Errorf("MoveCursorUp(next paragraph) = %d, want %d (empty line)",
			up, p.emptyLine)
	}
}

// TestParagraphClickPastEnd verifies that clicking past the end of a
// paragraph's last line places the cursor at the paragraph end, not on
// its final character.
func TestParagraphClickPastEnd(t *testing.T) {
	l, p := paragraphLayout(t)

	// Click far to the right of the paragraph's last line.
	y := l.Lines[1].Rect.Y + l.Lines[1].Rect.Height/2
	idx := l.GetClosestOffset(10000, y)
	if idx != p.paraEnd {
		t.Errorf("GetClosestOffset(past paragraph end) = %d, want %d",
			idx, p.paraEnd)
	}

	// Clicking on the empty line lands at its start.
	ey := l.Lines[2].Rect.Y + l.Lines[2].Rect.Height/2
	idx2 := l.GetClosestOffset(50, ey)
	if idx2 != p.emptyLine {
		t.Errorf("GetClosestOffset(empty line) = %d, want %d",
			idx2, p.emptyLine)
	}
}

// TestParagraphDeleteKeepsNewlines verifies that grapheme deletes around
// a paragraph break remove exactly one cluster, not the whole newline run.
func TestParagraphDeleteKeepsNewlines(t *testing.T) {
	l, p := paragraphLayout(t)

	// Backspace at the paragraph end removes only the final character.
	res := DeleteBackward(l.Text, l, p.paraEnd)
	if res.DeletedText != "a" {
		t.Errorf("DeleteBackward(para end) deleted %q, want %q", res.DeletedText, "a")
	}

	// Delete at the paragraph end removes only the first newline.
	res2 := DeleteForward(l.Text, l, p.paraEnd)
	if res2.DeletedText != "\n" {
		t.Errorf("DeleteForward(para end) deleted %q, want %q", res2.DeletedText, "\n")
	}
}

// TestNewlineCursorEdgeCases covers boundary texts around the newline
// caret stops: a leading newline (empty first line), a trailing newline
// (end-of-text stop), a single newline (two-step line crossing), and an
// all-newline input (no panic, monotonic stepping).
func TestNewlineCursorEdgeCases(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout := func(text string) Layout {
		l, err := ctx.LayoutText(text, TextConfig{
			Style: TextStyle{FontName: "Sans 20"},
		})
		if err != nil {
			t.Fatalf("LayoutText: %v", err)
		}
		return l
	}

	t.Run("leading_newline", func(t *testing.T) {
		// "\n\nabc": bytes 0 and 1 are newlines; the empty lines before
		// the text must be valid, distinct caret positions.
		l := layout("\n\nabc")
		for _, pos := range []int{0, 1} {
			if _, ok := l.GetCursorPos(pos); !ok {
				t.Errorf("GetCursorPos(%d) = ok=false, want geometry", pos)
			}
		}
		if got := l.MoveCursorRight(0); got != 1 {
			t.Errorf("MoveCursorRight(0) = %d, want 1", got)
		}
		if got := l.MoveCursorRight(1); got != 2 {
			t.Errorf("MoveCursorRight(1) = %d, want 2 (first text char)", got)
		}
	})

	t.Run("trailing_newline", func(t *testing.T) {
		// "abc\n": the newline at byte 3 is the line end, and 4 is
		// end-of-text. Right arrow must stop on both.
		l := layout("abc\n")
		if got := l.MoveCursorRight(2); got != 3 {
			t.Errorf("MoveCursorRight(2) = %d, want 3 (line end)", got)
		}
		if got := l.MoveCursorRight(3); got != 4 {
			t.Errorf("MoveCursorRight(3) = %d, want 4 (end of text)", got)
		}
		if got := l.MoveCursorRight(4); got != 4 {
			t.Errorf("MoveCursorRight(4) = %d, want 4 (clamped)", got)
		}
	})

	t.Run("single_newline", func(t *testing.T) {
		// "a\nb": crossing the line break visits the newline position
		// before the next line's first character.
		l := layout("a\nb")
		if got := l.MoveCursorRight(0); got != 1 {
			t.Errorf("MoveCursorRight(0) = %d, want 1 (newline)", got)
		}
		if got := l.MoveCursorRight(1); got != 2 {
			t.Errorf("MoveCursorRight(1) = %d, want 2 (next line)", got)
		}
		if got := l.MoveCursorLeft(2); got != 1 {
			t.Errorf("MoveCursorLeft(2) = %d, want 1 (newline)", got)
		}
	})

	t.Run("all_newlines", func(t *testing.T) {
		// Every byte is a caret stop; stepping must be monotonic and
		// never exceed the text length.
		l := layout("\n\n\n")
		pos := 0
		for want := 1; want <= 3; want++ {
			pos = l.MoveCursorRight(pos)
			if pos != want {
				t.Errorf("step %d: MoveCursorRight = %d, want %d", want, pos, want)
			}
		}
		if pos != l.MoveCursorRight(pos) {
			t.Errorf("MoveCursorRight(%d) should clamp at end of text", pos)
		}
	})
}
