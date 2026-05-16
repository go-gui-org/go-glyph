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

### Status: Darwin — DONE (2026-05-16)

Both fixes landed. Uncommitted as of this writing.

**Fix 1 — font-name parse cache** (`coretext_types_darwin.go`):
Package-level `fontNameParseCache map[string]parsedFontName` guarded
by `fontNameParseMu sync.RWMutex`. Hot path is an RLock map read —
zero allocations. Bounded at 512 entries; overflow is silently dropped
(set is tiny in practice). `lookupParsedFontName` replaces the
per-call `parseFamilyFromFontName` + `parseSizeFromFontName` +
`strings.ToLower` chain throughout `resolveCTFontParams`.
Cache is process-wide (not per-Context) — faster and safe because the
parse is a pure function of the font name string.

**Fix 2 — metrics cache** (`cache_darwin.go`, `context_darwin.go`):
New `ctMetricsCache` keyed by `ctMetricsKey{family, size, bold, italic}`
(resolved params, not CTFontRef pointer — correct across font
create/close cycles). Capacity 32, drop-one-on-overflow policy.
`Context.fontMetrics` and `Context.metricsForStyle` wrap the cache;
`metricsForStyle` avoids the `newCTFont` CGo call entirely on cache
hits. Old `metricsCache` (LRU, `uint64`-keyed) is now build-tagged
`!darwin || glyph_pango`; darwin uses `ctMetricsCache` exclusively.

**Resolved questions:**
- `ctx.metrics` was declared but unwired on darwin — now wired via the
  new `ctMetricsCache` type.
- Font-name parse cache is package-level (process-wide), not per-Context.
- Eviction policy: cap at 512 with silent drop for parse cache; cap at
  32 with drop-one for metrics cache.

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

### Other platforms — TODO if profiling shows the same shape

The same two fixes apply to wasm and android if heap profiles show
equivalent hot spots. Darwin is done; apply only if measured.

- `context_wasm.go:103,109` — `parseSizeFromFontName` /
  `parseFamilyFromFontName` per call. Same package-level parse-cache
  pattern applies.
- `context_android.go` — `metrics metricsCache` declared; confirm
  whether wired. `freetype_types_android.go:247,318` has the equivalent
  FreeType metrics path.

Don't preemptively port — measure first.
