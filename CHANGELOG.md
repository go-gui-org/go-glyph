# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
