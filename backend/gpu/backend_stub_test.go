//go:build (!darwin && !linux && !windows) || ios || android

package gpu

import (
	"math"
	"strings"
	"testing"
	"unsafe"
)

var dummy int // dummy allocation for unsafe.Pointer in stub tests

func TestNew_StubReturnsError(t *testing.T) {
	be, err := New(unsafe.Pointer(&dummy), 1.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error from stub gpuInitGo, got nil")
	}
	if be != nil {
		t.Errorf("expected nil backend on error, got %v", be)
	}
	if !strings.Contains(err.Error(), "no GPU backend") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNew_ZeroDPIScaleClamped(t *testing.T) {
	// Zero dpi is clamped to 1.0 before gpuInitGo. The stub
	// sees 1.0 and returns its usual error.
	be, err := New(unsafe.Pointer(&dummy), 0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no GPU backend") {
		t.Errorf("unexpected error (dpi clamp may have failed): %v", err)
	}
}

func TestNew_NegativeDPIScaleClamped(t *testing.T) {
	be, err := New(unsafe.Pointer(&dummy), -2.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error, got nil")
	}
}

func TestNew_NaN_DPIScaleClamped(t *testing.T) {
	// NaN > 0 is false, so !(dpiScale > 0) catches it. The stub
	// receives 1.0 and returns its usual error — no NaN propagation.
	be, err := New(unsafe.Pointer(&dummy), float32(math.NaN()))
	if err == nil {
		be.Destroy()
		t.Fatal("expected error from stub after NaN clamp, got nil")
	}
}

func TestNew_InfDPIScalePassthrough(t *testing.T) {
	// +Inf passes through the guard (not NaN, not <= 0). The stub
	// ignores the dpiScale argument and returns its usual error,
	// so this validates that +Inf does not crash.
	be, err := New(unsafe.Pointer(&dummy), float32(math.Inf(1)))
	if err == nil {
		be.Destroy()
		t.Fatal("expected error, got nil")
	}
}

func TestWindowFlag_Stub(t *testing.T) {
	if f := WindowFlag(); f != 0 {
		t.Errorf("expected WindowFlag=0 on stub, got %d", f)
	}
}

func TestWindowDrawableSize_Stub(t *testing.T) {
	w, h := WindowDrawableSize(unsafe.Pointer(&dummy))
	if w != 0 || h != 0 {
		t.Errorf("expected WindowDrawableSize=(0,0), got (%d,%d)", w, h)
	}
}
