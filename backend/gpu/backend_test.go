package gpu

import (
	"testing"
)

// --- Batch tests (pure Go, no CGo) ---

func TestBatchAppend6SingleQuad(t *testing.T) {
	var b batch
	b.append6(42,
		Vertex{PosX: 0, PosY: 0, TexU: 0, TexV: 0},
		Vertex{PosX: 1, PosY: 0, TexU: 1, TexV: 0},
		Vertex{PosX: 1, PosY: 1, TexU: 1, TexV: 1},
		Vertex{PosX: 0, PosY: 1, TexU: 0, TexV: 1},
	)

	if len(b.verts) != 6 {
		t.Fatalf("expected 6 verts, got %d", len(b.verts))
	}
	if len(b.cmds) != 1 {
		t.Fatalf("expected 1 cmd, got %d", len(b.cmds))
	}

	// Verify triangle fan split: v0, v1, v2, v0, v2, v3.
	// verts[3] repeats verts[0] (v0), verts[4] repeats verts[2] (v2).
	if b.verts[3] != b.verts[0] {
		t.Errorf("verts[3] should repeat verts[0] (v0), got %+v vs %+v",
			b.verts[3], b.verts[0])
	}
	if b.verts[4] != b.verts[2] {
		t.Errorf("verts[4] should repeat verts[2] (v2), got %+v vs %+v",
			b.verts[4], b.verts[2])
	}

	cmd := b.cmds[0]
	if cmd.textureID != 42 {
		t.Errorf("expected texID 42, got %d", cmd.textureID)
	}
	if cmd.firstVert != 0 {
		t.Errorf("expected firstVert 0, got %d", cmd.firstVert)
	}
	if cmd.vertCount != 6 {
		t.Errorf("expected vertCount 6, got %d", cmd.vertCount)
	}
}

func TestBatchAppendMultipleQuads(t *testing.T) {
	var b batch
	zero := Vertex{}

	for i := 0; i < 3; i++ {
		b.append6(uint64(i), zero, zero, zero, zero)
	}
	if len(b.verts) != 18 {
		t.Errorf("expected 18 verts (3×6), got %d", len(b.verts))
	}
	if len(b.cmds) != 3 {
		t.Errorf("expected 3 cmds, got %d", len(b.cmds))
	}
	if b.cmds[0].firstVert != 0 {
		t.Errorf("cmd 0 firstVert: want 0, got %d", b.cmds[0].firstVert)
	}
	if b.cmds[1].firstVert != 6 {
		t.Errorf("cmd 1 firstVert: want 6, got %d", b.cmds[1].firstVert)
	}
	if b.cmds[2].firstVert != 12 {
		t.Errorf("cmd 2 firstVert: want 12, got %d", b.cmds[2].firstVert)
	}
}

func TestBatchReset(t *testing.T) {
	var b batch
	zero := Vertex{}
	b.append6(1, zero, zero, zero, zero)
	b.append6(2, zero, zero, zero, zero)

	b.reset()

	if len(b.verts) != 0 {
		t.Errorf("verts not cleared: got %d", len(b.verts))
	}
	if len(b.cmds) != 0 {
		t.Errorf("cmds not cleared: got %d", len(b.cmds))
	}
	// Verify backing array capacity is retained (sliced to zero,
	// not reallocated).
	if cap(b.verts) == 0 {
		t.Errorf("verts capacity lost after reset")
	}
	if cap(b.cmds) == 0 {
		t.Errorf("cmds capacity lost after reset")
	}
}

// TestNew_NilWindow verifies the nil-pointer guard in New returns
// an error before reaching CGo. Safe on all platforms.
func TestNew_NilWindow(t *testing.T) {
	be, err := New(nil, 1.0)
	if err == nil {
		be.Destroy()
		t.Fatal("expected error for nil nativeWindow, got nil")
	}
	if be != nil {
		t.Errorf("expected nil backend on error, got %v", be)
	}
}
