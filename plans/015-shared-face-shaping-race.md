# Plan 015: Two Contexts can shape text with the same font at the same time without a data race

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 3d7b0fc..HEAD -- freetype_types_puregoft.go ink_bounds_puregoft.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: MEDIUM (shaping hot path; allocation guard tests must hold)
- **Depends on**: none. Plan 009 (CI race job) depends on this plan.
- **Category**: bug (data race)
- **Planned at**: commit `3d7b0fc`, 2026-11-13

## Why this matters

Every `Context` in a process shares one parsed `*font.Face` per font file
(`faceCache` in `freetype_types_puregoft.go`). go-gui creates one Context per
window. When two windows lay out text on two goroutines, the Go race detector
reports 4 data races. They were found by running the test in Step 1 under
`-race` on 2026-11-13:

- Reports 1 and 2: `faceCovers` → `(*font.Face).NominalGlyph` writes the Face's
  cmap caches (`cmapCache`, `cmapNotSupportedCache`) with no lock. Call path:
  `LayoutText` → `buildScriptFallbacks` (`layout_ft.go:204`) → `ftFont.covers` →
  `faceCovers` (`freetype_types_puregoft.go:576`).
- Reports 3 and 4: `safeShape` → `harfbuzz` `Shape` writes the same Face's
  `extentsCache` and `cmapCache`. The pooled `harfbuzz.Font` values are
  separate, but each one is built with `harfbuzz.NewFont(cf.face)`, so all of
  them share one `*font.Face` and its caches.

The current comments say the opposite. They say the pool "hands each shape an
exclusively-owned font", and that the Face's only mutable state is guarded by
`cf.mu`. That is wrong: `font.Face` (go-text v0.3.5, `font/font.go:620-629`)
holds three lazy caches plus ppem, and none is safe for concurrent use:

```go
type Face struct {
	*Font

	extentsCache          extentsCache
	cmapCache             cache21_19_8 // supported runes, mapping to GID
	cmapNotSupportedCache cache21_0_13 // not supported runes

	coords       []tables.Coord
	xPpem, yPpem uint16
}
```

The `*font.Font` inside it holds the parsed tables. Nothing mutates it after
parse, and `Font.Cmap.Lookup` uses value receivers that only read (go-text
`font/cmap.go`, all `Lookup` methods).

A second, quieter problem has the same cause. The color-glyph render path calls
`cf.face.SetPpem` under `cf.mu` (`bitmap_puregoft.go:646`). Shaping reads the
same Face without the lock, so shaped extents for bitmap fonts can depend on
which size another window last rendered.

After this plan:

- Each pooled shaping font owns a private `*font.Face` built on the shared
  `*font.Font`. Shaping never touches `cf.face`.
- Cmap coverage checks that run without `cf.mu` call the read-only
  `face.Cmap.Lookup` instead of the caching `NominalGlyph`.

## Current state

`freetype_types_puregoft.go:34-78` (comments wrong, `getHB` shares the Face):

```go
// cachedFace holds a parsed go-text face plus the cheap-to-read
// attributes the backend queries repeatedly. The parsed face is shared
// across ftFont values; its mutable state is the bitmap ppem (set under
// mu by the color-glyph path) and the extent cache GlyphExtents fills
// (read under mu by capEm), so concurrent renders/measurements across
// Contexts cannot corrupt each other's strike selection or measurements.
type cachedFace struct {
	mu    sync.Mutex // guards ppem-dependent access to face (SetPpem+GlyphData)
	face  *font.Face
	...
	// hbPool recycles HarfBuzz shaping fonts. NewFont builds the face's
	// shaping accelerators and is the single largest source of layout
	// allocations, so rebuilding one per shape (per line) is wasteful. A Font
	// carries lazy mutable state that Shape writes, so it cannot be shared
	// concurrently; the pool hands each shape an exclusively-owned font that
	// it returns when done, amortizing NewFont across shapes without a lock.
	hbPool sync.Pool
}

func (cf *cachedFace) getHB(scale int32) *harfbuzz.Font {
	f, _ := cf.hbPool.Get().(*harfbuzz.Font)
	if f == nil {
		f = harfbuzz.NewFont(cf.face)
	}
	f.XScale, f.YScale = scale, scale
	return f
}
```

