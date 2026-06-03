package glyph

import "unsafe"

// PangoGlyphUnknownFlag is the flag bit set on glyph indices that
// Pango could not map to the font.
const PangoGlyphUnknownFlag = 0x10000000

// Layout is the result of text shaping. It contains positioned glyph
// runs, hit-test rectangles, line boundaries, and cursor attributes.
type Layout struct {
	CharRectByIndex map[int]int // byte index → CharRects index
	LogAttrByIndex  map[int]int // byte index → LogAttrs index
	Text            string
	ClonedObjectIDs []string
	Items           []Item
	Glyphs          []Glyph
	CharRects       []CharRect
	Lines           []Line
	LogAttrs        []LogAttr

	// Pre-sorted cursor/word boundary caches, built once.
	cursorPositions []int   // Sorted valid cursor byte indices.
	wordStarts      []int   // Sorted word-start byte indices.
	wordEnds        []int   // Sorted word-end byte indices.
	Width           float32 // Logical width.
	Height          float32 // Logical height.
	VisualWidth     float32 // Ink width.
	VisualHeight    float32 // Ink height.

}

// CursorPosition represents the geometry for rendering a text cursor.
type CursorPosition struct {
	X      float32
	Y      float32
	Height float32
}

// LogAttr holds character classification for cursor/word boundaries.
type LogAttr struct {
	IsCursorPosition bool
	IsWordStart      bool
	IsWordEnd        bool
	IsLineBreak      bool
}

// CharRect maps a character byte index to its bounding rectangle.
type CharRect struct {
	Rect  Rect
	Index int // Byte index in the original text.
}

// Line describes one line of a laid-out paragraph.
type Line struct {
	StartIndex       int
	Length           int
	Rect             Rect // Logical bounding box relative to layout.
	IsParagraphStart bool
}

// Item is a run of glyphs sharing the same font and attributes.
type Item struct {
	Style TextStyle

	FTFace   unsafe.Pointer // *C.FT_FaceRec, set during layout.
	RunText  string
	ObjectID string

	CSSFont string // WASM only: CSS font string for glyph rasterization.

	Width   float64
	X       float64 // Run position relative to layout.
	Y       float64 // Baseline y relative to layout.
	Ascent  float64
	Descent float64

	GlyphStart int
	GlyphCount int
	StartIndex int
	Length     int

	// Decoration metrics.
	UnderlineOffset        float64
	UnderlineThickness     float64
	StrikethroughOffset    float64
	StrikethroughThickness float64

	StrokeWidth float32

	Color   Color
	BgColor Color

	StrokeColor Color

	HasUnderline     bool
	HasStrikethrough bool
	HasBgColor       bool
	HasStroke        bool
	UseOriginalColor bool // True for emoji — skip tinting.
	IsObject         bool
}

// Glyph holds a shaped glyph index and its positioning offsets.
type Glyph struct {
	Index     uint32
	XOffset   float64
	YOffset   float64
	XAdvance  float64
	YAdvance  float64
	Codepoint uint32 // Cluster byte length (use with Index as byte offset).
	GlyphID   uint16 // Resolved CGGlyph/HB glyph ID (0 = unknown, use text).
}

// GlyphPlacement specifies absolute screen position and rotation
// for a single glyph. Used with DrawLayoutPlaced for text-on-curve.
type GlyphPlacement struct {
	X     float32 // Absolute screen x.
	Y     float32 // Absolute screen y (baseline).
	Angle float32 // Rotation in radians, 0 = upright.
}

// GlyphInfo provides the absolute position and advance of a glyph
// within a Layout. Returned by GlyphPositions.
type GlyphInfo struct {
	X       float32
	Y       float32 // Baseline y.
	Advance float32
	Index   int // Index into Layout.Glyphs.
}

// GlyphPositions returns the absolute position, advance, and index
// of every glyph in the layout.
func (l *Layout) GlyphPositions() []GlyphInfo {
	if len(l.Glyphs) == 0 {
		return nil
	}
	result := make([]GlyphInfo, 0, len(l.Glyphs))
	for _, item := range l.Items {
		cx := float32(item.X)
		cy := float32(item.Y)
		end := item.GlyphStart + item.GlyphCount
		for i := item.GlyphStart; i < end; i++ {
			if i < 0 || i >= len(l.Glyphs) {
				continue
			}
			g := l.Glyphs[i]
			if (g.Index & PangoGlyphUnknownFlag) != 0 {
				cx += float32(g.XAdvance)
				cy -= float32(g.YAdvance)
				continue
			}
			result = append(result, GlyphInfo{
				X:       cx + float32(g.XOffset),
				Y:       cy - float32(g.YOffset),
				Advance: float32(g.XAdvance),
				Index:   i,
			})
			cx += float32(g.XAdvance)
			cy -= float32(g.YAdvance)
		}
	}
	return result
}
