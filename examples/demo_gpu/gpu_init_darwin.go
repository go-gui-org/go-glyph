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

const sdlWindowMetal = 0x00002000

func gpuWindowFlag() uint32 {
	return sdlWindowMetal
}

func gpuDrawableSize(win *sdl.Window) (int, int) {
	var w, h C.int
	C.SDL_Metal_GetDrawableSize((*C.SDL_Window)(unsafe.Pointer(win)), &w, &h)
	return int(w), int(h)
}

func gpuInitHandle(win *sdl.Window) (unsafe.Pointer, func()) {
	view := C.SDL_Metal_CreateView((*C.SDL_Window)(unsafe.Pointer(win)))
	layer := C.SDL_Metal_GetLayer(view)
	return layer, func() { C.SDL_Metal_DestroyView(view) }
}
