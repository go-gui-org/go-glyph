//go:build darwin && !glyph_pango

package glyph

import (
	"strings"
	"testing"
)

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

func TestLayoutDarwin_RTL_VisualOrder(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	// Two Hebrew RTL words separated by a space should render in visual RTL
	// order, so the first logical word starts to the visual right of the
	// second logical word.
	const text = "שלום עולם"
	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	l, err := ctx.LayoutText(text, cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.CharRects) < 3 {
		t.Fatalf("too few CharRects (%d)", len(l.CharRects))
	}

	var xFirstWord, xSecondWord float32
	firstFound, secondFound := false, false
	for _, cr := range l.CharRects {
		switch cr.Index {
		case 0:
			xFirstWord = cr.Rect.X
			firstFound = true
		case len("שלום "):
			xSecondWord = cr.Rect.X
			secondFound = true
		}
	}
	if !firstFound || !secondFound {
		t.Fatalf("could not find expected CharRects (first=%v second=%v)", firstFound, secondFound)
	}
	if xFirstWord <= xSecondWord {
		t.Errorf("visual RTL order: first word x=%.1f should be > second word x=%.1f",
			xFirstWord, xSecondWord)
	}
}

func TestLayoutDarwin_RTL_WordOrderAcrossSpace(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	const text = "مرحبا بالعالم"
	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	l, err := ctx.LayoutText(text, cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.CharRects) < 3 {
		t.Fatalf("too few CharRects (%d)", len(l.CharRects))
	}

	var xFirstWord, xSecondWord float32
	firstFound, secondFound := false, false
	for _, cr := range l.CharRects {
		switch cr.Index {
		case 0:
			xFirstWord = cr.Rect.X
			firstFound = true
		case len("مرحبا "):
			xSecondWord = cr.Rect.X
			secondFound = true
		}
	}
	if !firstFound || !secondFound {
		t.Fatalf("could not find expected CharRects (first=%v second=%v)", firstFound, secondFound)
	}
	if xFirstWord <= xSecondWord {
		t.Errorf("visual RTL word order: first word x=%.1f should be > second word x=%.1f",
			xFirstWord, xSecondWord)
	}
}

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
	chars := []charInfo{{text: "a", byteI: 0, byteL: 1}}
	if order := visualOrderForLine("a", chars, 0, 0); order != nil {
		t.Errorf("empty range: got %v, want nil", order)
	}
	if order := visualOrderForLine("a", chars, 1, 1); order != nil {
		t.Errorf("equal bounds: got %v, want nil", order)
	}
}

func TestVisualOrderForLine_BadBounds_ReturnsNil(t *testing.T) {
	chars := []charInfo{{text: "a", byteI: 0, byteL: 1}}
	if order := visualOrderForLine("a", chars, -1, 1); order != nil {
		t.Errorf("negative start: got %v, want nil", order)
	}
	if order := visualOrderForLine("a", chars, 0, 2); order != nil {
		t.Errorf("end > len(chars): got %v, want nil", order)
	}
}

func TestVisualOrderForLine_PurelyLTR_IdentityOrder(t *testing.T) {
	text := "ab"
	chars := []charInfo{
		{text: "a", byteI: 0, byteL: 1},
		{text: "b", byteI: 1, byteL: 1},
	}
	order := visualOrderForLine(text, chars, 0, 2)
	if len(order) != 2 || order[0] != 0 || order[1] != 1 {
		t.Errorf("LTR order=%v, want [0 1]", order)
	}
}

