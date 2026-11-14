//go:build linux || darwin || windows

package glyph

import (
	"sync"
	"testing"
)

// TestConcurrentContextsShareFaces guards the shared faceCache and hbPool across
// Contexts. A multi-window app (one Context per window in go-gui) shapes text from
// several goroutines through the same cached faces. The test is only meaningful
// under -race. It also checks that shaping is not corrupted: the width of one fixed
// layout, recorded in each goroutine, must equal a sequential layout of the same
// text.
func TestConcurrentContextsShareFaces(t *testing.T) {
	const (
		numCtx = 4
		iters  = 50
	)
	texts := []string{
		"Hello, world",
		"office",
		"مرحبا",
		"漢字",
		"😀",
	}
	// Fixed probe layout used for the width comparison.
	const probeText = "office"
	const probeFont = "Sans 24"

	ctxs := make([]*Context, numCtx)
	for i := range ctxs {
		ctx, err := NewContext(1.0)
		if err != nil {
			t.Fatal(err)
		}
		defer ctx.Free()
		ctxs[i] = ctx
	}

	widths := make([]float32, numCtx)
	var wg sync.WaitGroup
	for g, ctx := range ctxs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range iters {
				// Alternate sizes so shared faces see different ppem values.
				font := "Sans 12"
				if i%2 == 1 {
					font = "Sans 24"
				}
				for _, s := range texts {
					cfg := TextConfig{Style: TextStyle{FontName: font}}
					if _, err := ctx.LayoutText(s, cfg); err != nil {
						t.Errorf("goroutine %d: LayoutText(%q, %s): %v", g, s, font, err)
						return
					}
				}
				if i == iters-1 {
					cfg := TextConfig{Style: TextStyle{FontName: probeFont}}
					l, err := ctx.LayoutText(probeText, cfg)
					if err != nil {
						t.Errorf("goroutine %d: probe layout: %v", g, err)
						return
					}
					widths[g] = l.Width
				}
			}
		}()
	}
	wg.Wait()

	cfg := TextConfig{Style: TextStyle{FontName: probeFont}}
	want, err := ctxs[0].LayoutText(probeText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for g, w := range widths {
		if w != want.Width {
			t.Errorf("goroutine %d: width = %v, sequential width = %v", g, w, want.Width)
		}
	}
}
