// Command demo_gpu renders sample text using the glyph library
// with an SDL2 window and raw Metal backend.
package main

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"unsafe"

	"github.com/go-gui-org/go-glyph"
	"github.com/go-gui-org/go-glyph/backend/gpu"

	"github.com/veandco/go-sdl2/sdl"
)

func init() { runtime.LockOSThread() }

const (
	screenW = 800
	screenH = 700
)

// app holds state shared between the main loop and the
// event-watch callback (which fires during live resize).
type app struct {
	window  *sdl.Window
	backend *gpu.Backend
	ts      *glyph.TextSystem
	frame   int
}

func (a *app) render() {
	a.backend.BeginFrame()
	drawAll(a.ts, a.frame)
	a.ts.Commit()
	w, h := a.window.GetSize()
	a.backend.EndFrame(0.96, 0.96, 0.96, 1.0, int(w), int(h))
	a.frame++
}

func main() {
	// Ensure per-monitor DPI awareness so SDL reports physical
	// pixels and Windows DWM does not magnify the window.
	// DPI_SCALING makes SDL auto-scale window dimensions by the
	// display DPI so the window keeps its nominal physical size
	// and GL_GetDrawableSize reports the scaled backbuffer.
	// Requires SDL 2.24+; older SDL silently ignores DPI_SCALING
	// and fonts will render at 1x (tiny) on high-DPI displays.
	glyph.SetDPIAwareWindows()
	sdl.SetHint("SDL_WINDOWS_DPI_AWARENESS", "permonitorv2")
	sdl.SetHint("SDL_WINDOWS_DPI_SCALING", "1")

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		log.Fatal(err)
	}
	defer sdl.Quit()

	window, err := sdl.CreateWindow("glyph demo (Metal)",
		sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
		screenW, screenH,
		sdl.WINDOW_SHOWN|sdl.WINDOW_RESIZABLE|
			sdl.WINDOW_ALLOW_HIGHDPI|gpu.WindowFlag())
	if err != nil {
		log.Fatal(err)
	}
	defer window.Destroy()

	// Compute DPI scale from physical/logical size ratio.
	physW, _ := gpu.WindowDrawableSize(unsafe.Pointer(window))
	winW, _ := window.GetSize()
	dpiScale := float32(1.0)
	if winW > 0 {
		dpiScale = float32(physW) / float32(winW)
	}

	backend, err := gpu.New(unsafe.Pointer(window), dpiScale)
	if err != nil {
		log.Fatal(err)
	}
	defer backend.Destroy()

	ts, err := glyph.NewTextSystem(backend)
	if err != nil {
		log.Fatal(err)
	}
	defer ts.Free()

	a := &app{window: window, backend: backend, ts: ts}

	// Event watcher fires during live resize so the content
	// redraws continuously while the user drags.
	sdl.AddEventWatchFunc(func(event sdl.Event, _ any) bool {
		if ev, ok := event.(*sdl.WindowEvent); ok {
			if ev.Event == sdl.WINDOWEVENT_EXPOSED ||
				ev.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
				a.render()
			}
		}
		return true // keep watching
	}, nil)

	running := true
	for running {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch event.(type) {
			case *sdl.QuitEvent:
				running = false
			}
		}
		a.render()
	}
}

// --- drawing ---

