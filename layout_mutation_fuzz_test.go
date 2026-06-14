package glyph

import (
	"testing"
)

func FuzzInsertText(f *testing.F) {
	f.Add("Hello", 5, " World")
	f.Add("", 0, "x")
	f.Add("abc", 3, "")
	f.Fuzz(func(t *testing.T, text string, cursor int, insert string) {
		r := InsertText(text, cursor, insert)
		if r.CursorPos < 0 {
			t.Error("cursor < 0")
		}
		c := clampIndex(cursor, len(text))
		want := text[:c] + insert + text[c:]
		if r.NewText != want {
			t.Errorf("NewText mismatch: got %q, want %q", r.NewText, want)
		}
	})
}

func FuzzDeleteSelection(f *testing.F) {
	f.Add("Hello World", 5, 11)
	f.Add("abc", 0, 3)
	f.Fuzz(func(t *testing.T, text string, cursor, anchor int) {
		r := DeleteSelection(text, cursor, anchor)
		if r.CursorPos < 0 || r.CursorPos > len(r.NewText) {
			t.Error("cursor out of bounds")
		}
		// Compute expected: the deleted range is [min(c,a), max(c,a)).
		c := clampIndex(cursor, len(text))
		a := clampIndex(anchor, len(text))
		if c == a {
			if r.NewText != text {
				t.Error("no selection should return original text")
			}
			return
		}
		lo, hi := c, a
		if lo > hi {
			lo, hi = hi, lo
		}
		want := text[:lo] + text[hi:]
		if r.NewText != want {
			t.Errorf("NewText mismatch: got %q, want %q", r.NewText, want)
		}
		if r.DeletedText != text[lo:hi] {
			t.Errorf("DeletedText mismatch: got %q, want %q",
				r.DeletedText, text[lo:hi])
		}
	})
}

func FuzzGetSelectedText(f *testing.F) {
	f.Add("Hello World", 6, 11)
	f.Fuzz(func(t *testing.T, text string, cursor, anchor int) {
		_ = GetSelectedText(text, cursor, anchor)
	})
}

func FuzzCutSelection(f *testing.F) {
	f.Add("Hello World", 0, 5)
	f.Fuzz(func(t *testing.T, text string, cursor, anchor int) {
		// Verify no panic on adversarial inputs.
		_, _ = CutSelection(text, cursor, anchor)
	})
}

func FuzzDeleteBackward(f *testing.F) {
	l := testLayout()
	f.Add(3)
	f.Add(0)
	f.Add(len(l.Text))
	f.Fuzz(func(t *testing.T, cursor int) {
		r := DeleteBackward(l.Text, l, cursor)
		if r.CursorPos > len(r.NewText) {
			t.Errorf("cursor past end: pos=%d len=%d", r.CursorPos, len(r.NewText))
		}
	})
}

func FuzzDeleteForward(f *testing.F) {
	l := testLayout()
	f.Add(3)
	f.Add(0)
	f.Add(len(l.Text))
	f.Fuzz(func(t *testing.T, cursor int) {
		r := DeleteForward(l.Text, l, cursor)
		if r.CursorPos > len(r.NewText) {
			t.Errorf("cursor past end: pos=%d len=%d", r.CursorPos, len(r.NewText))
		}
	})
}
