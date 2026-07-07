//go:build windows

package gpu

/*
#cgo LDFLAGS: -lopengl32 -lgdi32
#include "gl_wgl.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Win32Handle identifies a caller-owned Win32 window for the WGL backend.
// Callers pass unsafe.Pointer(&Win32Handle{...}) as gpu.New's nativeWindow.
type Win32Handle struct {
	HWND uintptr // Win32 window handle
}

// gpuCtx wraps the opaque C GLCtx pointer.
type gpuCtx struct {
	ptr *C.GLCtx
}

func gpuInitGo(nativeWindow unsafe.Pointer, dpiScale float32) (*gpuCtx, error) {
	h := (*Win32Handle)(nativeWindow)
	if h == nil || h.HWND == 0 {
		return nil, fmt.Errorf("gpu: nativeWindow must be a valid *Win32Handle")
	}
	ctx := C.glCtxInit(C.uintptr_t(h.HWND), C.float(dpiScale))
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
