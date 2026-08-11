//go:build js && wasm

// Command showcase_web renders the go-glyph showcase in a browser
// using the Canvas2D web backend. Same 22 sections as showcase_gpu.
package main

import (
	"strconv"
	"syscall/js"

	"github.com/go-gui-org/go-glyph"
	"github.com/go-gui-org/go-glyph/backend/web"
	ss "github.com/go-gui-org/go-glyph/examples/showcase_sections"
)

var (
	shared  *ss.App
	be      *web.Backend
	sects   []ss.Section
	scrollY float32
	canvasW int
	canvasH int
)

func main() {
	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "canvas")
	if canvas.IsNull() || canvas.IsUndefined() {
		js.Global().Get("console").Call("error",
			"canvas element not found")
		return
	}

	// Back the canvas at the device pixel ratio: the drawing buffer is
	// physical pixels, the CSS box stays the window's logical size, and the
	// backend installs the matching base transform each frame. Everything
	// below keeps working in logical units, and the built-in box glyphs get
	// a real pixel grid to snap to on a HiDPI display.
	dpr := float32(js.Global().Get("devicePixelRatio").Float())
	// !(dpr > 0) also catches NaN, so a broken read cannot poison the
	// canvas transform.
	if !(dpr > 0) {
		dpr = 1
	}
	// ?dpr=N overrides it, so the same machine can compare 1x against 2x —
	// which is the only way to see whether the built-in box glyphs really
	// track the pixel grid rather than one particular display. Clamped: a
	// wild value would size the backing store absurdly and the conversion
	// to int is implementation-defined past float32's range.
	if v := queryParam("dpr"); v != "" {
		if f, err := strconv.ParseFloat(v, 32); err == nil && f > 0 {
			dpr = float32(min(f, 8))
		}
	}
	resizeCanvas := func() {
		w := js.Global().Get("innerWidth").Int()
		h := js.Global().Get("innerHeight").Int()
		canvasW = w
		canvasH = h
		canvas.Set("width", int(float32(w)*dpr))
		canvas.Set("height", int(float32(h)*dpr))
		style := canvas.Get("style")
		style.Set("width", strconv.Itoa(w)+"px")
		style.Set("height", strconv.Itoa(h)+"px")
	}
	resizeCanvas()

	be = web.New(canvas, dpr)

	ts, err := glyph.NewTextSystem(be)
	if err != nil {
		js.Global().Get("console").Call("error",
			"NewTextSystem failed: "+err.Error())
		return
	}

	shared = &ss.App{TS: ts, Backend: be}
	sects = ss.BuildSections()

	// Event listeners.
	js.Global().Call("addEventListener", "resize",
		js.FuncOf(func(_ js.Value, _ []js.Value) any {
			resizeCanvas()
			return nil
		}))

	canvas.Call("addEventListener", "wheel",
		js.FuncOf(func(_ js.Value, args []js.Value) any {
			e := args[0]
			e.Call("preventDefault")
			scrollY += float32(e.Get("deltaY").Float()) * 0.5
			clampScroll()
			return nil
		}))

	canvas.Call("addEventListener", "mousemove",
		js.FuncOf(func(_ js.Value, args []js.Value) any {
			e := args[0]
			shared.MouseX = int32(e.Get("offsetX").Int())
			shared.MouseY = int32(e.Get("offsetY").Int())
			return nil
		}))

	js.Global().Call("addEventListener", "keydown",
		js.FuncOf(func(_ js.Value, args []js.Value) any {
			handleKey(args[0])
			return nil
		}))

	// requestAnimationFrame render loop.
	var renderFunc js.Func
	renderFunc = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		render()
		js.Global().Call("requestAnimationFrame", renderFunc)
		return nil
	})
	js.Global().Call("requestAnimationFrame", renderFunc)

	// Block forever.
	select {}
}

// queryParam reads one search parameter from the page URL, or "" if absent.
func queryParam(name string) string {
	params := js.Global().Get("URLSearchParams").New(
		js.Global().Get("location").Get("search"))
	v := params.Call("get", name)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

func handleKey(e js.Value) {
	key := e.Get("key").String()
	switch key {
	case "Home":
		scrollY = 0
	case "End":
		scrollY = totalHeight() - float32(canvasH)
	case "PageUp":
		scrollY -= float32(canvasH) * 0.8
	case "PageDown":
		scrollY += float32(canvasH) * 0.8
	case "ArrowUp":
		scrollY -= 40
	case "ArrowDown":
		scrollY += 40
	}
	clampScroll()
}

func totalHeight() float32 {
	h := float32(20)
	for _, s := range sects {
		h += s.Height + ss.SectionGap
	}
	return h
}

func clampScroll() {
	max := totalHeight() - float32(canvasH)
	if max < 0 {
		max = 0
	}
	if scrollY > max {
		scrollY = max
	}
	if scrollY < 0 {
		scrollY = 0
	}
}

func render() {
	be.BeginFrame(
		float32(ss.BgColor.R)/255, float32(ss.BgColor.G)/255,
		float32(ss.BgColor.B)/255, 1.0)
	drawSections()
	shared.TS.Commit()
	be.EndFrame()
	shared.Frame++
}

func drawSections() {
	cw := float32(canvasW) - ss.Margin*2
	y := float32(20) - scrollY

	for i := range sects {
		s := &sects[i]

		if y+s.Height < 0 {
			y += s.Height + ss.SectionGap
			continue
		}
		if y > float32(canvasH) {
			break
		}

		if i > 0 {
			be.DrawFilledRect(glyph.Rect{
				X: ss.Margin, Y: y - ss.SectionGap/2,
				Width: cw, Height: 1,
			}, ss.Divider)
		}

		_ = shared.TS.DrawText(ss.Margin, y, s.Title, glyph.TextConfig{
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
