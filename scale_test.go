package glyph

import (
	"math"
	"testing"
)

// newScaleTestSystem builds a TextSystem over the recording backend, which
// reports a 1.0 scale, so every case below starts from a known factor.
func newScaleTestSystem(t *testing.T) *TextSystem {
	t.Helper()
	ts, err := NewTextSystem(newRecordingBackend())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ts.Free)
	return ts
}

func TestSetDPIScaleUpdatesFactors(t *testing.T) {
	ts := newScaleTestSystem(t)

	ts.SetDPIScale(2.0)

	if got := ts.DPIScale(); got != 2.0 {
		t.Errorf("DPIScale: got %v, want 2.0", got)
	}
	if got := ts.ctx.ScaleFactor(); got != 2.0 {
		t.Errorf("ctx.ScaleFactor: got %v, want 2.0", got)
	}
	if got := ts.ctx.scaleInv; got != 0.5 {
		t.Errorf("ctx.scaleInv: got %v, want 0.5", got)
	}
	if got := ts.renderer.scaleFactor; got != 2.0 {
		t.Errorf("renderer.scaleFactor: got %v, want 2.0", got)
	}
	if got := ts.renderer.scaleInv; got != 0.5 {
		t.Errorf("renderer.scaleInv: got %v, want 0.5", got)
	}
}

// A cached layout describes the old density: the cache key does not hash the
// scale, so a stale entry would be served for the same text at the new one.
func TestSetDPIScalePurgesCaches(t *testing.T) {
	ts := newScaleTestSystem(t)

	if _, err := ts.LayoutTextCached("scale", TextConfig{}); err != nil {
		t.Fatal(err)
	}
	if len(ts.cache) == 0 {
		t.Fatal("layout cache empty before rescale; test proves nothing")
	}

	ts.SetDPIScale(1.5)

	if len(ts.cache) != 0 {
		t.Errorf("layout cache: got %d entries, want 0", len(ts.cache))
	}
	if len(ts.renderer.cache) != 0 {
		t.Errorf("glyph cache: got %d entries, want 0",
			len(ts.renderer.cache))
	}
}

// An unchanged scale must not purge: resize handlers call this every frame of
// a drag, and a purge per frame would re-rasterize the whole screen.
func TestSetDPIScaleSameValueKeepsCache(t *testing.T) {
	ts := newScaleTestSystem(t)

	if _, err := ts.LayoutTextCached("scale", TextConfig{}); err != nil {
		t.Fatal(err)
	}
	before := len(ts.cache)

	ts.SetDPIScale(ts.DPIScale())

	if len(ts.cache) != before {
		t.Errorf("layout cache: got %d entries, want %d (unchanged)",
			len(ts.cache), before)
	}
}

func TestSetDPIScaleRejectsNonPositive(t *testing.T) {
	for _, s := range []float32{0, -1, float32(math.NaN())} {
		ts := newScaleTestSystem(t)
		ts.SetDPIScale(s)
		if got := ts.DPIScale(); got != 1.0 {
			t.Errorf("SetDPIScale(%v): scale became %v, want 1.0", s, got)
		}
	}
}

func TestSetDPIScaleRejectsNonFiniteAndExcessive(t *testing.T) {
	for _, s := range []float32{
		float32(math.Inf(1)), float32(math.Inf(-1)), 11, 100, float32(1e6),
	} {
		ts := newScaleTestSystem(t)
		if _, err := ts.LayoutTextCached("scale", TextConfig{}); err != nil {
			t.Fatal(err)
		}
		before := len(ts.cache)
		ts.SetDPIScale(s)
		if got := ts.DPIScale(); got != 1.0 {
			t.Errorf("SetDPIScale(%v): scale became %v, want 1.0", s, got)
		}
		if len(ts.cache) != before {
			t.Errorf("SetDPIScale(%v): purged cache on rejected scale", s)
		}
	}
}

func TestSetDPIScaleNilContextNoPanic(t *testing.T) {
	ts := &TextSystem{}
	// Must not panic and must remain 0.
	ts.SetDPIScale(2.0)
	if got := ts.DPIScale(); got != 0 {
		t.Errorf("DPIScale on nil ctx: got %v, want 0", got)
	}
}

func TestDPIScaleNilContext(t *testing.T) {
	ts := &TextSystem{}
	if got := ts.DPIScale(); got != 0 {
		t.Errorf("DPIScale: got %v, want 0", got)
	}
}