func TestVisualOrderForLine_PurelyRTL_ReversedOrder(t *testing.T) {
	// "שב": two Hebrew chars (2 bytes each). RTL bidi reverses logical order.
	text := "שב"
	chars := []charInfo{
		{text: "ש", byteI: 0, byteL: 2},
		{text: "ב", byteI: 2, byteL: 2},
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
// mergeStyles
// ---------------------------------------------------------------------------

func TestMergeStyles_EmptyRunInheritsBase(t *testing.T) {
	base := TextStyle{FontName: "Sans 16", Size: 14, Color: Color{R: 255, A: 255}}
	got := mergeStyles(base, TextStyle{})
	if got.FontName != "Sans 16" {
		t.Errorf("FontName=%q, want %q", got.FontName, "Sans 16")
	}
	if got.Size != 14 {
		t.Errorf("Size=%v, want 14", got.Size)
	}
	if got.Color.R != 255 {
		t.Errorf("Color.R=%d, want 255", got.Color.R)
	}
}

func TestMergeStyles_RunColorOverridesBase(t *testing.T) {
	base := TextStyle{Color: Color{R: 255, A: 255}}
	run := TextStyle{Color: Color{G: 128, A: 255}}
	got := mergeStyles(base, run)
	if got.Color.R != 0 || got.Color.G != 128 || got.Color.A != 255 {
		t.Errorf("Color=%v, want {G:128 A:255}", got.Color)
	}
}

func TestMergeStyles_RunSizeZeroFallsBackToBase(t *testing.T) {
	base := TextStyle{Size: 20}
	got := mergeStyles(base, TextStyle{Size: 0})
	if got.Size != 20 {
		t.Errorf("Size=%v, want 20", got.Size)
	}
}

func TestMergeStyles_RunFontNameOverridesBase(t *testing.T) {
	base := TextStyle{FontName: "Sans 16"}
	run := TextStyle{FontName: "Serif 12"}
	got := mergeStyles(base, run)
	if got.FontName != "Serif 12" {
		t.Errorf("FontName=%q, want %q", got.FontName, "Serif 12")
	}
}

// ---------------------------------------------------------------------------
// LayoutRichText
// ---------------------------------------------------------------------------

func TestLayoutRichText_EmptyRuns_ReturnsEmptyLayout(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	l, err := ctx.LayoutRichText(RichText{}, cfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(l.Glyphs) != 0 || len(l.Items) != 0 {
		t.Errorf("empty runs: got %d glyphs %d items, want 0 0",
			len(l.Glyphs), len(l.Items))
	}
}

func TestLayoutRichText_TwoRuns_GlyphCountSumMatchesBoth(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	rt := RichText{Runs: []StyleRun{
		{Text: "Hello", Style: TextStyle{}},
		{Text: "World", Style: TextStyle{}},
	}}
	l, err := ctx.LayoutRichText(rt, cfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	// 10 chars → 10 glyphs (no ligatures across "HelloWorld").
	if len(l.Glyphs) != 10 {
		t.Errorf("glyph count=%d, want 10", len(l.Glyphs))
	}
}

func TestLayoutRichText_Superscript_YShiftPositive(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	rt := RichText{Runs: []StyleRun{
		{Text: "x", Style: TextStyle{
			Size: 16,
			Features: &FontFeatures{
				OpenTypeFeatures: []FontFeature{{Tag: "sups", Value: 1}},
			},
		}},
	}}
	l, err := ctx.LayoutRichText(rt, cfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	if l.Glyphs[0].YOffset <= 0 {
		t.Errorf("superscript YOffset=%v, want >0", l.Glyphs[0].YOffset)
	}
}

func TestLayoutRichText_Subscript_YShiftNegative(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	rt := RichText{Runs: []StyleRun{
		{Text: "x", Style: TextStyle{
			Size: 16,
			Features: &FontFeatures{
				OpenTypeFeatures: []FontFeature{{Tag: "subs", Value: 1}},
			},
		}},
	}}
	l, err := ctx.LayoutRichText(rt, cfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	if l.Glyphs[0].YOffset >= 0 {
		t.Errorf("subscript YOffset=%v, want <0", l.Glyphs[0].YOffset)
	}
}

func TestLayoutRichText_InlineObject_WidthReserved(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	const objectPts = float32(30)
	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	rt := RichText{Runs: []StyleRun{
		{Text: "￼", Style: TextStyle{
			Object: &InlineObject{ID: "img1", Width: objectPts},
		}},
	}}
	l, err := ctx.LayoutRichText(rt, cfg)
	if err != nil {
		t.Fatalf("LayoutRichText: %v", err)
	}
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs for inline object")
	}
	// XAdvance = objectWidth / scaleFactor = (objectPts * 1.0) / 1.0 = objectPts.
	if l.Glyphs[0].XAdvance != float64(objectPts) {
		t.Errorf("inline object XAdvance=%v, want %v", l.Glyphs[0].XAdvance, objectPts)
	}
}

// ---------------------------------------------------------------------------
// buildVerticalLayout (via OrientationVertical)
// ---------------------------------------------------------------------------

func TestLayoutDarwin_Vertical_GlyphCountMatchesChars(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style:       TextStyle{FontName: "Sans 16"},
		Orientation: OrientationVertical,
	}
	l, err := ctx.LayoutText("ABC", cfg)
	if err != nil {
		t.Fatalf("LayoutText vertical: %v", err)
	}
	if len(l.Glyphs) != 3 {
		t.Errorf("vertical glyph count=%d, want 3", len(l.Glyphs))
	}
}

func TestLayoutDarwin_Vertical_NewlineSkipped(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style:       TextStyle{FontName: "Sans 16"},
		Orientation: OrientationVertical,
	}
	l, err := ctx.LayoutText("A\nB", cfg)
	if err != nil {
		t.Fatalf("LayoutText vertical: %v", err)
	}
	// Newline skipped: only 'A' and 'B' become glyphs.
	if len(l.Glyphs) != 2 {
		t.Errorf("vertical with newline: glyph count=%d, want 2", len(l.Glyphs))
	}
}

func TestLayoutDarwin_Vertical_YAdvanceIsNegative(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	cfg := TextConfig{
		Style:       TextStyle{FontName: "Sans 16"},
		Orientation: OrientationVertical,
	}
	l, err := ctx.LayoutText("AB", cfg)
	if err != nil {
		t.Fatalf("LayoutText vertical: %v", err)
	}
	for i, g := range l.Glyphs {
		if g.YAdvance >= 0 {
			t.Errorf("glyph[%d].YAdvance=%v, want <0", i, g.YAdvance)
		}
	}
}

// ---------------------------------------------------------------------------
// shapeTextClusters — RTL word merge
// ---------------------------------------------------------------------------

func TestShapeTextClusters_Arabic_MergesWordIntoOneCluster(t *testing.T) {
	font := newCTFont(TextStyle{FontName: "Sans 16"}, 1.0)
	defer font.close()

	// Single Arabic word (5 RTL non-space chars). After the RTL merge pass,
	// no two adjacent clusters in the result may both be RTL and non-space —
	// that is the invariant the merge must maintain regardless of how many
	// CTLine runs CoreText internally uses for the word.
	const word = "مرحبا"
	sc := shapeTextClusters(font, word)
	if len(sc) == 0 {
		t.Fatal("no clusters returned for Arabic text")
	}
	for i := 1; i < len(sc); i++ {
		p, c := sc[i-1], sc[i]
		pt := word[p.byteStart : p.byteStart+p.byteLen]
		ct := word[c.byteStart : c.byteStart+c.byteLen]
		if p.isRTL && pt != " " && pt != "\t" &&
			c.isRTL && ct != " " && ct != "\t" {
			t.Errorf("adjacent RTL non-space clusters at [%d,%d]: merge did not fire", i-1, i)
		}
	}
}

// ---------------------------------------------------------------------------
// LayoutText — error path
// ---------------------------------------------------------------------------

func TestLayoutText_ExceedsMaxLength_ReturnsError(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	text := strings.Repeat("a", MaxTextLength+1)
	cfg := TextConfig{Style: TextStyle{FontName: "Sans 16"}}
	_, err := ctx.LayoutText(text, cfg)
	if err == nil {
		t.Error("expected error for text exceeding MaxTextLength, got nil")
	}
}

// ---------------------------------------------------------------------------
// buildLayout — WrapChar / WrapWordChar
// ---------------------------------------------------------------------------

func TestLayoutDarwin_WrapChar_BreaksAtCharBoundary(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	// No spaces: WrapWord cannot break. WrapChar must produce multiple lines.
	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
		Block: BlockStyle{Width: 20, Wrap: WrapChar},
	}
	l, err := ctx.LayoutText("ABCDEF", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Lines) < 2 {
		t.Errorf("WrapChar: got %d lines, want ≥2", len(l.Lines))
	}
}

func TestLayoutDarwin_WrapWordChar_FallsBackToChar(t *testing.T) {
	ctx := newDarwinTestContext(t)
	defer ctx.Free()

	// One very long word; WrapWord has no break point, WrapWordChar falls back
	// to char-breaking and must produce multiple lines.
	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
		Block: BlockStyle{Width: 30, Wrap: WrapWordChar},
	}
	l, err := ctx.LayoutText("Averylongword", cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(l.Lines) < 2 {
		t.Errorf("WrapWordChar fallback: got %d lines, want ≥2", len(l.Lines))
	}
}
