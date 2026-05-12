# Roadmap

Future work for go-glyph. Each item has a "why" anchored in measured
allocation behavior, a "where" pointing at the code, and a sketch of
the fix so a later session can resume without re-discovering context.

## Performance: per-layout font-name re-parsing and metrics requery

### Why

Heap profile of a terminal emulator client (sibling repo `go-term`,
`cmd/demo` 60s run) shows the following per-LayoutText allocators
dominate on darwin:

| Allocator                                | Allocations / 60s |
| ---------------------------------------- | ----------------- |
| `parseFamilyFromFontName`                | 3.24M flat        |
| `strings.Fields` (via the above)         | 2.39M             |
| `ctFont.metrics`                         | 1.97M             |
| `internal/bytealg.MakeNoZero` (Fields)   | 1.61M             |
| `(*Context).buildLayout`                 | 2.62M             |

These fire on every cache miss in `TextSystem.getOrCreateLayout`
(`glyph.go:246`). A terminal generates a new cache key for every
unique row-text on every frame, so cache-miss frequency is high and
each miss currently re-parses the Pango-style font name and re-queries
CoreText for font metrics — values that are stable across an entire
session's typical workload (one or two `TextStyle.FontName` strings,
one or two sizes).

Workload-specific note: `go-term` already trims trailing whitespace
from run text to collapse tail-padding variants into shared cache
entries (`term/widget.go` `flushRun`). That cut roughly 25% of
glyph-quad allocations but did nothing for these per-miss costs — the
miss path itself is the bottleneck.

### Where

Darwin (highest leverage — most users today):

- `coretext_types_darwin.go:283` — `ctFont.metrics()` calls into
  cgo `ctFontGetMetrics` every invocation. Result is a pure function
  of the `CTFontRef`, which is owned by a `ctFont` value that already
  lives in a context cache. Memoize on the `ctFont` (or on the cgo
  ref). The triple `(ascent, descent, leading)` is three `float64`s —
  a four-word struct.
- `coretext_types_darwin.go:334` — `parseFamilyFromFontName(name)`
  is allocation-heavy via `strings.Fields` and `strings.ToLower` in
  the style-word loop. Same input (`style.FontName`) on every call
  for a given session.
- `coretext_types_darwin.go:312` — `parseSizeFromFontName(name)`
  also calls `strings.Fields`. Same input each call.
- `coretext_types_darwin.go:289` — `resolveFontFamilyDarwin(fontName)`
  is the outer wrapper that calls `parseFamilyFromFontName`. Called
  from `resolveCTFontParams` at line 156, which is in the layout hot
  path.

Existing infra to lean on:

- `cache.go` defines an LRU `metricsCache` keyed by `uint64`.
  `context_darwin.go:35` *already declares* `metrics metricsCache` on
  the darwin Context. It is initialized in `newContext` (line 52) but
  appears to never be read on darwin — confirm with
  `grep -n 'ctx\.metrics\.' *_darwin.go layout_darwin.go`. Wire it in.

Other platforms (lower priority but same shape):

- `context_wasm.go:103,109` — same `parseSizeFromFontName` /
  `parseFamilyFromFontName` per call.
- `context_android.go:26` — `metrics metricsCache` declared, same
  question of whether it's wired.
- `freetype_types_android.go:247,318` — equivalent FreeType path.

### Sketch

Two independent fixes; each can land separately.

**Fix 1 — memoize font-name parsing on `TextStyle.FontName`.**

The parse output is `(family string, size float32)` plus style flags.
A `map[string]parsedFontName` (or `sync.Map` if contention matters,
which it shouldn't — these are mostly main-thread) on the `Context`
keyed by raw `FontName`. The map will hold a handful of entries even
for long sessions. Place the cache near `resolveCTFontParams` so it
covers both `parseFamilyFromFontName` and `parseSizeFromFontName` in
one lookup.

Test plan: existing `coretext_types_darwin_test.go` exercises
`parseFamilyFromFontName` directly — keep those passing. Add a
benchmark that calls `resolveCTFontParams` in a tight loop with a
fixed `TextStyle` and assert zero allocs after warm-up.

**Fix 2 — wire `ctx.metrics` cache into `ctFont.metrics`.**

Key the cache by `ctFont.ref` (CTFontRef is a pointer-sized handle
suitable for `uint64`). On miss, call `ctFontGetMetrics`; on hit,
return the stored triple. Cache capacity in `newContext` already
exists — confirm it's non-zero; if it is hard-coded to 0, set a small
positive default (16–32 is plenty).

Test plan: `context_test.go:53` (`TestFontMetrics`) already exercises
the metrics path. Add a benchmark that calls `FontMetrics` repeatedly
with the same cfg and assert zero allocs after warm-up.

### Expected impact

Eliminating these per-miss allocations should cut the alloc_objects
top-N by roughly 50% over a `go-term` 60s scrolling session and drop
NumGC by another 15–25% on top of the structural wins already in
place. Steady-state per-frame heap should be dominated by
`metalGlyphBackend.DrawTexturedQuad` (one quad per glyph, intrinsic).

### Don't get distracted by

- `buildLayout` itself shows ~30 MB *in_use* in pprof — that is the
  layout cache holding entries, working as designed, not a leak.
- `DrawTexturedQuad` allocation count is the metal backend's per-glyph
  quad struct. That's a different problem (pool the structs in the
  backend), outside go-glyph's scope.

### Unresolved

- Confirm the `metrics` field on `Context` is unused on darwin (the
  grep above) rather than wired through a path I missed.
- Decide whether the font-name parse cache lives on `Context` (per
  session) or as a package-level `sync.Map` (process-wide). Per-
  context is safer for tests that build many contexts; package-level
  is slightly faster.
- Pick the eviction policy for the font-name parse cache. The set is
  small; a plain `map` with no eviction is probably fine.
