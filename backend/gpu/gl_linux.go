//go:build linux && !android

package gpu

/*
#cgo pkg-config: gl x11
#include "gl_glx.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// X11Handle identifies a caller-owned X11 window for the GLX backend.
// Callers pass unsafe.Pointer(&X11Handle{...}) as gpu.New's nativeWindow.
type X11Handle struct {
	Display unsafe.Pointer // Xlib Display*
	Window  uintptr        // X11 Window XID
}

// gpuCtx wraps the opaque C GLCtx pointer.
type gpuCtx struct {
	ptr *C.GLCtx
}

func gpuInitGo(nativeWindow unsafe.Pointer, dpiScale float32) (*gpuCtx, error) {
	h := (*X11Handle)(nativeWindow)
	if h == nil || h.Display == nil || h.Window == 0 {
		return nil, fmt.Errorf("gpu: nativeWindow must be a valid *X11Handle")
	}
	ctx := C.glCtxInit(h.Display, C.ulong(h.Window), C.float(dpiScale))
	if ctx == nil {
		return nil, fmt.Errorf("gpu: glCtxInit failed")
	}
	return &gpuCtx{ptr: ctx}, nil
}

func (m *gpuCtx) newTexture(w, h int) uint64 {
	return uint64(C.glCtxNewTex(m.ptr, C.int(w), C.int(h)))
}

func (m *gpuCtx) updateTexture(id uint64, data []byte, w, h int) {
	if len(data) == 0 {
		return
	}
	C.glCtxUpdateTex(m.ptr, C.uint64_t(id),
		unsafe.Pointer(&data[0]), C.int(w), C.int(h))
}

func (m *gpuCtx) deleteTexture(id uint64) {
	C.glCtxDeleteTex(m.ptr, C.uint64_t(id))
}

func (m *gpuCtx) render(verts []Vertex, cmds []drawCmd,
	clearR, clearG, clearB, clearA float32,
	logicalW, logicalH int) error {

	var vp, cp unsafe.Pointer
	vc := len(verts)
	cc := len(cmds)
	if vc > 0 {
		vp = unsafe.Pointer(&verts[0])
	}
	if cc > 0 {
		cp = unsafe.Pointer(&cmds[0])
	}
	rc := C.glCtxRender(m.ptr,
		vp, C.int(vc),
		cp, C.int(cc),
		C.float(clearR), C.float(clearG),
		C.float(clearB), C.float(clearA),
		C.int(logicalW), C.int(logicalH))
	if rc != 0 {
		return fmt.Errorf("gpu: glCtxRender failed")
	}
	return nil
}

func (m *gpuCtx) drawableSize() (int, int) {
	var w, h C.int
	C.glCtxGetDrawableSize(m.ptr, &w, &h)
	return int(w), int(h)
}

func (m *gpuCtx) destroy() {
	if m.ptr != nil {
		C.glCtxDestroy(m.ptr)
		m.ptr = nil
	}
}
