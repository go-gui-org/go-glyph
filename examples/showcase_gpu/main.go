// Command showcase_gpu is a comprehensive feature gallery for the
// glyph library using an SDL2 window with raw Metal backend.
// Scroll with mouse wheel or Page Up/Down, Home/End keys.
package main

import (
	"runtime"
	"unsafe"

	"github.com/go-gui-org/go-glyph"
	"github.com/go-gui-org/go-glyph/backend/gpu"
	ss "github.com/go-gui-org/go-glyph/examples/showcase_sections"

	"github.com/veandco/go-sdl2/sdl"
)

func init() { runtime.LockOSThread() }

const (
	screenW = 1000
	screenH = 800
)

type app struct {
	window  *sdl.Window
	backend *gpu.Backend
	shared  *ss.App
	sects   []ss.Section
	scrollY float32
}

func main() {
	// Ensure per-monitor DPI awareness so SDL reports physical
	// pixels and Windows DWM does not magnify the window.
	// DPI_SCALING makes SDL auto-scale window dimensions by the
	// display DPI so the window keeps its nominal physical size.
	// Requires SDL 2.24+; older SDL silently ignores DPI_SCALING
	// and fonts will render at 1x (tiny) on high-DPI displays.
	glyph.SetDPIAwareWindows()
	sdl.SetHint("SDL_WINDOWS_DPI_AWARENESS", "permonitorv2")
	sdl.SetHint("SDL_WINDOWS_DPI_SCALING", "1")

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		panic(err)
	}
	defer sdl.Quit()

	win, err := sdl.CreateWindow("Go-Glyph showcase (Metal)",
		sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
		screenW, screenH,
		sdl.WINDOW_SHOWN|sdl.WINDOW_RESIZABLE|
			sdl.WINDOW_ALLOW_HIGHDPI|gpu.WindowFlag())
	if err != nil {
		panic(err)
	}
	defer func() { _ = win.Destroy() }()

	physW, _ := gpu.WindowDrawableSize(unsafe.Pointer(win))
	winW, _ := win.GetSize()
	dpi := float32(1)
	if winW > 0 {
		dpi = float32(physW) / float32(winW)
	}

	be, err := gpu.New(unsafe.Pointer(win), dpi)
	if err != nil {
		panic(err)
	}
	defer be.Destroy()

	ts, err := glyph.NewTextSystem(be)
	if err != nil {
		panic(err)
	}
	defer ts.Free()

	a := &app{
		window:  win,
		backend: be,
		shared:  &ss.App{TS: ts, Backend: be},
		sects:   ss.BuildSections(),
	}

	sdl.AddEventWatchFunc(func(ev sdl.Event, _ any) bool {
		if we, ok := ev.(*sdl.WindowEvent); ok {
			if we.Event == sdl.WINDOWEVENT_EXPOSED ||
				we.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
				a.render()
			}
		}
		return true
	}, nil)

	for {
		for ev := sdl.PollEvent(); ev != nil; ev = sdl.PollEvent() {
			switch e := ev.(type) {
			case *sdl.QuitEvent:
				return
			case *sdl.MouseWheelEvent:
				a.scrollY -= float32(e.Y) * 40
				a.clampScroll()
			case *sdl.MouseMotionEvent:
				a.shared.MouseX = e.X
				a.shared.MouseY = e.Y
			case *sdl.KeyboardEvent:
				if e.Type == sdl.KEYDOWN {
					a.handleKey(e.Keysym.Sym)
				}
			}
		}
		a.render()
	}
}

func (a *app) handleKey(sym sdl.Keycode) {
	_, wh := a.window.GetSize()
	switch sym {
	case sdl.K_HOME:
		a.scrollY = 0
	case sdl.K_END:
		a.scrollY = a.totalHeight() - float32(wh)
	case sdl.K_PAGEUP:
		a.scrollY -= float32(wh) * 0.8
	case sdl.K_PAGEDOWN:
		a.scrollY += float32(wh) * 0.8
	case sdl.K_UP:
		a.scrollY -= 40
	case sdl.K_DOWN:
		a.scrollY += 40
	}
	a.clampScroll()
}

func (a *app) totalHeight() float32 {
	h := float32(20)
	for _, s := range a.sects {
		h += s.Height + ss.SectionGap
	}
	return h
}

func (a *app) clampScroll() {
	_, wh := a.window.GetSize()
	max := a.totalHeight() - float32(wh)
	if max < 0 {
		max = 0
	}
	if a.scrollY > max {
		a.scrollY = max
	}
	if a.scrollY < 0 {
		a.scrollY = 0
	}
}

func (a *app) render() {
	a.backend.BeginFrame()
	a.drawSections()
	a.shared.TS.Commit()
	w, h := a.window.GetSize()
	_ = a.backend.EndFrame(
		float32(ss.BgColor.R)/255, float32(ss.BgColor.G)/255,
		float32(ss.BgColor.B)/255, 1.0,
		int(w), int(h))
	a.shared.Frame++
}

func (a *app) drawSections() {
	ww, wh := a.window.GetSize()
	cw := float32(ww) - ss.Margin*2
	y := float32(20) - a.scrollY

	for i := range a.sects {
		s := &a.sects[i]

		if y+s.Height < 0 {
			y += s.Height + ss.SectionGap
			continue
		}
		if y > float32(wh) {
			break
		}

		if i > 0 {
			a.backend.DrawFilledRect(glyph.Rect{
				X: ss.Margin, Y: y - ss.SectionGap/2,
				Width: cw, Height: 1,
			}, ss.Divider)
		}

		_ = a.shared.TS.DrawText(ss.Margin, y, s.Title, glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName:      "Sans 11",
				Typeface:      glyph.TypefaceBold,
				Color:         ss.Accent,
				LetterSpacing: 2,
			},
		})

		s.Draw(a.shared, ss.Margin, y+30, cw)

		y += s.Height + ss.SectionGap
	}
}
