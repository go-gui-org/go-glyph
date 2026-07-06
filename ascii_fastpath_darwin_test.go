//go:build darwin && !glyph_pango

package glyph

import (
	"math"
	"testing"
)

// monoStyle is a monospace TextStyle for fast-path tests. Menlo ships
// with every macOS install and carries the monospace symbolic trait.
func monoStyle() TextStyle { return TextStyle{FontName: "Menlo 16"} }

const asciiSample = "ls -la /usr/bin | grep '.go' && echo OK_123"

// TestASCIIFastPathEquivalence asserts the fast path yields glyph IDs
// and advances identical to the full CoreText shaper for a pure-ASCII
// monospace run — the issue's hard correctness bar. Positions and Width
// are derived from these by the shared buildLayout path, so equal glyphs
// + advances imply an equivalent Layout.
func TestASCIIFastPathEquivalence(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	cfg := TextConfig{Style: monoStyle()}
	font := newCTFont(cfg.Style, ctx.scaleFactor)
	if font.ref == 0 {
		t.Fatal("failed to create Menlo CTFont")
	}
	defer font.close()

	fast, ok := ctx.tryASCIIClusters(font, cfg, asciiSample)
	if !ok {
		t.Fatal("fast path declined a pure-ASCII monospace run")
	}
	shaped := shapeTextClusters(font, asciiSample)
	if len(fast) != len(shaped) {
		t.Fatalf("cluster count: fast=%d shaper=%d", len(fast), len(shaped))
	}
	for i := range fast {
		if fast[i].glyphID != shaped[i].glyphID {
			t.Errorf("byte %d (%q): glyphID fast=%d shaper=%d",
				i, asciiSample[i], fast[i].glyphID, shaped[i].glyphID)
		}
		if math.Abs(fast[i].advance-shaped[i].advance) > 0.01 {
			t.Errorf("byte %d (%q): advance fast=%.4f shaper=%.4f",
				i, asciiSample[i], fast[i].advance, shaped[i].advance)
		}
	}

	// End-to-end: LayoutText (which now routes through the fast path)
	// produces a width equal to N × the fixed cell advance.
	lay, err := ctx.LayoutText(asciiSample, cfg)
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	wantW := fast[0].advance * float64(len(asciiSample)) / float64(ctx.scaleFactor)
	if math.Abs(float64(lay.Width)-wantW) > 0.1 {
		t.Errorf("Layout.Width = %.3f, want %.3f", lay.Width, wantW)
	}
}

// TestASCIIFastPathDeclinesLigatures verifies the correctness fence: a
// monospace run with an ASCII-altering OpenType feature (calt) active
// must fall back to the full shaper, since ligatures fuse bytes into one
// glyph and break the one-byte-one-glyph invariant.
func TestASCIIFastPathDeclinesLigatures(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	style := monoStyle()
	style.Features = &FontFeatures{
		OpenTypeFeatures: []FontFeature{{Tag: "calt", Value: 1}},
	}
	cfg := TextConfig{Style: style}
	font := newCTFont(cfg.Style, ctx.scaleFactor)
	defer font.close()

	if _, ok := ctx.tryASCIIClusters(font, cfg, "!= -> =="); ok {
		t.Error("fast path accepted a run with calt ligatures active")
	}
	// Disabled ligatures (Value 0) must NOT disqualify.
	style.Features.OpenTypeFeatures[0].Value = 0
	if !asciiMonospaceEligible(TextConfig{Style: style}, "abc") {
		t.Error("fast path declined despite calt explicitly disabled")
	}
}

// TestASCIIFastPathDeclinesProportional verifies a proportional font is
// rejected (no monospace symbolic trait), and that non-ASCII bytes in a
// monospace font are rejected too.
func TestASCIIFastPathDeclinesProportional(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	propCfg := TextConfig{Style: TextStyle{FontName: "Helvetica 16"}}
	propFont := newCTFont(propCfg.Style, ctx.scaleFactor)
	defer propFont.close()
	if _, ok := ctx.tryASCIIClusters(propFont, propCfg, "abc"); ok {
		t.Error("fast path accepted a proportional font")
	}

	monoCfg := TextConfig{Style: monoStyle()}
	monoFont := newCTFont(monoCfg.Style, ctx.scaleFactor)
	defer monoFont.close()
	if _, ok := ctx.tryASCIIClusters(monoFont, monoCfg, "café"); ok {
		t.Error("fast path accepted a run with non-ASCII bytes")
	}
}

