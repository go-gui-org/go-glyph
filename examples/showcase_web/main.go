//go:build js && wasm

// Command showcase_web renders the go-glyph showcase in a browser
// using the Canvas2D web backend. Same 22 sections as showcase_gpu.
package main

import (
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

	// Match canvas to window logical size.
	resizeCanvas := func() {
		w := js.Global().Get("innerWidth").Int()
		h := js.Global().Get("innerHeight").Int()
		canvasW = w
		canvasH = h
		canvas.Set("width", w)
		canvas.Set("height", h)
	}
	resizeCanvas()

	be = web.New(canvas, 1.0)

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
