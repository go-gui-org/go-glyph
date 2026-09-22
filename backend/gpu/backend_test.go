package gpu

import (
	"math"
	"testing"

	glyph "github.com/go-gui-org/go-glyph"
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

// --- UpdateTexture guard tests (no CGo) ---
//
// UpdateTexture must reject a buffer smaller than w*h*4 and an unknown
// texture ID before calling into the C backend, which reads w*h*4 bytes
// and would read out of bounds otherwise. A nil gpu makes a regressed
// guard panic, so "no panic" is a real assertion of the early return.

func TestUpdateTexture_ShortBufferNoOp(t *testing.T) {
	const id = glyph.TextureID(7)
	b := &Backend{
		gpu:     nil, // any CGo call would panic
		widths:  map[glyph.TextureID]int{id: 2},
		heights: map[glyph.TextureID]int{id: 2},
	}
	// 2x2 RGBA needs 16 bytes; supply fewer.
	b.UpdateTexture(id, make([]byte, 4))
}

func TestUpdateTexture_UnknownIDNoOp(t *testing.T) {
	b := &Backend{
		gpu:     nil,
		widths:  map[glyph.TextureID]int{},
		heights: map[glyph.TextureID]int{},
	}
	// Unknown id → w==h==0 → guard returns before touching gpu.
	b.UpdateTexture(glyph.TextureID(99), make([]byte, 16))
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

// --- Transformed-fill tests (pure Go, no CGo) ---

func TestDrawFilledRectTransformed_Corners(t *testing.T) {
	b := &Backend{
		gpu:     nil, // batch append is pure Go; any CGo call would panic
		widths:  map[glyph.TextureID]int{},
		heights: map[glyph.TextureID]int{},
	}
	dst := glyph.Rect{X: 1, Y: 2, Width: 10, Height: 20}
	tr := glyph.AffineTranslation(5, 7)
	b.DrawFilledRectTransformed(dst, glyph.Color{R: 255, A: 255}, tr)

	if len(b.batch.verts) != 6 {
		t.Fatalf("expected 6 verts, got %d", len(b.batch.verts))
	}
	// Translation shifts every corner by (5, 7).
	want := [][2]float32{{6, 9}, {16, 9}, {16, 29}, {6, 29}}
	got := [][2]float32{
		{b.batch.verts[0].PosX, b.batch.verts[0].PosY},
		{b.batch.verts[1].PosX, b.batch.verts[1].PosY},
		{b.batch.verts[2].PosX, b.batch.verts[2].PosY},
		{b.batch.verts[5].PosX, b.batch.verts[5].PosY},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("corner %d = %v, want %v", i, got[i], want[i])
		}
	}
	if b.batch.cmds[0].textureID != 0 {
		t.Errorf("fill textureID = %d, want 0 (white tex)",
			b.batch.cmds[0].textureID)
	}
}

func TestDrawFilledRectTransformed_NonFiniteNoOp(t *testing.T) {
	b := &Backend{
		gpu:     nil,
		widths:  map[glyph.TextureID]int{},
		heights: map[glyph.TextureID]int{},
	}
	nan := float32(math.NaN())
	b.DrawFilledRectTransformed(
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{R: 255, G: 255, B: 255, A: 255},
		glyph.AffineTransform{XX: 1, YY: 1, X0: nan})
	if len(b.batch.verts) != 0 {
		t.Errorf("NaN transform drew %d verts, want 0",
			len(b.batch.verts))
	}
}

func TestDrawTexturedQuadTransformed_NonFiniteNoOp(t *testing.T) {
	const id = glyph.TextureID(3)
	b := &Backend{
		gpu:     nil,
		widths:  map[glyph.TextureID]int{id: 8},
		heights: map[glyph.TextureID]int{id: 8},
	}
	nan := float32(math.NaN())
	b.DrawTexturedQuadTransformed(id,
		glyph.Rect{X: 0, Y: 0, Width: 8, Height: 8},
		glyph.Rect{X: 0, Y: 0, Width: 8, Height: 8},
		glyph.Color{R: 255, G: 255, B: 255, A: 255},
		glyph.AffineTransform{XX: 1, YY: 1, X0: nan})
	if len(b.batch.verts) != 0 {
		t.Errorf("NaN transform drew %d verts, want 0",
			len(b.batch.verts))
	}
}

func TestDrawFilledRectTransformed_DegenerateSizeNoOp(t *testing.T) {
	b := &Backend{
		gpu:     nil,
		widths:  map[glyph.TextureID]int{},
		heights: map[glyph.TextureID]int{},
	}
	for _, dst := range []glyph.Rect{
		{Width: 0, Height: 10},
		{Width: 10, Height: 0},
		{Width: -5, Height: 10},
		{Width: 10, Height: float32(math.Inf(1))},
	} {
		b.DrawFilledRectTransformed(dst,
			glyph.Color{A: 255}, glyph.AffineIdentity())
	}
	if len(b.batch.verts) != 0 {
		t.Errorf("degenerate rects drew %d verts, want 0",
			len(b.batch.verts))
	}
}

// --- UpdateTextureRect guard tests (no CGo) ---

// The backend must advertise sub-rectangle uploads, or the atlas falls
// back to a whole-page upload for every new glyph.
var _ glyph.RectTextureUpdater = (*Backend)(nil)

func TestValidTextureRect(t *testing.T) {
	const texW, texH = 8, 4
	const stride = texW * 4
	full := stride * texH
	for _, tc := range []struct {
		name            string
		dataLen, stride int
		x, y, w, h      int
		want            bool
	}{
		{"whole texture", full, stride, 0, 0, texW, texH, true},
		{"inner region", full, stride, 2, 1, 3, 2, true},
		{"last pixel", full, stride, texW - 1, texH - 1, 1, 1, true},
		{"zero width", full, stride, 0, 0, 0, 1, false},
		{"negative x", full, stride, -1, 0, 2, 2, false},
		{"past right edge", full, stride, 7, 0, 2, 1, false},
		{"past bottom edge", full, stride, 0, 3, 1, 2, false},
		{"stride below row", full, 8, 0, 0, 3, 1, false},
		{"stride not pixels", full, stride + 1, 0, 0, 1, 1, false},
		{"short data", full - 1, stride, 0, 0, texW, texH, false},
		{"huge region", full, stride, 0, 0, math.MaxInt, 1, false},
	} {
		got := validTextureRect(texW, texH, tc.dataLen, tc.stride,
			tc.x, tc.y, tc.w, tc.h)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A rejected region must return before the CGo call; a nil gpu makes a
// regressed guard panic.
func TestUpdateTextureRect_InvalidNoOp(t *testing.T) {
	const id = glyph.TextureID(7)
	b := &Backend{
		gpu:     nil,
		widths:  map[glyph.TextureID]int{id: 4},
		heights: map[glyph.TextureID]int{id: 4},
	}
	b.UpdateTextureRect(id, make([]byte, 8), 16, 0, 0, 4, 4)       // short
	b.UpdateTextureRect(id, make([]byte, 64), 16, 2, 0, 4, 1)      // outside
	b.UpdateTextureRect(glyph.TextureID(99), make([]byte, 64), 16, // unknown
		0, 0, 1, 1)
}
