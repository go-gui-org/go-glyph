//go:build linux || darwin

package glyph

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// makeFIFO creates a named pipe at path. On cleanup it opens the pipe for writing
// without blocking. Before the fix, a reader is stuck in os.Open on the pipe. The
// writer open lets that reader return, so the test binary can exit.
func makeFIFO(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo not available: %v", err)
	}
	t.Cleanup(func() {
		// O_NONBLOCK makes a write-only open fail with ENXIO if no reader waits,
		// which is fine. We ignore that error.
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})
}

// waitOrFail runs fn in a goroutine and fails the test if it takes over 2 s.
func waitOrFail(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s blocked on a FIFO", what)
	}
}

// TestDescribeFontFacesSkipsFIFO is the regression test for font discovery
// hanging on a named pipe: os.Open on a FIFO waits for a writer, so a FIFO called
// "x.ttf" in a font directory blocked NewContext forever. A symlink to a FIFO must
// be rejected too.
func TestDescribeFontFacesSkipsFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.ttf")
	makeFIFO(t, fifo)
	link := filepath.Join(dir, "link.ttf")
	if err := os.Symlink(fifo, link); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{fifo, link} {
		var got []faceInfo
		waitOrFail(t, "describeFontFaces", func() { got = describeFontFaces(p) })
		if got != nil {
			t.Errorf("describeFontFaces(%s) = %v, want nil", p, got)
		}
	}
}

// TestWalkDirsSkipsFIFO checks that a FIFO in a scanned font directory does not
// stop the walk, and that a real font next to it is still found.
func TestWalkDirsSkipsFIFO(t *testing.T) {
	dir := t.TempDir()
	copyTestFont(t, dir)
	makeFIFO(t, filepath.Join(dir, "pipe.ttf"))

	ctx := newScanCtx()
	waitOrFail(t, "walkDirs", func() { newFontScan(ctx).walkDirs([]string{dir}, nil) })
	if len(ctx.fontPaths) == 0 {
		t.Error("real font was not found")
	}
}
