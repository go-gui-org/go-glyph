//go:build android || linux || darwin

package glyph

import "testing"

// ---------------------------------------------------------------------------
// buildByteToRuneIndexSlice
// ---------------------------------------------------------------------------

func TestBuildByteToRuneIndexSlice_NonRuneStartIsNegOne(t *testing.T) {
	// "é" = U+00E9: 2 UTF-8 bytes. Byte 1 is not a rune start.
	m := buildByteToRuneIndexSlice("é")
	if len(m) != 3 {
		t.Fatalf("len=%d, want 3", len(m))
	}
	if m[0] != 0 {
		t.Errorf("m[0]=%d, want 0", m[0])
	}
	if m[1] != -1 {
		t.Errorf("m[1]=%d, want -1 (continuation byte)", m[1])
	}
	if m[2] != 1 {
		t.Errorf("m[2]=%d, want 1 (sentinel = rune count)", m[2])
	}
}

func TestBuildByteToRuneIndexSlice_SentinelEqualsRuneCount(t *testing.T) {
	cases := []struct {
		s     string
		runes int
	}{
		{"", 0},
		{"A", 1},
		{"é", 1},
		{"😀", 1},
		{"a😀b", 3},
		{"Hello", 5},
	}
	for _, tc := range cases {
		m := buildByteToRuneIndexSlice(tc.s)
		if last := m[len(m)-1]; last != tc.runes {
			t.Errorf("%q: sentinel=%d, want %d", tc.s, last, tc.runes)
		}
	}
}

func TestBuildByteToRuneIndexSlice_SurrogatePlane(t *testing.T) {
	// 😀 = U+1F600: 4 UTF-8 bytes, 1 rune. Bytes 1-3 must be -1.
	m := buildByteToRuneIndexSlice("😀")
	if len(m) != 5 {
		t.Fatalf("len=%d, want 5", len(m))
	}
	if m[0] != 0 {
		t.Errorf("m[0]=%d, want 0", m[0])
	}
	for i := 1; i <= 3; i++ {
		if m[i] != -1 {
			t.Errorf("m[%d]=%d, want -1", i, m[i])
		}
	}
	if m[4] != 1 {
		t.Errorf("m[4]=%d, want 1 (rune count)", m[4])
	}
}

// ---------------------------------------------------------------------------
// visualOrderForLine
// ---------------------------------------------------------------------------

func TestVisualOrderForLine_EmptyRange_ReturnsNil(t *testing.T) {
	chars := []charBidiInfo{{byteI: 0, byteL: 1}}
	if order := visualOrderForLine("a", chars, 0, 0); order != nil {
		t.Errorf("empty range: got %v, want nil", order)
	}
	if order := visualOrderForLine("a", chars, 1, 1); order != nil {
		t.Errorf("equal bounds: got %v, want nil", order)
	}
}

func TestVisualOrderForLine_BadBounds_ReturnsNil(t *testing.T) {
	chars := []charBidiInfo{{byteI: 0, byteL: 1}}
	if order := visualOrderForLine("a", chars, -1, 1); order != nil {
		t.Errorf("negative start: got %v, want nil", order)
	}
	if order := visualOrderForLine("a", chars, 0, 2); order != nil {
		t.Errorf("end > len(chars): got %v, want nil", order)
	}
}

func TestVisualOrderForLine_PurelyLTR_IdentityOrder(t *testing.T) {
	text := "ab"
	chars := []charBidiInfo{
		{byteI: 0, byteL: 1},
		{byteI: 1, byteL: 1},
	}
	order := visualOrderForLine(text, chars, 0, 2)
	if len(order) != 2 || order[0] != 0 || order[1] != 1 {
		t.Errorf("LTR order=%v, want [0 1]", order)
	}
}

func TestVisualOrderForLine_PurelyRTL_ReversedOrder(t *testing.T) {
	// "שב": two Hebrew chars (2 bytes each). RTL bidi reverses logical order.
	text := "שב"
	chars := []charBidiInfo{
		{byteI: 0, byteL: 2},
		{byteI: 2, byteL: 2},
	}
	order := visualOrderForLine(text, chars, 0, 2)
	if len(order) != 2 {
		t.Fatalf("RTL: len=%d, want 2", len(order))
	}
	if order[0] != 1 || order[1] != 0 {
		t.Errorf("RTL order=%v, want [1 0]", order)
	}
}
