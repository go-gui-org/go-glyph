// Command demo renders sample text using the glyph library
// with an Ebitengine window.
package main

import (
	"fmt"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/go-gui-org/go-glyph"
	glyphebi "github.com/go-gui-org/go-glyph/backend/ebitengine"
)

const (
	screenW = 800
	screenH = 600
)

// Game implements ebiten.Game.
type Game struct {
	ts      *glyph.TextSystem
	backend *glyphebi.Backend
	frame   int
	scale   float64
	scrollY float32
}

func (g *Game) Update() error {
	g.frame++
	_, dy := ebiten.Wheel()
	g.scrollY -= float32(dy) * 30
	if g.scrollY < 0 {
		g.scrollY = 0
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.backend.SetTarget(screen)

	// Background.
	screen.Fill(bgColor)

	y := float32(20) - g.scrollY
	black := gc(0, 0, 0, 255)

	// --- Basic Latin text ---
	g.drawSection(10, y, "Basic Latin Text")
	y += 30
	g.drawText(20, y, "Hello, World!", "Sans 24", black)
	y += 32
	g.drawText(20, y, "The quick brown fox jumps over the lazy dog.",
		"Serif 16", gc(80, 40, 0, 255))
	y += 28

	// --- Bold / Italic ---
	g.drawSection(10, y, "Typeface Variants")
	y += 30
	g.drawStyled(20, y, "Bold text", "Sans 18", glyph.TypefaceBold)
	y += 26
	g.drawStyled(20, y, "Italic text", "Sans 18", glyph.TypefaceItalic)
	y += 26
	g.drawStyled(20, y, "Bold Italic text", "Sans 18", glyph.TypefaceBoldItalic)
	y += 32

	// --- Word wrapping ---
	g.drawSection(10, y, "Word Wrapping (300px)")
	y += 30
	g.drawWrapped(20, y, "This is a longer paragraph that should wrap "+
		"at word boundaries when it exceeds the maximum width of three "+
		"hundred pixels. The layout engine handles this automatically.",
		"Sans 14", 300)
	y += 90

	// --- Alignment ---
	g.drawSection(10, y, "Alignment (width=400)")
	y += 30
	g.drawAligned(20, y, "Left aligned", "Sans 14", 400, glyph.AlignLeft)
	y += 22
	g.drawAligned(20, y, "Center aligned", "Sans 14", 400, glyph.AlignCenter)
	y += 22
	g.drawAligned(20, y, "Right aligned", "Sans 14", 400, glyph.AlignRight)
	y += 32

	// --- Gradient ---
	g.drawSection(10, y, "Gradient Text")
	y += 30
	g.drawGradient(20, y, "Rainbow Gradient", "Sans 28")
	y += 40

	// --- Underline / Strikethrough ---
	g.drawSection(10, y, "Decorations")
	y += 30
	g.drawDecorated(20, y, "Underlined text", "Sans 16", true, false)
	y += 24
	g.drawDecorated(20, y, "Strikethrough text", "Sans 16", false, true)
	y += 32

	// --- Rotation ---
	g.drawSection(10, y, "Rotated Text")
	y += 30
	angle := float32(g.frame%360) * math.Pi / 180.0
	g.drawRotated(120, y+40, "Spinning!", "Sans 20", angle)

	// --- Right column: emoji + CJK ---
	rx := float32(450)
	ry := float32(20) - g.scrollY

	g.drawSection(rx, ry, "Emoji")
	ry += 30
	g.drawText(rx+10, ry,
		"\U0001F680 \U0001F525 \U0001F44D \U0001F600 \u2764\ufe0f",
		"Sans 24", black)
	ry += 40

	g.drawSection(rx, ry, "CJK Characters")
	ry += 30
	g.drawText(rx+10, ry, "\u4f60\u597d\u4e16\u754c",
		"Sans 22", gc(0, 0, 128, 255))
	ry += 34
	g.drawText(rx+10, ry, "\u3053\u3093\u306b\u3061\u306f",
		"Sans 22", gc(128, 0, 64, 255))
	ry += 40

	g.drawSection(rx, ry, "RTL (Arabic / Hebrew)")
	ry += 30
	g.drawText(rx+10, ry,
		"\u0645\u0631\u062d\u0628\u0627 \u0628\u0627\u0644\u0639\u0627\u0644\u0645",
		"Sans 22", gc(0, 100, 0, 255))
	ry += 34
	g.drawText(rx+10, ry, "\u05e9\u05dc\u05d5\u05dd \u05e2\u05d5\u05dc\u05dd",
		"Sans 22", gc(100, 0, 100, 255))
	ry += 40

	g.drawSection(rx, ry, "Letter Spacing")
	ry += 30
	g.drawSpaced(rx+10, ry, "S P A C E D", "Sans 16", 3.0)
	ry += 28
	g.drawSpaced(rx+10, ry, "Tight", "Sans 16", -1.0)
	ry += 40

	g.drawSection(rx, ry, "Font Sizes")
	ry += 30
	for _, size := range []int{10, 14, 18, 24, 32} {
		g.drawText(rx+10, ry, fmt.Sprintf("%dpt sample", size),
			fmt.Sprintf("Sans %d", size), black)
		ry += float32(size) + 8
	}

	g.ts.Commit()
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return int(float64(outsideWidth) * g.scale),
		int(float64(outsideHeight) * g.scale)
}

// --- helpers ---

var bgColor = color.RGBA{R: 245, G: 245, B: 245, A: 255}

// gc is shorthand for glyph.Color with keyed fields.
func gc(r, g, b, a uint8) glyph.Color {
	return glyph.Color{R: r, G: g, B: b, A: a}
}

func (g *Game) drawSection(x, y float32, title string) {
	_ = g.ts.DrawText(x, y, title, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: "Sans Bold 13",
			Color:    gc(100, 100, 100, 255),
		},
	})
}

