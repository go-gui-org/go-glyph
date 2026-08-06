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
