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

	// StrokeWidth is outline width in points (0 = no stroke).
	StrokeWidth float32
	Color       Color
	// BgColor is the background highlight color behind the text run.
	BgColor Color

	StrokeColor Color

	Underline     bool
	Strikethrough bool
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
}
