package glyph

import (
	"math"
	"testing"
)

// bgTestLayout returns a layout with one bg item and no glyphs.
// The background loop runs before any glyph work, so no font or
// shaping is needed.
func bgTestLayout() Layout {
	return Layout{
		Text: "Hi",
		Items: []Item{
			{
				X: 5, Y: 30, Width: 100, Ascent: 20, Descent: 5,
				HasBgColor: true,
				BgColor:    Color{1, 2, 3, 255},
			},
		},
	}
}

func TestTransformedBackgroundRotates(t *testing.T) {
	backend := newRecordingBackend()
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	rot := AffineRotation(float32(math.Pi) * 0.5)
	r.DrawLayoutTransformed(bgTestLayout(), 10, 20, rot)

	if len(backend.filledRects) != 0 {
		t.Fatalf("axis-aligned fills = %d, want 0 (bg must rotate)",
			len(backend.filledRects))
	}
	if len(backend.filledTransformed) != 1 {
		t.Fatalf("transformed fills = %d, want 1",
			len(backend.filledTransformed))
	}
	got := backend.filledTransformed[0]
	// dst stays in layout coords: the origin lives in t.
	wantDst := Rect{X: 5, Y: 10, Width: 100, Height: 25}
	if got.Dst != wantDst {
		t.Errorf("dst = %+v, want %+v (origin must stay in t)",
			got.Dst, wantDst)
	}
	wantT := AffineTranslation(10, 20).Multiply(rot)
	if got.Transform != wantT {
		t.Errorf("transform = %+v, want %+v", got.Transform, wantT)
	}
	// The transformed corner must match origin + rotated point.
	gx, gy := got.Transform.Apply(5, 10)
	rx, ry := rot.Apply(5, 10)
	if !near(gx, 10+rx) || !near(gy, 20+ry) {
		t.Errorf("corner = (%v, %v), want (%v, %v)",
			gx, gy, 10+rx, 20+ry)
	}
}

func TestIdentityBackgroundUnchanged(t *testing.T) {
	backend := newRecordingBackend()
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	r.DrawLayout(bgTestLayout(), 10, 20)

	if len(backend.filledTransformed) != 0 {
		t.Fatalf("transformed fills = %d, want 0 under identity",
			len(backend.filledTransformed))
	}
	if len(backend.filledRects) != 1 {
		t.Fatalf("fills = %d, want 1", len(backend.filledRects))
	}
	want := Rect{X: 15, Y: 30, Width: 100, Height: 25}
	if backend.filledRects[0].Dst != want {
		t.Errorf("dst = %+v, want %+v",
			backend.filledRects[0].Dst, want)
	}
}

func TestTransformedDecorationRotates(t *testing.T) {
	backend := newRecordingBackend()
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	layout := bgTestLayout()
	item := &layout.Items[0]
	item.HasBgColor = false
	item.HasUnderline = true
	item.UnderlineOffset = 2
	item.UnderlineThickness = 1
	item.Color = Color{9, 9, 9, 255}

	rot := AffineRotation(float32(math.Pi) * 0.5)
	r.DrawLayoutTransformed(layout, 0, 0, rot)

	if len(backend.filledTransformed) != 1 {
		t.Fatalf("transformed fills = %d, want 1 (underline)",
			len(backend.filledTransformed))
	}
	// Underline rect in layout coords: Y = baseline + offset -
	// thickness = 30 + 2 - 1.
	wantDst := Rect{X: 5, Y: 31, Width: 100, Height: 1}
	if backend.filledTransformed[0].Dst != wantDst {
		t.Errorf("dst = %+v, want %+v",
			backend.filledTransformed[0].Dst, wantDst)
	}
}

func TestNonFiniteTransformDrawsNothing(t *testing.T) {
	backend := newRecordingBackend()
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	nan := float32(math.NaN())
	bad := []AffineTransform{
		{XX: nan, YY: 1},
		{XX: 1, YY: float32(math.Inf(1))},
		AffineTranslation(nan, 0),
	}
	for _, tr := range bad {
		backend.filledRects = nil
		backend.filledTransformed = nil
		backend.drawCalls = nil
		r.DrawLayoutTransformed(bgTestLayout(), 0, 0, tr)
		if len(backend.filledRects) != 0 ||
			len(backend.filledTransformed) != 0 ||
			len(backend.drawCalls) != 0 {
			t.Errorf("transform %+v: drew output, want nothing", tr)
		}
	}
	// Non-finite origin draws nothing either, on both axes and on
	// both the identity and the transformed path.
	inf := float32(math.Inf(-1))
	origins := [][2]float32{{nan, 0}, {0, nan}, {inf, 0}, {0, inf}}
	for _, tr := range []AffineTransform{AffineIdentity(), AffineRotation(0.5)} {
		for _, o := range origins {
			backend.filledRects = nil
			backend.filledTransformed = nil
			backend.drawCalls = nil
			r.DrawLayoutTransformed(bgTestLayout(), o[0], o[1], tr)
			if len(backend.filledRects) != 0 ||
				len(backend.filledTransformed) != 0 ||
				len(backend.drawCalls) != 0 {
				t.Errorf("origin %v, transform %+v: drew output, want nothing",
					o, tr)
			}
		}
	}
}

// fallbackBackend lacks DrawFilledRectTransformed, so the
// renderer must take the axis-aligned fallback path.
type fallbackBackend struct {
	mockBackend
	rects []Rect
}

func (b *fallbackBackend) DrawFilledRect(dst Rect, _ Color) {
	b.rects = append(b.rects, dst)
}

func TestTransformedFillFallbackWithoutExtension(t *testing.T) {
	backend := &fallbackBackend{mockBackend: *newMockBackend()}
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()

	rot := AffineRotation(float32(math.Pi) * 0.5)
	r.DrawLayoutTransformed(bgTestLayout(), 10, 20, rot)

	if len(backend.rects) != 1 {
		t.Fatalf("fallback rects = %d, want 1", len(backend.rects))
	}
	// Fallback moves the origin but keeps axis alignment.
	combined := AffineTranslation(10, 20).Multiply(rot)
	wantX, wantY := combined.Apply(5, 10)
	want := Rect{X: wantX, Y: wantY, Width: 100, Height: 25}
	if backend.rects[0] != want {
		t.Errorf("rect = %+v, want %+v", backend.rects[0], want)
	}
}
