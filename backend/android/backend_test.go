//go:build android

package android

import (
	"testing"

	"github.com/go-gui-org/go-glyph"
)

func newTestBackend() *Backend {
	return &Backend{
		widths:   make(map[glyph.TextureID]int),
		heights:  make(map[glyph.TextureID]int),
		dpiScale: 2.0,
	}
}

func TestDPIScale(t *testing.T) {
	b := newTestBackend()
	if b.DPIScale() != 2.0 {
		t.Errorf("DPIScale: got %f, want 2.0", b.DPIScale())
	}
}

func TestBeginFrame_ResetsBatch(t *testing.T) {
	b := newTestBackend()
	b.DrawFilledRect(
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{R: 255, G: 255, B: 255, A: 255},
	)
	if len(b.batch.verts) == 0 {
		t.Fatal("expected vertices before BeginFrame")
	}
	b.BeginFrame()
	if len(b.batch.verts) != 0 {
		t.Errorf("verts after BeginFrame: %d, want 0", len(b.batch.verts))
	}
	if len(b.batch.cmds) != 0 {
		t.Errorf("cmds after BeginFrame: %d, want 0", len(b.batch.cmds))
	}
}

func TestDrawFilledRect_Basic(t *testing.T) {
	b := newTestBackend()
	b.DrawFilledRect(
		glyph.Rect{X: 10, Y: 20, Width: 100, Height: 50},
		glyph.Color{R: 255, G: 0, B: 0, A: 255},
	)
	if len(b.batch.verts) != 6 {
		t.Fatalf("verts: got %d, want 6", len(b.batch.verts))
	}
	if len(b.batch.cmds) != 1 {
		t.Fatalf("cmds: got %d, want 1", len(b.batch.cmds))
	}
	if b.batch.cmds[0].textureID != 0 {
		t.Errorf("textureID: got %d, want 0", b.batch.cmds[0].textureID)
	}
	// Check quad corners (tri1: v0,v1,v2  tri2: v0,v2,v3).
	v := b.batch.verts
	if v[0].PosX != 10 || v[0].PosY != 20 {
		t.Errorf("v0: got (%f,%f), want (10,20)", v[0].PosX, v[0].PosY)
	}
	if v[1].PosX != 110 || v[1].PosY != 20 {
		t.Errorf("v1: got (%f,%f), want (110,20)", v[1].PosX, v[1].PosY)
	}
	if v[2].PosX != 110 || v[2].PosY != 70 {
		t.Errorf("v2: got (%f,%f), want (110,70)", v[2].PosX, v[2].PosY)
	}
	// v[3] == v0 (second triangle), v[4] == v2, v[5] == v3
	if v[5].PosX != 10 || v[5].PosY != 70 {
		t.Errorf("v3: got (%f,%f), want (10,70)", v[5].PosX, v[5].PosY)
	}
	// Color should be set on all vertices.
	for i, vi := range v {
		if vi.R != 255 || vi.G != 0 || vi.B != 0 || vi.A != 255 {
			t.Errorf("v[%d] color: got (%d,%d,%d,%d), want (255,0,0,255)",
				i, vi.R, vi.G, vi.B, vi.A)
		}
	}
}

func TestDrawFilledRect_ZeroWidth(t *testing.T) {
	b := newTestBackend()
	b.DrawFilledRect(
		glyph.Rect{X: 0, Y: 0, Width: 0, Height: 50},
		glyph.Color{},
	)
	if len(b.batch.verts) != 0 {
		t.Errorf("zero-width rect produced %d vertices", len(b.batch.verts))
	}
}

func TestDrawFilledRect_ZeroHeight(t *testing.T) {
	b := newTestBackend()
	b.DrawFilledRect(
		glyph.Rect{X: 0, Y: 0, Width: 50, Height: 0},
		glyph.Color{},
	)
	if len(b.batch.verts) != 0 {
		t.Errorf("zero-height rect produced %d vertices", len(b.batch.verts))
	}
}

func TestDrawFilledRect_NegativeSize(t *testing.T) {
	b := newTestBackend()
	b.DrawFilledRect(
		glyph.Rect{X: 0, Y: 0, Width: -10, Height: 50},
		glyph.Color{},
	)
	if len(b.batch.verts) != 0 {
		t.Errorf("negative-width rect produced %d vertices", len(b.batch.verts))
	}
}

func TestDrawFilledRect_Coalesces(t *testing.T) {
	b := newTestBackend()
	c := glyph.Color{R: 255, G: 255, B: 255, A: 255}
	b.DrawFilledRect(glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10}, c)
	b.DrawFilledRect(glyph.Rect{X: 20, Y: 0, Width: 10, Height: 10}, c)
	if len(b.batch.verts) != 12 {
		t.Fatalf("verts: got %d, want 12", len(b.batch.verts))
	}
	// Both use textureID 0 → coalesced into one command.
	if len(b.batch.cmds) != 1 {
		t.Errorf("cmds: got %d, want 1 (coalesced)", len(b.batch.cmds))
	}
}

