//go:build linux && !android

package gpu

import (
	"testing"
	"unsafe"
)

// The X11Handle guard in gpuInitGo must reject invalid handles before
// any CGo call, so these run without an X display or GPU. Without the
// guard a nil Display would reach XGetWindowAttributes in C and crash.

func TestNew_InvalidX11Handle_NilDisplay(t *testing.T) {
	h := &X11Handle{Display: nil, Window: 42}
	be, err := New(unsafe.Pointer(h), 1.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error for nil Display, got nil")
	}
	if be != nil {
		t.Errorf("expected nil backend on error, got %v", be)
	}
}

func TestNew_InvalidX11Handle_ZeroWindow(t *testing.T) {
	// Non-nil Display so only the zero-Window branch triggers; the
	// guard returns before Display is ever dereferenced in C.
	dummy := new(byte)
	h := &X11Handle{Display: unsafe.Pointer(dummy), Window: 0}
	be, err := New(unsafe.Pointer(h), 1.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error for zero Window, got nil")
	}
	if be != nil {
		t.Errorf("expected nil backend on error, got %v", be)
	}
}
