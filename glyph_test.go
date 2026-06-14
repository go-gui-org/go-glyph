//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

import (
	"math"
	"testing"
	"time"
)

// recordingBackend extends mockBackend with draw call recording.
type recordingBackend struct {
	mockBackend
	drawCalls   []drawCall
	filledRects []filledRectCall
}

type drawCall struct {
	TextureID TextureID
	Src       Rect
	Dst       Rect
	Color     Color
}

type filledRectCall struct {
	Dst   Rect
	Color Color
}

func newRecordingBackend() *recordingBackend {
	return &recordingBackend{
		mockBackend: mockBackend{textures: make(map[TextureID][]byte)},
	}
}

func (r *recordingBackend) DrawTexturedQuad(id TextureID, src, dst Rect, c Color) {
	r.drawCalls = append(r.drawCalls, drawCall{id, src, dst, c})
}

func (r *recordingBackend) DrawFilledRect(dst Rect, c Color) {
	r.filledRects = append(r.filledRects, filledRectCall{dst, c})
}

func (r *recordingBackend) DrawTexturedQuadTransformed(id TextureID,
	src, dst Rect, c Color, t AffineTransform) {
	r.drawCalls = append(r.drawCalls, drawCall{id, src, dst, c})
}

func TestNewTextSystem(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	if ts.ctx == nil {
		t.Error("nil context")
	}
	if ts.renderer == nil {
		t.Error("nil renderer")
	}
}

func TestTextSystemDrawText(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{
			FontName: "Sans 16",
			Color:    Color{0, 0, 0, 255},
		},
	}
	err = ts.DrawText(100, 200, "Hello", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls after DrawText + Commit")
	}
}

func TestTextSystemTextWidth(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
	}
	w, err := ts.TextWidth("Hello World", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w <= 0 {
		t.Errorf("expected positive width, got %f", w)
	}
}

func TestTextSystemTextHeight(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
	}
	h, err := ts.TextHeight("Hello World", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if h <= 0 {
		t.Errorf("expected positive height, got %f", h)
	}
}

func TestTextSystemFontHeight(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
	}
	h, err := ts.FontHeight(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if h <= 0 {
		t.Errorf("expected positive font height, got %f", h)
	}
}

func TestTextSystemLayoutText(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 14"},
	}
	layout, err := ts.LayoutText("Test layout", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Items) == 0 {
		t.Error("no items in layout")
	}
	if layout.Width <= 0 {
		t.Error("layout width <= 0")
	}
}

func TestTextSystemLayoutCache(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 14"},
	}

	// First call creates cache entry.
	l1, err := ts.LayoutTextCached("Cached text", cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Second call should return same layout.
	l2, err := ts.LayoutTextCached("Cached text", cfg)
	if err != nil {
		t.Fatal(err)
	}

	if l1.Width != l2.Width || l1.Height != l2.Height {
		t.Error("cached layout mismatch")
	}
}

func TestTextSystemEmptyText(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 14"}}
	err = ts.DrawText(0, 0, "", cfg)
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func TestTextSystemDrawLayoutTransformed(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{
			FontName: "Sans 16",
			Color:    Color{255, 0, 0, 255},
		},
	}
	layout, err := ts.LayoutText("Rotated", cfg)
	if err != nil {
		t.Fatal(err)
	}

	ts.DrawLayoutTransformed(layout, 100, 100, AffineRotation(0.5))
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls for transformed layout")
	}
}

func TestTextSystemDrawLayoutWithGradient(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
	}
	layout, err := ts.LayoutText("Gradient", cfg)
	if err != nil {
		t.Fatal(err)
	}

	gradient := &GradientConfig{
		Direction: GradientHorizontal,
		Stops: []GradientStop{
			{Position: 0, Color: Color{255, 0, 0, 255}},
			{Position: 1, Color: Color{0, 0, 255, 255}},
		},
	}
	ts.DrawLayoutWithGradient(layout, 50, 50, gradient)
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls for gradient layout")
	}
}

func TestTextSystemDrawLayoutTransformedWithGradient(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 16"},
	}
	layout, err := ts.LayoutText("Gradient transform", cfg)
	if err != nil {
		t.Fatal(err)
	}

	gradient := &GradientConfig{
		Direction: GradientHorizontal,
		Stops: []GradientStop{
			{Position: 0, Color: Color{255, 0, 0, 255}},
			{Position: 1, Color: Color{0, 0, 255, 255}},
		},
	}
	ts.DrawLayoutTransformedWithGradient(
		layout, 20, 30, AffineRotation(0.25), gradient,
	)
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls for transformed gradient layout")
	}
}

