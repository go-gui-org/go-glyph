//go:build linux || darwin || windows

package glyph

import (
	"sync"
	"testing"
)

// TestConcurrentRenderSharesFace guards the render path of the shared cachedFace.
// renderColorGlyph sets the bitmap ppem on cf.face under cf.mu. renderGlyphByID and
// renderRun read that ppem through GlyphData on the same face. Two Contexts (two
// go-gui windows) that draw emoji at the same time run both paths on one face. The
// test is only meaningful under -race.
func TestConcurrentRenderSharesFace(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()

	const emoji = "\U0001F600"
	// Find a color font that the bitmap path can draw. Only such a font has its
	// ppem set by renderColorGlyph.
	var path string
	for _, p := range ftColorFallbacksSingleton {
		if _, ok := renderColorGlyph(nil, p, 32, emoji); ok {
			path = p
			break
		}
	}
	if path == "" {
		t.Skip("no bitmap color-emoji font installed")
	}
	cf := loadCachedFace(path)
	if cf == nil {
		t.Fatalf("loadCachedFace(%q) = nil", path)
	}
	buf := shapeWith(cf, 32, emoji)
	if buf == nil || len(buf.Info) == 0 {
		t.Fatalf("%q did not shape in %q", emoji, path)
	}
	gid := uint32(buf.Info[0].Glyph)
	releaseShapeBuffer(buf)

	const iters = 50
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range iters {
			// Two sizes, so SetPpem writes a new value each time.
			renderColorGlyph(nil, path, float64(20+i%2*20), emoji)
		}
	}()
	go func() {
		defer wg.Done()
		for range iters {
			renderGlyphByID(nil, path, 32, 0, 0, gid)
			renderRun(nil, path, 32, 0, 0, emoji, "", 0, false)
		}
	}()
	wg.Wait()
}
