//go:build android || linux || darwin || windows

package glyph

import "testing"

// The atlas upload contract: glyphs rasterized during a draw call reach
// the GPU at the frame boundary. Hosts call Commit after their draw pass
// and before the render pass samples the textures, so Commit-time
// uploads are visible to the same frame's quads (issue #89) — and
// batching them costs one full-page upload per frame per page instead of
// one per draw call, which a terminal frame's hundreds of per-glyph
// calls would multiply into GB-scale traffic.

// TestDrawLayoutUploadsAtCommit is the regression test for issue #89:
// glyphs rasterized during DrawLayout must reach the GPU before the
// render pass samples the quads. The upload happens at Commit; a draw
// call alone must not upload.
func TestDrawLayoutUploadsAtCommit(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Hello", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.DrawLayout(layout, 10, 20)
	if n := countUploads(b); n != 0 {
		t.Fatalf("DrawLayout uploaded %d pages; uploads must wait for Commit", n)
	}
	r.Commit()
	assertDrawnTexturesUploaded(t, b)
}

// TestDrawLayoutPlacedUploadsAtCommit covers DrawLayoutPlaced, which
// shares the resolve/emit structure of DrawLayout.
func TestDrawLayoutPlacedUploadsAtCommit(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Hello", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{X: float32(i) * 12, Y: 20}
	}

	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.DrawLayoutPlaced(layout, placements)
	if n := countUploads(b); n != 0 {
		t.Fatalf("DrawLayoutPlaced uploaded %d pages; uploads must wait for Commit", n)
	}
	r.Commit()
	assertDrawnTexturesUploaded(t, b)
}

// TestDrawLayoutStrokeUploadsAtCommit covers the stroke-outline resolve
// pass (the first resolve loop in drawLayoutImpl), which the plain-text
// tests above never exercise: HasStroke items take a different guard and
// cache key (strokeWidth), so a guard or key drift there would go unnoticed.
func TestDrawLayoutStrokeUploadsAtCommit(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Stroke", TextConfig{
		Style: TextStyle{
			FontName:    "Sans 16",
			StrokeWidth: 1.5,
			Color:       Color{0, 0, 0, 255},
		},
	})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}
	if !layout.Items[0].HasStroke {
		t.Fatal("expected HasStroke from StrokeWidth config")
	}

	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.DrawLayout(layout, 10, 20)
	if n := countUploads(b); n != 0 {
		t.Fatalf("DrawLayout uploaded %d pages; uploads must wait for Commit", n)
	}
	r.Commit()
	assertDrawnTexturesUploaded(t, b)

	// Six Latin glyphs: one stroke-outline quad plus one fill quad each.
	if len(b.drawCalls) != 12 {
		t.Errorf("drew %d quads, want 12 (stroke + fill per glyph)",
			len(b.drawCalls))
	}
}

// TestDrawLayoutWarmFrameNoUpload guards against the frame-boundary
// batching regressing into upload-per-draw-call: a warm frame (all
// glyphs cached) must not upload any page.
func TestDrawLayoutWarmFrameNoUpload(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Hello", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.DrawLayout(layout, 10, 20)
	r.Commit()
	coldUploads := countUploads(b)
	if coldUploads == 0 {
		t.Fatal("cold frame should upload at least one page")
	}

	r.DrawLayout(layout, 10, 20)
	r.Commit()
	if got := countUploads(b); got != coldUploads {
		t.Errorf("warm frame uploaded %d times, want %d", got, coldUploads)
	}
}

// assertDrawnTexturesUploaded fails unless every texture a quad drew
// into this frame has been uploaded to the backend by now (i.e. by the
// frame's Commit). Textures with no glyph insertions are never dirty and
// need no re-upload — only drawn textures count.
func assertDrawnTexturesUploaded(t *testing.T, b *recordingBackend) {
	t.Helper()
	uploaded := make(map[TextureID]bool)
	for _, op := range b.ops {
		if op.kind == "upload" {
			uploaded[op.id] = true
		}
	}
	for _, op := range b.ops {
		if op.kind == "draw" && !uploaded[op.id] {
			t.Errorf("quad for texture %d drawn without a frame upload", op.id)
		}
	}
}

func countUploads(b *recordingBackend) int {
	n := 0
	for _, op := range b.ops {
		if op.kind == "upload" {
			n++
		}
	}
	return n
}

