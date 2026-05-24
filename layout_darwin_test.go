//go:build darwin && !glyph_pango

package glyph

import "testing"

// ---------------------------------------------------------------------------
// buildUTF16ToByteSlice
// ---------------------------------------------------------------------------

func TestBuildUTF16ToByteSlice_Empty(t *testing.T) {
	m := buildUTF16ToByteSlice("")
	// Only the sentinel should be present.
	if len(m) != 1 || m[0] != 0 {
		t.Errorf("empty string: got %v, want [0]", m)
	}
}

func TestBuildUTF16ToByteSlice_ASCII(t *testing.T) {
	m := buildUTF16ToByteSlice("Hi")
	// 'H'→0, 'i'→1, sentinel→2
	want := []int{0, 1, 2}
	if !intsEqual(m, want) {
		t.Errorf("ASCII: got %v, want %v", m, want)
	}
}

func TestBuildUTF16ToByteSlice_BMP_Multibyte(t *testing.T) {
	// é = U+00E9: 2 UTF-8 bytes, 1 UTF-16 code unit.
	m := buildUTF16ToByteSlice("é")
	want := []int{0, 2}
	if !intsEqual(m, want) {
		t.Errorf("BMP multibyte: got %v, want %v", m, want)
	}
}

func TestBuildUTF16ToByteSlice_SurrogatePair_TwoEntriesSameOffset(t *testing.T) {
	// 😀 = U+1F600: 4 UTF-8 bytes, 2 UTF-16 code units (surrogate pair).
	// Both surrogate positions must map to the same byte offset (0).
	m := buildUTF16ToByteSlice("😀")
	want := []int{0, 0, 4}
	if !intsEqual(m, want) {
		t.Errorf("surrogate pair: got %v, want %v", m, want)
	}
}

func TestBuildUTF16ToByteSlice_Mixed_ASCII_And_Surrogate(t *testing.T) {
	// "a😀b": 'a'→0, 😀→(1,1), 'b'→5, sentinel→6
	m := buildUTF16ToByteSlice("a\U0001F600b")
	want := []int{0, 1, 1, 5, 6}
	if !intsEqual(m, want) {
		t.Errorf("mixed: got %v, want %v", m, want)
	}
}

func TestBuildUTF16ToByteSlice_SentinelEqualsStringLen(t *testing.T) {
	texts := []string{"", "A", "é", "😀", "a😀b", "Hello, 世界"}
	for _, text := range texts {
		m := buildUTF16ToByteSlice(text)
		if last := m[len(m)-1]; last != len(text) {
			t.Errorf("%q: sentinel=%d, want %d", text, last, len(text))
		}
	}
}

func intsEqual(a, b []int) bool {
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

// ---------------------------------------------------------------------------
// shapeTextClusters
// ---------------------------------------------------------------------------

func TestShapeTextClusters_EmptyText_ReturnsNil(t *testing.T) {
	font := newCTFont(TextStyle{FontName: "Sans 16"}, 1.0)
	defer font.close()
	if sc := shapeTextClusters(font, ""); sc != nil {
		t.Errorf("empty text: got %d clusters, want nil", len(sc))
	}
}

func TestShapeTextClusters_ZeroFont_ReturnsNil(t *testing.T) {
	if sc := shapeTextClusters(ctFont{}, "Hello"); sc != nil {
		t.Errorf("zero font: got %d clusters, want nil", len(sc))
	}
}

func TestShapeTextClusters_ASCII_CoversFullText(t *testing.T) {
	font := newCTFont(TextStyle{FontName: "Sans 16"}, 1.0)
	defer font.close()

	text := "Hello"
	sc := shapeTextClusters(font, text)
	if len(sc) == 0 {
		t.Fatal("no clusters returned for ASCII text")
	}

	covered := make([]bool, len(text))
	for _, cl := range sc {
		for b := cl.byteStart; b < cl.byteStart+cl.byteLen && b < len(text); b++ {
			covered[b] = true
		}
	}
	for i, ok := range covered {
		if !ok {
			t.Errorf("byte %d of %q not covered by any cluster", i, text)
		}
	}
}

func TestShapeTextClusters_NewlineGapFilling_AllBytesCovered(t *testing.T) {
	font := newCTFont(TextStyle{FontName: "Sans 16"}, 1.0)
	defer font.close()

	// CoreText emits no glyph for '\n'; gap-filling must cover it.
	text := "a\nb"
	sc := shapeTextClusters(font, text)
	if len(sc) == 0 {
		t.Fatal("no clusters returned")
	}

	covered := make([]bool, len(text))
	for _, cl := range sc {
		for b := cl.byteStart; b < cl.byteStart+cl.byteLen && b < len(text); b++ {
			covered[b] = true
		}
	}
	for i, ok := range covered {
		if !ok {
			t.Errorf("byte %d of %q not covered", i, text)
		}
	}
}

func TestShapeTextClusters_Emoji_CoversAllBytes(t *testing.T) {
	font := newCTFont(TextStyle{FontName: "Sans 16"}, 1.0)
	defer font.close()

	text := "a\U0001F600b" // 'a' + 😀 + 'b' = 6 bytes
	sc := shapeTextClusters(font, text)
	if len(sc) == 0 {
		t.Fatal("no clusters returned for emoji text")
	}

	covered := make([]bool, len(text))
	for _, cl := range sc {
		for b := cl.byteStart; b < cl.byteStart+cl.byteLen && b < len(text); b++ {
			covered[b] = true
		}
	}
	for i, ok := range covered {
		if !ok {
			t.Errorf("byte %d of %q not covered", i, text)
		}
	}
}

// ---------------------------------------------------------------------------
// buildLayout (via LayoutText) — Darwin-specific behaviour
// ---------------------------------------------------------------------------

func newDarwinTestContext(t *testing.T) *Context {
	t.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	return ctx
}

func TestLayoutDarwin_CarriageReturn_NotEmittedAsGlyph(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	lCR, err := ctx.LayoutText("a\rb", cfg)
	if err != nil {
		t.Fatalf("LayoutText a\\rb: %v", err)
	}
	lLF, err := ctx.LayoutText("a\nb", cfg)
	if err != nil {
		t.Fatalf("LayoutText a\\nb: %v", err)
	}
	// Both \r and \n must be skipped; glyph counts should match.
	if len(lCR.Glyphs) != len(lLF.Glyphs) {
		t.Errorf("\\r layout glyphs=%d, \\n layout glyphs=%d; want equal",
			len(lCR.Glyphs), len(lLF.Glyphs))
	}
	// Verify \r itself doesn't appear as a glyph character.
	for i, g := range lCR.Glyphs {
		ch := glyphText(lCR.Text, g)
		if ch == "\r" {
			t.Errorf("glyph[%d] is \\r — should have been skipped", i)
		}
	}
}

func TestLayoutDarwin_GlyphsHaveNonZeroGlyphID(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	// No overrides → shaping path taken; primary-font glyphs get non-zero IDs.
	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	l, err := ctx.LayoutText("Hello", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs in layout")
	}
	found := false
	for _, g := range l.Glyphs {
		if g.GlyphID != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("all GlyphIDs are zero — shaping path not propagating IDs")
	}
}

func TestLayoutDarwin_SimpleASCII_GlyphCountMatchesChars(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	l, err := ctx.LayoutText("ABC", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Glyphs) != 3 {
		t.Errorf("glyphs=%d, want 3", len(l.Glyphs))
	}
}
