package ebitengine

import (
	"testing"

	"github.com/go-gui-org/go-glyph"
)

func TestBackend_New(t *testing.T) {
	b := New(nil, 1.0)
	if b == nil {
		t.Fatal("New returned nil")
	}
	if b.nextID != 0 {
		t.Errorf("nextID = %d, want 0", b.nextID)
	}
	if len(b.textures) != 0 {
		t.Error("textures should be empty")
	}
}

func TestBackend_NewZeroScale(t *testing.T) {
	b := New(nil, 0)
	if b.dpiScale != 1.0 {
		t.Errorf("dpiScale = %f, want 1.0 (clamped)", b.dpiScale)
	}
}

func TestBackend_SetTarget(t *testing.T) {
	b := New(nil, 1.0)
	if b.target != nil {
		t.Error("target should be nil initially")
	}
}

func TestBackend_NewTexture(t *testing.T) {
	b := New(nil, 1.0)
	id := b.NewTexture(100, 50)
	if id == 0 {
		t.Error("NewTexture should return non-zero ID")
	}
	if _, ok := b.textures[id]; !ok {
		t.Error("NewTexture should store image in textures map")
	}
	if b.widths[id] != 100 {
		t.Errorf("widths[%d] = %d, want 100", id, b.widths[id])
	}
	if b.heights[id] != 50 {
		t.Errorf("heights[%d] = %d, want 50", id, b.heights[id])
	}
}

func TestBackend_DeleteTexture(t *testing.T) {
	b := New(nil, 1.0)
	id := b.NewTexture(10, 10)
	b.DeleteTexture(id)
	if _, ok := b.textures[id]; ok {
		t.Error("DeleteTexture should remove from textures")
	}
	if _, ok := b.widths[id]; ok {
		t.Error("DeleteTexture should remove from widths")
	}
	if _, ok := b.heights[id]; ok {
		t.Error("DeleteTexture should remove from heights")
	}
}

func TestBackend_DeleteTextureInvalidNoPanic(t *testing.T) {
	b := New(nil, 1.0)
	b.DeleteTexture(99999) // Must not panic.
}

func TestBackend_DPIScale(t *testing.T) {
	b := New(nil, 2.5)
	if got := b.DPIScale(); got != 2.5 {
		t.Errorf("DPIScale = %f, want 2.5", got)
	}
}

// Verify DrawBackend interface is satisfied.
var _ glyph.DrawBackend = (*Backend)(nil)
