// Package glyph provides high-quality text shaping, layout, and rendering
// for GPU-accelerated applications. Text shaping and rasterization are
// pure Go on all platforms, exposed behind a backend-agnostic
// [DrawBackend] interface.
//
// # Platform matrix
//
//	OS          Shaper              Rasterizer
//	Linux       HarfBuzz (go-text)  x/image/vector
//	macOS       HarfBuzz (go-text)  x/image/vector
//	iOS         HarfBuzz (go-text)  x/image/vector
//	Windows     HarfBuzz (go-text)  x/image/vector
//	Android     HarfBuzz (go-text)  x/image/vector
//	WASM        Canvas2D            Canvas2D
//
// # Quick start
//
//	backend := ebitengine.NewBackend() // or gpu, web, etc.
//	ts, err := glyph.NewTextSystem(backend)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer ts.Free()
//
//	layout, err := ts.LayoutText("Hello, world!", glyph.TextConfig{
//	    Style: glyph.TextStyle{FontName: "Sans 18"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ts.DrawLayout(layout, 10, 10)
//	ts.Commit()
//
// # Core concepts
//
// [TextSystem] is the main entry point. It manages a text context, a
// [Renderer] (glyph atlas + draw calls), and a layout cache.
//
// Pre-computed layouts from [TextSystem.LayoutText] or
// [TextSystem.LayoutRichText] can be drawn repeatedly. For one-shot rendering,
// use [TextSystem.DrawText] which handles layout and caching internally.
//
// [TextConfig] controls rendering: [TextStyle] sets font, color, decorations,
// stroke, and letter spacing. [BlockStyle] sets wrapping, alignment, and
// indentation. Enable Pango markup with [TextConfig.UseMarkup].
//
//	cfg := glyph.TextConfig{
//	    Style: glyph.TextStyle{
//	        FontName:      "Sans 16",
//	        Typeface:      glyph.TypefaceBold,
//	        Color:         glyph.Color{R: 255, A: 255},
//	        Underline:     true,
//	        LetterSpacing: 2.0,
//	        StrokeWidth:   1.5,
//	        StrokeColor:   glyph.Color{A: 255},
//	    },
//	    Block: glyph.BlockStyle{
//	        Wrap:   glyph.WrapWord,
//	        Width:  400,
//	        Align:  glyph.AlignCenter,
//	        Indent: 20,
//	    },
//	    UseMarkup: false,
//	}
//
// # Gradients
//
//	cfg := glyph.TextConfig{
//	    Style: glyph.TextStyle{FontName: "Sans 28"},
//	    Gradient: &glyph.GradientConfig{
//	        Direction: glyph.GradientHorizontal,
//	        Stops: []glyph.GradientStop{
//	            {Color: glyph.Color{R: 255, A: 255}, Position: 0},
//	            {Color: glyph.Color{B: 255, A: 255}, Position: 1},
//	        },
//	    },
//	}
//	ts.DrawText(x, y, "Gradient text", cfg)
//
// # Rich text
//
// Render multiple styles in one layout:
//
//	rt := glyph.RichText{
//	    Runs: []glyph.StyleRun{
//	        {Text: "Bold ", Style: glyph.TextStyle{
//	            FontName: "Sans 16", Typeface: glyph.TypefaceBold,
//	            Color: glyph.Color{A: 255},
//	        }},
//	        {Text: "and italic", Style: glyph.TextStyle{
//	            FontName: "Sans 16", Typeface: glyph.TypefaceItalic,
//	            Color: glyph.Color{R: 200, A: 255},
//	        }},
//	    },
//	}
//	layout, _ := ts.LayoutRichText(rt, cfg)
//	ts.DrawLayout(layout, x, y)
//
// # Pango markup
//
//	cfg := glyph.TextConfig{
//	    Style: glyph.TextStyle{FontName: "Sans 16"},
//	    UseMarkup: true,
//	}
//	ts.DrawText(x, y, "<b>Bold</b> and <i>italic</i>", cfg)
//
// # Layout queries
//
// All query methods operate on a pre-computed [Layout]:
//
//	layout, _ := ts.LayoutText(text, cfg)
//
//	idx := layout.HitTest(mouseX, mouseY)
//	rect, ok := layout.GetCharRect(idx)
//	cursor, ok := layout.GetCursorPos(idx)
//	rects := layout.GetSelectionRects(start, end)
//
//	next := layout.MoveCursorRight(idx)
//	prev := layout.MoveCursorLeft(idx)
//	up := layout.MoveCursorUp(idx, preferredX)
//	down := layout.MoveCursorDown(idx, preferredX)
//
//	start, end := layout.GetWordAtIndex(idx)
//	pStart, pEnd := layout.GetParagraphAtIndex(idx, text)
//
// # Transforms
//
//	transform := glyph.AffineRotation(0.3).
//	    Multiply(glyph.AffineSkew(0.2, 0))
//	ts.DrawLayoutTransformed(layout, x, y, transform)
//
// For simple rotation:
//
//	ts.DrawLayoutRotated(layout, x, y, angleRadians)
//
// # Glyph placements
//
// Position each glyph independently (e.g. text on a path). DrawLayoutPlaced
// needs one placement per entry in layout.Glyphs, so size the slice to
// len(layout.Glyphs) and index it by GlyphInfo.Index — the glyph count is not
// the grapheme count (ligatures collapse clusters, marks add glyphs), and
// GlyphPositions omits unknown glyphs:
//
//	positions := layout.GlyphPositions()
//	placements := make([]glyph.GlyphPlacement, len(layout.Glyphs))
//	for i := range placements {
//	    placements[i] = glyph.GlyphPlacement{X: offX, Y: offY} // off-screen default
//	}
//	for _, g := range positions {
//	    placements[g.Index] = glyph.GlyphPlacement{
//	        X: pathX(g.X), Y: pathY(g.Y), Angle: pathAngle(g.X),
//	    }
//	}
//	ts.DrawLayoutPlaced(layout, placements)
//
// # Text mutation
//
// Package-level functions for editing text:
//
//	result := glyph.InsertText(text, cursor, "hello")
//	result = glyph.DeleteBackward(text, layout, cursor)
//	result = glyph.DeleteForward(text, layout, cursor)
//	result = glyph.DeleteSelection(text, cursor, anchor)
//	result = glyph.InsertReplacingSelection(text, cursor, anchor, "new")
//	selected := glyph.GetSelectedText(text, cursor, anchor)
//
// Undo/redo:
//
//	um := glyph.NewUndoManager(100)
//	um.RecordMutation(result, insertedText, cursorBefore, anchorBefore)
//	if undo := um.Undo(currentText); undo != nil { ... }
//	if redo := um.Redo(currentText); redo != nil { ... }
//
// # Backends
//
// [DrawBackend] is the interface for plugging in a rendering framework.
// Five backends are provided:
//   - [github.com/go-gui-org/go-glyph/backend/ebitengine]: Ebitengine integration.
//   - [github.com/go-gui-org/go-glyph/backend/gpu]: raw OpenGL 3.3 / Metal.
//   - [github.com/go-gui-org/go-glyph/backend/web]: HTML Canvas (WASM).
//   - [github.com/go-gui-org/go-glyph/backend/android]: Android GPU.
//   - [github.com/go-gui-org/go-glyph/backend/ios]: iOS Metal.
//
// See the sub-package documentation for usage details.
//
// # Thread Safety
//
// [Context], [Renderer], [TextSystem], and [GlyphAtlas] are not safe
// for concurrent use. Call all glyph methods from the main/render goroutine.
//
// # Sub-packages
//
//   - [github.com/go-gui-org/go-glyph/accessibility]: screen-reader tree management.
//   - [github.com/go-gui-org/go-glyph/ime]: IME bridge (macOS/Linux).
package glyph