func TestTextSystemDrawLayoutPlaced(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{
			FontName: "Sans 16",
			Color:    Color{0, 0, 0, 255},
		},
	}
	layout, err := ts.LayoutText("ABC", cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(layout.Glyphs) == 0 {
		t.Skip("no glyphs to place")
	}

	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{
			X:     float32(50 + i*20),
			Y:     100,
			Angle: 0,
		}
	}

	ts.DrawLayoutPlaced(layout, placements)
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls for placed layout")
	}
}

func TestRendererGlyphCacheEviction(t *testing.T) {
	backend := newRecordingBackend()
	renderer, err := NewRendererWithConfig(backend, 1.0, 256, 256,
		RendererConfig{MaxGlyphCacheEntries: 256})
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Free()

	if renderer.maxCacheEntries != 256 {
		t.Errorf("maxCacheEntries = %d, want 256", renderer.maxCacheEntries)
	}
}

func TestRendererCommit(t *testing.T) {
	backend := newRecordingBackend()
	renderer, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Free()

	fc := renderer.atlas.FrameCounter
	renderer.Commit()
	if renderer.atlas.FrameCounter != fc+1 {
		t.Error("frame counter not incremented")
	}
}

func TestCacheKeyDifferentText(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 14"}}
	k1 := ts.getCacheKey("Hello", cfg)
	k2 := ts.getCacheKey("World", cfg)
	if k1 == k2 {
		t.Error("different text should produce different keys")
	}
}

func TestCacheKeyDifferentSize(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg1 := TextConfig{Style: TextStyle{FontName: "Sans 14"}}
	cfg2 := TextConfig{Style: TextStyle{FontName: "Sans 24"}}
	k1 := ts.getCacheKey("Test", cfg1)
	k2 := ts.getCacheKey("Test", cfg2)
	if k1 == k2 {
		t.Error("different font size should produce different keys")
	}
}

func TestCacheKeyGradientExcluded(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg1 := TextConfig{Style: TextStyle{FontName: "Sans 14"}}
	cfg2 := TextConfig{
		Style: TextStyle{FontName: "Sans 14"},
		Gradient: &GradientConfig{
			Stops: []GradientStop{{Position: 0, Color: Color{255, 0, 0, 255}}},
		},
	}
	k1 := ts.getCacheKey("Test", cfg1)
	k2 := ts.getCacheKey("Test", cfg2)
	if k1 != k2 {
		t.Error("gradient should not affect cache key")
	}
}

func TestCacheKeyDifferentLineSpacing(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg1 := TextConfig{
		Style: TextStyle{FontName: "Sans 14"},
		Block: BlockStyle{Width: 120, Wrap: WrapWord},
	}
	cfg2 := TextConfig{
		Style: TextStyle{FontName: "Sans 14"},
		Block: BlockStyle{
			Width:       120,
			Wrap:        WrapWord,
			LineSpacing: 6,
		},
	}
	k1 := ts.getCacheKey("Test", cfg1)
	k2 := ts.getCacheKey("Test", cfg2)
	if k1 == k2 {
		t.Error("different line spacing should produce different keys")
	}
}

