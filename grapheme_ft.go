//go:build android || (linux)

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
// rivo/uniseg for UAX #29 grapheme cluster segmentation.
func segmentGraphemes(text string) []graphemeCluster {
	if len(text) == 0 {
		return nil
	}
	clusters := make([]graphemeCluster, 0,
		utf8.RuneCountInString(text))
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
