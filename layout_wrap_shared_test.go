//go:build android || linux || darwin || windows || (js && wasm)

package glyph

import "testing"

// wrapFixed runs wrapLines over s with every cluster (one byte each here) 10
// units wide, so expected line widths are easy to state.
func wrapFixed(s string, wrapWidth, indent float64, mode WrapMode, canBreak []bool) []lineInfo {
	return wrapLines(nil, len(s), mode, wrapWidth, indent,
		func(int) float64 { return 10 },
		func(i int) string { return s[i : i+1] },
		canBreak)
}

// A word wrap at a space must record the width of the text before the space.
// It used to record the width up to the overflowing cluster minus that
// cluster, which counted the space and the start of the next word.
func TestWrapLines_WordWrapWidthExcludesNextWord(t *testing.T) {
	lines := wrapFixed("aa bbbb", 45, 0, WrapWord, nil)
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
	lines := wrapFixed("aaaa b", 45, 0, WrapWord, nil)
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
	lines := wrapFixed("aaabbb", 45, 0, WrapWord, canBreak)
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
	lines := wrapFixed("ab\ncd", -1, 0, WrapWord, nil)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	if lines[0].endChar != 2 || lines[1].startChar != 3 || lines[1].width != 20 {
		t.Errorf("lines = %+v", lines)
	}
}

func TestWrapLines_HardLineBreakForms(t *testing.T) {
	for name, clusters := range map[string][]string{
		"lf":   {"a", "\n", "b"},
		"cr":   {"a", "\r", "b"},
		"crlf": {"a", "\r\n", "b"},
	} {
		t.Run(name, func(t *testing.T) {
			lines := wrapLines(nil, len(clusters), WrapNone, -1, 0,
				func(int) float64 { return 10 },
				func(i int) string { return clusters[i] }, nil)
			if len(lines) != 2 {
				t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
			}
			if got := lines[0]; got.startChar != 0 || got.endChar != 1 || got.width != 10 {
				t.Errorf("line 0 = %+v, want {0 1 10}", got)
			}
			if got := lines[1]; got.startChar != 2 || got.endChar != 3 || got.width != 10 {
				t.Errorf("line 1 = %+v, want {2 3 10}", got)
			}
		})
	}
}

func TestHardLineBreakAttrs(t *testing.T) {
	text := "a\r\nb\rc\nd"
	attrs := make([]LogAttr, 0, 3)
	byIndex := make(map[int]int)
	attrs = appendHardLineBreakAttrs(text, attrs, byIndex)

	if len(attrs) != 3 {
		t.Fatalf("got %d attrs, want 3", len(attrs))
	}
	for _, index := range []int{1, 4, 6} {
		ai, ok := byIndex[index]
		if !ok || !attrs[ai].IsCursorPosition || !attrs[ai].IsLineBreak {
			t.Errorf("byte %d = (%d, %v), want cursor line-break attr", index, ai, ok)
		}
	}
	if _, ok := byIndex[2]; ok {
		t.Error("CRLF interior byte unexpectedly has a caret attr")
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

// Indent shrinks the wrap width of the offset lines (Pango semantics) and
// each line records whether it starts a paragraph.
func TestWrapLines_Indent(t *testing.T) {
	tests := []struct {
		indent float64
		want   []lineInfo
	}{
		{0, []lineInfo{{0, 3, 30, true}, {4, 7, 30, false}}},
		{20, []lineInfo{{0, 1, 10, true}, {2, 5, 30, false}, {6, 7, 10, false}}},
		{-20, []lineInfo{{0, 3, 30, true}, {4, 5, 10, false}, {6, 7, 10, false}}},
	}
	for _, tc := range tests {
		got := wrapFixed("a b c d", 45, tc.indent, WrapWord, nil)
		if len(got) != len(tc.want) {
			t.Errorf("indent %v: got %+v, want %+v", tc.indent, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("indent %v: line %d = %+v, want %+v",
					tc.indent, i, got[i], tc.want[i])
			}
		}
	}
}

// The line after a hard break starts a paragraph.
func TestWrapLines_IndentHardBreakParaStart(t *testing.T) {
	got := wrapFixed("a b\nc d", 45, 20, WrapWord, nil)
	var found bool
	for _, li := range got {
		if li.startChar == 4 {
			found = true
			if !li.paraStart {
				t.Errorf("line at 4 paraStart = false: %+v", got)
			}
		}
	}
	if !found {
		t.Errorf("no line starts at cluster 4: %+v", got)
	}
}