// TestASCIIFastPathDeclinesAnyFeature verifies the fast path declines
// any enabled OpenType feature — not just ligatures. Non-ligature
// features (ss01, cv01, zero, case, etc.) produce different glyphs for
// the same code points and must not be served from the feature-agnostic
// cache. This is a regression test for the bug where hasASCIIAlteringFeatures
// only checked asciiLigatureTags.
func TestASCIIFastPathDeclinesAnyFeature(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	font := newCTFont(monoStyle(), ctx.scaleFactor)
	if font.ref == 0 {
		t.Fatal("failed to create Menlo CTFont")
	}
	defer font.close()

	tests := []struct {
		tag   string
		value int
	}{
		{"ss01", 1},
		{"ss02", 2},
		{"cv01", 1},
		{"zero", 1},
		{"case", 1},
		{"onum", 1},
		{"tnum", 1},
	}
	for _, tt := range tests {
		style := monoStyle()
		style.Features = &FontFeatures{
			OpenTypeFeatures: []FontFeature{{Tag: tt.tag, Value: tt.value}},
		}
		cfg := TextConfig{Style: style}
		if _, ok := ctx.tryASCIIClusters(font, cfg, "abc"); ok {
			t.Errorf("fast path accepted run with %s=%d — any feature "+
				"should disqualify", tt.tag, tt.value)
		}
	}
}

// TestASCIIFastPathDeclinesAnyFeatureDisabled verifies that features
// explicitly set to Value=0 (disabled) do NOT disqualify the fast path.
func TestASCIIFastPathDeclinesAnyFeatureDisabled(t *testing.T) {
	style := monoStyle()
	style.Features = &FontFeatures{
		OpenTypeFeatures: []FontFeature{
			{Tag: "ss01", Value: 0},
			{Tag: "calt", Value: 0},
			{Tag: "cv01", Value: 0},
		},
	}
	if !asciiMonospaceEligible(TextConfig{Style: style}, "abc") {
		t.Error("fast path declined with all features explicitly disabled")
	}
}

// TestASCIIFastPathCacheHit verifies the asciiTable cache returns the
// same table for equivalent styles and distinct tables for different
// styles (keyed on family, size, bold, italic).
func TestASCIIFastPathCacheHit(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	style := monoStyle()
	font := newCTFont(style, ctx.scaleFactor)
	if font.ref == 0 {
		t.Fatal("failed to create Menlo CTFont")
	}
	defer font.close()

	t1 := ctx.asciiTable(style, font)
	if t1 == nil || !t1.ok {
		t.Fatal("first asciiTable call failed")
	}
	t2 := ctx.asciiTable(style, font)
	if t2 != t1 {
		t.Error("same-style cache miss — got different pointer")
	}

	// Different size — must produce distinct table.
	smallStyle := style
	smallStyle.Size = 10
	smallFont := newCTFont(smallStyle, ctx.scaleFactor)
	if smallFont.ref == 0 {
		t.Fatal("failed to create Menlo 10 CTFont")
	}
	defer smallFont.close()
	tSmall := ctx.asciiTable(smallStyle, smallFont)
	if tSmall == nil || !tSmall.ok {
		t.Fatal("small-size asciiTable call failed")
	}
	if tSmall == t1 {
		t.Error("different sizes returned same cached table")
	}
	if tSmall.glyphs['M'] == t1.glyphs['M'] &&
		tSmall.adv['M'] == t1.adv['M'] {
		t.Log("different sizes have identical 'M' glyph+advance (uncommon but possible)")
	}
}

// TestASCIIFastPathAfterFree verifies the asciiTable nil-map recovery
// path: after Free() clears the map, a subsequent call to asciiTable
// re-allocates it and does not panic.
func TestASCIIFastPathAfterFree(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	style := monoStyle()
	font := newCTFont(style, ctx.scaleFactor)
	if font.ref == 0 {
		t.Fatal("failed to create Menlo CTFont")
	}
	defer font.close()

	// Populate cache.
	t1 := ctx.asciiTable(style, font)
	if t1 == nil || !t1.ok {
		t.Fatal("initial asciiTable failed")
	}

	ctx.Free()

	t2 := ctx.asciiTable(style, font)
	if t2 == nil || !t2.ok {
		t.Fatal("asciiTable after Free returned nil or !ok — nil-map recovery failed")
	}
	if ctx.asciiTables == nil {
		t.Fatal("asciiTables map was not re-allocated after Free")
	}
}