`freetype_types_puregoft.go:571-581`:

```go
func faceCovers(face *font.Face, text string) bool {
	for _, r := range text {
		if isDefaultIgnorable(r) {
			continue
		}
		if _, ok := face.NominalGlyph(r); !ok {
			return false
		}
	}
	return true
}
```

`ink_bounds_puregoft.go:113-123`. `glyphGID` runs before the lock that its
caller takes at line 100, so this `NominalGlyph` is also unguarded:

```go
func glyphGID(cf *cachedFace, text string, g Glyph) (font.GID, bool) {
	if g.Shaped || g.GlyphID != 0 {
		return font.GID(g.GlyphID), true
	}
	ch := glyphText(text, g)
	r, n := utf8.DecodeRuneInString(ch)
	if n == 0 || n != len(ch) || r == utf8.RuneError {
		return 0, false
	}
	return cf.face.NominalGlyph(r)
}
```

Accesses to `cf.face` that already hold `cf.mu` stay unchanged: `capEm`
(`fallback_fit_puregoft.go:194-213`), the ink-extent read
(`ink_bounds_puregoft.go:100-107`) and the color-bitmap path
(`bitmap_puregoft.go:645-656`).

Conventions: comments are plain, short sentences in present tense, with no "we".
Wrap at about 90 columns. Look at `lockedCall`
(`freetype_types_puregoft.go:506`) for how this file explains a concurrency rule
in a comment.

## Commands you will need

| Purpose       | Command                                                              | Expected on success  |
| ------------- | -------------------------------------------------------------------- | -------------------- |
| Race test     | `go test -race -run TestConcurrentContextsShareFaces -count=3 .`     | `ok`, no `DATA RACE` |
| Pool + covers | `go test -run 'TestPooledFontRescales\|Covers\|Coverage' -count=1 .` | `ok`                 |
| Alloc guards  | `go test -run 'NoAlloc\|NoAllocs\|Allocs' -count=1 ./...`            | all `ok`             |
| All, race     | `make test-race`                                                     | all `ok`             |
| Full gate     | `make prepush`                                                       | exit 0               |

## Scope

**In scope**:

- `freetype_types_puregoft.go`: `getHB`, `faceCovers`, and the doc comments on
  `cachedFace`, its `mu` field and its `hbPool` field.
- `ink_bounds_puregoft.go`: `glyphGID` only.
- `concurrent_contexts_puregoft_test.go` (create).
- `CHANGELOG.md`: one bullet under `## [Unreleased]` → `### Fixed`.

**Out of scope**:

- `bitmap_puregoft.go`. Its `GlyphData` calls at lines 338, 366 and 827 run
  without `cf.mu`. `GlyphData` may read the ppem that line 646 writes. That is a
  separate, unproven render-path race. Do not touch it. It is listed as a
  deferred finding in `plans/README.md`.
- `fallback_fit_puregoft.go`. `capEm` already holds `cf.mu`.
- `.github/workflows/ci.yml`. Plan 009 adds the CI race job after this plan
  lands.
- The `coverage` / `coverageMap` code. It already uses `Cmap.Lookup`.
- Any change to `go.mod` or the go-text dependency.

## Git workflow

- Branch: `advisor/015-shared-face-shaping-race`
- One commit, e.g.
  `fix: give each pooled shaping font its own Face so Contexts can shape concurrently`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing race test

Create `concurrent_contexts_puregoft_test.go` with exactly this content:

