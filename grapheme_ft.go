//go:build android || linux || darwin || windows

package glyph

import (
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// graphemeCluster represents one user-perceived character.
type graphemeCluster struct {
	text  string
	byteI int
	byteL int
}

// segmentGraphemes splits text into grapheme clusters using
// rivo/uniseg for UAX #29 grapheme cluster segmentation. The result is
// written into dst (truncated first; may be nil), reusing its backing array
// when capacity suffices. Callers pass the Context's scratch buffer so
// repeated layout calls do not reallocate the cluster slice.
func segmentGraphemes(dst []graphemeCluster, text string) []graphemeCluster {
	// Truncate up front so a caller passing a non-empty slice can never get
	// stale elements prepended to the new segmentation, and so empty text
	// hands the scratch's backing array straight back instead of dropping it.
	clusters := dst[:0]
	if len(text) == 0 {
		return clusters
	}
	if need := utf8.RuneCountInString(text); cap(clusters) < need {
		clusters = make([]graphemeCluster, 0, need)
	}
	gr := uniseg.NewGraphemes(text)
	byteIdx := 0
	for gr.Next() {
		s := gr.Str()
		clusters = append(clusters, graphemeCluster{
			text:  s,
			byteI: byteIdx,
			byteL: len(s),
		})
		byteIdx += len(s)
	}
	return clusters
}

// glyphText extracts the original cluster text for a glyph.
// Index stores byte offset, Codepoint stores byte length.
func glyphText(text string, g Glyph) string {
	start := int(g.Index)
	end := start + int(g.Codepoint)
	if start >= 0 && end <= len(text) {
		return text[start:end]
	}
	return ""
}
