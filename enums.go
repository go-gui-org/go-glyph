package glyph

// Alignment specifies horizontal text alignment within the layout box.
type Alignment int

const (
	AlignLeft   Alignment = iota // Left-aligned (default).
	AlignCenter                  // Center-aligned.
	AlignRight                   // Right-aligned.
)

// WrapMode defines how text wraps when exceeding max width.
type WrapMode int

const (
	WrapNone     WrapMode = -1 // Do not wrap, even when a width is set.
	WrapWord     WrapMode = 0  // Wrap at word boundaries (PANGO_WRAP_WORD).
	WrapChar     WrapMode = 1  // Wrap at character boundaries (PANGO_WRAP_CHAR).
	WrapWordChar WrapMode = 2  // Wrap at word, fallback to char (PANGO_WRAP_WORD_CHAR).
)

// TextOrientation defines the flow direction of text.
type TextOrientation int

const (
	OrientationHorizontal TextOrientation = iota
	OrientationVertical                   // Vertical flow, upright chars (CJK).
)

// GradientDirection controls the axis of color interpolation.
type GradientDirection int

const (
	GradientHorizontal GradientDirection = iota // Left to right.
	GradientVertical                            // Top to bottom.
	GradientDiagonal                            // Top-left to bottom-right.
)

// Typeface specifies bold/italic style programmatically.
type Typeface int

const (
	TypefaceRegular    Typeface = iota // Default — preserves font_name style.
	TypefaceBold                       // Override to bold.
	TypefaceItalic                     // Override to italic.
	TypefaceBoldItalic                 // Override to bold+italic.
)

// CompositionPhase tracks IME preedit state.
type CompositionPhase int

const (
	CompositionNone CompositionPhase = iota
	CompositionStarted
	CompositionUpdating
	CompositionCommitted
)

// ClauseStyle identifies the visual style of an IME clause.
type ClauseStyle int

const (
	ClauseRaw       ClauseStyle = iota // Unconverted input.
	ClauseConverted                    // Converted but not selected.
	ClauseSelected                     // Currently selected clause.
)

// OperationType identifies the kind of undo operation.
type OperationType int

const (
	OpInsert OperationType = iota
	OpDelete
	OpReplace
)