```go
//go:build linux || darwin || windows

package glyph

import (
	"sync"
	"testing"
)

// TestConcurrentContextsShareFaces guards the shared faceCache and hbPool across
// Contexts. A multi-window app (one Context per window in go-gui) shapes text from
// several goroutines through the same cached faces. The test is only meaningful
// under -race. It also checks that shaping is not corrupted: the width of one fixed
// layout, recorded in each goroutine, must equal a sequential layout of the same
// text.
func TestConcurrentContextsShareFaces(t *testing.T) {
	const (
		numCtx = 4
		iters  = 50
	)
	texts := []string{
		"Hello, world",
		"office",
		"مرحبا",
		"漢字",
		"😀",
	}
	// Fixed probe layout used for the width comparison.
	const probeText = "office"
	const probeFont = "Sans 24"

	ctxs := make([]*Context, numCtx)
	for i := range ctxs {
		ctx, err := NewContext(1.0)
		if err != nil {
			t.Fatal(err)
		}
		defer ctx.Free()
		ctxs[i] = ctx
	}

	widths := make([]float32, numCtx)
	var wg sync.WaitGroup
	for g, ctx := range ctxs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range iters {
				// Alternate sizes so shared faces see different ppem values.
				font := "Sans 12"
				if i%2 == 1 {
					font = "Sans 24"
				}
				for _, s := range texts {
					cfg := TextConfig{Style: TextStyle{FontName: font}}
					if _, err := ctx.LayoutText(s, cfg); err != nil {
						t.Errorf("goroutine %d: LayoutText(%q, %s): %v", g, s, font, err)
						return
					}
				}
				if i == iters-1 {
					cfg := TextConfig{Style: TextStyle{FontName: probeFont}}
					l, err := ctx.LayoutText(probeText, cfg)
					if err != nil {
						t.Errorf("goroutine %d: probe layout: %v", g, err)
						return
					}
					widths[g] = l.Width
				}
			}
		}()
	}
	wg.Wait()

	cfg := TextConfig{Style: TextStyle{FontName: probeFont}}
	want, err := ctxs[0].LayoutText(probeText, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for g, w := range widths {
		if w != want.Width {
			t.Errorf("goroutine %d: width = %v, sequential width = %v", g, w, want.Width)
		}
	}
}
```

**Verify**:
`go test -race -run TestConcurrentContextsShareFaces -count=1 . 2>&1 | grep -c 'WARNING: DATA RACE'`
→ a number ≥ 1. The stack traces name `faceCovers` and/or `safeShape`. If the
count is 0, STOP (see the STOP conditions).

### Step 2: Give each pooled shaping font its own Face

In `getHB`, replace `harfbuzz.NewFont(cf.face)` with:

```go
		// A private Face per shaping font. font.Face keeps lazy caches
		// (cmap, glyph extents) and the bitmap ppem, and Shape writes them.
		// Every pooled font that wrapped cf.face shared those caches, so two
		// Contexts shaping the same font raced. The *font.Font holds the
		// parsed tables and is read-only after parse, so it is safe to share.
		// The private Face also never sees the ppem the color path sets on
		// cf.face, so shaped extents do not depend on render order.
		f = harfbuzz.NewFont(font.NewFace(cf.face.Font))
```

Rewrite the `hbPool` field comment. Keep its first two sentences. Replace the
rest with: each pooled font owns a private `font.Face`, because the Face carries
lazy caches that `Shape` writes. The pool hands each shape a font it owns alone,
so shaping needs no lock.

Rewrite the `cachedFace` doc comment and the `mu` field comment so they are
true:

- `cf.face` is shared by every Context.
- Its mutable state is the bitmap ppem and its lazy caches (cmap and glyph
  extents).
- Code that calls a `cf.face` method that fills a cache or sets ppem must hold
  `mu`. Today that is `capEm`, the ink-extent read and the color-glyph path.
- Shaping uses the private Faces in `hbPool`.
- Cmap checks done without `mu` must use `cf.face.Cmap.Lookup`, which only
  reads.

**Verify**: `go build ./...` → exit 0.
`go test -run 'TestPooledFontRescales' -count=1 .` → `ok`.

### Step 3: Read-only cmap lookups where `cf.mu` is not held

1. In `faceCovers`, replace `face.NominalGlyph(r)` with `face.Cmap.Lookup(r)`.
   Add one comment line: `Cmap.Lookup` only reads, while `NominalGlyph` writes
   the shared Face's cmap cache, and this runs without `cf.mu`.
2. In `glyphGID` (`ink_bounds_puregoft.go`), replace
   `return cf.face.NominalGlyph(r)` with `return cf.face.Cmap.Lookup(r)`. Add
   the same one-line reason.

The results do not change. `NominalGlyph` is only `Cmap.Lookup` with a cache in
front (go-text `font/font.go:643-657`).

