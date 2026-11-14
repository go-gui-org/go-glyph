//go:build linux || darwin || windows

package glyph

import (
	"math"
	"reflect"
	"testing"
)

const indentTestText = "alpha beta gamma delta epsilon zeta eta theta iota kappa"

// indentLayout lays out text with the given indent and wrap width (0 width
// means no wrapping).
func indentLayout(t *testing.T, text string, ind, width float32) Layout {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()
	l, err := ctx.LayoutText(text, TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
		Block: BlockStyle{Wrap: WrapWord, Width: width, Indent: ind},
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// indentNear allows 1e-3 of float slack.
func indentNear(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestLayoutIndentPositive(t *testing.T) {
	l := indentLayout(t, indentTestText, 30, 200)
	if len(l.Lines) < 2 {
		t.Fatalf("got %d lines, want >= 2", len(l.Lines))
	}
	for i, ln := range l.Lines {
		want := float32(0)
		if i == 0 {
			want = 30
		}
		if !indentNear(ln.Rect.X, want) {
			t.Errorf("line %d Rect.X = %v, want %v", i, ln.Rect.X, want)
		}
		if ln.Rect.X+ln.Rect.Width > 200+1e-3 {
			t.Errorf("line %d right edge %v > 200", i, ln.Rect.X+ln.Rect.Width)
		}
	}
	if l.Width > 200+1e-3 {
		t.Errorf("Width = %v > 200", l.Width)
	}
	if len(l.CharRects) == 0 || l.CharRects[0].Rect.X < 30-1e-3 {
		t.Errorf("first CharRect X < 30: %+v", l.CharRects)
	}
}

func TestLayoutIndentHanging(t *testing.T) {
	l := indentLayout(t, indentTestText, -30, 200)
	if len(l.Lines) < 2 {
		t.Fatalf("got %d lines, want >= 2", len(l.Lines))
	}
	for i, ln := range l.Lines {
		want := float32(30)
		if i == 0 {
			want = 0
		}
		if !indentNear(ln.Rect.X, want) {
			t.Errorf("line %d Rect.X = %v, want %v", i, ln.Rect.X, want)
		}
		if ln.Rect.X+ln.Rect.Width > 200+1e-3 {
			t.Errorf("line %d right edge %v > 200", i, ln.Rect.X+ln.Rect.Width)
		}
	}
	for i, cr := range l.CharRects {
		if cr.Rect.X < -1e-3 {
			t.Errorf("CharRect %d X = %v < 0", i, cr.Rect.X)
		}
	}
}

func TestLayoutIndentCaretFollowsLine(t *testing.T) {
	l := indentLayout(t, indentTestText, -30, 200)
	if len(l.Lines) < 2 {
		t.Fatalf("got %d lines, want >= 2", len(l.Lines))
	}
	pos, ok := l.GetCursorPos(l.Lines[1].StartIndex)
	if !ok {
		t.Fatal("GetCursorPos failed")
	}
	if math.Abs(float64(pos.X-30)) > 1 {
		t.Errorf("caret X = %v, want ~30", pos.X)
	}
}

func TestLayoutIndentPerParagraph(t *testing.T) {
	l := indentLayout(t, "one\ntwo", 30, 0)
	if len(l.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(l.Lines))
	}
	for i, ln := range l.Lines {
		if !indentNear(ln.Rect.X, 30) {
			t.Errorf("line %d Rect.X = %v, want 30", i, ln.Rect.X)
		}
	}
}

func TestLayoutIndentZeroUnchanged(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()
	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
		Block: BlockStyle{Wrap: WrapWord, Width: 200},
	}
	a, err := ctx.LayoutText(indentTestText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Block.Indent = 0
	b, err := ctx.LayoutText(indentTestText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Lines, b.Lines) || a.Width != b.Width {
		t.Error("indent 0 layout differs")
	}
}
