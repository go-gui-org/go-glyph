# Plan 013: Commit stops walking the whole layout cache every frame

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 6d6a018..HEAD -- glyph.go` If
> any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none (if plan 011 landed first, `glyph.go` has new nil guards.
  They do not touch the functions below.)
- **Category**: perf
- **Planned at**: commit `6d6a018`, 2026-11-13

## Why this matters

`TextSystem.Commit` is called once per frame. It calls `pruneCache`, which
iterates over every entry of the layout cache (up to `maxCacheEntries` = 8192)
to drop the entries not used for `evictionAge` (5000 ms). Entries can only
expire on a 5-second scale, so scanning the full map 60 times a second is almost
all wasted work, and the cost grows with the cache. Separately,
`evictOldestLayouts`, which runs whenever an insert finds the cache full,
allocates an `[]int64` of the full cache size each time. After this plan,
`pruneCache` scans at most a few times per eviction period, and eviction reuses
one scratch slice. The first step measures the cost. If it is too small to
matter, the plan stops there.

## Current state

- `glyph.go` (untagged) — `TextSystem`, its cache, and the two functions.

Fields, `glyph.go:26-33`:

```go
type TextSystem struct {
	ctx             *Context
	renderer        *Renderer
	cache           map[uint64]*cachedLayout
	evictionAge     int64    // Milliseconds. Default 5000.
	maxCacheEntries int      // Max layout cache entries. Default 8192.
	tempFontFiles   []string // Temp files backing AddFontBytes fonts.
}
```

`Commit`, `glyph.go:147-153`:

```go
func (ts *TextSystem) Commit() {
	if ts.renderer == nil {
		return
	}
	ts.renderer.Commit()
	ts.pruneCache()
}
```

`pruneCache` and `evictOldestLayouts`, `glyph.go:431-470` (abridged):

```go
func (ts *TextSystem) pruneCache() {
	if len(ts.cache) == 0 {
		return
	}
	now := time.Now().UnixMilli()
	for k, item := range ts.cache {
		if now-item.lastAccess > ts.evictionAge {
			delete(ts.cache, k)
		}
	}
}

