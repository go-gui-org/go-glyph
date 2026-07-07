//go:build linux

package main

import (
	"fmt"
	"unsafe"

	"github.com/go-gui-org/go-glyph/backend/gpu"

	"github.com/veandco/go-sdl2/sdl"
)

// gpuWindowFlag returns the SDL window flag for OpenGL rendering.
// SDL creates the window with a GLX-compatible visual, which the
// native GLX backend then matches when creating its context.
func gpuWindowFlag() uint32 {
	return sdl.WINDOW_OPENGL
}

// gpuDrawableSize returns the physical drawable size of an
// SDL OpenGL window in pixels.
func gpuDrawableSize(win *sdl.Window) (int, int) {
	w, h := win.GLGetDrawableSize()
	return int(w), int(h)
}

// gpuInitHandle extracts the native X11 Display and Window from the
// SDL window and returns a *gpu.X11Handle for gpu.New. The GPU backend
// links GLX/X11, not SDL2, so consumers can supply their own windowing.
func gpuInitHandle(win *sdl.Window) (unsafe.Pointer, func()) {
	info, err := win.GetWMInfo()
	if err != nil {
		fmt.Println("gpu: GetWMInfo:", err)
		return nil, func() {}
	}
	if info.Subsystem != sdl.SYSWM_X11 {
		fmt.Println("gpu: GLX backend requires the X11 SDL video driver")
		return nil, func() {}
	}
	x11 := info.GetX11Info()
	h := &gpu.X11Handle{
		Display: x11.Display,
		Window:  uintptr(x11.Window),
	}
	return unsafe.Pointer(h), func() {}
}
