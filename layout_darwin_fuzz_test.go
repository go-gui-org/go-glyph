//go:build darwin && !glyph_pango

package glyph

import (
	"testing"
	"unicode/utf8"
)

func FuzzBuildUTF16ToByteSlice(f *testing.F) {
	seeds := []string{"", "a", "é", "\U0001F600", "a\U0001F600b", "Hello, 世界"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		m := buildUTF16ToByteSlice(text)

		// Invariant: always at least one element (the sentinel).
		if len(m) == 0 {
			t.Fatal("buildUTF16ToByteSlice returned empty slice")
		}

		// Invariant: last element equals len(text).
		if m[len(m)-1] != len(text) {
			t.Errorf("sentinel: got %d, want len(text)=%d", m[len(m)-1], len(text))
		}

		// Invariant: non-decreasing (UTF-16 offsets are monotonic).
		for i := 1; i < len(m); i++ {
			if m[i] < m[i-1] {
				t.Errorf("non-monotonic at %d: %v", i, m)
				break
			}
		}

		// Invariant: first element is 0.
		if m[0] != 0 {
			t.Errorf("first element = %d, want 0", m[0])
		}

		// Invariant: all indices are within [0, len(text)].
		for i, v := range m {
			if v < 0 || v > len(text) {
				t.Errorf("index %d out of bounds: m[%d] = %d, len(text)=%d",
					v, i, v, len(text))
			}
		}

		// For valid UTF-8, verify surrogate pairs produce duplicate indices.
		if utf8.ValidString(text) {
			for i := 0; i < len(text); {
				r, sz := utf8.DecodeRuneInString(text[i:])
				if r > 0xFFFF {
					// Should produce two entries at same byte offset.
					// Find two consecutive equal entries starting at i.
					found := 0
					for _, v := range m {
						if v == i {
							found++
						}
					}
					if found != 2 {
						t.Errorf("surrogate pair at byte %d: got %d entries, want 2", i, found)
					}
				}
				i += sz
			}
		}
	})
}
