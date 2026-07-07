// Package gpu provides a native GPU [glyph.DrawBackend] via CGo.
//
// Rendering uses Metal on macOS and native OpenGL 3.3 on Linux (GLX) and
// Windows (WGL). No platform requires SDL2; the caller owns the window and
// passes a native handle to [New].
//
// Create a backend with [New], then pass it to glyph.NewRenderer each frame:
//
//	// macOS (Metal)
//	b, err := gpu.New(metalLayerPtr, dpiScale)
//	// Linux (GLX):  gpu.New(unsafe.Pointer(&gpu.X11Handle{Display, Window}), dpiScale)
//	// Windows (WGL): gpu.New(unsafe.Pointer(&gpu.Win32Handle{HWND}), dpiScale)
//
//	renderer := glyph.NewRenderer(b, ctx)
//
//	// Per-frame loop:
//	b.BeginFrame()
//	renderer.DrawLayout(layout, x, y)
//	b.EndFrame(0, 0, 0, 1, logW, logH)
package gpu
