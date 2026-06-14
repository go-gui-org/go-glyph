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

### Phase B — Regression testing (2026-06-14)

**Problem:** Root package had 26 test files, but backends were largely
untested, no fuzz tests existed, and platform layout code diverged across
four large `layout_*.go` files.

**Landed (9 new test files, 15 fuzz targets, 1,378 LOC):**

- **Cache-key regression:** `cache_pango_test.go` (7 tests, Pango
  metricsCache LRU), `renderer_test.go` (11 tests, glyph cache eviction,
  page keys, subpixel bins), `glyph_test.go` (5 tests, layout cache TTL
  pruning and 25% eviction).
- **Layout equivalence:** `layout_equivalence_test.go` (8 shared
  invariants verified on both Pango and Darwin).
- **Backend contract:** `backend_contract_test.go`
  (callRecordingBackend + 8 interface tests),
  `backend/ebitengine/backend_test.go` (7 smoke tests).
- **Fuzz:** `layout_darwin_fuzz_test.go` (FuzzBuildUTF16ToByteSlice),
  `layout_mutation_fuzz_test.go` (6 targets), `layout_query_fuzz_test.go`
  (6 targets).
- **Accessibility:** `accessibility/manager_test.go` (11 tests, Manager
  tree build, recording, announcer integration).

**Lint config:** Migrated `.golangci.yml` to v2 schema (`e63b567`).
CGo export comments added to `pango_cgo.go` (`3f62840`).

---

## Future

### Phase C — Performance (measure, then port) (2026-06-14 benchmarks landed)

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

**Landed — Benchmarks:**
- `BenchmarkLayoutText`, `BenchmarkLayoutTextCached`,
  `BenchmarkLayoutRichText` added to `context_darwin_test.go`.
- `BenchmarkFontMetrics` updated with warm-up + `b.ResetTimer()`.
- Baseline captured on Apple M5:
  - `LayoutText`: 33.7 µs/op, 34.8 KB, 103 allocs
  - `LayoutTextCached`: 68 ns/op, 0 B, 0 allocs (cache-hit ~500× faster)
  - `LayoutRichText`: 76.5 µs/op, 20.3 KB, 92 allocs
  - All cache-hit benchmarks (FontMetrics, ResolveCTFontParams,
    LookupParsedFontName) confirm 0 allocs/op.
- Entry point: `go test -bench=LayoutText -benchmem ./...` on macOS.

**Remaining:** Profile wasm/android client (go-term) to measure uncached
alloc shape before porting Darwin-style caches.

---

### Phase D — Platform maintenance cost (2026-06-14 de-duplication landed)

**Problem:** ~102 build-tagged files; helpers like `parseSizeFromStyle`
and `mergeStyles` are duplicated across `layout_darwin.go`,
`layout_wasm.go`, and `layout_android.go`.

**Where:** `layout_*.go`, `doc.go`, `README.md`.

**Landed — De-duplication:**
- Extracted `parseSizeFromStyle` and `mergeStyles` to `layout_shared.go`
  (pure Go, no build tags). Both functions were byte-for-byte identical
  across all three platform files.
- Removed duplicates from `layout_darwin.go`, `layout_wasm.go`,
  `layout_android.go`; removed now-unused `cmp` imports.
- Moved `mergeStyles` tests from `layout_darwin_test.go` to
  `layout_shared_test.go` (no build tags, runs on all platforms).
- All tests pass, zero lint issues.

**Remaining:**
- Add a platform matrix to `doc.go` / `README.md` (shaper/rasterizer per
  OS; list all backends — SDL2, web/WASM, iOS, Android, not just
  Ebitengine and GPU).
- When touching a platform file, add a minimal platform-specific test
  (`layout_darwin_test.go`, `dwrite_smoke_windows_test.go` pattern).

**Do not:** Big-bang unification of `layout_*` implementations.

---

### Phase E — Feature completeness and release hygiene

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
  glyph) plus full Phase B regression suite (41 test files, 15 fuzz targets)
- Recent releases: struct alignment (v1.8.1), bidi/shaped rendering
  (v1.8.0), Darwin alloc caches (v1.7.1)

Focus new effort on **Phase C** (measured performance on wasm/android),
**Phase D** (platform maintenance de-duplication), and **Phase E**
(feature completeness: Windows color emoji, SVG gradients, changelog)
— not broad rewrites.
