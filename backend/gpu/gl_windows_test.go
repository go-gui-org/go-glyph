//go:build windows

package gpu

import (
	"testing"
	"unsafe"
)

// The Win32Handle guard in gpuInitGo must reject invalid handles before
// any CGo call, so these run without a window or GPU. Without the guard a
// zero HWND would reach GetDC in C and fail deeper in.

func TestNew_InvalidWin32Handle_Nil(t *testing.T) {
	be, err := New(unsafe.Pointer((*Win32Handle)(nil)), 1.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error for nil Win32Handle, got nil")
	}
	if be != nil {
		t.Errorf("expected nil backend on error, got %v", be)
	}
}

func TestNew_InvalidWin32Handle_ZeroHWND(t *testing.T) {
	h := &Win32Handle{HWND: 0}
	be, err := New(unsafe.Pointer(h), 1.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error for zero HWND, got nil")
	}
	if be != nil {
		t.Errorf("expected nil backend on error, got %v", be)
	}
}
