package glyph

import (
	"runtime"
	"testing"
)

// skipOnWASM skips a test that checks native-only behavior when it runs as
// js/wasm (make test-wasm). The wasm build draws through the page canvas with
// fillText, not through DrawBackend or the glyph cache, and has no file system.
func skipOnWASM(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("js/wasm: " + why)
	}
}