func TestDrawTexturedQuad_UVs(t *testing.T) {
	b := newTestBackend()
	b.widths[1] = 200
	b.heights[1] = 100

	b.DrawTexturedQuad(1,
		glyph.Rect{X: 50, Y: 25, Width: 100, Height: 50},
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{R: 255, G: 255, B: 255, A: 255},
	)
	if len(b.batch.verts) != 6 {
		t.Fatalf("verts: got %d, want 6", len(b.batch.verts))
	}
	// UV v0 = (50/200, 25/100) = (0.25, 0.25)
	v0 := b.batch.verts[0]
	if v0.TexU != 0.25 || v0.TexV != 0.25 {
		t.Errorf("v0 UV: got (%f,%f), want (0.25,0.25)", v0.TexU, v0.TexV)
	}
	// UV v2 = (150/200, 75/100) = (0.75, 0.75)
	v2 := b.batch.verts[2]
	if v2.TexU != 0.75 || v2.TexV != 0.75 {
		t.Errorf("v2 UV: got (%f,%f), want (0.75,0.75)", v2.TexU, v2.TexV)
	}
}

func TestDrawTexturedQuad_ZeroTextureDims(t *testing.T) {
	b := newTestBackend()
	b.widths[1] = 0
	b.heights[1] = 0

	b.DrawTexturedQuad(1,
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{},
	)
	if len(b.batch.verts) != 0 {
		t.Error("zero-dim texture should produce no vertices")
	}
}

func TestDrawTexturedQuad_MissingTexture(t *testing.T) {
	b := newTestBackend()
	// Texture 99 not registered → widths/heights return 0.
	b.DrawTexturedQuad(99,
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{},
	)
	if len(b.batch.verts) != 0 {
		t.Error("unregistered texture should produce no vertices")
	}
}

func TestDrawTexturedQuadTransformed_Identity(t *testing.T) {
	b := newTestBackend()
	b.widths[1] = 100
	b.heights[1] = 100

	b.DrawTexturedQuadTransformed(1,
		glyph.Rect{X: 0, Y: 0, Width: 100, Height: 100},
		glyph.Rect{X: 10, Y: 20, Width: 30, Height: 40},
		glyph.Color{R: 255, G: 255, B: 255, A: 255},
		glyph.AffineIdentity(),
	)
	if len(b.batch.verts) != 6 {
		t.Fatalf("verts: got %d, want 6", len(b.batch.verts))
	}
	v := b.batch.verts
	if v[0].PosX != 10 || v[0].PosY != 20 {
		t.Errorf("v0: got (%f,%f), want (10,20)", v[0].PosX, v[0].PosY)
	}
	// v2 = bottom-right = (10+30, 20+40) = (40, 60)
	if v[2].PosX != 40 || v[2].PosY != 60 {
		t.Errorf("v2: got (%f,%f), want (40,60)", v[2].PosX, v[2].PosY)
	}
}

func TestDrawTexturedQuadTransformed_Translation(t *testing.T) {
	b := newTestBackend()
	b.widths[1] = 100
	b.heights[1] = 100

	tr := glyph.AffineTranslation(100, 200)
	b.DrawTexturedQuadTransformed(1,
		glyph.Rect{X: 0, Y: 0, Width: 100, Height: 100},
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{R: 255, G: 255, B: 255, A: 255},
		tr,
	)
	v := b.batch.verts
	if v[0].PosX != 100 || v[0].PosY != 200 {
		t.Errorf("v0: got (%f,%f), want (100,200)", v[0].PosX, v[0].PosY)
	}
	if v[2].PosX != 110 || v[2].PosY != 210 {
		t.Errorf("v2: got (%f,%f), want (110,210)", v[2].PosX, v[2].PosY)
	}
}

func TestDrawTexturedQuadTransformed_ZeroTexDims(t *testing.T) {
	b := newTestBackend()
	b.widths[1] = 0
	b.heights[1] = 100

	b.DrawTexturedQuadTransformed(1,
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10},
		glyph.Color{},
		glyph.AffineIdentity(),
	)
	if len(b.batch.verts) != 0 {
		t.Error("zero-width texture should produce no vertices")
	}
}

func TestMixedDrawCommands_BatchOrdering(t *testing.T) {
	b := newTestBackend()
	b.widths[1] = 64
	b.heights[1] = 64
	c := glyph.Color{R: 255, G: 255, B: 255, A: 255}

	// filled rect (texID=0), textured quad (texID=1), filled rect (texID=0)
	b.DrawFilledRect(glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10}, c)
	b.DrawTexturedQuad(1,
		glyph.Rect{X: 0, Y: 0, Width: 64, Height: 64},
		glyph.Rect{X: 0, Y: 0, Width: 10, Height: 10}, c,
	)
	b.DrawFilledRect(glyph.Rect{X: 20, Y: 0, Width: 10, Height: 10}, c)

	if len(b.batch.cmds) != 3 {
		t.Fatalf("cmds: got %d, want 3 (alternating textures)", len(b.batch.cmds))
	}
	if b.batch.cmds[0].textureID != 0 {
		t.Errorf("cmd[0] tex: got %d, want 0", b.batch.cmds[0].textureID)
	}
	if b.batch.cmds[1].textureID != 1 {
		t.Errorf("cmd[1] tex: got %d, want 1", b.batch.cmds[1].textureID)
	}
	if b.batch.cmds[2].textureID != 0 {
		t.Errorf("cmd[2] tex: got %d, want 0", b.batch.cmds[2].textureID)
	}
}
