//go:build linux || darwin

package glyph

import "testing"

// TestLayoutEquivalenceFT runs the shared cross-platform layout
// invariant suite against the FreeType+HarfBuzz backend, mirroring the
// Pango (layout_test.go) and CoreText (layout_darwin_test.go) runs.
func TestLayoutEquivalenceFT(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Free()

	runLayoutEquivCases(t, LayoutEquivCases(), ctx.LayoutText)
}
