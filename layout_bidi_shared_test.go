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
	// Pure LTR text (< U+0590) returns nil via the fast-path — no
	// reordering needed and the caller iterates sequentially without
	// allocating a slice.
	text := "ab"
	chars := []charBidiInfo{
		{byteI: 0, byteL: 1},
		{byteI: 1, byteL: 1},
	}
	if order := visualOrderForLine(text, chars, 0, 2); order != nil {
		t.Errorf("pure LTR: got %v, want nil (fast-path skip)", order)
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

// ---------------------------------------------------------------------------
// isPureLTR
// ---------------------------------------------------------------------------

func TestIsPureLTR_ASCII(t *testing.T) {
	if !isPureLTR("Hello, world! 123") {
		t.Error("ASCII text should be pure LTR")
	}
	if !isPureLTR("") {
		t.Error("empty string should be pure LTR")
	}
}

func TestIsPureLTR_BelowThreshold(t *testing.T) {
	// U+058F is Armenian dram, the last character below the U+0590
	// Hebrew threshold.
	if !isPureLTR(string([]rune{0x058F})) {
		t.Error("U+058F (below threshold) should be pure LTR")
	}
}

func TestIsPureLTR_AtThreshold(t *testing.T) {
	// U+0590 is Hebrew accent, the first RTL character.
	if isPureLTR(string([]rune{0x0590})) {
		t.Error("U+0590 (at threshold) should NOT be pure LTR")
	}
}

func TestIsPureLTR_Hebrew(t *testing.T) {
	if isPureLTR("שלום") {
		t.Error("Hebrew text should NOT be pure LTR")
	}
}

func TestIsPureLTR_Arabic(t *testing.T) {
	if isPureLTR("مرحبا") {
		t.Error("Arabic text should NOT be pure LTR")
	}
}

func TestIsPureLTR_Mixed_LTR_RTL(t *testing.T) {
	// English + Hebrew mixed: contains a Hebrew char so NOT pure LTR.
	if isPureLTR("hello שלום") {
		t.Error("mixed LTR+RTL should NOT be pure LTR")
	}
}

func TestIsPureLTR_BidiControl(t *testing.T) {
	// LRM (U+200E) and RLM (U+200F) are above the threshold and must
	// take the full path.
	if isPureLTR(string([]rune{0x200E})) {
		t.Error("LRM (U+200E) should NOT be pure LTR")
	}
	if isPureLTR(string([]rune{0x200F})) {
		t.Error("RLM (U+200F) should NOT be pure LTR")
	}
}

func TestIsPureLTR_CJK(t *testing.T) {
	// CJK characters (U+4E00–U+9FFF) are above U+0x0590. They are
	// LTR but take the full bidi path — the threshold catches only
	// the absence of RTL scripts, not the presence of LTR ones.
	// This is the safe (conservative) choice.
	if isPureLTR("日本語") {
		t.Error("CJK text should NOT be pure LTR (above threshold)")
	}
}

func TestIsPureLTR_Emoji(t *testing.T) {
	// Emoji (U+1F600) is above U+0590; must take the full bidi path.
	if isPureLTR("😀") {
		t.Error("emoji should NOT be pure LTR")
	}
}

func TestIsPureLTR_InvalidUTF8(t *testing.T) {
	// An incomplete surrogate byte (0x80) is not valid UTF-8. Go's range
	// replaces it with U+FFFD (>= 0x0590), so isPureLTR returns false.
	if isPureLTR(string([]byte{0x80})) {
		t.Error("invalid UTF-8 should NOT be pure LTR")
	}
}

// ---------------------------------------------------------------------------
// visualOrderForLine — fast-path integration
// ---------------------------------------------------------------------------

func TestVisualOrderForLine_PureLTR_ReturnsNil(t *testing.T) {
	text := "the quick brown fox"
	chars := make([]charBidiInfo, len(text))
	for i := range len(text) {
		chars[i] = charBidiInfo{byteI: i, byteL: 1}
	}
	if order := visualOrderForLine(text, chars, 0, len(chars)); order != nil {
		t.Errorf("pure LTR: got %v, want nil (fast-path)", order)
	}
}

func TestVisualOrderForLine_RTL_StillWorks(t *testing.T) {
	// Hebrew — must still produce a correct reversed order.
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

func TestVisualOrderForLine_EmbeddedRTL(t *testing.T) {
	// "aשb": English + Hebrew + English. Contains a Hebrew char so the
	// fast-path is skipped; full bidi reordering should produce the
	// correct visual order.
	text := "aשb"
	chars := []charBidiInfo{
		{byteI: 0, byteL: 1}, // "a"
		{byteI: 1, byteL: 2}, // "ש"
		{byteI: 3, byteL: 1}, // "b"
	}
	order := visualOrderForLine(text, chars, 0, 3)
	if len(order) != 3 {
		t.Fatalf("embedded RTL: len=%d, want 3", len(order))
	}
}

func TestVisualOrderForLine_BoundaryAtThreshold(t *testing.T) {
	// U+058F (Armenian dram) is the last character below U+0590.
	// Single char below threshold: fast-path → nil.
	below := string([]rune{0x058F})
	chars := []charBidiInfo{{byteI: 0, byteL: len(below)}}
	if order := visualOrderForLine(below, chars, 0, 1); order != nil {
		t.Errorf("U+058F (below threshold): got %v, want nil", order)
	}

	// U+0590 (Hebrew accent) is at the threshold: NOT pure LTR.
	at := string([]rune{0x0590})
	chars = []charBidiInfo{{byteI: 0, byteL: len(at)}}
	order := visualOrderForLine(at, chars, 0, 1)
	if order == nil {
		t.Error("U+0590 (at threshold): got nil, want non-nil (full path)")
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkVisualOrderForLine_PureASCII(b *testing.B) {
	text := "the quick brown fox jumps over the lazy dog"
	chars := make([]charBidiInfo, len(text))
	for i := range len(text) {
		chars[i] = charBidiInfo{byteI: i, byteL: 1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = visualOrderForLine(text, chars, 0, len(chars))
	}
}
