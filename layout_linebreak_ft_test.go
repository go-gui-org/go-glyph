//go:build linux || darwin || windows

package glyph

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// lineBreakTestLayout lays out text unwrapped to learn its natural width,
// then again wrapped at a fraction of that width, so the tests hold under
// any installed font (real CJK glyphs or .notdef tofu advances alike).
func lineBreakTestLayout(t *testing.T, ctx *Context, text string,
	mode WrapMode, divisor float32) Layout {
	t.Helper()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 20"}}
	unwrapped, err := ctx.LayoutText(text, cfg)
	if err != nil {
		t.Fatalf("LayoutText (unwrapped): %v", err)
	}
	if unwrapped.Width <= 0 {
		t.Fatalf("unwrapped layout width = %f, want > 0", unwrapped.Width)
	}

	cfg.Block = BlockStyle{Width: unwrapped.Width / divisor, Wrap: mode}
	wrapped, err := ctx.LayoutText(text, cfg)
	if err != nil {
		t.Fatalf("LayoutText (wrapped): %v", err)
	}
	return wrapped
}

// checkLineBoundaries asserts every line starts on a UTF-8 rune boundary —
// an off-by-one in the UAX #14 byte-offset -> char-index mapping would
// produce a line starting mid-cluster.
func checkLineBoundaries(t *testing.T, l Layout) {
	t.Helper()
	for i, ln := range l.Lines {
		if ln.StartIndex < 0 || ln.StartIndex > len(l.Text) {
			t.Fatalf("line %d StartIndex %d out of range [0,%d]",
				i, ln.StartIndex, len(l.Text))
		}
		if ln.StartIndex < len(l.Text) &&
			!utf8.RuneStart(l.Text[ln.StartIndex]) {
			t.Errorf("line %d starts mid-rune at byte %d", i, ln.StartIndex)
		}
		if ln.StartIndex+ln.Length > len(l.Text) {
			t.Errorf("line %d [%d,+%d) exceeds text len %d",
				i, ln.StartIndex, ln.Length, len(l.Text))
		}
	}
}

// TestLayoutTextCJKWrapWord_BreaksWithoutSpaces verifies the UAX #14
// pre-pass supplies break opportunities for unspaced CJK text: WrapWord has
// no spaces to break at, so wrapping happens only if canBreakBefore marked
// inter-ideograph positions. A regression that leaves the table empty
// yields a single overflowing line.
func TestLayoutTextCJKWrapWord_BreaksWithoutSpaces(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	text := "日本語のテキストは分かち書きしない"
	l := lineBreakTestLayout(t, ctx, text, WrapWord, 3)

	if len(l.Lines) < 2 {
		t.Fatalf("unspaced CJK with WrapWord produced %d line(s), want >= 2",
			len(l.Lines))
	}
	checkLineBoundaries(t, l)

	// CJK breaks drop no characters (unlike space breaks), so the line
	// spans must reassemble the full text.
	var sb strings.Builder
	for _, ln := range l.Lines {
		sb.WriteString(l.Text[ln.StartIndex : ln.StartIndex+ln.Length])
	}
	if sb.String() != text {
		t.Errorf("lines reassemble to %q, want %q", sb.String(), text)
	}
}

// TestLayoutTextCJKWrapWord_NoBreakBeforeClosingPunct verifies UAX #14
// semantics survive the pre-pass rewrite: a closing punctuation like
// U+3002 IDEOGRAPHIC FULL STOP (line-break class CL) must not begin a
// line under WrapWord, which breaks only at marked opportunities.
func TestLayoutTextCJKWrapWord_NoBreakBeforeClosingPunct(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	text := "ああああ。ああああ。ああああ。ああああ。"
	l := lineBreakTestLayout(t, ctx, text, WrapWord, 3)

	if len(l.Lines) < 2 {
		t.Fatalf("got %d line(s), want >= 2 (assertion otherwise vacuous)",
			len(l.Lines))
	}
	checkLineBoundaries(t, l)
	for i, ln := range l.Lines {
		if i == 0 || ln.Length == 0 {
			continue
		}
		lineText := l.Text[ln.StartIndex : ln.StartIndex+ln.Length]
		if strings.HasPrefix(lineText, "。") {
			t.Errorf("line %d starts with closing punctuation: %q",
				i, lineText)
		}
	}
}

// TestLayoutTextCJKWrapWordChar_Wraps covers the WrapWordChar mode over
// unspaced CJK: UAX #14 opportunities wrap first, char-splitting only as
// a last resort; either way the text must occupy multiple lines.
func TestLayoutTextCJKWrapWordChar_Wraps(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	text := "中文文本没有空格需要按字断行"
	l := lineBreakTestLayout(t, ctx, text, WrapWordChar, 3)

	if len(l.Lines) < 2 {
		t.Fatalf("unspaced CJK with WrapWordChar produced %d line(s), want >= 2",
			len(l.Lines))
	}
	checkLineBoundaries(t, l)
}

// TestLayoutTextMixedLatinCJKWrapWord_Wraps exercises the merge over text
// where space breaks and UAX #14 breaks interleave.
func TestLayoutTextMixedLatinCJKWrapWord_Wraps(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	text := "abc 日本語のテキスト処理 def"
	l := lineBreakTestLayout(t, ctx, text, WrapWord, 3)

	if len(l.Lines) < 2 {
		t.Fatalf("mixed Latin/CJK with WrapWord produced %d line(s), want >= 2",
			len(l.Lines))
	}
	checkLineBoundaries(t, l)
}

// TestLayoutTextCJKSingleCluster_NoBreakPass covers the len(chars) <= 1
// guard: a single ideograph at a tiny wrap width must lay out as one line
// without entering the break pre-pass.
func TestLayoutTextCJKSingleCluster_NoBreakPass(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	l, err := ctx.LayoutText("日", TextConfig{
		Style: TextStyle{FontName: "Sans 20"},
		Block: BlockStyle{Width: 1, Wrap: WrapWordChar},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Lines) != 1 {
		t.Errorf("single cluster produced %d lines, want 1", len(l.Lines))
	}
	checkLineBoundaries(t, l)
}
