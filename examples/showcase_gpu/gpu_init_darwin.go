//go:build darwin && !ios

package main

/*
#cgo darwin,arm64 CFLAGS: -I/opt/homebrew/include/SDL2
#cgo darwin,amd64 CFLAGS: -I/usr/local/include/SDL2
#include <SDL_metal.h>
*/
import "C"
import (
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

// sdlWindowMetal is the SDL window-creation flag for Metal rendering.
// sdL2's Go binding omits this constant.
const sdlWindowMetal = 0x00002000

// gpuWindowFlag returns the SDL window flag for Metal rendering.
func gpuWindowFlag() uint32 {
	return sdlWindowMetal
}

// gpuDrawableSize returns the physical drawable size of an
// SDL Metal window in pixels.
func gpuDrawableSize(win *sdl.Window) (int, int) {
	var w, h C.int
	C.SDL_Metal_GetDrawableSize((*C.SDL_Window)(unsafe.Pointer(win)), &w, &h)
	return int(w), int(h)
}

// gpuInitHandle extracts the CAMetalLayer from an SDL window
// for passing to gpu.New. The returned cleanup function destroys
// the intermediate SDL_MetalView; call it after gpu.New succeeds.
func gpuInitHandle(win *sdl.Window) (unsafe.Pointer, func()) {
	view := C.SDL_Metal_CreateView((*C.SDL_Window)(unsafe.Pointer(win)))
	layer := C.SDL_Metal_GetLayer(view)
	return layer, func() { C.SDL_Metal_DestroyView(view) }
}
