//go:build !darwin || ios

package main

import (
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

// gpuWindowFlag returns the SDL window flag for OpenGL rendering.
func gpuWindowFlag() uint32 {
	return sdl.WINDOW_OPENGL
}

// gpuDrawableSize returns the physical drawable size of an
// SDL OpenGL window in pixels.
func gpuDrawableSize(win *sdl.Window) (int, int) {
	w, h := win.GLGetDrawableSize()
	return int(w), int(h)
}

// gpuInitHandle returns the SDL_Window pointer for passing
// to gpu.New on platforms that use OpenGL. The cleanup function
// is a no-op (the window is destroyed separately).
func gpuInitHandle(win *sdl.Window) (unsafe.Pointer, func()) {
	return unsafe.Pointer(win), func() {}
}
