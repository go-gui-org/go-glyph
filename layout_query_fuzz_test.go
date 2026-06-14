package glyph

import (
	"testing"
)

func FuzzLayoutMoveCursorLeft(f *testing.F) {
	l := testLayout()
	f.Add(3)
	f.Add(0)
	f.Add(len(l.Text))
	f.Fuzz(func(t *testing.T, byteIndex int) {
		r := l.MoveCursorLeft(byteIndex)
		if r > byteIndex {
			t.Errorf("MoveCursorLeft(%d) = %d, moved right", byteIndex, r)
		}
		if r < 0 {
			t.Errorf("MoveCursorLeft(%d) = %d, negative", byteIndex, r)
		}
	})
}

func FuzzLayoutMoveCursorRight(f *testing.F) {
	l := testLayout()
	f.Add(3)
	f.Add(0)
	f.Add(len(l.Text))
	f.Fuzz(func(t *testing.T, byteIndex int) {
		r := l.MoveCursorRight(byteIndex)
		if r < byteIndex {
			t.Errorf("MoveCursorRight(%d) = %d, moved left", byteIndex, r)
		}
		if r < 0 || r > len(l.Text) {
			t.Errorf("MoveCursorRight(%d) = %d, out of range [0,%d]",
				byteIndex, r, len(l.Text))
		}
	})
}

func FuzzLayoutHitTest(f *testing.F) {
	l := testLayout()
	f.Add(float32(15), float32(5))
	f.Add(float32(-1), float32(-1))
	f.Add(float32(100), float32(100))
	f.Fuzz(func(t *testing.T, x, y float32) {
		idx := l.HitTest(x, y)
		if idx < -1 || idx > len(l.Text) {
			t.Errorf("HitTest(%f,%f) = %d, out of range [-1,%d]",
				x, y, idx, len(l.Text))
		}
	})
}

func FuzzLayoutGetClosestOffset(f *testing.F) {
	l := testLayout()
	f.Add(float32(35), float32(10))
	f.Add(float32(-10), float32(-10))
	f.Fuzz(func(t *testing.T, x, y float32) {
		idx := l.GetClosestOffset(x, y)
		if idx < 0 || idx > len(l.Text) {
			t.Errorf("GetClosestOffset(%f,%f) = %d, out of range [0,%d]",
				x, y, idx, len(l.Text))
		}
	})
}

func FuzzLayoutMoveCursorWordLeft(f *testing.F) {
	l := testLayout()
	f.Add(8)
	f.Add(0)
	f.Add(len(l.Text))
	f.Fuzz(func(t *testing.T, byteIndex int) {
		r := l.MoveCursorWordLeft(byteIndex)
		if r < 0 || r > len(l.Text) {
			t.Errorf("MoveCursorWordLeft(%d) = %d, out of range [0,%d]",
				byteIndex, r, len(l.Text))
		}
	})
}

func FuzzLayoutMoveCursorWordRight(f *testing.F) {
	l := testLayout()
	f.Add(0)
	f.Add(len(l.Text))
	f.Fuzz(func(t *testing.T, byteIndex int) {
		r := l.MoveCursorWordRight(byteIndex)
		if r < 0 || r > len(l.Text) {
			t.Errorf("MoveCursorWordRight(%d) = %d, out of range [0,%d]",
				byteIndex, r, len(l.Text))
		}
	})
}
