//go:build android || linux || darwin || windows || (js && wasm)

package glyph

import "testing"

// wrapFixed runs wrapLines over s with every cluster (one byte each here) 10
// units wide, so expected line widths are easy to state.
func wrapFixed(s string, wrapWidth float64, mode WrapMode, canBreak []bool) []lineInfo {
	return wrapLines(nil, len(s), mode, wrapWidth,
		func(int) float64 { return 10 },
		func(i int) string { return s[i : i+1] },
		canBreak)
}

// A word wrap at a space must record the width of the text before the space.
// It used to record the width up to the overflowing cluster minus that
// cluster, which counted the space and the start of the next word.
func TestWrapLines_WordWrapWidthExcludesNextWord(t *testing.T) {
	lines := wrapFixed("aa bbbb", 45, WrapWord, nil)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if got := lines[0]; got.startChar != 0 || got.endChar != 2 || got.width != 20 {
		t.Errorf("line 0 = %+v, want {0 2 20}", got)
	}
	if got := lines[1]; got.startChar != 3 || got.endChar != 7 || got.width != 40 {
		t.Errorf("line 1 = %+v, want {3 7 40}", got)
	}
}

// The space itself can be the cluster that overflows. The line then ends at
// that space and its width is everything before it.
func TestWrapLines_OverflowOnSpace(t *testing.T) {
	lines := wrapFixed("aaaa b", 45, WrapWord, nil)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if got := lines[0]; got.endChar != 4 || got.width != 40 {
		t.Errorf("line 0 = %+v, want end 4 width 40", got)
	}
	if got := lines[1]; got.startChar != 5 || got.width != 10 {
		t.Errorf("line 1 = %+v, want start 5 width 10", got)
	}
}

// A UAX #14 opportunity (no space) splits the line before the opportunity.
func TestWrapLines_BreakOpportunity(t *testing.T) {
	canBreak := []bool{false, false, false, true, false, false}
	lines := wrapFixed("aaabbb", 45, WrapWord, canBreak)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if got := lines[0]; got.endChar != 3 || got.width != 30 {
		t.Errorf("line 0 = %+v, want end 3 width 30", got)
	}
	if got := lines[1]; got.startChar != 3 || got.width != 30 {
		t.Errorf("line 1 = %+v, want start 3 width 30", got)
	}
}

func TestWrapLines_HardNewline(t *testing.T) {
	lines := wrapFixed("ab\ncd", -1, WrapWord, nil)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if lines[0].endChar != 2 || lines[1].startChar != 3 || lines[1].width != 20 {
		t.Errorf("lines = %+v", lines)
	}
}

func TestLineBreakOpportunities(t *testing.T) {
	text := "ab cd"
	got := lineBreakOpportunities(nil, text, len(text), func(i int) int { return i })
	want := []bool{false, false, false, true, false}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("canBreak = %v, want %v", got, want)
		}
	}
}
