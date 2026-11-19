//go:build linux || darwin || windows

package glyph

import (
	"math"
	"testing"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
)

func TestTypoFromOS2(t *testing.T) {
	const upem = 2048.0
	segoeHhea := 2210.0 + 514 // hhea asc + desc, gap 0
	os2 := func(asc, desc, gap int16, fs uint16) tables.Os2 {
		return tables.Os2{STypoAscender: asc, STypoDescender: desc,
			STypoLineGap: gap, FsSelection: fs}
	}
	tests := []struct {
		name string
		os2  tables.Os2
		hhea float64
		want typoMetrics
	}{
		{"segoe ui", os2(1491, -431, 269, 0), segoeHhea,
			typoMetrics{asc: 1491, desc: 431, gap: 269, ok: true}},
		{"negative gap clamps", os2(1491, -431, -50, 0), segoeHhea,
			typoMetrics{asc: 1491, desc: 431, gap: 0, ok: true}},
		{"no hhea", os2(1491, -431, 269, 0), math.Inf(1),
			typoMetrics{asc: 1491, desc: 431, gap: 269, ok: true}},
		{"use typo metrics set", os2(1491, -431, 269, fsUseTypoMetrics), segoeHhea,
			typoMetrics{}},
		{"zero ascent", os2(0, -431, 0, 0), segoeHhea, typoMetrics{}},
		{"descender above baseline", os2(1491, 100, 0, 0), segoeHhea, typoMetrics{}},
		{"placeholder box", os2(1000, -200, 0, 0), segoeHhea, typoMetrics{}},
		{"not tighter than hhea", os2(2000, -700, 100, 0), segoeHhea, typoMetrics{}},
		{"equal to hhea", os2(2210, -514, 0, 0), segoeHhea, typoMetrics{}},
		{"nan hhea", os2(1491, -431, 269, 0), math.NaN(), typoMetrics{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := typoFromOS2(tc.os2, tc.hhea, upem); got != tc.want {
				t.Errorf("typoFromOS2 = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestMetricsTypoLineBox verifies metrics() builds the line box from the
// face's typo metrics, scaled to size, with leading folded in.
func TestMetricsTypoLineBox(t *testing.T) {
	cf := &cachedFace{upem: 2048,
		typo: typoMetrics{asc: 1491, desc: 431, gap: 269, ok: true}}
	// The typo branch never reads the face; it only has to be non-nil.
	f := ftFont{cf: cf, face: &font.Face{}, size: 16, upem: 2048}
	a, d, l := f.metrics()
	if math.Abs(a+d-minLineHeightRatio*16) > 1e-9 || l != 0 || math.Abs(a-13.340625) > 1e-6 {
		t.Errorf("metrics = (%v, %v, %v), want height %v, ascent 13.340625, leading 0",
			a, d, l, minLineHeightRatio*16)
	}
}
