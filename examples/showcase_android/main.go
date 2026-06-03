//go:build android

// Command showcase_android is the Android showcase app for go-glyph.
// Compiled as a c-shared .so and loaded by a native Android app.
package main

/*
#cgo LDFLAGS: -llog
#include <stdint.h>
#include <stdlib.h>
#include <android/log.h>

#define LOG_TAG "glyph"
static void logError(const char *msg) {
    __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, "%s", msg);
}
*/
import "C"
import (
	"sync"
	"unsafe"

	"github.com/go-gui-org/go-glyph"
	"github.com/go-gui-org/go-glyph/backend/android"
	ss "github.com/go-gui-org/go-glyph/examples/showcase_sections"
)

var (
	mu           sync.Mutex
	backend      *android.Backend
	ts           *glyph.TextSystem
	shared       *ss.App
	sects        []ss.Section
	scrollY      float32
	winW         int // logical width
	winH         int // logical height
	dpiScale     float32
	initFailed   bool
	cachedTotalH float32
)

func androidLog(msg string) {
	cs := C.CString(msg)
	C.logError(cs)
	C.free(unsafe.Pointer(cs))
}

//export GlyphStart
func GlyphStart(windowPtr uintptr, w, h int, scale float32) {
	dpiScale = scale
	if dpiScale <= 0 {
		dpiScale = 1
	}
	winW = int(float32(w) / dpiScale)
	winH = int(float32(h) / dpiScale)
	var err error
	backend, err = android.New(unsafe.Pointer(windowPtr), scale)
	if err != nil {
		androidLog("GlyphStart: " + err.Error())
		initFailed = true
		return
	}
	ts, err = glyph.NewTextSystem(backend)
	if err != nil {
		androidLog("GlyphStart: " + err.Error())
		initFailed = true
		return
	}
	shared = &ss.App{TS: ts, Backend: backend}
	sects = ss.BuildSections()
	cachedTotalH = totalHeight()
}

//export GlyphRender
func GlyphRender(w, h int) {
	if initFailed {
		return
	}
	winW = int(float32(w) / dpiScale)
	winH = int(float32(h) / dpiScale)

	mu.Lock()
	sy := scrollY
	mu.Unlock()

	backend.BeginFrame()
	drawSections(sy)
	ts.Commit()
	if err := backend.EndFrame(
		float32(ss.BgColor.R)/255, float32(ss.BgColor.G)/255,
		float32(ss.BgColor.B)/255, 1.0,
		winW, winH); err != nil {
		androidLog("EndFrame: " + err.Error())
	}
	shared.Frame++
}

//export GlyphScroll
func GlyphScroll(dy float32) {
	mu.Lock()
	scrollY += dy / dpiScale
	clampScroll()
	mu.Unlock()
}

//export GlyphTouch
func GlyphTouch(x, y float32) {
	mu.Lock()
	shared.MouseX = int32(x / dpiScale)
	shared.MouseY = int32(y / dpiScale)
	mu.Unlock()
}

//export GlyphResize
func GlyphResize(w, h int) {
	winW = int(float32(w) / dpiScale)
	winH = int(float32(h) / dpiScale)
	mu.Lock()
	clampScroll()
	mu.Unlock()
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
	mx := cachedTotalH - float32(winH)
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

func drawSections(sy float32) {
	cw := float32(winW) - ss.Margin*2
	y := float32(20) - sy

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

		if err := ts.DrawText(ss.Margin, y, s.Title, glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName:      "Sans 11",
				Typeface:      glyph.TypefaceBold,
				Color:         ss.Accent,
				LetterSpacing: 2,
			},
		}); err != nil {
			androidLog("DrawText: " + err.Error())
		}

		s.Draw(shared, ss.Margin, y+30, cw)
		y += s.Height + ss.SectionGap
	}
}

func main() {}
