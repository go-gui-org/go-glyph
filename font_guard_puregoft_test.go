//go:build linux || darwin || windows

package glyph

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// A panic inside the locked call must still release the face mutex. If it
// did not, every later color render of the shared face would deadlock.
func TestLockedFaceCallReleasesLockOnPanic(t *testing.T) {
	cf := &cachedFace{}
	ok := cf.lockedCall(func() { panic("bad table") })
	if ok {
		t.Fatal("lockedCall reported success after a panic")
	}
	if !cf.mu.TryLock() {
		t.Fatal("face mutex still held after panic")
	}
	cf.mu.Unlock()
}

func TestLockedFaceCallOK(t *testing.T) {
	cf := &cachedFace{}
	n := 0
	if !cf.lockedCall(func() { n++ }) {
		t.Fatal("lockedCall reported failure without a panic")
	}
	if n != 1 {
		t.Fatalf("fn ran %d times, want 1", n)
	}
	if !cf.mu.TryLock() {
		t.Fatal("face mutex still held after normal return")
	}
	cf.mu.Unlock()
}

func TestGuardGlyphLoadConvertsPanic(t *testing.T) {
	res, err := guardGlyphLoad(func() (LoadGlyphResult, error) { panic("x") })
	if err == nil {
		t.Fatal("expected an error after a panic")
	}
	if !strings.HasPrefix(err.Error(), "glyph: ") {
		t.Fatalf("error %q lacks the \"glyph: \" prefix", err)
	}
	if res != (LoadGlyphResult{}) {
		t.Fatalf("result not zero after panic: %+v", res)
	}
}

func TestGuardGlyphLoadPassesThrough(t *testing.T) {
	want := errors.New("load failed")
	_, err := guardGlyphLoad(func() (LoadGlyphResult, error) {
		return LoadGlyphResult{}, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// One defective font must drop only itself: describeFontFaces returns nil
// and AddFontFile reports a normal error instead of crashing the process.
func TestDescribeFontFacesRecoversPanic(t *testing.T) {
	ctx, err := NewContext(1)
	if err != nil {
		t.Skipf("NewContext: %v", err)
	}
	defer ctx.Free()

	var path string
	for _, p := range ctx.fontPaths {
		path, _ = splitFacePath(p)
		break
	}
	if path == "" {
		t.Skip("no installed font found")
	}

	orig := describeFace
	t.Cleanup(func() { describeFace = orig })
	describeFace = func(*ot.Loader, []byte) (font.Description, []byte) {
		panic("bad table")
	}

	if got := describeFontFaces(path); got != nil {
		t.Fatalf("describeFontFaces = %v, want nil after panic", got)
	}
	err = ctx.AddFontFile(path)
	if err == nil {
		t.Fatal("AddFontFile succeeded for a font whose parse panics")
	}
	if !strings.HasPrefix(err.Error(), "glyph: ") {
		t.Fatalf("error %q lacks the \"glyph: \" prefix", err)
	}
}
