//go:build android || linux || darwin || windows

package glyph

import (
	"strings"
	"testing"
	"unsafe"

	xbidi "golang.org/x/text/unicode/bidi"
)

func newReviewContext(t *testing.T) *Context {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	t.Cleanup(ctx.Free)
	return ctx
}

var reviewCfg = TextConfig{Style: TextStyle{FontName: "Sans 16"}}

// A word-wrapped line must be exactly as wide as its own text.
func TestLayoutText_WrappedLineWidth(t *testing.T) {
	ctx := newReviewContext(t)
	hello, err := ctx.LayoutText("hello", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := ctx.LayoutText("hello world", reviewCfg)
	cfg := reviewCfg
	cfg.Block = BlockStyle{Width: full.Width - 1, Wrap: WrapWord}
	l, _ := ctx.LayoutText("hello world", cfg)
	if len(l.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(l.Lines))
	}
	if d := l.Lines[0].Rect.Width - hello.Width; d > 0.01 || d < -0.01 {
		t.Errorf("line 0 width = %v, want %v", l.Lines[0].Rect.Width, hello.Width)
	}
}

// Mutation helpers must not panic on a cursor outside the text or on a
// layout built for a longer text.
func TestMutations_OutOfRangeCursorNoPanic(t *testing.T) {
	ctx := newReviewContext(t)
	l, _ := ctx.LayoutText("abc def", reviewCfg)
	calls := map[string]func() MutationResult{
		"DeleteForward-neg":       func() MutationResult { return DeleteForward("abc def", l, -2) },
		"DeleteForward-past":      func() MutationResult { return DeleteForward("abc", l, 9) },
		"DeleteToWordBoundary":    func() MutationResult { return DeleteToWordBoundary("abc", l, 7) },
		"DeleteToWordBoundaryNeg": func() MutationResult { return DeleteToWordBoundary("abc", l, -1) },
		"DeleteToWordEnd":         func() MutationResult { return DeleteToWordEnd("abc", l, 2) },
		"DeleteToLineStart":       func() MutationResult { return DeleteToLineStart("abc", l, 7) },
		"DeleteToLineEnd":         func() MutationResult { return DeleteToLineEnd("abc", l, 1) },
		"DeleteBackward-stale":    func() MutationResult { return DeleteBackward("ab", l, 2) },
	}
	for name, f := range calls {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			r := f()
			if r.CursorPos < 0 || r.CursorPos > len(r.NewText)+len(r.DeletedText) {
				t.Errorf("cursor %d out of range for %q", r.CursorPos, r.NewText)
			}
		})
	}
}

