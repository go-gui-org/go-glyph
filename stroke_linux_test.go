//go:build linux && !android

package glyph

import (
	"os"
	"testing"
)

// findAFont returns a usable TTF path from common Linux locations.
func findAFont() string {
	cands := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
	}
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func alphaCoverage(r *rasterResult) int {
	if r == nil {
		return 0
	}
	n := 0
	for i := 3; i < len(r.data); i += 4 {
		if r.data[i] > 0 {
			n++
		}
	}
	return n
}

func TestStrokerProducesBand(t *testing.T) {
	fp := findAFont()
	if fp == "" {
		t.Skip("no system font found")
	}
	const size, sub = 48.0, 0.0
	fill, _ := renderMonoRun(fp, size, sub, "H", "", 0, false)
	stroke, _ := renderStrokedRun(fp, size, 3.0, sub, "H", "", 0, false)
	if fill == nil || stroke == nil {
		t.Fatalf("nil raster: fill=%v stroke=%v", fill == nil, stroke == nil)
	}
	// Stroke radius 3 must enlarge the bitmap and add ink around the glyph.
	if stroke.w <= fill.w || stroke.h <= fill.h {
		t.Errorf("stroke not larger: fill=%dx%d stroke=%dx%d",
			fill.w, fill.h, stroke.w, stroke.h)
	}
	fc, sc := alphaCoverage(fill), alphaCoverage(stroke)
	if sc <= fc {
		t.Errorf("stroke ink (%d) not greater than fill ink (%d)", sc, fc)
	}
	t.Logf("fill=%dx%d ink=%d  stroke=%dx%d ink=%d",
		fill.w, fill.h, fc, stroke.w, stroke.h, sc)
}
