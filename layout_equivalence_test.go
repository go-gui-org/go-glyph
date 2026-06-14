package glyph

import "testing"

// layoutEquivCase is a shared table-driven layout invariant test.
// The check function receives the Layout result and asserts invariants
// that should hold on all platforms (Pango, CoreText, DirectWrite, etc.).
type layoutEquivCase struct {
	name  string
	text  string
	cfg   TextConfig
	check func(t *testing.T, l Layout)
}

// runLayoutEquivCases runs shared table-driven layout invariant tests.
// factory is ctx.LayoutText (platform-specific).
func runLayoutEquivCases(t *testing.T, cases []layoutEquivCase,
	factory func(string, TextConfig) (Layout, error)) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := factory(tc.text, tc.cfg)
			if err != nil {
				t.Fatalf("LayoutText: %v", err)
			}
			tc.check(t, l)
		})
	}
}

// LayoutEquivCases returns shared layout invariant tests that must pass
// on all platforms.
func LayoutEquivCases() []layoutEquivCase {
	return []layoutEquivCase{
		{
			name: "line_count_simple",
			text: "Hello\nWorld",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				if len(l.Lines) < 2 {
					t.Errorf("expected at least 2 lines, got %d", len(l.Lines))
				}
			},
		},
		{
			name: "line_count_single",
			text: "Hello",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				if len(l.Lines) < 1 {
					t.Error("expected at least 1 line")
				}
			},
		},
		{
			name: "line_count_wrapped",
			text: "This is a very long line of text that should wrap",
			cfg: TextConfig{
				Style: TextStyle{FontName: "Sans 20"},
				Block: BlockStyle{Width: 100, Wrap: WrapWord},
			},
			check: func(t *testing.T, l Layout) {
				if len(l.Lines) < 2 {
					t.Errorf("wrapped text should produce >= 2 lines, got %d", len(l.Lines))
				}
			},
		},
		{
			name: "width_height_positive",
			text: "Hello",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				if l.Width <= 0 {
					t.Errorf("Width = %f, want > 0", l.Width)
				}
				if l.Height <= 0 {
					t.Errorf("Height = %f, want > 0", l.Height)
				}
				if l.VisualWidth <= 0 {
					t.Errorf("VisualWidth = %f, want > 0", l.VisualWidth)
				}
				if l.VisualHeight <= 0 {
					t.Errorf("VisualHeight = %f, want > 0", l.VisualHeight)
				}
			},
		},
		{
			name: "cursor_positions_present",
			text: "Hello",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				if len(l.LogAttrs) != len(l.Text)+1 {
					t.Errorf("LogAttrs len = %d, want %d", len(l.LogAttrs), len(l.Text)+1)
				}
				if len(l.LogAttrByIndex) != len(l.Text)+1 {
					t.Errorf("LogAttrByIndex len = %d, want %d",
						len(l.LogAttrByIndex), len(l.Text)+1)
				}
			},
		},
		{
			name: "char_rects_count",
			text: "Hello",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				if len(l.CharRects) == 0 {
					t.Error("CharRects should not be empty")
				}
				if len(l.CharRects) > len(l.Text) {
					t.Errorf("CharRects len %d > text len %d", len(l.CharRects), len(l.Text))
				}
			},
		},
		{
			name: "empty_text",
			text: "",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				// Empty text should produce empty or minimal result.
				if len(l.Lines) > 0 {
					t.Logf("empty text produced %d lines (platform-dependent)", len(l.Lines))
				}
				// Must not have more items than makes sense.
				if len(l.Items) > 1 {
					t.Errorf("empty text produced %d items", len(l.Items))
				}
			},
		},
		{
			name: "glyph_positions_nonempty",
			text: "AB",
			cfg:  TextConfig{Style: TextStyle{FontName: "Sans 20"}},
			check: func(t *testing.T, l Layout) {
				pos := l.GlyphPositions()
				if len(pos) < 2 {
					t.Errorf("glyph positions = %d, want >= 2", len(pos))
				}
			},
		},
	}
}
