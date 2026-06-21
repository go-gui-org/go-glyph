# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.11.0] - 2026-06-21

### Added

- **GPU backend tests:** batch vertex ordering, multi-quad offset, reset
  capacity retention, nil-window error guard, stub-platform DPI clamping
  and NaN/Inf handling (`backend/gpu/backend_test.go`,
  `backend/gpu/backend_stub_test.go`).

### Changed

- **`backend/sdl2` extracted to separate Go module** at
  `github.com/go-gui-org/go-glyph/backend/sdl2`. Root module no longer
  depends on `github.com/veandco/go-sdl2`. Downstream users who do not
  use the SDL2 backend will no longer pull the binding.
- **`backend/gpu` Metal path no longer requires SDL2 C headers on macOS.**
  `metalInit` accepts `CAMetalLayer*` directly instead of `SDL_Window*`.
  Removed `-I/opt/homebrew/include/SDL2` and `-I/usr/local/include/SDL2`
  CGO flags. The caller (e.g. go-gui) owns window and layer creation.
- **Breaking (macOS):** `gpu.New()` parameter is now `CAMetalLayer*`
  instead of `SDL_Window*`. `WindowFlag()` and `WindowDrawableSize()`
  removed from macOS Metal backend (still present on Linux/Windows OpenGL).
- **DPI guard hardened:** `!(dpiScale > 0)` catches NaN, ±Inf, zero, and
  negative in one IEEE-754 expression. Removed `dpiScale` round-trip
  through CGo — C init functions never used it.
- Updated `demo_gpu` and `showcase_gpu` examples with platform-split
  helpers for the new API.
- CI: bumped GitHub Actions to latest major versions.

### Security

- `gpu.New()` nil-window guard returns error before CGo instead of passing
  a nil pointer to C.
- `metalInit` NULL guard in C returns early before any allocation.
- `backend/gpu` Metal path links only Apple frameworks (Metal, QuartzCore,
  Foundation) — no third-party C libraries.

## [1.10.0] - 2026-06-14

### Added

- **Phase B regression tests** (9 new test files, 15 fuzz targets, 1,378 LOC):
  cache-key regression, layout equivalence across Pango/Darwin, backend contract
  tests, accessibility manager tests, layout mutation and query fuzzing.
- **Phase C benchmarks:** `BenchmarkLayoutText`, `BenchmarkLayoutTextCached`,
  `BenchmarkLayoutRichText` in `context_darwin_test.go`. Baseline on Apple M5:
  cached layout 68 ns/op (0 allocs, ~500× faster than uncached 33.7 µs).
- Platform matrix in `doc.go` and `README.md` (shaper/rasterizer per OS).
  All six backends documented: ebitengine, gpu, sdl2, web, android, ios.
- **`GradientDiagonal`** direction for gradient text fills (top-left to
  bottom-right). De-duplicated `gradientColorForGlyph` across all draw backends.

### Changed

- **Phase D de-duplication:** Extracted `parseSizeFromStyle` and `mergeStyles`
  to `layout_shared.go` (pure Go, no build tags). Removed duplicates from
  `layout_darwin.go`, `layout_wasm.go`, `layout_android.go`.
- Prerequisites in `README.md` now list platform-specific requirements
  (macOS and Windows need no native C libraries for the root package).
- Module path: `github.com/mike-ward/go-glyph` → `github.com/go-gui-org/go-glyph`.
- Migrated `.golangci.yml` to v2 schema.
- README audited for stale sections: expanded backend table (6 backends),
  examples table (8 entries), clarified macOS prerequisites, removed false
  go.mod claim.

### Fixed

- **Glyph cache key collision:** cache keys now hash text (plus features, with
  GlyphID as ligature tiebreaker) — `GlyphID` alone was not unique across fonts
  or sizes.
- FreeType download: added SourceForge fallback and XZ validation.
- CGo export comments: added Go-style comments to single exported consts in
  `pango_cgo.go` for staticcheck compliance.
- Ebitengine CI: fixed headless environment detection in `examples/gpu`.
- Build tag on `layout_shared.go` and `layout_shared_test.go` (was breaking
  non-Darwin builds).
- `parseSizeFromFontName` stub for Linux and unsupported platforms.
- CONTRIBUTING license corrected to MIT (was incorrectly PolyForm
  Noncommercial).

## [1.8.1] - 2026-06-03

### Changed

- Reorder struct fields for optimal memory alignment across 10 files; reduces struct sizes by eliminating inter-field padding

## [1.8.0] - 2026-05-24

### Added

- Darwin: shaped glyph rendering via CTLine cluster shaping with OpenType calt/liga support
- Darwin: paragraph-level bidirectional reordering via `golang.org/x/text/unicode/bidi`
- Context: monospace-aware fallback font families with deduplication

### Fixed

- Darwin: duplicate-glyph bug in RTL shapeTextClusters

### Changed

- examples: add `golang.org/x/text` to go.mod/go.sum for all example modules

## [1.7.1] - 2026-05-16

### Fixed

- Darwin: wrap CGo entry points in `@autoreleasepool` to drain Apple-framework autoreleases
- Darwin: memoize font-name parsing; wire metrics cache to eliminate per-miss CGo allocs

### Changed

- examples/showcase_gpu: discard `Destroy`/`EndFrame` errors explicitly (errcheck)

## [1.7.0] - 2026-04-30

### Added

- Darwin: CoreText backend is now the default; legacy Pango path moved
  behind the `glyph_pango` build tag
- Darwin: arbitrary OpenType feature tags forwarded to CoreText
- Darwin: font variation axes and inline-object placeholders
- Darwin: per-run style by splitting per-line Items at run boundaries

### Fixed

- Darwin: preserve RGB channels for color emoji
- Darwin: pass sub/sup OpenType features through to CoreText
- Darwin: restore sub/sup size-scaling fallback
- README.md formatting

### Changed

- Darwin: drop dead types, gate metrics cache helpers behind build tag

## [1.6.5] - 2026-04-13

### Changed

- Modernize codebase with Go 1.26 idioms: min/max builtins, for-range loops,
  clear(), variadic max(), deleted redundant helpers

## [1.6.4] - 2026-04-08

### Added

- DirectWrite color emoji support on Windows
- Claude automation prompts and configuration

### Fixed

- Windows DPI handling in DirectWrite backend

### Changed

- Tidy example module dependencies to match root go.mod

## [1.6.3] - 2026-04-05

### Changed

- Update dependencies: ebiten v2.9.9, uniseg v0.4.7, purego v0.10.0

## [1.6.2] - 2026-04-05

### Fixed

- Correctness and robustness issues from adversarial code review

### Changed

- Windows CI: native CGo job with MSYS2, dynamic path resolution

## [1.6.1] - 2026-04-02

### Fixed

- Windows: `AddFontFile` now registers fonts via `AddFontResourceExW`
  instead of silently succeeding as a no-op
- Windows: grapheme clusters now render full cluster text instead of
  only the first rune (fixes emoji sequences and combining marks)
- Windows: malformed Pango markup returns error and falls back to
  plain text instead of silently truncating content

### Changed

- README: description and architecture reflect multi-platform backends
  (GDI on Windows, CoreText on iOS)
