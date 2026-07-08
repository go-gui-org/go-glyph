//go:build !js && !windows

package glyph

import "testing"

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

func TestDrawLayoutWithComposition(t *testing.T) {
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
	layout, err := ts.LayoutText("Test", cfg)
	if err != nil {
		t.Fatal(err)
	}

	cs := NewCompositionState()
	ts.Renderer().DrawLayoutWithComposition(layout, 10, 10, &cs)
	ts.Commit()

	if len(backend.drawCalls) == 0 {
		t.Error("no draw calls from DrawLayoutWithComposition")
	}
}