func (g *Game) drawText(x, y float32, text, font string, c glyph.Color) {
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: c},
	})
}

func (g *Game) drawStyled(x, y float32, text, font string, tf glyph.Typeface) {
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: font,
			Typeface: tf,
			Color:    gc(0, 0, 0, 255),
		},
	})
}

func (g *Game) drawWrapped(x, y float32, text, font string, width float32) {
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: gc(0, 0, 0, 255)},
		Block: glyph.BlockStyle{Wrap: glyph.WrapWord, Width: width},
	})
}

func (g *Game) drawAligned(x, y float32, text, font string,
	width float32, align glyph.Alignment) {
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: gc(0, 0, 0, 255)},
		Block: glyph.BlockStyle{Width: width, Align: align},
	})
}

func (g *Game) drawGradient(x, y float32, text, font string) {
	grad := &glyph.GradientConfig{
		Direction: glyph.GradientHorizontal,
		Stops: []glyph.GradientStop{
			{Color: gc(255, 0, 0, 255), Position: 0.0},
			{Color: gc(255, 165, 0, 255), Position: 0.25},
			{Color: gc(0, 128, 0, 255), Position: 0.5},
			{Color: gc(0, 0, 255, 255), Position: 0.75},
			{Color: gc(128, 0, 128, 255), Position: 1.0},
		},
	}
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style:    glyph.TextStyle{FontName: font, Color: gc(255, 255, 255, 255)},
		Gradient: grad,
	})
}

func (g *Game) drawDecorated(x, y float32, text, font string,
	underline, strike bool) {
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:      font,
			Color:         gc(0, 0, 0, 255),
			Underline:     underline,
			Strikethrough: strike,
		},
	})
}

func (g *Game) drawRotated(x, y float32, text, font string, angle float32) {
	layout, err := g.ts.LayoutText(text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: gc(200, 50, 50, 255)},
	})
	if err != nil {
		return
	}
	g.ts.DrawLayoutRotated(layout, x, y, angle)
}

func (g *Game) drawSpaced(x, y float32, text, font string, spacing float32) {
	_ = g.ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:      font,
			Color:         gc(0, 0, 0, 255),
			LetterSpacing: spacing,
		},
	})
}

func main() {
	scale := ebiten.Monitor().DeviceScaleFactor()

	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowTitle("glyph demo")

	backend := glyphebi.New(nil, float32(scale))

	ts, err := glyph.NewTextSystem(backend)
	if err != nil {
		log.Fatal(err)
	}
	defer ts.Free()

	game := &Game{
		ts:      ts,
		backend: backend,
		scale:   scale,
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
