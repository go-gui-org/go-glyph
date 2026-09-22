//go:build (!darwin && !linux && !windows) || ios || android

package gpu

import (
	"fmt"
	"unsafe"
)

type gpuCtx struct{}

func gpuInitGo(_ unsafe.Pointer, _ float32) (*gpuCtx, error) {
	return nil, fmt.Errorf("gpu: no GPU backend for this platform")
}

func (m *gpuCtx) newTexture(_, _ int) uint64                              { return 0 }
func (m *gpuCtx) updateTexture(_ uint64, _ []byte, _, _ int)              {}
func (m *gpuCtx) updateTextureRect(_ uint64, _ []byte, _, _, _, _, _ int) {}
func (m *gpuCtx) deleteTexture(_ uint64)                                  {}
func (m *gpuCtx) render(_ []Vertex, _ []drawCmd, _, _, _, _ float32, _, _ int) error {
	return fmt.Errorf("gpu: no GPU backend for this platform")
}
func (m *gpuCtx) drawableSize() (int, int) { return 0, 0 }
func (m *gpuCtx) destroy()                 {}

func WindowFlag() uint32                             { return 0 }
func WindowDrawableSize(_ unsafe.Pointer) (int, int) { return 0, 0 }
