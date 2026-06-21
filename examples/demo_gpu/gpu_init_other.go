//go:build !darwin || ios

package main

import (
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

func gpuWindowFlag() uint32 {
	return sdl.WINDOW_OPENGL
}

func gpuDrawableSize(win *sdl.Window) (int, int) {
	w, h := win.GLGetDrawableSize()
	return int(w), int(h)
}

func gpuInitHandle(win *sdl.Window) (unsafe.Pointer, func()) {
	return unsafe.Pointer(win), func() {}
}
