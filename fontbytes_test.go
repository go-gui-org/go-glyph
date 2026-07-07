//go:build !js

package glyph

import (
	"bytes"
	"os"
	"testing"
)

// TestWriteTempFontFile verifies the temp-file backing used by
// AddFontBytes: bytes land in a real file and the caller can remove it.
func TestWriteTempFontFile(t *testing.T) {
	data := []byte("not a real font, just bytes")
	path, err := writeTempFontFile(data)
	if err != nil {
		t.Fatalf("writeTempFontFile: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty temp path")
	}
	defer func() { _ = os.Remove(path) }()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp font: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("temp font contents = %q, want %q", got, data)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove temp font: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("temp font still present after remove: %v", err)
	}
}
