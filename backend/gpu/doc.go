// Package gpu provides a native GPU [glyph.DrawBackend] via CGo.
//
// On macOS, rendering uses Metal into a caller-provided CAMetalLayer
// (no SDL2 required). On Linux and Windows, rendering uses OpenGL 3.3
// into an SDL2 window.
//
// Create a backend with [New], then pass it to glyph.NewRenderer each frame:
//
//	// macOS (Metal)
//	b, err := gpu.New(metalLayerPtr, dpiScale)
//	// Linux / Windows (OpenGL)
//	b, err := gpu.New(sdlWindowPtr, dpiScale)
//
//	renderer := glyph.NewRenderer(b, ctx)
//
//	// Per-frame loop:
//	b.BeginFrame()
//	renderer.DrawLayout(layout, x, y)
//	b.EndFrame(0, 0, 0, 1, logW, logH)
package gpu
