//go:build windows

package glyph

import "syscall"

// SetDPIAwareWindows marks the current process as per-monitor DPI
// aware v2. Must be called before creating a window so window queries
// return physical pixels and Windows DWM does not bilinear-magnify
// the backbuffer — otherwise glyphs rasterize at 1x logical pixels
// and look thicker/fuzzier on high-DPI displays.
//
// Order of attempts: SetProcessDpiAwarenessContext (Win 10 1703+),
// SetProcessDpiAwareness (Win 8.1+), SetProcessDPIAware (Vista+).
// Safe to call multiple times; later calls are no-ops once awareness
// is set.
func SetDPIAwareWindows() {
	user32 := syscall.NewLazyDLL("user32.dll")

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is -4 as a HANDLE.
	const perMonitorV2 = ^uintptr(3)
	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		if r, _, _ := p.Call(perMonitorV2); r != 0 {
			return
		}
	}

	// PROCESS_PER_MONITOR_DPI_AWARE = 2.
	shcore := syscall.NewLazyDLL("shcore.dll")
	if p := shcore.NewProc("SetProcessDpiAwareness"); p.Find() == nil {
		if r, _, _ := p.Call(2); r == 0 {
			return
		}
	}

	// Vista+ system-DPI-only fallback.
	_, _, _ = user32.NewProc("SetProcessDPIAware").Call()
}