// TestASCIIMonospaceEligible exercises the cheap gate guards: empty text,
// UseMarkup, vertical orientation, and boundary/control bytes.
func TestASCIIMonospaceEligible(t *testing.T) {
	// Empty text.
	if asciiMonospaceEligible(TextConfig{Style: monoStyle()}, "") {
		t.Error("empty text should not be eligible")
	}

	// UseMarkup.
	if asciiMonospaceEligible(TextConfig{Style: monoStyle(), UseMarkup: true},
		"abc") {
		t.Error("markup text should not be eligible")
	}

	// Vertical orientation.
	if asciiMonospaceEligible(TextConfig{
		Style:       monoStyle(),
		Orientation: OrientationVertical,
	}, "abc") {
		t.Error("vertical orientation should not be eligible")
	}

	// Control characters (0x00–0x1F) — tab, newline, etc.
	for _, c := range []string{"\x00", "\x0a", "\x0d", "\x1b", "\x1f"} {
		text := "ab" + c
		if asciiMonospaceEligible(TextConfig{Style: monoStyle()}, text) {
			t.Errorf("control char 0x%02X should not be eligible", c[0])
		}
	}

	// DEL (0x7F).
	if asciiMonospaceEligible(TextConfig{Style: monoStyle()}, "\x7f") {
		t.Error("DEL should not be eligible")
	}

	// Printable boundary characters (0x20 space, 0x7E tilde) — eligible.
	if !asciiMonospaceEligible(TextConfig{Style: monoStyle()}, " ") {
		t.Error("space (0x20) should be eligible")
	}
	if !asciiMonospaceEligible(TextConfig{Style: monoStyle()}, "~") {
		t.Error("tilde (0x7E) should be eligible")
	}

	// Non-ASCII byte (> 0x7E).
	if asciiMonospaceEligible(TextConfig{Style: monoStyle()},
		"abc\x80") {
		t.Error("non-ASCII byte > 0x7E should not be eligible")
	}
}

// TestASCIIDeclineFallbackToShaper verifies that when the fast path
// declines (e.g., features active), the fallback through buildLayout →
// shapeTextClusters still produces correct glyphs and Layout.
func TestASCIIDeclineFallbackToShaper(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	style := monoStyle()
	style.Features = &FontFeatures{
		OpenTypeFeatures: []FontFeature{{Tag: "ss01", Value: 1}},
	}
	cfg := TextConfig{Style: style}

	const text = "test fallback run"
	lay, err := ctx.LayoutText(text, cfg)
	if err != nil {
		t.Fatalf("LayoutText with features: %v", err)
	}
	if len(lay.Glyphs) == 0 {
		t.Error("LayoutText with features returned no glyphs")
	}
	if lay.Width <= 0 {
		t.Errorf("LayoutText with features: width=%.2f, want > 0", lay.Width)
	}
}

// BenchmarkASCIIClustersFast measures the warm-cache fast path: it should
// allocate only the returned cluster slice (one alloc) and make no CGo
// shaping call.
func BenchmarkASCIIClustersFast(b *testing.B) {
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	cfg := TextConfig{Style: monoStyle()}
	font := newCTFont(cfg.Style, ctx.scaleFactor)
	defer font.close()
	ctx.asciiTable(cfg.Style, font) // warm the cache

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, ok := ctx.tryASCIIClusters(font, cfg, asciiSample); !ok {
			b.Fatal("fast path declined")
		}
	}
}

// BenchmarkShapeClustersFull measures the full CoreText shaper on the
// same run, for throughput/alloc comparison against the fast path.
func BenchmarkShapeClustersFull(b *testing.B) {
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()
	font := newCTFont(monoStyle(), ctx.scaleFactor)
	defer font.close()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if sc := shapeTextClusters(font, asciiSample); len(sc) == 0 {
			b.Fatal("shaper returned no clusters")
		}
	}
}
