# Roadmap

Future work for go-glyph. Items are ordered by suggested execution phase.
Each active item notes the problem, where in the code it lives, and the
intended fix so a later session can resume without re-discovering context.

## Completed

### Darwin: font-name parse cache and metrics cache (2026-05-16, v1.7.1)

Heap profiles from a `go-term` 60s run showed per-layout-cache-miss
allocations dominated by `parseFamilyFromFontName`, `strings.Fields`,
and `ctFont.metrics` on every miss in `TextSystem.getOrCreateLayout`
(`glyph.go`).

**Landed:**
- Package-level font-name parse cache in `coretext_types_darwin.go`
  (512-entry cap, `sync.RWMutex`).
- `ctMetricsCache` in `cache_darwin.go` / `context_darwin.go` keyed by
  resolved font params (32-entry cap, drop-one-on-overflow).

**Not a problem:** `buildLayout` retained heap in pprof is the layout
cache working as designed. Per-glyph quad struct allocs in Metal backends
are outside go-glyph's scope.

### Glyph cache key hashing (2026-06-14)

`GlyphID` alone is not unique across fonts or sizes. Cache keys now
always hash text (plus features, with `GlyphID` as ligature tiebreaker).
Regression tests in `renderer_darwin_test.go`.

### CI and contributor hygiene (2026-06-14)

`CONTRIBUTING.md` and `CLAUDE.md` required `golangci-lint run ./...`,
but there was no `.golangci.yml` and CI ran only `go test`, `go build`,
and `go vet` — no lint, `-race`, or coverage.

**Landed:**
- `.golangci.yml` with `govet`, `errcheck`, `staticcheck`, `unused`,
  `gofmt`, `goimports` (v2 format, tuned for Linux CI / Pango backend).
- CI `lint` job on `ubuntu-latest` (`golangci-lint-action@v9`).
- `go vet` added to `test` (ubuntu) and `test-macos` jobs.
- Coverage artifact upload in `test` job (`go test -coverprofile`).
- Makefile targets: `check`, `test-race`, `coverage`.
- Fixed `staticcheck` ST1016 (inconsistent receiver names in `affine.go`).
- Fixed `gofmt`/`goimports` formatting in backend and example files.

---

## Future

### Phase B — Regression testing

**Problem:** Root package has 26 test files, but backends are largely
untested, there are no fuzz tests, and only 7 benchmarks exist. Platform
layout code diverges across four large `layout_*.go` files.

| Package              | Source | Tests |
| -------------------- | ------ | ----- |
| root `glyph`         | 80     | 26    |
| `accessibility`      | 5      | 1     |
| `backend/ebitengine` | 1      | 0     |
| `backend/gpu`        | 7      | 0     |
| `backend/sdl2`       | 3      | 0     |
| `ime`                | 2      | 1     |

**Where:** `renderer_darwin_test.go`, `layout_test.go`, `cache.go`,
`cache_darwin.go`, `cache_pango.go`, `backend/ebitengine/backend.go`,
`accessibility/`.

**Fix (prioritized):**
1. **Cache-key regression suite** — Extend the `renderer_darwin_test.go`
   pattern to atlas/layout/pango caches.
2. **Layout equivalence fixtures** — Shared table-driven cases in
   `layout_test.go` for line count, width/height, cursor positions, and
   word boundaries (ASCII, emoji, RTL where the platform can run).
3. **Backend contract tests** — `DrawBackend` fake to test `Renderer`
   tinting, transforms, and color-glyph passthrough; ebitengine smoke
   tests (pure Go, easiest win).
4. **Fuzz targets** — `buildUTF16ToByteSlice`, `layout_mutation.go`,
   `layout_query.go`.
5. **Accessibility** — Manager tree build/announce integration tests
   beyond emoji/announcer coverage.

**Avoid:** Full GPU pixel golden tests across backends (high flake cost).
Prefer geometry/metric assertions.

---

## Phase C — Performance (measure, then port)

**Problem:** wasm and android may still re-parse font names and re-query
metrics on every layout cache miss, as Darwin did before v1.7.1.

**Where:**
- `context_wasm.go` — `parseSizeFromFontName` / `parseFamilyFromFontName`
  per call.
- `context_android.go` — `metricsCache` declared; confirm wiring.
  `freetype_types_android.go` — FreeType metrics path.

**Fix:** Do not port preemptively. Profile a real client first (e.g.
`go-term` on wasm/android). If the alloc shape matches Darwin, apply the
same package-level parse cache and metrics cache pattern.

**Benchmarks:** Document a baseline entry point
(`go test -bench=LayoutText -benchmem ./...` on macOS). Expand existing
benchmarks in `atlas_test.go`, `bitmap_test.go`, and
`coretext_types_darwin_test.go` for cache-hit steady state.

---

## Phase D — Platform maintenance cost

**Problem:** ~102 build-tagged files; helpers like `parseSizeFromStyle`
and `mergeStyles` are duplicated across `layout_darwin.go`,
`layout_wasm.go`, and `layout_android.go`.

**Where:** `layout_*.go`, `doc.go`, `README.md`.

**Fix (incremental):**
- Extract pure-Go helpers into `layout.go` or `layout_shared.go` (no
  build tags). Keep CGO/platform calls in tagged files.
- Add a platform matrix to `doc.go` / `README.md` (shaper/rasterizer per
  OS; list all backends — SDL2, web/WASM, iOS, Android, not just
  Ebitengine and GPU).
- When touching a platform file, add a minimal platform-specific test
  (`layout_darwin_test.go`, `dwrite_smoke_windows_test.go` pattern).

**Do not:** Big-bang unification of `layout_*` implementations.

---

## Phase E — Feature completeness and release hygiene

**Windows color emoji:** `dwrite_windows.go` adds DirectWrite COLR
support; verify end-to-end rendering in an example and add a regression
test beyond `dwrite_smoke_windows_test.go`. Wire `isEmojiRune` in
`gdi_windows.go` if still unused.

**SVG diagonal gradients.** One remaining code TODO in
  `gui/svg_cache.go` — blocked on glyph angle support.

**Changelog:** Record unreleased fixes (glyph cache key hashing) in
`CHANGELOG.md` before the next tag.

---

## Already in good shape

- Multi-platform CI (Linux, macOS, Windows, WASM, iOS, Android)
- Substantial root-package tests (layout, atlas, composition, context,
  glyph)
- Recent releases: struct alignment (v1.8.1), bidi/shaped rendering
  (v1.8.0), Darwin alloc caches (v1.7.1)

Focus new effort on **enforcement** (lint/race), **regression nets**
(caches, layout fixtures, backends), and **measured** performance — not
broad rewrites.
