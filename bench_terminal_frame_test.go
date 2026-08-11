package glyph

import "testing"

// BenchmarkTerminalFrame1500 reproduces the issue #92 measurement: a
// terminal frame issues ~1500 per-glyph DrawLayout calls (1-3 glyphs
// each) over an 80x25 cell grid, all at integer cell positions (subpixel
// bin 0). Steady state = glyph cache warm; the issue target is 0
// allocs/op after the scratch-buffer fix (was ~3000/frame).
func BenchmarkTerminalFrame1500(b *testing.B) {
	benchTerminalFrame(b, []string{"a", "e", "fg", "hi", "l", "m", "n", "o",
		"p", "rs", "t", "uv", "w", "x", "y", "z", " "}, TextConfig{})
}

// BenchmarkTerminalFrameBoxDrawing is the same frame drawn from the
// codepoints the built-in rasterizer owns (issue #99), with the grid
// caller's real cell declared. Same target: 0 allocs/op in steady state.
func BenchmarkTerminalFrameBoxDrawing(b *testing.B) {
	benchTerminalFrame(b, []string{"─", "│", "┌", "┐", "└", "┘", "├", "┤",
		"┬", "┴", "┼", "═", "║", "╔", "╬", "█", "▀", "▄", "░", "▒", "╭"},
		TextConfig{Style: TextStyle{CellWidth: 12, CellHeight: 16}})
}

func benchTerminalFrame(b *testing.B, cellTexts []string, cfg TextConfig) {
	b.Helper()
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Fatal(err)
	}
	defer ctx.Free()

	const cells = 1500
	layouts := make([]Layout, cells)
	xs := make([]float32, cells)
	ys := make([]float32, cells)
	for i := range layouts {
		l, err := ctx.LayoutText(cellTexts[i%len(cellTexts)], cfg)
		if err != nil {
			b.Fatal(err)
		}
		layouts[i] = l
		xs[i] = float32((i % 80) * 12)
		ys[i] = float32((i / 80) * 16)
	}
	if len(layouts[0].Items) == 0 || layouts[0].Items[0].FontPath == "" {
		b.Skip("no font path resolved on this host")
	}

	r, err := NewRenderer(newMockBackend(), 1.0)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Free()

	// Warm the glyph cache: one full frame, then Commit (uploads happen
	// at Commit, so the warm frame also publishes the atlas pages).
	for j := range layouts {
		r.DrawLayout(layouts[j], xs[j], ys[j])
	}
	r.Commit()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range layouts {
			r.DrawLayout(layouts[j], xs[j], ys[j])
		}
		r.Commit()
	}
}
