// Package showcase_sections contains the 22 showcase section
// drawing functions shared by showcase_gpu and showcase_web.
package showcase_sections

import "github.com/go-gui-org/go-glyph"

// App holds state shared across all showcase sections.
type App struct {
	TS        *glyph.TextSystem
	Backend   glyph.DrawBackend
	Frame     int
	SubpixelX float32
	MouseX    int32
	MouseY    int32
}

// Section describes a single showcase section.
type Section struct {
	Title  string
	Height float32
	Draw   func(a *App, x, y, w float32)
}

// Dark theme palette.
var (
	BgColor   = GC(20, 20, 25, 255)
	TextColor = GC(220, 220, 225, 255)
	DimColor  = GC(140, 140, 150, 255)
	Accent    = GC(100, 160, 255, 255)
	Warm      = GC(255, 140, 80, 255)
	Cool      = GC(80, 180, 255, 255)
	Divider   = GC(50, 50, 60, 255)
	Highlight = GC(255, 220, 80, 255)
	CodeGreen = GC(160, 220, 140, 255)
)

// GC is a helper to construct a Color.
func GC(r, g, b, a uint8) glyph.Color {
	return glyph.Color{R: r, G: g, B: b, A: a}
}

// Layout constants.
const (
	Margin     = 40
	SectionGap = 30
)

// BuildSections returns all 22 showcase sections.
func BuildSections() []Section {
	return []Section{
		{"INTRO", 100, DrawIntro},
		{"TYPOGRAPHY", 200, DrawTypography},
		{"DECORATIONS", 110, DrawDecorations},
		{"TEXT STROKE", 150, DrawStroke},
		{"LAYOUT", 220, DrawLayout},
		{"RICH TEXT", 60, DrawRichText},
		{"PANGO MARKUP", 60, DrawMarkup},
		{"GRADIENTS", 160, DrawGradients},
		{"INTERNATIONALIZATION", 260, DrawI18n},
		{"BIDIRECTIONAL TEXT", 120, DrawBidi},
		{"OPENTYPE FEATURES", 220, DrawOpenType},
		{"SUBSCRIPTS & SUPERSCRIPTS", 120, DrawSubSup},
		{"LETTER SPACING", 140, DrawSpacing},
		{"FONT SIZES", 250, DrawSizes},
		{"ROTATED TEXT", 200, DrawRotated},
		{"VERTICAL TEXT", 200, DrawVertical},
		{"TEXT ON PATH", 290, DrawPathText},
		{"SKEWED TEXT", 160, DrawSkewed},
		{"SUBPIXEL RENDERING", 120, DrawSubpixel},
		{"HIT TESTING", 100, DrawHitTest},
		{"DIRECT TEXT RENDERING", 160, DrawDirectText},
		{"TRANSFORMS", 180, DrawTransforms},
	}
}
