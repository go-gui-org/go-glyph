//go:build android

package main

import (
	"sync"
	"testing"

	ss "github.com/go-gui-org/go-glyph/examples/showcase_sections"
)

func resetGlobals() {
	scrollY = 0
	winW = 0
	winH = 0
	dpiScale = 1
	cachedTotalH = 0
	initFailed = false
	sects = nil
}

// --- totalHeight ---

func TestTotalHeight_WithSections(t *testing.T) {
	resetGlobals()
	sects = ss.BuildSections()
	h := totalHeight()
	expected := float32(20)
	for _, s := range sects {
		expected += s.Height + ss.SectionGap
	}
	if h != expected {
		t.Errorf("totalHeight() = %f, want %f", h, expected)
	}
}

func TestTotalHeight_NoSections(t *testing.T) {
	resetGlobals()
	h := totalHeight()
	if h != 20 {
		t.Errorf("totalHeight() = %f, want 20", h)
	}
}

// --- clampScroll ---

func TestClampScroll_ContentFitsInWindow(t *testing.T) {
	resetGlobals()
	cachedTotalH = 100
	winH = 200
	scrollY = 50
	clampScroll()
	if scrollY != 0 {
		t.Errorf("scrollY = %f, want 0 (content fits)", scrollY)
	}
}

func TestClampScroll_NegativeScroll(t *testing.T) {
	resetGlobals()
	cachedTotalH = 500
	winH = 200
	scrollY = -10
	clampScroll()
	if scrollY != 0 {
		t.Errorf("scrollY = %f, want 0", scrollY)
	}
}

func TestClampScroll_ExceedsMax(t *testing.T) {
	resetGlobals()
	cachedTotalH = 500
	winH = 200
	scrollY = 400
	clampScroll()
	if scrollY != 300 {
		t.Errorf("scrollY = %f, want 300", scrollY)
	}
}

func TestClampScroll_WithinRange(t *testing.T) {
	resetGlobals()
	cachedTotalH = 500
	winH = 200
	scrollY = 150
	clampScroll()
	if scrollY != 150 {
		t.Errorf("scrollY = %f, want 150 (unchanged)", scrollY)
	}
}

func TestClampScroll_ExactlyAtMax(t *testing.T) {
	resetGlobals()
	cachedTotalH = 500
	winH = 200
	scrollY = 300
	clampScroll()
	if scrollY != 300 {
		t.Errorf("scrollY = %f, want 300 (at max)", scrollY)
	}
}

func TestClampScroll_ZeroContent(t *testing.T) {
	resetGlobals()
	cachedTotalH = 0
	winH = 200
	scrollY = 10
	clampScroll()
	if scrollY != 0 {
		t.Errorf("scrollY = %f, want 0 (no content)", scrollY)
	}
}

// --- thread safety ---

func TestScrollConcurrency(t *testing.T) {
	resetGlobals()
	cachedTotalH = 10000
	winH = 500

	var wg sync.WaitGroup

	// Simulate UI thread scrolling.
	wg.Go(func() {
		for i := 0; i < 1000; i++ {
			mu.Lock()
			scrollY += 1
			clampScroll()
			mu.Unlock()
		}
	})

	// Simulate GL thread reading.
	wg.Go(func() {
		for i := 0; i < 1000; i++ {
			mu.Lock()
			_ = scrollY
			mu.Unlock()
		}
	})

	wg.Wait()

	mu.Lock()
	sy := scrollY
	mu.Unlock()
	if sy < 0 || sy > 9500 {
		t.Errorf("scrollY = %f, out of valid range", sy)
	}
}

func TestScrollAndClampConcurrency(t *testing.T) {
	resetGlobals()
	cachedTotalH = 100
	winH = 80

	var wg sync.WaitGroup

	// Writer: scroll up repeatedly.
	wg.Go(func() {
		for i := 0; i < 500; i++ {
			mu.Lock()
			scrollY += 5
			clampScroll()
			mu.Unlock()
		}
	})

	// Writer: scroll down repeatedly.
	wg.Go(func() {
		for i := 0; i < 500; i++ {
			mu.Lock()
			scrollY -= 5
			clampScroll()
			mu.Unlock()
		}
	})

	wg.Wait()

	mu.Lock()
	sy := scrollY
	mu.Unlock()
	// Max scroll = 100 - 80 = 20.
	if sy < 0 || sy > 20 {
		t.Errorf("scrollY = %f, want [0, 20]", sy)
	}
}