// evictOldestLayouts drops the oldest 25% of cache entries.
func (ts *TextSystem) evictOldestLayouts() {
	n := len(ts.cache)
	if n == 0 {
		return
	}
	times := make([]int64, 0, n)
	for _, item := range ts.cache {
		times = append(times, item.lastAccess)
	}
	slices.Sort(times)
	...
```

`cachedLayout` (top of `glyph.go`) has `layout Layout`, `lastAccess int64` (Unix
ms) and `hitTest bool`. `getOrCreateLayout` sets `lastAccess` on every hit and
insert.

No test covers `pruneCache` or `evictOldestLayouts` today. No benchmark covers
`Commit`.

Conventions: the repo optimizes for zero steady-state allocations
(`BenchmarkTerminalFrame1500`, `bench_terminal_frame_test.go`, must stay at
`0 allocs/op`). TextSystem test exemplar: `TestTextSystemDrawText` in
`test_helpers_test.go:91-114` (`newRecordingBackend()`, `NewTextSystem`,
`defer ts.Free()`).

## Commands you will need

| Purpose   | Command                                                                    | Expected on success |
| --------- | -------------------------------------------------------------------------- | ------------------- |
| Benchmark | `go test -run '^$' -bench 'BenchmarkCommitFullCache' -benchmem -count=5 .` | numbers recorded    |
| Tests     | `go test -run 'TestLayoutCache' -count=1 -v .`                             | all `PASS`          |
| Allocs    | `go test -run '^$' -bench 'BenchmarkTerminalFrame1500' -benchmem .`        | `0 allocs/op`       |
| Full gate | `make prepush`                                                             | exit 0              |

## Scope

**In scope**:

- `glyph.go` — `TextSystem` (two new fields), `Commit` (no change expected),
  `pruneCache`, `evictOldestLayouts`
- `layout_cache_test.go` (create; untagged)
- `CHANGELOG.md` — one bullet under `### Changed`

**Out of scope**:

- Cache key hashing (`getCacheKey`).
- The eviction policy itself (25% oldest on overflow, `evictionAge` expiry).
  Only when and how often the work runs changes.
- The renderer's glyph cache (`cache.go`, `renderer_*.go`).

## Git workflow

- Branch: `advisor/013-layout-cache-prune-throttle`
- One commit, e.g. `perf: throttle layout-cache pruning; reuse eviction scratch`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Benchmark the current cost

Create `layout_cache_test.go` (`package glyph`, untagged). Add
`BenchmarkCommitFullCache`: build a TextSystem with `newRecordingBackend()`,
then fill `ts.cache` directly with 8192 entries,
`ts.cache[uint64(i)] = &cachedLayout{lastAccess: time.Now().UnixMilli()}`. This
is white-box and avoids real layout cost. Call `b.ReportAllocs()` and
`b.ResetTimer()`, then call `ts.Commit()` in the loop. Entries stay fresh,
because the benchmark runs well under 5 s, so nothing is deleted and every
iteration scans the full map.

Add `BenchmarkEvictOldestLayouts`: for each iteration, refill the cache to 8192
entries with increasing `lastAccess` (outside the timer, using
`b.StopTimer()`/`b.StartTimer()`), then call `ts.evictOldestLayouts()`.

**Verify**: run the benchmark command and record `ns/op` and `allocs/op` for
both. Expected roughly: Commit tens of µs/op at 0 allocs. Evict 1 alloc of about
64 KiB/op.

**Decision gate:** if `BenchmarkCommitFullCache` is **below 5 µs/op**, the
per-frame cost is negligible. STOP and report the numbers (see STOP conditions).
Do not change code.

### Step 2: Tests for current expiry behavior (must pass before and after)

In `layout_cache_test.go`, add:

- `TestLayoutCacheExpiresStaleEntries`: insert one entry with
  `lastAccess = now - ts.evictionAge - 1` and one fresh entry. Call
  `ts.pruneCache()`. The stale one is gone and the fresh one stays.
- `TestLayoutCacheEvictsOldestQuarter`: insert 100 entries with `lastAccess`
  1..100. Call `ts.evictOldestLayouts()`. Exactly 25 remain gone, and the 75
  survivors are the ones with `lastAccess > 25`.

**Verify**: `go test -run 'TestLayoutCache' -count=1 -v .` → both `PASS` on the
current code.

### Step 3: Throttle `pruneCache`

Add two fields to `TextSystem`:

```go
	lastPrune   int64   // Unix ms of the last full prune scan.
	evictTimes  []int64 // Scratch for evictOldestLayouts, reused across calls.
```

Change `pruneCache` to return early when `now-ts.lastPrune < ts.evictionAge/4`
(that is 1250 ms by default). Otherwise it sets `ts.lastPrune = now` and scans
as before. Write a comment that explains it: an entry can outlive `evictionAge`
by at most a quarter period, entries expire on a seconds scale, and a per-frame
full scan wasted work that grew with the cache. Keep the `len(ts.cache) == 0`
early return.

Then update `TestLayoutCacheExpiresStaleEntries` so it resets `ts.lastPrune = 0`
before calling `pruneCache`, and add `TestLayoutCachePruneThrottled`: call
`pruneCache` once (it scans), insert a stale entry, and call `pruneCache` again
right away. The stale entry is still present. Set
`ts.lastPrune -= ts.evictionAge` and call it again. Now the entry is gone.

**Verify**: `go test -run 'TestLayoutCache' -count=1 -v .` → all 3 `PASS`.

### Step 4: Reuse the eviction scratch

In `evictOldestLayouts`, replace `times := make([]int64, 0, n)` with
`times := ts.evictTimes[:0]`. Append as before, and at the end store
`ts.evictTimes = times` so the grown slice is kept. Comment: it is reused
because the cache sits at its cap in steady state, so this runs on every insert
burst.

**Verify**: `go test -run 'TestLayoutCache' -count=1 .` → `ok`. Benchmark
command → `BenchmarkEvictOldestLayouts` shows `0 allocs/op` after the first
iteration (`allocs/op` reported as 0). `BenchmarkCommitFullCache` shows at least
a 10× lower `ns/op` than the Step 1 numbers.

### Step 5: Guards, changelog, gate

`CHANGELOG.md` `### Changed`: "**Layout-cache pruning is throttled.** `Commit`
scans the layout cache for expired entries at most every quarter of the eviction
age (1.25 s by default), not every frame. Cache eviction no longer allocates. An
unused entry can now live up to 25% longer than `evictionAge`."

**Verify**: `BenchmarkTerminalFrame1500` → `0 allocs/op`. `make prepush` →
exit 0.

## Test plan

- `TestLayoutCacheExpiresStaleEntries`: expiry still works (written before the
  change, still passes after).
- `TestLayoutCacheEvictsOldestQuarter`: the 25%-oldest policy is unchanged.
- `TestLayoutCachePruneThrottled`: the new throttle skips a scan inside the
  interval and runs one after it.
- `BenchmarkCommitFullCache`, `BenchmarkEvictOldestLayouts`: before/after
  numbers for the review.

## Done criteria

- [ ] `go test -run 'TestLayoutCache' -count=1 .` → `ok` (3 tests)
- [ ] `BenchmarkCommitFullCache` `ns/op` is ≥10× lower than the Step 1 baseline
      (both numbers in the report)
- [ ] `BenchmarkEvictOldestLayouts` reports `0 allocs/op`
- [ ] `grep -n 'make(\[\]int64' glyph.go` prints nothing
- [ ] `BenchmarkTerminalFrame1500` reports `0 allocs/op`
- [ ] `make prepush` exits 0
- [ ] Only `glyph.go`, `layout_cache_test.go`, `CHANGELOG.md` changed (plus
      `plans/README.md`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Step 1 gate: `BenchmarkCommitFullCache` < 5 µs/op. Report both benchmark
  results and stop. The advisor will mark the finding rejected as not worth it.
- A Step 2 test fails on the unchanged code. That would mean the current
  eviction does not behave as described. Report the failure.
- Any change seems to require touching the eviction policy or the cache key.

## Maintenance notes

- If `evictionAge` becomes configurable at runtime, the throttle interval
  (`evictionAge/4`) follows it automatically. Keep it derived, not a separate
  constant.
- `Purge` clears the cache but leaves `lastPrune`. That is harmless: the next
  scan simply finds nothing.
