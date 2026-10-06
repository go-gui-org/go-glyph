//go:build android || linux || darwin || windows

package glyph

import (
	"reflect"
	"testing"
)

// hitTestText mixes a wrap, a newline, and an RTL run so every kind of
// hit-test data (rects, attrs, word bounds, charRTL) is non-trivial.
const hitTestText = "hello world\nשלום abc"

func newHitTestSystem(t *testing.T) *TextSystem {
	t.Helper()
	ts, err := NewTextSystem(newRecordingBackend())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ts.Free)
	return ts
}

func hasHitTestData(l Layout) bool {
	return l.CharRects != nil || l.CharRectByIndex != nil ||
		l.LogAttrs != nil || l.LogAttrByIndex != nil ||
		l.cursorPositions != nil || l.wordStarts != nil || l.wordEnds != nil ||
		l.charRTL != nil
}

// The draw and measure paths never read hit-test data, so the layout they
// cache must not carry it: it was over half of what buildLayout allocated.
func TestTextWidthCachesLayoutWithoutHitTestData(t *testing.T) {
	ts := newHitTestSystem(t)
	cfg := TextConfig{Style: TextStyle{Size: 16}}

	if _, err := ts.TextWidth(hitTestText, cfg); err != nil {
		t.Fatal(err)
	}
	if len(ts.cache) != 1 {
		t.Fatalf("cache entries: got %d, want 1", len(ts.cache))
	}
	for _, item := range ts.cache {
		if hasHitTestData(item.layout) {
			t.Error("TextWidth cached a layout with hit-test data")
		}
		if len(item.layout.Glyphs) == 0 || len(item.layout.Items) == 0 {
			t.Error("TextWidth layout lost its glyphs or items")
		}
	}
}

// A layout cached by TextWidth must not leak into LayoutTextCached, whose
// callers hit-test: the entry is rebuilt with the data, in place.
func TestLayoutTextCachedUpgradesMeasureOnlyEntry(t *testing.T) {
	ts := newHitTestSystem(t)
	cfg := TextConfig{Style: TextStyle{Size: 16}}

	if _, err := ts.TextWidth(hitTestText, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := ts.LayoutTextCached(hitTestText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ts.LayoutText(hitTestText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Error("LayoutTextCached after TextWidth differs from LayoutText")
	}
	if len(ts.cache) != 1 {
		t.Errorf("cache entries: got %d, want 1 (upgrade in place)", len(ts.cache))
	}

	// The upgraded entry now serves the measure path too.
	w, err := ts.TextWidth(hitTestText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w != want.Width {
		t.Errorf("TextWidth: got %v, want %v", w, want.Width)
	}
}

// NoHitTesting drops only the hit-test data. Everything the renderer and the
// measure calls read must match a full layout exactly.
func TestNoHitTestingKeepsRenderData(t *testing.T) {
	ts := newHitTestSystem(t)
	for _, wrap := range []float32{0, 40} {
		cfg := TextConfig{Style: TextStyle{Size: 16}}
		cfg.Block.Width = wrap
		full, err := ts.LayoutText(hitTestText, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if !hasHitTestData(full) {
			t.Fatal("full layout has no hit-test data")
		}
		cfg.NoHitTesting = true
		lean, err := ts.LayoutText(hitTestText, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if hasHitTestData(lean) {
			t.Errorf("wrap %v: NoHitTesting layout has hit-test data", wrap)
		}
		full.CharRects, full.CharRectByIndex = nil, nil
		full.LogAttrs, full.LogAttrByIndex = nil, nil
		full.cursorPositions, full.wordStarts, full.wordEnds = nil, nil, nil
		full.charRTL = nil
		if !reflect.DeepEqual(lean, full) {
			t.Errorf("wrap %v: NoHitTesting changed render data", wrap)
		}
	}
}

// Hit-test queries on a layout without the data must not panic.
func TestNoHitTestingQueriesAreSafe(t *testing.T) {
	ts := newHitTestSystem(t)
	l, err := ts.LayoutText(hitTestText, TextConfig{
		Style: TextStyle{Size: 16}, NoHitTesting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = l.HitTest(5, 5)
	_ = l.GetClosestOffset(5, 5)
	_, _ = l.GetCharRect(0)
	_, _ = l.GetCursorPos(3)
	_ = l.MoveCursorRight(0)
	_ = l.MoveCursorLeft(3)
	_ = l.MoveCursorWordRight(0)
	_ = l.MoveCursorWordLeft(3)
	_ = l.GetSelectionRects(0, 5)
}
