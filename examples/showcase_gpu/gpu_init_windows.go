//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"github.com/go-gui-org/go-glyph/backend/gpu"

	"github.com/veandco/go-sdl2/sdl"
)

// gpuWindowFlag returns the SDL window flag for OpenGL rendering.
// SDL creates the window with a pixel format the native WGL backend then
// reuses when creating its context.
func gpuWindowFlag() uint32 {
	return sdl.WINDOW_OPENGL
}

// gpuDrawableSize returns the physical drawable size of an
// SDL OpenGL window in pixels.
func gpuDrawableSize(win *sdl.Window) (int, int) {
	w, h := win.GLGetDrawableSize()
	return int(w), int(h)
}

// gpuInitHandle extracts the native Win32 HWND from the SDL window and
// returns a *gpu.Win32Handle for gpu.New. The GPU backend links WGL/GDI,
// not SDL2, so consumers can supply their own windowing.
func gpuInitHandle(win *sdl.Window) (unsafe.Pointer, func()) {
	info, err := win.GetWMInfo()
	if err != nil {
		fmt.Println("gpu: GetWMInfo:", err)
		return nil, func() {}
	}
	if info.Subsystem != sdl.SYSWM_WINDOWS {
		fmt.Println("gpu: WGL backend requires the Windows SDL video driver")
		return nil, func() {}
	}
	h := &gpu.Win32Handle{
		HWND: uintptr(info.GetWindowsInfo().Window),
	}
	return unsafe.Pointer(h), func() {}
}
