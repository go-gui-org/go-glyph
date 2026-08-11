package glyph

// TextConfig holds configuration for text layout and rendering.
type TextConfig struct {
	Style        TextStyle
	Gradient     *GradientConfig // nil = no gradient.
	Block        BlockStyle
	Orientation  TextOrientation
	UseMarkup    bool
	NoHitTesting bool
}

// TextStyle represents the visual style of a run of text.
type TextStyle struct {
	Features *FontFeatures
	Object   *InlineObject
	// FontName is a Pango font description string, e.g. "Sans Italic Light 15".
	FontName string
	// Typeface overrides weight/style in FontName when not TypefaceRegular.
	Typeface Typeface
	// Size overrides the size in FontName (points). 0 = use FontName.
	Size float32
	// LetterSpacing is extra spacing between characters (points).
	LetterSpacing float32

	// EmojiBoxWidth, when > 0, is the target box width (logical px) that a
	// color/emoji glyph should fill. Grid callers (terminals) set it to
	// cells×cellWidth so emoji scale to fill their reserved cell box —
	// preserving aspect, centered — instead of the font's narrower natural
	// emoji advance. 0 keeps the default advance-clamped sizing.
	//
	// A rich-text run that leaves this at 0 inherits the value from
	// TextConfig.Style, so the box is declared once for the whole layout.
	EmojiBoxWidth float32

	// CellWidth and CellHeight are the grid cell size in logical px. Grid
	// callers (terminals) set their real cell so the built-in box-drawing
	// and block-element glyphs fill it exactly and neighbouring cells abut.
	// 0 derives the cell from the glyph advance and the run's
	// ascent+descent, which still gives a uniform stroke weight but leaves
	// a sub-pixel overlap wherever the advance is fractional.
	//
	// A rich-text run that leaves either at 0 inherits it from
	// TextConfig.Style, so the cell is declared once for the whole layout.
	CellWidth  float32
	CellHeight float32

	// StrokeWidth is outline width in points (0 = no stroke).
	StrokeWidth float32
	Color       Color
	// BgColor is the background highlight color behind the text run.
	BgColor Color

	StrokeColor Color

	Underline     bool
	Strikethrough bool

	// NoBuiltinBoxGlyphs keeps the font's own glyph for U+2500–257F and
	// U+2580–259F instead of the built-in cell-sized rasterization. Set it
	// for proportional text, or when the font's box art is preferred.
	//
	// Every backend draws the same geometry: native backends rasterize it
	// into the atlas, and WASM replays it as Canvas2D fills and paths. The
	// built-in path is skipped for stroked runs (StrokeWidth > 0), and on
	// WASM also for layouts drawn under a rotation or skew, where snapping
	// the cell to the pixel grid means nothing. U+E0B0–E0B3 (Powerline)
	// are synthesized only when the font reports no glyph, which Canvas2D
	// cannot do, so on WASM they always come from the font.
	//
	// The flag has no unset sentinel, so rich-text runs OR it with
	// TextConfig.Style: setting it on the base style opts out every run, and
	// a run cannot opt back in.
	NoBuiltinBoxGlyphs bool
}

// BlockStyle defines paragraph-level layout properties.
type BlockStyle struct {
	Tabs  []int
	Align Alignment
	Wrap  WrapMode
	// Width is the wrapping width. -1 = no wrapping.
	Width float32
	// Indent determines first-line indentation. Negative = hanging indent.
	Indent float32
	// LineSpacing adds extra vertical space after each line except the last.
	LineSpacing float32
}

// DefaultBlockStyle returns a BlockStyle with standard defaults.
func DefaultBlockStyle() BlockStyle {
	return BlockStyle{
		Align: AlignLeft,
		Wrap:  WrapWord,
		Width: -1,
	}
}

// FontFeature represents an OpenType feature tag and value.
type FontFeature struct {
	Tag   string
	Value int
}

// FontAxis represents a variable font axis tag and value.
type FontAxis struct {
	Tag   string
	Value float32
}

// FontFeatures holds OpenType features and variable font axes.
type FontFeatures struct {
	OpenTypeFeatures []FontFeature
	VariationAxes    []FontAxis
}

// InlineObject represents an embedded non-text element in a layout.
type InlineObject struct {
	ID     string
	Width  float32 // Points.
	Height float32
	Offset float32 // Baseline offset.
}

// StyleRun is a text segment with its own style.
type StyleRun struct {
	Style TextStyle
	Text  string
}

// RichText is a sequence of styled runs.
type RichText struct {
	Runs []StyleRun
}

// TextMetrics contains metrics for a specific font configuration.
// All values are in pixels.
type TextMetrics struct {
	Ascender  float32
	Descender float32
	Height    float32
	LineGap   float32
	// LineHeight is the recommended baseline-to-baseline advance for stacking
	// lines: Ascender+Descender+LineGap, floored to a minimum ratio of the em
	// so fonts with no line-gap hint do not render cramped. Prefer this over
	// Height when laying out multiple lines.
	LineHeight float32
}
