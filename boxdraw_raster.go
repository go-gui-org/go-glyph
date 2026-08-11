//go:build linux || darwin || windows

package glyph

// loadBoxGlyphFT rasterizes a built-in box-drawing, block-element or
// Powerline glyph and inserts it into the atlas. The bitmap is exactly the
// cell box, with a zero left bearing and a top bearing that seats the cell
// bottom on baseline+descent, so the emitted quad covers the cell and
// nothing else.
func loadBoxGlyphFT(atlas *GlyphAtlas, m boxMetrics) (LoadGlyphResult, error) {
	if m.cellW < 1 || m.cellH < 1 {
		return LoadGlyphResult{}, nil
	}
	if _, err := checkAllocationSize(m.cellW, m.cellH, 4); err != nil {
		return LoadGlyphResult{}, err
	}

	// The scratch buffers are grow-only and reused across calls, so this
	// path allocates nothing in steady state. ensureAlpha does not zero, and
	// drawBoxGlyph only ever raises coverage, so clear first.
	cov := atlas.ensureAlpha(m.cellW, m.cellH).Pix
	clear(cov)
	drawBoxGlyph(cov, m)

	// Same expansion the font path uses: white RGB with coverage in alpha.
	data := atlas.ensureRGBA(m.cellW, m.cellH).Pix
	for i, a := range cov {
		o := i * 4
		data[o+0] = 255
		data[o+1] = 255
		data[o+2] = 255
		data[o+3] = a
	}

	return insertRaster(atlas, &rasterResult{
		data: data,
		w:    m.cellW,
		h:    m.cellH,
		left: 0,
		top:  m.top,
	})
}
