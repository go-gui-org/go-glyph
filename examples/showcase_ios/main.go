//go:build ios

// Command showcase_ios is the iOS showcase app for go-glyph.
// Compiled as a c-archive and linked into a native Xcode project.
package main

/*
#include <stdint.h>
*/
import "C"
import (
	"unsafe"

	"github.com/go-gui-org/go-glyph"
	"github.com/go-gui-org/go-glyph/backend/ios"
	ss "github.com/go-gui-org/go-glyph/examples/showcase_sections"
)

var (
	backend *ios.Backend
	ts      *glyph.TextSystem
	shared  *ss.App
	sects   []ss.Section
	scrollY float32
	winW    int
	winH    int
)

//export GlyphStart
func GlyphStart(layerPtr uintptr, w, h int, scale float32) {
	winW = w
	winH = h
	var err error
	backend, err = ios.New(unsafe.Pointer(layerPtr), scale)
	if err != nil {
		panic(err)
	}
	ts, err = glyph.NewTextSystem(backend)
	if err != nil {
		panic(err)
	}
	shared = &ss.App{TS: ts, Backend: backend}
	sects = ss.BuildSections()
}

//export GlyphRender
func GlyphRender(w, h int) {
	winW = w
	winH = h
	backend.BeginFrame()
	drawSections()
	ts.Commit()
	_ = backend.EndFrame(
		float32(ss.BgColor.R)/255, float32(ss.BgColor.G)/255,
		float32(ss.BgColor.B)/255, 1.0,
		w, h)
	shared.Frame++
}

//export GlyphScroll
func GlyphScroll(dy float32) {
	scrollY += dy
	clampScroll()
}

//export GlyphTouch
func GlyphTouch(x, y float32) {
	shared.MouseX = int32(x)
	shared.MouseY = int32(y)
}

//export GlyphResize
func GlyphResize(w, h int) {
	winW = w
	winH = h
	clampScroll()
}

//export GlyphDestroy
func GlyphDestroy() {
	if ts != nil {
		ts.Free()
	}
	if backend != nil {
		backend.Destroy()
	}
}

func totalHeight() float32 {
	h := float32(20)
	for _, s := range sects {
		h += s.Height + ss.SectionGap
	}
	return h
}

func clampScroll() {
	mx := totalHeight() - float32(winH)
	if mx < 0 {
		mx = 0
	}
	if scrollY > mx {
		scrollY = mx
	}
	if scrollY < 0 {
		scrollY = 0
	}
}

func drawSections() {
	cw := float32(winW) - ss.Margin*2
	y := float32(20) - scrollY

	for i := range sects {
		s := &sects[i]
		if y+s.Height < 0 {
			y += s.Height + ss.SectionGap
			continue
		}
		if y > float32(winH) {
			break
		}

		if i > 0 {
			backend.DrawFilledRect(glyph.Rect{
				X: ss.Margin, Y: y - ss.SectionGap/2,
				Width: cw, Height: 1,
			}, ss.Divider)
		}

		_ = ts.DrawText(ss.Margin, y, s.Title, glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName:      "Sans 11",
				Typeface:      glyph.TypefaceBold,
				Color:         ss.Accent,
				LetterSpacing: 2,
			},
		})

		s.Draw(shared, ss.Margin, y+30, cw)
		y += s.Height + ss.SectionGap
	}
}

func main() {}