// TestDrawLayoutSteadyStateNoAlloc is the regression test for issue #92:
// the resolve-then-emit passes must not allocate their CachedGlyph
// scratch per draw call. Baseline: 2 allocs per DrawLayout (fills +
// strokes) and 1 per DrawLayoutPlaced (cgs), each sized to the glyph
// count — a terminal frame's ~1500 per-glyph calls paid ~3000 allocs.
// AllocsPerRun warms up twice before measuring, so the glyph cache is
// warm and only steady-state draws are counted.
func TestDrawLayoutSteadyStateNoAlloc(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	layout, err := ctx.LayoutText("Hello", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText: %v", err)
	}
	if len(layout.Items) == 0 || layout.Items[0].FontPath == "" {
		t.Skip("no font path resolved on this host; skipping")
	}

	placements := make([]GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{X: float32(i) * 12, Y: 20}
	}

	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	// Warm the glyph cache and scratch buffers before measuring.
	r.DrawLayout(layout, 10, 20)
	r.Commit()

	if n := testing.AllocsPerRun(50, func() {
		r.DrawLayout(layout, 10, 20)
	}); n != 0 {
		t.Errorf("DrawLayout steady state allocates %.1f times per call, want 0", n)
	}

	if n := testing.AllocsPerRun(50, func() {
		r.DrawLayoutPlaced(layout, placements)
	}); n != 0 {
		t.Errorf("DrawLayoutPlaced steady state allocates %.1f times per call, want 0", n)
	}
}

// TestScratch covers the scratch helper's contract directly: growth only
// when capacity is insufficient, zeroing of every returned slot (stale
// entries would emit phantom quads), truncation on smaller requests, and
// the retention cap that falls back to transient allocations.
func TestScratch(t *testing.T) {
	r := &Renderer{}

	out := scratch(&r.scratchFills, 4)
	if len(out) != 4 {
		t.Fatalf("len = %d, want 4", len(out))
	}
	for i, cg := range out {
		if cg != (CachedGlyph{}) {
			t.Fatalf("slot %d not zeroed: %+v", i, cg)
		}
	}

	// Poison every slot, then grow: the new slice must be fully zeroed.
	for i := range out {
		out[i] = CachedGlyph{Width: 1, Page: 7}
	}
	out = scratch(&r.scratchFills, 8)
	for i, cg := range out {
		if cg != (CachedGlyph{}) {
			t.Fatalf("slot %d retained poison after growth: %+v", i, cg)
		}
	}

	// Truncation: a smaller request reuses the buffer and zeroes the head.
	for i := range out {
		out[i] = CachedGlyph{Width: 1, Page: 7}
	}
	out = scratch(&r.scratchFills, 2)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	for i, cg := range out {
		if cg != (CachedGlyph{}) {
			t.Fatalf("slot %d retained poison after truncation: %+v", i, cg)
		}
	}

	// Cap: an oversized request is transient and never grows the retained
	// buffer (a host-built Layout could otherwise pin the memory forever).
	before := cap(r.scratchFills)
	if out := scratch(&r.scratchFills, maxScratchGlyphs+1); len(out) != maxScratchGlyphs+1 {
		t.Fatalf("oversized len = %d, want %d", len(out), maxScratchGlyphs+1)
	}
	if cap(r.scratchFills) != before {
		t.Fatalf("oversized request grew retained buffer: %d -> %d",
			before, cap(r.scratchFills))
	}

	// Zero size is a no-op that keeps the retained buffer.
	if out := scratch(&r.scratchFills, 0); len(out) != 0 {
		t.Fatalf("empty len = %d, want 0", len(out))
	}
	if cap(r.scratchFills) == 0 {
		t.Fatal("empty request dropped the retained buffer")
	}
}

// TestDrawLayoutScratchReuseAcrossCalls guards the issue #92 scratch
// buffers at the draw level: successive calls with different layouts
// (longer, then shorter) and different entry points on one renderer must
// not emit phantom quads from stale scratch slots.
func TestDrawLayoutScratchReuseAcrossCalls(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	long, err := ctx.LayoutText("Hello World", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText(long): %v", err)
	}
	short, err := ctx.LayoutText("Hi", TextConfig{})
	if err != nil {
		t.Fatalf("LayoutText(short): %v", err)
	}
	if len(long.Items) == 0 || long.Items[0].FontPath == "" ||
		len(short.Items) == 0 {
		t.Skip("no font path resolved on this host; skipping")
	}

	placements := make([]GlyphPlacement, len(long.Glyphs))
	for i := range placements {
		placements[i] = GlyphPlacement{X: float32(i) * 12, Y: 20}
	}

	b := newRecordingBackend()
	r, err := NewRenderer(b, 1.0)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	defer r.Free()

	r.DrawLayout(long, 10, 20)
	r.Commit()
	afterLong := len(b.drawCalls)

	r.DrawLayoutPlaced(long, placements)
	afterPlaced := len(b.drawCalls)
	if got := afterPlaced - afterLong; got != afterLong {
		t.Errorf("DrawLayoutPlaced drew %d quads, want %d (same layout as DrawLayout)",
			got, afterLong)
	}

	r.DrawLayout(short, 10, 20)
	if got := len(b.drawCalls) - afterPlaced; got != 2 {
		t.Errorf("short layout drew %d quads, want 2 "+
			"(stale scratch slots emitted phantom quads)", got)
	}
}
