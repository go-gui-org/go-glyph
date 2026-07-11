//go:build android || linux || darwin || windows

package glyph

import "testing"

func TestComputeRunText(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		start        int
		length       int
		glyphCount   int
		gIndex       uint32
		wantRun      string
		wantTargetID int
	}{
		{
			name:   "zero length yields no context",
			text:   "hello",
			start:  0,
			length: 0, glyphCount: 3, gIndex: 0,
			wantRun: "", wantTargetID: 0,
		},
		{
			name:  "negative start yields no context",
			text:  "hello",
			start: -1, length: 5, glyphCount: 3, gIndex: 0,
			wantRun: "", wantTargetID: 0,
		},
		{
			name:  "end past text yields no context",
			text:  "hi",
			start: 0, length: 10, glyphCount: 3, gIndex: 0,
			wantRun: "", wantTargetID: 0,
		},
		{
			name:  "single-glyph item needs no context",
			text:  "hello",
			start: 0, length: 5, glyphCount: 1, gIndex: 2,
			wantRun: "", wantTargetID: 0,
		},
		{
			name:  "multi-glyph LTR run",
			text:  "hello",
			start: 0, length: 5, glyphCount: 5, gIndex: 2,
			wantRun: "hello", wantTargetID: 2,
		},
		{
			name:  "sub-run offset within larger text",
			text:  "XYhelloZ",
			start: 2, length: 5, glyphCount: 5, gIndex: 4,
			wantRun: "hello", wantTargetID: 2,
		},
		{
			// "café": é is a 2-byte rune starting at byte offset 3, so its
			// rune index within the run is 3.
			name:  "multi-byte rune index",
			text:  "café",
			start: 0, length: 5, glyphCount: 4, gIndex: 3,
			wantRun: "café", wantTargetID: 3,
		},
		{
			name:  "glyph index past run end falls back to isolated shaping",
			text:  "hello",
			start: 0, length: 5, glyphCount: 5, gIndex: 99,
			wantRun: "", wantTargetID: 0,
		},
		{
			name:  "glyph index before run start falls back to isolated shaping",
			text:  "hello",
			start: 3, length: 2, glyphCount: 5, gIndex: 1,
			wantRun: "", wantTargetID: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := Item{
				StartIndex: tc.start,
				Length:     tc.length,
				GlyphCount: tc.glyphCount,
			}
			g := Glyph{Index: tc.gIndex}
			gotRun, gotTarget := computeRunText(tc.text, item, g)
			if gotRun != tc.wantRun || gotTarget != tc.wantTargetID {
				t.Errorf("computeRunText = (%q, %d), want (%q, %d)",
					gotRun, gotTarget, tc.wantRun, tc.wantTargetID)
			}
		})
	}
}
