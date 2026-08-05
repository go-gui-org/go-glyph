//go:build android || linux || darwin || windows

package glyph

import "testing"

// TestDrawLayoutUploadsBeforeQuads is the regression test for issue #89:
// glyphs rasterized during DrawLayout must reach the GPU before any quad
// that samples them is emitted, even though hosts only call Commit after
// the draw pass. Without the in-call upload, the first appearance of a
// glyph samples stale atlas texels and renders blank for one frame.
func TestDrawLayoutUploadsBeforeQuads(t *testing.T) {
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
	assertUploadsPrecedeDraws(t, b)
}

// TestDrawLayoutPlacedUploadsBeforeQuads covers DrawLayoutPlaced, which
// shares the interleaved resolve/emit bug fixed for DrawLayout.
func TestDrawLayoutPlacedUploadsBeforeQuads(t *testing.T) {
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
	assertUploadsPrecedeDraws(t, b)
}

// TestDrawLayoutStrokeUploadsBeforeQuads covers the stroke-outline resolve
// pass (the first resolve loop in drawLayoutImpl), which the plain-text
// tests above never exercise: HasStroke items take a different guard and
// cache key (strokeWidth), so a guard or key drift there would go unnoticed.
func TestDrawLayoutStrokeUploadsBeforeQuads(t *testing.T) {
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
	assertUploadsPrecedeDraws(t, b)

	// Six Latin glyphs: one stroke-outline quad plus one fill quad each.
	if len(b.drawCalls) != 12 {
		t.Errorf("drew %d quads, want 12 (stroke + fill per glyph)",
			len(b.drawCalls))
	}
}

// TestDrawLayoutWarmFrameNoUpload guards against the ordering fix
// regressing into upload-per-draw-call: a warm frame (all glyphs cached)
// must not upload any page.
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
	coldUploads := countUploads(b)
	if coldUploads == 0 {
		t.Fatal("cold frame should upload at least one page")
	}

	r.DrawLayout(layout, 10, 20)
	if got := countUploads(b); got != coldUploads {
		t.Errorf("warm frame uploaded %d times, want %d", got, coldUploads)
	}
}

// assertUploadsPrecedeDraws walks the backend op log and fails for every
// textured draw whose texture was not uploaded earlier in the log.
func assertUploadsPrecedeDraws(t *testing.T, b *recordingBackend) {
	t.Helper()
	uploaded := make(map[TextureID]bool)
	for _, op := range b.ops {
		switch op.kind {
		case "upload":
			uploaded[op.id] = true
		case "draw":
			if !uploaded[op.id] {
				t.Errorf("quad for texture %d emitted before its upload",
					op.id)
			}
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
