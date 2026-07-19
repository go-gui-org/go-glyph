//go:build android

package glyph

import "testing"

func TestSegmentGraphemes_ASCII(t *testing.T) {
	clusters := segmentGraphemes(nil, "hello")
	if len(clusters) != 5 {
		t.Fatalf("got %d clusters, want 5", len(clusters))
	}
	for i, want := range []string{"h", "e", "l", "l", "o"} {
		if clusters[i].text != want {
			t.Errorf("cluster[%d] = %q, want %q",
				i, clusters[i].text, want)
		}
	}
}

func TestSegmentGraphemes_Empty(t *testing.T) {
	clusters := segmentGraphemes(nil, "")
	if len(clusters) != 0 {
		t.Fatalf("got %d clusters for empty string", len(clusters))
	}
}

func TestSegmentGraphemes_Emoji(t *testing.T) {
	// Family emoji: should be 1 grapheme cluster.
	text := "\U0001F468\u200D\U0001F469\u200D\U0001F467"
	clusters := segmentGraphemes(nil, text)
	if len(clusters) != 1 {
		t.Errorf("got %d clusters for family emoji, want 1",
			len(clusters))
	}
}

func TestSegmentGraphemes_ByteOffsets(t *testing.T) {
	clusters := segmentGraphemes(nil, "aé")
	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2", len(clusters))
	}
	if clusters[0].byteI != 0 || clusters[0].byteL != 1 {
		t.Errorf("cluster[0]: byteI=%d byteL=%d, want 0/1",
			clusters[0].byteI, clusters[0].byteL)
	}
	if clusters[1].byteI != 1 || clusters[1].byteL != 2 {
		t.Errorf("cluster[1]: byteI=%d byteL=%d, want 1/2",
			clusters[1].byteI, clusters[1].byteL)
	}
}

func TestGlyphText(t *testing.T) {
	text := "hello"
	g := Glyph{Index: 1, Codepoint: 3}
	got := glyphText(text, g)
	if got != "ell" {
		t.Errorf("glyphText = %q, want %q", got, "ell")
	}
}

func TestGlyphText_OutOfBounds(t *testing.T) {
	text := "hi"
	g := Glyph{Index: 5, Codepoint: 2}
	got := glyphText(text, g)
	if got != "" {
		t.Errorf("glyphText out of bounds = %q, want empty", got)
	}
}
