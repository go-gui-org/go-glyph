// Package glyph provides high-quality text shaping, layout, and rendering
// for GPU-accelerated applications. It wraps Pango (for Unicode layout,
// bidirectional text, and complex script support) and FreeType (for glyph
// rasterization with subpixel positioning) behind a backend-agnostic
// [DrawBackend] interface.
//
// # Quick start
//
//	ctx, err := glyph.NewContext(2.0) // 2× Retina scale
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	layout, err := ctx.LayoutText("Hello, world!", glyph.TextConfig{
//	    Style: glyph.TextStyle{FontName: "Sans 18"},
//	    Block: glyph.DefaultBlockStyle(),
//	})
//
//	renderer := glyph.NewRenderer(backend, ctx)
//	renderer.DrawLayout(layout, 10, 10)
//
// # Architecture
//
// [Context] owns FreeType and Pango state. [Renderer] draws shaped layouts
// through a [DrawBackend]. Two backends are provided:
//   - [github.com/go-gui-org/go-glyph/backend/ebitengine]: Ebitengine integration.
//   - [github.com/go-gui-org/go-glyph/backend/gpu]: raw OpenGL 3.3 via SDL2.
//
// # Thread Safety
//
// [Context], [Renderer], [TextSystem], and [GlyphAtlas] are not safe
// for concurrent use. In a typical application, call all glyph
// methods from the main/render goroutine.
//
// # Sub-packages
//
//   - [github.com/go-gui-org/go-glyph/accessibility]: screen-reader tree management.
//   - [github.com/go-gui-org/go-glyph/ime]: IME bridge (macOS/Linux).
package glyph
