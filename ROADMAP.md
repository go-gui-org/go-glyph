# Roadmap

Future work for go-glyph. Ordered by suggested execution phase.
Each active item notes the problem, where it lives, and the intended fix.

## Completed

### CI and contributor hygiene (2026-06-14)

- `.golangci.yml` v2 config with `govet`, `errcheck`, `staticcheck`, `unused`, `gofmt`, `goimports`.
- CI `lint` job, `go vet` in test jobs, coverage artifact upload.
- Makefile targets: `check`, `test-race`, `coverage`.

### Phase B — Regression testing (2026-06-14)

9 new test files, 15 fuzz targets, 1,378 LOC: cache-key regression,
layout equivalence, backend contract tests, layout mutation/query fuzzing,
accessibility manager tests.

### Phase C — Performance benchmarks (2026-06-14)

`BenchmarkLayoutText`, `BenchmarkLayoutTextCached`, `BenchmarkLayoutRichText`
in `context_darwin_test.go`. Baseline on Apple M5: cached layout 68 ns/op
(0 allocs, ~500× faster than uncached 33.7 µs). `BenchmarkFontMetrics` updated.

### Phase D — Platform de-duplication (2026-06-14)

`parseSizeFromStyle` and `mergeStyles` extracted to `layout_shared.go` (pure Go,
no build tags). Duplicates removed from `layout_darwin.go`, `layout_wasm.go`,
`layout_android.go`. Platform matrix added to `doc.go` and `README.md`.

### Phase E — Features and release hygiene (2026-06-14)

- `GradientDiagonal` direction, `gradientColorForGlyph` de-duplicated from
  5 platform files into `gradient.go`.
- `parseSizeFromFontName` stub for Windows so `layout_shared.go` compiles.
- Changelog recorded unreleased fixes in `CHANGELOG.md` → v1.10.0.

### Darwin font-name parse cache and metrics cache (2026-05-16, v1.7.1)

Package-level caches in `coretext_types_darwin.go` and `cache_darwin.go`.

### Glyph cache key hashing (2026-06-14)

Cache keys now hash text + features with `GlyphID` as ligature tiebreaker.

### SDL2 extraction (2026-06-21)

- `backend/sdl2` extracted to separate Go module (`go.mod`, `go.sum`).
  Root module no longer depends on `github.com/veandco/go-sdl2`.
- `backend/gpu` Metal path no longer requires SDL2 C headers on macOS.
  `metalInit` accepts `CAMetalLayer*` directly instead of `SDL_Window*`.
- GPU backend tests added (`backend_test.go`, `backend_stub_test.go`) —
  batch logic, nil-window guard, DPI clamping, stub-platform paths.
- DPI guard hardened: `!(dpiScale > 0)` catches NaN, ±Inf, zero, negative
  in one IEEE-754 expression.
- `dpiScale` round-trip through CGo removed — C init functions ignore it;
  the value lives in `Backend.dpiScale`.
- Examples updated: platform-split `gpu_init_darwin.go` / `gpu_init_other.go`
  in `demo_gpu` and `showcase_gpu`.

---

## Future

### Phase C remaining — wasm/android profiling

**Problem:** wasm and android may re-parse font names and re-query metrics
on every layout cache miss, as Darwin did before v1.7.1.

**Where:** `context_wasm.go`, `context_android.go`, `freetype_types_android.go`.

**Fix:** Profile a real client first (e.g. `go-term` on wasm/android).
If the alloc shape matches Darwin, apply the same package-level parse cache
and metrics cache pattern.

### Phase D remaining — platform-specific tests

When touching a platform file, add a minimal platform-specific test
(`layout_darwin_test.go`, `dwrite_smoke_windows_test.go` pattern).
Do **not** big-bang unify `layout_*` implementations.

### Phase E remaining — Windows color emoji verification

DirectWrite COLR support and `isEmojiRune` are wired in
`dwrite_windows.go`, `layout_windows.go`, `renderer_load_windows.go`,
tested in `helpers_windows_test.go`. Verify end-to-end in an example
and add a regression test beyond the smoke test (requires Windows).

### Phase F — SDL2 elimination

**Goal:** Remove SDL2 from all backends entirely; archive `backend/sdl2`.

**Done:**
- `backend/sdl2` extracted to separate module (root no longer depends on go-sdl2).
- `backend/gpu` Metal path no longer requires SDL2 C headers on macOS.

**Remaining:**
- `backend/gpu` on Linux/Windows still requires SDL2 C headers and
  `libSDL2` for OpenGL context creation (`gl_linux.go`, `gl_windows.go`,
  `gl_sdl.c`, `gl_sdl.h`). Replace with native WGL (Windows) and GLX
  or EGL (Linux).
- Remove `WindowFlag()` and `WindowDrawableSize()` from `gl_linux.go`
  and `gl_windows.go`. Callers will use platform-native window creation.
- Delete example CGo glue (`gpu_init_darwin.go`). Callers provide a
  native layer/window handle directly to `gpu.New()`.
- Archive or delete `backend/sdl2/`.

### Phase G — GPU backend test coverage

**Done:** batch tests, nil-window guard, DPI clamping, stub-platform paths.

**Remaining:**
- Integration tests against a real GPU (headless EGL/Metal device).
- `DrawBackend` contract conformance suite exercising all interface methods.
- End-to-end: create window, render text, capture framebuffer, verify pixels.

---

## Already in good shape

- Multi-platform CI (Linux, macOS, Windows, WASM, iOS, Android).
- Substantial root-package tests (41 test files, 15 fuzz targets).
- Recent releases: v1.10.0 (gradients, de-dup, CI), v1.8.1 (struct alignment),
  v1.8.0 (bidi/shaped rendering), v1.7.1 (Darwin alloc caches).