**Verify**:
`go test -race -run TestConcurrentContextsShareFaces -count=3 . 2>&1 | grep -c 'WARNING: DATA RACE'`
→ `0`, and the test passes.
`go test -run 'TestPooledFontRescales|Covers|Coverage' -count=1 .` → `ok`.

### Step 4: Allocation guards

**Verify**: `go test -run 'NoAlloc|NoAllocs|Allocs' -count=1 ./...` → all `ok`.
The steady-state draw guards are `TestDrawLayoutSteadyStateNoAlloc` and
`TestDrawCompositionNoAllocs`. A pooled-font miss now also allocates one Face,
which costs about 9 KB of fixed cmap caches plus 20 bytes per glyph. Steady
state must still hit the pool and allocate nothing. If a guard fails, STOP.

### Step 5: Changelog and gate

Add under `## [Unreleased]` → `### Fixed` (match the bold first sentence of the
nearby bullets, and wrap at 80 columns):

"**Shaping from several Contexts at once is race-free.** Two Contexts (for
example, two go-gui windows) that lay out text with the same font on different
goroutines no longer race on the font's internal caches. Shaped widths no longer
depend on the size at which a color glyph was last drawn."

**Verify**: `make test-race` → all `ok`, and no `DATA RACE` in its output.
`make prepush` → exit 0.

## Test plan

- `TestConcurrentContextsShareFaces` (new): 4 Contexts on 4 goroutines × 50
  iterations over Latin, a ligature, Arabic, CJK and emoji at two sizes. It must
  report races before Step 2 and none after Step 3, under `-race -count=3`.
  Widths from the goroutines must equal a sequential layout.
- Existing `TestPooledFontRescales`, the coverage tests in
  `text_robustness_puregoft_test.go` (line 603 compares `coverage.covers` with
  `faceCovers`), and the allocation guards keep passing.

## Done criteria

- [ ] `go test -race -run TestConcurrentContextsShareFaces -count=3 . 2>&1 | grep -c 'DATA RACE'`
      prints `0`, and the test passes
- [ ] `grep -n 'NewFont(cf.face)' freetype_types_puregoft.go` prints nothing
- [ ] `grep -n 'NominalGlyph' freetype_types_puregoft.go ink_bounds_puregoft.go | grep -v '//'`
      prints nothing (comments that explain the change may name it)
- [ ] `make prepush` exits 0
- [ ] `git status` shows changes only in `freetype_types_puregoft.go`,
      `ink_bounds_puregoft.go`, `concurrent_contexts_puregoft_test.go`,
      `CHANGELOG.md` (and `plans/README.md`)

## STOP conditions

- Step 1 shows no race on this machine. Report the output. Do not write the fix
  without a test that failed first.
- After Step 3 the race detector still reports a race. Report the first 30 lines
  of each trace. If it is in `bitmap_puregoft.go` (`GlyphData` against
  `SetPpem`), that is the out-of-scope render-path finding. Report it and stop.
  Do not widen the scope.
- `go test` fails to compile because `cf.face.Font` or `Cmap.Lookup` does not
  exist. The go-text version may have changed. Report
  `go list -m github.com/go-text/typesetting`.
- An allocation guard fails in Step 4.
- The width check fails after the fix. That would mean private Faces shape
  differently from the shared one. Report both widths.

## Maintenance notes

- Rule for future code: shaping uses a pooled font's private Face. Any other
  call on `cf.face` that fills a cache (`NominalGlyph`, `GlyphExtents`,
  `HorizontalAdvance`) or sets state (`SetPpem`, `SetCoords`) must hold `cf.mu`.
  Read-only cmap checks use `cf.face.Cmap.Lookup`.
- Memory: each pooled font now holds its own extents cache (20 bytes × glyph
  count, about 1.3 MB for a 65k-glyph CJK font). The pool normally keeps one or
  two fonts per face. `sync.Pool` drops them at GC, and the idle face sweeper
  already deals with that.
- If variable-font support is added, `SetCoords` must be applied to each pooled
  font's private Face, not to `cf.face`.
- Plan 009 (CI `-race` job) depends on this plan. Without this fix, the job
  fails on `main`.