func drawAll(ts *glyph.TextSystem, frame int) {
	y := float32(20)
	black := gc(0, 0, 0, 255)

	// --- Basic Latin text ---
	drawSection(ts, 10, y, "Basic Latin Text")
	y += 30
	drawText(ts, 20, y, "Hello, World!", "Sans 24", black)
	y += 32
	drawText(ts, 20, y, "The quick brown fox jumps over the lazy dog.",
		"Serif 16", gc(80, 40, 0, 255))
	y += 28

	// --- Bold / Italic ---
	drawSection(ts, 10, y, "Typeface Variants")
	y += 30
	drawStyled(ts, 20, y, "Bold text", "Sans 18", glyph.TypefaceBold)
	y += 26
	drawStyled(ts, 20, y, "Italic text", "Sans 18", glyph.TypefaceItalic)
	y += 26
	drawStyled(ts, 20, y, "Bold Italic text", "Sans 18", glyph.TypefaceBoldItalic)
	y += 32

	// --- Word wrapping ---
	drawSection(ts, 10, y, "Word Wrapping (300px)")
	y += 30
	drawWrapped(ts, 20, y, "This is a longer paragraph that should wrap "+
		"at word boundaries when it exceeds the maximum width of three "+
		"hundred pixels. The layout engine handles this automatically.",
		"Sans 14", 300)
	y += 90

	// --- Alignment ---
	drawSection(ts, 10, y, "Alignment (width=400)")
	y += 30
	drawAligned(ts, 20, y, "Left aligned", "Sans 14", 400, glyph.AlignLeft)
	y += 22
	drawAligned(ts, 20, y, "Center aligned", "Sans 14", 400, glyph.AlignCenter)
	y += 22
	drawAligned(ts, 20, y, "Right aligned", "Sans 14", 400, glyph.AlignRight)
	y += 32

	// --- Gradient ---
	drawSection(ts, 10, y, "Gradient Text")
	y += 30
	drawGradient(ts, 20, y, "Rainbow Gradient", "Sans 28")
	y += 40

	// --- Underline / Strikethrough ---
	drawSection(ts, 10, y, "Decorations")
	y += 30
	drawDecorated(ts, 20, y, "Underlined text", "Sans 16", true, false)
	y += 24
	drawDecorated(ts, 20, y, "Strikethrough text", "Sans 16", false, true)
	y += 32

	// --- Rotation ---
	drawSection(ts, 10, y, "Rotated Text")
	y += 30
	angle := float32(frame%360) * math.Pi / 180.0
	drawRotated(ts, 120, y+40, "Spinning!", "Sans 20", angle)

	// --- Right column: emoji + CJK ---
	rx := float32(450)
	ry := float32(20)

	drawSection(ts, rx, ry, "Emoji")
	ry += 30
	drawText(ts, rx+10, ry,
		"\U0001F680 \U0001F525 \U0001F44D \U0001F600 \u2764\ufe0f",
		"Sans 24", black)
	ry += 40

	drawSection(ts, rx, ry, "CJK Characters")
	ry += 30
	drawText(ts, rx+10, ry, "\u4f60\u597d\u4e16\u754c",
		"Sans 22", gc(0, 0, 128, 255))
	ry += 34
	drawText(ts, rx+10, ry, "\u3053\u3093\u306b\u3061\u306f",
		"Sans 22", gc(128, 0, 64, 255))
	ry += 40

	drawSection(ts, rx, ry, "RTL (Arabic / Hebrew)")
	ry += 30
	drawText(ts, rx+10, ry,
		"\u0645\u0631\u062d\u0628\u0627 \u0628\u0627\u0644\u0639\u0627\u0644\u0645",
		"Sans 22", gc(0, 100, 0, 255))
	ry += 34
	drawText(ts, rx+10, ry, "\u05e9\u05dc\u05d5\u05dd \u05e2\u05d5\u05dc\u05dd",
		"Sans 22", gc(100, 0, 100, 255))
	ry += 40

	drawSection(ts, rx, ry, "Letter Spacing")
	ry += 30
	drawSpaced(ts, rx+10, ry, "S P A C E D", "Sans 16", 3.0)
	ry += 28
	drawSpaced(ts, rx+10, ry, "Tight", "Sans 16", -1.0)
	ry += 40

	drawSection(ts, rx, ry, "Font Sizes")
	ry += 30
	for _, size := range []int{10, 14, 18, 24, 32} {
		drawText(ts, rx+10, ry, fmt.Sprintf("%dpt sample", size),
			fmt.Sprintf("Sans %d", size), black)
		ry += float32(size) + 8
	}
}

// --- helpers ---

func gc(r, g, b, a uint8) glyph.Color {
	return glyph.Color{R: r, G: g, B: b, A: a}
}

func drawSection(ts *glyph.TextSystem, x, y float32, title string) {
	_ = ts.DrawText(x, y, title, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: "Sans Bold 13",
			Color:    gc(100, 100, 100, 255),
		},
	})
}

func drawText(ts *glyph.TextSystem, x, y float32, text, font string, c glyph.Color) {
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: c},
	})
}

func drawStyled(ts *glyph.TextSystem, x, y float32, text, font string, tf glyph.Typeface) {
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: font,
			Typeface: tf,
			Color:    gc(0, 0, 0, 255),
		},
	})
}

func drawWrapped(ts *glyph.TextSystem, x, y float32, text, font string, width float32) {
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: gc(0, 0, 0, 255)},
		Block: glyph.BlockStyle{Wrap: glyph.WrapWord, Width: width},
	})
}

func drawAligned(ts *glyph.TextSystem, x, y float32, text, font string,
	width float32, align glyph.Alignment) {
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: gc(0, 0, 0, 255)},
		Block: glyph.BlockStyle{Width: width, Align: align},
	})
}

func drawGradient(ts *glyph.TextSystem, x, y float32, text, font string) {
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
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style:    glyph.TextStyle{FontName: font, Color: gc(255, 255, 255, 255)},
		Gradient: grad,
	})
}

func drawDecorated(ts *glyph.TextSystem, x, y float32, text, font string,
	underline, strike bool) {
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:      font,
			Color:         gc(0, 0, 0, 255),
			Underline:     underline,
			Strikethrough: strike,
		},
	})
}

func drawRotated(ts *glyph.TextSystem, x, y float32, text, font string, angle float32) {
	layout, err := ts.LayoutText(text, glyph.TextConfig{
		Style: glyph.TextStyle{FontName: font, Color: gc(200, 50, 50, 255)},
	})
	if err != nil {
		return
	}
	ts.DrawLayoutRotated(layout, x, y, angle)
}

func drawSpaced(ts *glyph.TextSystem, x, y float32, text, font string, spacing float32) {
	_ = ts.DrawText(x, y, text, glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:      font,
			Color:         gc(0, 0, 0, 255),
			LetterSpacing: spacing,
		},
	})
}
