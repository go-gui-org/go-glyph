//go:build !js && !windows

package glyph

import (
	"math"
	"testing"
)

func TestDrawCompositionNotComposing(t *testing.T) {
	backend := newRecordingBackend()
	renderer, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Free()

	cs := NewCompositionState()
	renderer.DrawComposition(Layout{}, 0, 0, &cs, Color{0, 0, 0, 255})

	if len(backend.filledRects) != 0 {
		t.Error("should not draw when not composing")
	}
}

func TestDrawCompositionClauses(t *testing.T) {
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
	l, err := ts.LayoutText("He", cfg)
	if err != nil {
		t.Fatal(err)
	}

	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("He", 2)

	renderer := ts.Renderer()
	renderer.DrawComposition(l, 0, 0, &cs, Color{0, 0, 0, 255})

	if len(backend.filledRects) == 0 {
		t.Error("expected filled rects for composition")
	}
}

func newCompositionTestRenderer(t *testing.T) (*Renderer, *recordingBackend) {
	t.Helper()
	backend := newRecordingBackend()
	r, err := NewRenderer(backend, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Free)
	return r, backend
}

// The dimmed underline and cursor must scale the caller's alpha,
// not replace it.
func TestDrawCompositionKeepsAlpha(t *testing.T) {
	r, backend := newCompositionTestRenderer(t)
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("He", 1)

	r.DrawComposition(testLayout(), 0, 0, &cs, Color{10, 20, 30, 128})
	if len(backend.filledRects) != 2 {
		t.Fatalf("filled rects = %d, want 2 (underline + cursor)",
			len(backend.filledRects))
	}
	want := uint8(128 * 178 / 255)
	for i, fr := range backend.filledRects {
		if fr.Color.A != want {
			t.Errorf("rect %d alpha = %d, want %d", i, fr.Color.A, want)
		}
	}
}

// Under a transform the underline and cursor go through the
// transformed fill path so they stay on the glyphs.
func TestDrawCompositionTransformed(t *testing.T) {
	r, backend := newCompositionTestRenderer(t)
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("He", 1)

	r.DrawCompositionTransformed(testLayout(), 5, 7,
		AffineRotation(0.5), &cs, Color{0, 0, 0, 255})
	if len(backend.filledRects) != 0 {
		t.Errorf("axis-aligned fills = %d, want 0",
			len(backend.filledRects))
	}
	if len(backend.filledTransformed) != 2 {
		t.Fatalf("transformed fills = %d, want 2",
			len(backend.filledTransformed))
	}
	want := AffineTranslation(5, 7).Multiply(AffineRotation(0.5))
	if backend.filledTransformed[0].Transform != want {
		t.Errorf("transform = %+v, want %+v",
			backend.filledTransformed[0].Transform, want)
	}
}

func TestDrawCompositionTransformedNotFinite(t *testing.T) {
	r, backend := newCompositionTestRenderer(t)
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("He", 1)
	nan := float32(math.NaN())
	r.DrawCompositionTransformed(testLayout(), nan, 0,
		AffineIdentity(), &cs, Color{0, 0, 0, 255})
	if n := len(backend.filledRects) + len(backend.filledTransformed); n != 0 {
		t.Errorf("fills = %d, want 0 for non-finite origin", n)
	}
}

// Drawing composition every frame must not allocate.
func TestDrawCompositionNoAllocs(t *testing.T) {
	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("Hello", 2)
	cs.HandleClause(0, 2, 1)
	cs.HandleClause(2, 3, 2)
	l := testLayout()
	c := Color{0, 0, 0, 255}
	allocs := testing.AllocsPerRun(50, func() {
		r.DrawComposition(l, 0, 0, &cs, c)
	})
	if allocs != 0 {
		t.Errorf("allocs per DrawComposition = %v, want 0", allocs)
	}
}

// A nil composition state draws nothing instead of panicking.
func TestDrawCompositionNilState(t *testing.T) {
	r, backend := newCompositionTestRenderer(t)
	c := Color{0, 0, 0, 255}
	r.DrawComposition(testLayout(), 0, 0, nil, c)
	r.DrawCompositionTransformed(testLayout(), 0, 0,
		AffineRotation(0.5), nil, c)
	if n := len(backend.filledRects) + len(backend.filledTransformed); n != 0 {
		t.Errorf("fills = %d, want 0 for nil state", n)
	}
}
