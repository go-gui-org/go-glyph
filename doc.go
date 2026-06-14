// Package glyph provides high-quality text shaping, layout, and rendering
// for GPU-accelerated applications. It uses a platform-appropriate shaper
// and rasterizer per operating system, exposed behind a backend-agnostic
// [DrawBackend] interface.
//
// # Platform matrix
//
//	OS          Shaper              Rasterizer
//	Linux/BSD   Pango + HarfBuzz    FreeType + FontConfig
//	macOS *     CoreText            CoreText / CoreGraphics
//	Windows     GDI + DirectWrite   GDI + DirectWrite
//	Android     FreeType            FreeType
//	WASM        Canvas2D            Canvas2D
//
//	* macOS can also use the Pango/FreeType stack with the glyph_pango
//	  build tag.
//
// # Quick start
//
//	backend := ebitengine.NewBackend() // or sdl2, gpu, web, etc.
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
//
// # Architecture
//
// [Context] owns platform-specific shaper and font state. [Renderer] draws
// shaped layouts through a [DrawBackend]. Six backends are provided:
//   - [github.com/go-gui-org/go-glyph/backend/ebitengine]: Ebitengine integration.
//   - [github.com/go-gui-org/go-glyph/backend/gpu]: raw OpenGL 3.3 / Metal.
//   - [github.com/go-gui-org/go-glyph/backend/sdl2]: SDL2 rendering.
//   - [github.com/go-gui-org/go-glyph/backend/web]: HTML Canvas (WASM).
//   - [github.com/go-gui-org/go-glyph/backend/android]: Android GPU.
//   - [github.com/go-gui-org/go-glyph/backend/ios]: iOS Metal.
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