// An empty run is legal in rich text; it contributes no text.
func TestLayoutRichText_EmptyRun(t *testing.T) {
	ctx := newReviewContext(t)
	l, err := ctx.LayoutRichText(RichText{Runs: []StyleRun{
		{Text: "a"}, {Text: ""}, {Text: "b"},
	}}, reviewCfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if l.Text != "ab" {
		t.Errorf("Text = %q, want %q", l.Text, "ab")
	}
	if _, err := ctx.LayoutRichText(RichText{Runs: []StyleRun{{Text: ""}}},
		reviewCfg); err != nil {
		t.Errorf("all-empty runs: %v", err)
	}
}

// The total rich text is bounded, not only each run.
func TestLayoutRichText_TotalLengthBounded(t *testing.T) {
	ctx := newReviewContext(t)
	run := StyleRun{Text: strings.Repeat("a", MaxTextLength)}
	n := MaxRichTextLength/MaxTextLength + 1
	runs := make([]StyleRun, n)
	for i := range runs {
		runs[i] = run
	}
	if _, err := ctx.LayoutRichText(RichText{Runs: runs}, reviewCfg); err == nil {
		t.Error("want error for rich text over MaxRichTextLength")
	}
}

// A click in the trailing half of a character puts the caret after it.
func TestGetClosestOffset_TrailingHalf(t *testing.T) {
	ctx := newReviewContext(t)
	l, _ := ctx.LayoutText("abc", reviewCfg)
	r, _ := l.GetCharRect(1)
	y := r.Y + r.Height/2
	if got := l.GetClosestOffset(r.X+r.Width*0.8, y); got != 2 {
		t.Errorf("80%% into b: got %d, want 2", got)
	}
	if got := l.GetClosestOffset(r.X+r.Width*0.2, y); got != 1 {
		t.Errorf("20%% into b: got %d, want 1", got)
	}
	if got := l.GetClosestOffset(l.Width+50, y); got != 3 {
		t.Errorf("past end: got %d, want 3", got)
	}
	if got := l.GetClosestOffset(-50, y); got != 0 {
		t.Errorf("before start: got %d, want 0", got)
	}
}

// The fallback cache must not keep the caller's whole text alive through a
// substring key.
func TestCacheFallback_ClonesKey(t *testing.T) {
	ctx := newReviewContext(t)
	text := "xyz" + strings.Repeat("q", 100)
	key := text[1:2]
	ctx.cacheFallback(key, fbResolution{})
	for k := range ctx.fallbackResolve {
		if k == "y" && unsafe.StringData(k) == unsafe.StringData(key) {
			t.Fatal("cache key shares memory with the source text")
		}
	}
	for _, k := range ctx.resolveOrder {
		if k == "y" && unsafe.StringData(k) == unsafe.StringData(key) {
			t.Fatal("resolveOrder shares memory with the source text")
		}
	}
}

// Letter spacing goes between visual units. A cluster absorbed into a
// ligature is not one, so the ligature must not get extra spacing inside it.
func TestLetterSpacing_SkipsLigatureInterior(t *testing.T) {
	ctx := newReviewContext(t)
	const spacing = 5
	for _, s := range []string{"لا", "ffi", "fi"} {
		plain, err := ctx.LayoutText(s, reviewCfg)
		if err != nil {
			continue
		}
		absorbed := map[int]bool{}
		for _, g := range plain.Glyphs {
			if g.GlyphID == 0 && !g.Shaped && g.XAdvance < 0.01 {
				absorbed[int(g.Index)] = true
			}
		}
		if len(absorbed) == 0 {
			continue
		}
		cfg := reviewCfg
		cfg.Style.LetterSpacing = spacing
		spaced, _ := ctx.LayoutText(s, cfg)
		// One gap after every cluster except the last and except those
		// whose next cluster is absorbed.
		clusters := segmentGraphemes(nil, s)
		gaps := 0
		for i := 0; i+1 < len(clusters); i++ {
			if !absorbed[clusters[i+1].byteI] {
				gaps++
			}
		}
		want := plain.Width + float32(gaps*spacing)
		if d := spaced.Width - want; d > 0.01 || d < -0.01 {
			t.Errorf("%q: width %v, want %v", s, spaced.Width, want)
		}
		return
	}
	t.Skip("no ligature formed on this host")
}

// Paragraph direction comes from the paragraph's first strong character,
// and every line of the paragraph uses it.
func TestParagraphDirection(t *testing.T) {
	cases := []struct {
		text string
		want xbidi.Direction
	}{
		{"abc", xbidi.LeftToRight},
		{"123 אבג", xbidi.RightToLeft},
		{"אבג abc", xbidi.RightToLeft},
		{"abc אבג", xbidi.LeftToRight},
		{"123", xbidi.LeftToRight},
	}
	for _, c := range cases {
		if got := paragraphDirection(c.text); got != c.want {
			t.Errorf("paragraphDirection(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestVisualOrder_ForcedBaseDirection(t *testing.T) {
	text := "abc אבג"
	clusters := segmentGraphemes(nil, text)
	chars := make([]charBidiInfo, len(clusters))
	for i, cl := range clusters {
		chars[i] = charBidiInfo{byteI: cl.byteI, byteL: cl.byteL}
	}
	eq := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	ltr, _ := visualOrderForLineDir(text, chars, 0, len(chars), xbidi.LeftToRight, nil)
	if want := []int{0, 1, 2, 3, 6, 5, 4}; !eq(ltr, want) {
		t.Errorf("LTR order = %v, want %v", ltr, want)
	}
	rtl, _ := visualOrderForLineDir(text, chars, 0, len(chars), xbidi.RightToLeft, nil)
	if want := []int{6, 5, 4, 3, 0, 1, 2}; !eq(rtl, want) {
		t.Errorf("RTL order = %v, want %v", rtl, want)
	}
}

// The caret before a right-to-left character sits on its right edge.
func TestGetCursorPos_RTLCharUsesRightEdge(t *testing.T) {
	ctx := newReviewContext(t)
	l, err := ctx.LayoutText("אבג", reviewCfg)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := l.GetCharRect(0)
	if !ok {
		t.Fatal("no rect for byte 0")
	}
	cp, ok := l.GetCursorPos(0)
	if !ok {
		t.Fatal("no cursor at 0")
	}
	if d := cp.X - (r.X + r.Width); d > 0.01 || d < -0.01 {
		t.Errorf("caret X = %v, want right edge %v", cp.X, r.X+r.Width)
	}
}

// Vertical rich text keeps each run's style: items split at run boundaries.
func TestLayoutRichText_VerticalKeepsRunStyles(t *testing.T) {
	ctx := newReviewContext(t)
	red := Color{R: 255, A: 255}
	blue := Color{B: 255, A: 255}
	cfg := reviewCfg
	cfg.Orientation = OrientationVertical
	l, err := ctx.LayoutRichText(RichText{Runs: []StyleRun{
		{Text: "ab", Style: TextStyle{Color: red}},
		{Text: "cd", Style: TextStyle{Color: blue}},
	}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(l.Items))
	}
	if l.Items[0].Color != red || l.Items[1].Color != blue {
		t.Errorf("colors = %v, %v", l.Items[0].Color, l.Items[1].Color)
	}
	if l.Items[1].Y <= l.Items[0].Y {
		t.Errorf("second item Y %v not below first %v", l.Items[1].Y, l.Items[0].Y)
	}
}

// The space a soft wrap consumes is still a caret stop. Without it, a
// forward delete before the space removed the space as well, and the word
// before the wrap had no end.
func TestLayoutText_WrapSpaceIsCaretStop(t *testing.T) {
	ctx := newReviewContext(t)
	text := "hello world"
	full, _ := ctx.LayoutText(text, reviewCfg)
	cfg := reviewCfg
	cfg.Block = BlockStyle{Width: full.Width - 1, Wrap: WrapWord}
	l, _ := ctx.LayoutText(text, cfg)
	if len(l.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(l.Lines))
	}
	if r := DeleteForward(text, l, 4); r.DeletedText != "o" {
		t.Errorf("DeleteForward at 4 deleted %q, want %q", r.DeletedText, "o")
	}
	if s, e := l.GetWordAtIndex(2); s != 0 || e != 5 {
		t.Errorf("GetWordAtIndex(2) = (%d,%d), want (0,5)", s, e)
	}
	if s, e := l.GetWordAtIndex(8); s != 6 || e != 11 {
		t.Errorf("GetWordAtIndex(8) = (%d,%d), want (6,11)", s, e)
	}
}

// Numbers after right-to-left text sit at a higher embedding level (UAX #9
// rule I1) and must keep their digit order inside the reversed RTL span.
// x/text's Order returns runs in logical order, so this needs rule L2.
func TestVisualOrder_NumbersInRTL(t *testing.T) {
	text := "abc אבג 123"
	clusters := segmentGraphemes(nil, text)
	chars := make([]charBidiInfo, len(clusters))
	for i, cl := range clusters {
		chars[i] = charBidiInfo{byteI: cl.byteI, byteL: cl.byteL}
	}
	got, _ := visualOrderForLineDir(text, chars, 0, len(chars), xbidi.LeftToRight, nil)
	want := []int{0, 1, 2, 3, 8, 9, 10, 7, 6, 5, 4}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