func TestTextSystemAddFontFile(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	err = ts.AddFontFile("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestLayoutCacheMaxEntries(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	ts.maxCacheEntries = 10

	// Insert more than max entries.
	for i := range 20 {
		cfg := TextConfig{
			Style: TextStyle{FontName: "Sans 14", Size: float32(i + 1)},
		}
		_, err := ts.LayoutTextCached("test", cfg)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Batch eviction drops ~25%, so cache stays well under 2x max.
	if len(ts.cache) > 20 {
		t.Errorf("cache size %d, expected <= 20 after eviction", len(ts.cache))
	}
}

func TestAddFontFileClearsCache(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{Style: TextStyle{FontName: "Sans 14"}}
	_, err = ts.LayoutTextCached("cached", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts.cache) == 0 {
		t.Fatal("cache should not be empty")
	}

	// AddFontFile with invalid path still fails, cache unchanged.
	_ = ts.AddFontFile("/nonexistent.ttf")
	if len(ts.cache) == 0 {
		t.Error("cache should not be cleared on font load failure")
	}
}

func TestAffineMultiply(t *testing.T) {
	// Translation then rotation should compose.
	trans := AffineTranslation(10, 20)
	rot := AffineRotation(0)
	result := trans.Multiply(rot)

	// Rotation by 0 is identity, so result should equal trans.
	if math.Abs(float64(result.X0-10)) > 0.001 || math.Abs(float64(result.Y0-20)) > 0.001 {
		t.Errorf("Multiply with identity rotation: X0=%f Y0=%f", result.X0, result.Y0)
	}

	// Identity * Identity = Identity.
	id := AffineIdentity()
	result = id.Multiply(id)
	if result != id {
		t.Error("identity * identity != identity")
	}
}

func TestVerticalGradientSingleLineProducesDistinctColors(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{FontName: "Sans 24"},
	}
	layout, err := ts.LayoutText("ABC", cfg)
	if err != nil {
		t.Fatal(err)
	}

	gradient := &GradientConfig{
		Direction: GradientVertical,
		Stops: []GradientStop{
			{Position: 0, Color: Color{255, 0, 0, 255}},
			{Position: 1, Color: Color{0, 0, 255, 255}},
		},
	}
	ts.DrawLayoutWithGradient(layout, 50, 50, gradient)
	ts.Commit()

	if len(backend.drawCalls) < 2 {
		t.Fatalf("expected multiple draw calls for strip rendering, got %d",
			len(backend.drawCalls))
	}

	// Verify not all colors are identical (flat color = broken).
	first := backend.drawCalls[0].Color
	allSame := true
	for _, dc := range backend.drawCalls[1:] {
		if dc.Color != first {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("all strip colors identical; vertical gradient not working")
	}

	// Verify strip src rects tile correctly (no gaps, no overlap)
	// by checking that consecutive strips within the same glyph
	// have contiguous Y coordinates.
	for i := 1; i < len(backend.drawCalls); i++ {
		prev := backend.drawCalls[i-1]
		curr := backend.drawCalls[i]
		// Same src X means same glyph column.
		if prev.Src.X != curr.Src.X {
			continue
		}
		gap := curr.Src.Y - (prev.Src.Y + prev.Src.Height)
		if gap < -0.01 || gap > 0.01 {
			t.Errorf("strip src gap at index %d: prev.Y+H=%f, curr.Y=%f",
				i, prev.Src.Y+prev.Src.Height, curr.Src.Y)
		}
	}
}

func TestTextSystemDecorations(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	cfg := TextConfig{
		Style: TextStyle{
			FontName:  "Sans 16",
			Color:     Color{0, 0, 0, 255},
			Underline: true,
		},
	}
	err = ts.DrawText(10, 10, "Underlined", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts.Commit()

	if len(backend.filledRects) == 0 {
		t.Error("expected filled rect for underline decoration")
	}
}

// --- Layout cache TTL and eviction tests ---

func TestPruneCache_AgeEviction(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	// Insert entries with old timestamps.
	now := time.Now().UnixMilli()
	ts.evictionAge = 100 // 100ms TTL for test.

	ts.cache[1] = &cachedLayout{lastAccess: now - 200} // Old — should evict.
	ts.cache[2] = &cachedLayout{lastAccess: now - 50}  // Recent — survives.
	ts.cache[3] = &cachedLayout{lastAccess: now}       // Fresh — survives.

	ts.pruneCache()

	if _, ok := ts.cache[1]; ok {
		t.Error("key 1 (200ms old) should have been evicted (exceeds 100ms TTL)")
	}
	if _, ok := ts.cache[2]; !ok {
		t.Error("key 2 (50ms old) should survive (within 100ms TTL)")
	}
	if _, ok := ts.cache[3]; !ok {
		t.Error("key 3 (0ms old) should survive (within 100ms TTL)")
	}
}

func TestPruneCache_EmptyNoPanic(t *testing.T) {
	var ts TextSystem
	ts.pruneCache() // Must not panic.
}

func TestEvictOldestLayouts_TwentyFivePercent(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	// Insert 10 entries with ascending timestamps.
	for i := range 10 {
		ts.cache[uint64(i)] = &cachedLayout{lastAccess: int64(i)}
	}

	ts.evictOldestLayouts()

	// Should drop floor(10/4) = 2 entries (keys 0, 1).
	// At most 25% deleted.
	if len(ts.cache) < 7 || len(ts.cache) > 8 {
		t.Errorf("cache size = %d after 25%% eviction, want 8 (kept 80%%)",
			len(ts.cache))
	}
	if _, ok := ts.cache[0]; ok {
		t.Error("key 0 (oldest) should have been evicted")
	}
	if _, ok := ts.cache[1]; ok {
		t.Error("key 1 (second oldest) should have been evicted")
	}
	// Keys 2-9 should survive.
	for i := uint64(2); i < 10; i++ {
		if _, ok := ts.cache[i]; !ok {
			t.Errorf("key %d should survive", i)
		}
	}
}

func TestEvictOldestLayouts_EmptyNoPanic(t *testing.T) {
	var ts TextSystem
	ts.evictOldestLayouts() // Must not panic.
}

func TestEvictOldestLayouts_SingleEntry(t *testing.T) {
	backend := newRecordingBackend()
	ts, err := NewTextSystem(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Free()

	ts.cache[0] = &cachedLayout{lastAccess: 100}
	ts.evictOldestLayouts()

	// n/4 = 0, targetDelete = 1 — single entry evicted.
	if len(ts.cache) != 0 {
		t.Errorf("single entry should be evicted, got %d", len(ts.cache))
	}
}
