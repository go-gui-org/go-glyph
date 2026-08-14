package glyph

// InkBounds returns the ink (painted) bounding box of text laid out with
// cfg, in the same coordinate space DrawText uses: X/Y are offsets from
// the layout origin (the top-left of the line box), Y growing downward.
//
// The metric box a layout reports (Width/Height) is the advance box: it
// spans the font's ascent and descent and every glyph's side bearings,
// whether or not the glyph paints there. Centering a single glyph — an
// icon, a check mark — on that box therefore leaves it visibly off
// centre, because most glyphs paint no descender and have asymmetric
// bearings. Centre on this box instead.
//
// ok is false when the bounds cannot be measured (no layout, vertical
// orientation, or a face whose outlines cannot be read); callers fall
// back to the advance box.
func (ts *TextSystem) InkBounds(text string, cfg TextConfig) (Rect, bool) {
	if ts == nil || ts.ctx == nil {
		return Rect{}, false
	}
	if cfg.Orientation == OrientationVertical {
		return Rect{}, false
	}
	item, err := ts.getOrCreateLayout(text, cfg)
	if err != nil || item == nil {
		return Rect{}, false
	}
	return ts.ctx.LayoutInkBounds(&item.layout)
}
