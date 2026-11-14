# Plan 016: Glyph rendering from several Contexts at once is race-free

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 4154f31..HEAD -- bitmap_puregoft.go freetype_types_puregoft.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW (lock is taken only on an atlas miss, never in steady state)
- **Depends on**: none (plan 015, already merged, fixed the shaping half)
- **Category**: bug (data race)
- **Planned at**: commit `4154f31`, 2026-11-14

## Why this matters

Every `Context` in a process shares one parsed `*font.Face` per font file
(`cachedFace.face`, from `loadCachedFace(path)`). go-gui makes one Context per
window. The color-emoji path (`renderColorGlyph`) calls `cf.face.SetPpem` under
`cf.mu`. Three other render functions call `cf.face.GlyphData` on the same Face
**without** `cf.mu`. `GlyphData` reads the ppem that `SetPpem` writes (go-text
v0.3.5, `font/renderer.go:477` and `:486` read `f.xPpem, f.yPpem`). So two
windows that draw emoji at the same time race. The race can also pick the wrong
bitmap strike.

The race was confirmed on 2026-11-14 (macOS, Apple Color Emoji). One goroutine
called `renderColorGlyph` and another called `renderGlyphByID` and `renderRun`
on the same path. `-race` reported 3 races: `bitmap_puregoft.go:646` (`SetPpem`)
against `bitmap_puregoft.go:338` and `:366` (`GlyphData`). The fix below (wrap
each `GlyphData` in `cf.lockedCall`) was tried on a scratch copy. It gave 0
races over `-count=3`, and the render/emoji/alloc tests passed.

## Current state

`bitmap_puregoft.go:336-347`, in `renderRun` (the loop over shaped glyphs):

```go
		gd := cf.face.GlyphData(buf.Info[i].Glyph)
		if out, ok := gd.(font.GlyphOutline); ok && len(out.Segments) > 0 {
			glyphs = append(glyphs, placedGlyph{
				segs: out.Segments,
				penX: penX,
				xOff: float64(pos.XOffset) / 64.0,
				yOff: float64(pos.YOffset) / 64.0,
			})
		}
		penX += float64(pos.XAdvance) / 64.0
	}
	return rasterizeGlyphs(atlas, cf, size, strokeWidth, glyphs)
```

`bitmap_puregoft.go:362-371`, in `renderGlyphByID`:

```go
	cf := loadCachedFace(path)
	if cf == nil {
		return nil
	}
	gd := cf.face.GlyphData(font.GID(gid))
	out, ok := gd.(font.GlyphOutline)
	if !ok || len(out.Segments) == 0 {
		return nil
	}
```

`bitmap_puregoft.go:826-830`, in `renderCOLRGlyph` (loop over COLR layers):

```go
	for _, layer := range layers {
		gd := cf.face.GlyphData(font.GID(layer.GlyphID))
		out, ok := gd.(font.GlyphOutline)
		if !ok || len(out.Segments) == 0 {
			continue // empty/space layer contributes no ink
		}
```

The locked writer, `bitmap_puregoft.go:645-656` (do not change it):

```go
	if !cf.lockedCall(func() {
		cf.face.SetPpem(ppem, ppem)
		gd := cf.face.GlyphData(gid)
		...
		ext, extOK = cf.face.GlyphExtents(gid)
	}) {
		return nil, false
	}
```

The helper to use, `freetype_types_puregoft.go:521-531`:

```go
func (cf *cachedFace) lockedCall(fn func()) (ok bool) {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	fn()
	return true
}
```

Return values on failure. `renderRun` returns `(*rasterResult, bool)`. The bool
is `true` only when a notdef glyph was rejected (see `return nil, true` near
line 327). For "no ink" it is `false`. `renderGlyphByID` returns `*rasterResult`
only. `renderCOLRGlyph` returns `(*rasterResult, bool)`, and `false` means "not
drawn".

Conventions: comments are plain, short, present tense, with no "we". Wrap at
about 90 columns. The comment above the `lockedCall` at line 638 shows how this
file explains a lock.

## Commands you will need

| Purpose      | Command                                                          | Expected on success  |
| ------------ | ---------------------------------------------------------------- | -------------------- |
| Race test    | `go test -race -run TestConcurrentRenderSharesFace -count=3 .`   | `ok`, no `DATA RACE` |
| Render tests | `go test -run 'RenderGlyphByID\|Emoji\|Color\|COLR' -count=1 .`  | `ok`                 |
| Alloc guards | `go test -run 'NoAlloc\|NoAllocs\|Allocs' -count=1 ./...`        | all `ok`             |
| Shaping race | `go test -race -run TestConcurrentContextsShareFaces -count=1 .` | `ok`                 |
| Full gate    | `make prepush`                                                   | exit 0               |

On macOS, if a run is killed for memory, prefix `GOMEMLIMIT=2GiB` and add
`-p 1`.

## Scope

**In scope**:

- `bitmap_puregoft.go`: the three `cf.face.GlyphData` calls in `renderRun`,
  `renderGlyphByID` and `renderCOLRGlyph`, plus their comments.
- `freetype_types_puregoft.go`: the doc comment on `cachedFace` and on its `mu`
  field only (add the render functions to the list of locked callers).
- `concurrent_render_puregoft_test.go` (create).
- `CHANGELOG.md`: one bullet under `## [Unreleased]` → `### Fixed`.

**Out of scope**:

- The `SetPpem` block in `renderColorGlyph` (lines 645-656). It is already
  correct.
- `cf.face.COLR.Search` and `cf.face.CPAL` in `renderCOLRGlyph`. They read
  parsed tables that never change after parse.
- Replacing `GlyphData` with `GlyphDataOutline`. It would avoid the lock, but it
  changes which glyphs get an outline (COLR and bitmap glyphs often also have an
  outline fallback). That is a behavior change, not a race fix.
- Shaping code (`getHB`, `faceCovers`). Plan 015 fixed it.
- `go.mod` and the go-text dependency.

## Git workflow

- Branch: `advisor/016-render-path-face-lock`
- One commit, e.g.
  `fix: lock the shared face around render-path GlyphData calls`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing race test

Create `concurrent_render_puregoft_test.go` with exactly this content:

```go
//go:build linux || darwin || windows

package glyph

import (
	"sync"
	"testing"
)

// TestConcurrentRenderSharesFace guards the render path of the shared cachedFace.
// renderColorGlyph sets the bitmap ppem on cf.face under cf.mu. renderGlyphByID and
// renderRun read that ppem through GlyphData on the same face. Two Contexts (two
// go-gui windows) that draw emoji at the same time run both paths on one face. The
// test is only meaningful under -race.
func TestConcurrentRenderSharesFace(t *testing.T) {
	ctx, err := NewContext(1.0)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()

	const emoji = "\U0001F600"
	// Find a color font that the bitmap path can draw. Only such a font has its
	// ppem set by renderColorGlyph.
	var path string
	for _, p := range ftColorFallbacksSingleton {
		if _, ok := renderColorGlyph(nil, p, 32, emoji); ok {
			path = p
			break
		}
	}
	if path == "" {
		t.Skip("no bitmap color-emoji font installed")
	}
	cf := loadCachedFace(path)
	if cf == nil {
		t.Fatalf("loadCachedFace(%q) = nil", path)
	}
	buf := shapeWith(cf, 32, emoji)
	if buf == nil || len(buf.Info) == 0 {
		t.Fatalf("%q did not shape in %q", emoji, path)
	}
	gid := uint32(buf.Info[0].Glyph)
	releaseShapeBuffer(buf)

	const iters = 50
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range iters {
			// Two sizes, so SetPpem writes a new value each time.
			renderColorGlyph(nil, path, float64(20+i%2*20), emoji)
		}
	}()
	go func() {
		defer wg.Done()
		for range iters {
			renderGlyphByID(nil, path, 32, 0, 0, gid)
			renderRun(nil, path, 32, 0, 0, emoji, "", 0, false)
		}
	}()
	wg.Wait()
}
```

**Verify**:
`go test -race -run TestConcurrentRenderSharesFace -count=1 . 2>&1 | grep -c 'WARNING: DATA RACE'`
→ a number ≥ 1. The traces name `bitmap_puregoft.go:646` and
`bitmap_puregoft.go:338` or `:366`. If the test prints `SKIP`, or the count is
0, STOP (see the STOP conditions).

### Step 2: Lock the three `GlyphData` calls

In `renderRun`, replace `gd := cf.face.GlyphData(buf.Info[i].Glyph)` with:

```go
		// GlyphData reads the bitmap ppem that renderColorGlyph sets on the
		// shared face under cf.mu, so the read takes the same lock. The
		// outline segments it returns are new slices, safe to use after the
		// lock is released.
		var gd font.GlyphData
		if !cf.lockedCall(func() { gd = cf.face.GlyphData(buf.Info[i].Glyph) }) {
			return nil, false
		}
```

In `renderGlyphByID`, replace `gd := cf.face.GlyphData(font.GID(gid))` with the
same pattern. On failure, `return nil`. Use a one-line comment that refers to
the `renderRun` comment ("Same lock as renderRun: GlyphData reads the shared
ppem.").

In `renderCOLRGlyph`, replace `gd := cf.face.GlyphData(font.GID(layer.GlyphID))`
with the same pattern. On failure, `return nil, false`. Use the same one-line
comment.

`lockedCall` turns a go-text panic into `ok == false`. Before this change, a
panic there went up to `guardGlyphLoad`. Now the glyph is skipped instead. That
is the same "not drawn" outcome.

**Verify**: `go build ./...` → exit 0.
`grep -c 'cf.face.GlyphData' bitmap_puregoft.go` → `4`. All 4 are inside
`lockedCall` closures (3 new and 1 at the `SetPpem` site).

### Step 3: Update the `cachedFace` comments

In `freetype_types_puregoft.go`, in the `cachedFace` doc comment and the `mu`
field comment, add the render functions to the list of code that holds `mu`:
`renderRun`, `renderGlyphByID` and `renderCOLRGlyph` take it around `GlyphData`,
because `GlyphData` reads the ppem. Keep every other sentence.

**Verify**: `go vet .` → exit 0.

### Step 4: Race test, render tests, allocation guards

**Verify**:

- `go test -race -run TestConcurrentRenderSharesFace -count=3 . 2>&1 | grep -c 'WARNING: DATA RACE'`
  → `0`, and the test passes.
- `go test -race -run TestConcurrentContextsShareFaces -count=1 .` → `ok`.
- `go test -run 'RenderGlyphByID|Emoji|Color|COLR' -count=1 .` → `ok`.
- `go test -run 'NoAlloc|NoAllocs|Allocs' -count=1 ./...` → all `ok`. The lock
  is only taken on an atlas miss. Steady-state draws hit the glyph cache and do
  not reach these functions.

### Step 5: Changelog and gate

Add under `## [Unreleased]` → `### Fixed` (match the bold first sentence of the
nearby bullets, and wrap at 80 columns):

"**Drawing emoji from several Contexts at once is race-free.** Glyph rendering
now holds the shared font's lock when it reads glyph data. Before, a color emoji
drawn in one window could change the bitmap size that another window read at the
same time."

**Verify**: `make prepush` → exit 0.

## Test plan

- `TestConcurrentRenderSharesFace` (new): one goroutine sets ppem through
  `renderColorGlyph` at two sizes. Another reads through `renderGlyphByID` and
  `renderRun` on the same face. It must report races before Step 2 and none
  after, under `-race -count=3`. It skips when no bitmap color font is
  installed. Linux CI installs `fonts-noto-color-emoji` (CBDT) and macOS has
  Apple Color Emoji (sbix), so it runs on both. Windows (Segoe UI Emoji is COLR)
  skips.
- Existing: `bitmap_glyphid_puregoft_test.go` (by-ID rendering),
  `emoji_render_*_test.go`, `color_glyph_box_test.go`, and the allocation
  guards.

## Done criteria

- [ ] `go test -race -run TestConcurrentRenderSharesFace -count=3 . 2>&1 | grep -c 'DATA RACE'`
      prints `0`, and the test passes (not skipped) on macOS or Linux
- [ ] `grep -c 'cf.face.GlyphData' bitmap_puregoft.go` prints `4`
- [ ] `grep -c 'lockedCall' bitmap_puregoft.go` prints at least `5` (it is `2`
      before this plan: one comment and the `SetPpem` call)
- [ ] `go test -run 'NoAlloc|NoAllocs|Allocs' -count=1 ./...` → all `ok`
- [ ] `make prepush` exits 0
- [ ] `git status` shows changes only in `bitmap_puregoft.go`,
      `freetype_types_puregoft.go`, `concurrent_render_puregoft_test.go`,
      `CHANGELOG.md` (and `plans/README.md`)

## STOP conditions

- Step 1 skips, or reports no race. Report the output and the value of
  `ftColorFallbacksSingleton`. Do not write the fix without a test that failed
  first.
- After Step 2 the race detector still reports a race. Report the first 30 lines
  of each trace. Do not add locks to other functions.
- An allocation guard fails in Step 4.
- `font.GlyphData` is not the name of the interface type that
  `cf.face.GlyphData` returns (compile error). Report
  `go list -m github.com/go-text/typesetting`.

## Maintenance notes

- Rule after this plan: any call on the shared `cf.face` that reads or writes
  ppem (`GlyphData`, `GlyphDataBitmap`, `GlyphExtents`, `SetPpem`) runs under
  `cf.mu`, through `lockedCall`. Calls that only read parsed tables
  (`COLR.Search`, `CPAL`, `Cmap.Lookup`, `GlyphDataOutline`) do not need it.
- A later optimization can switch `renderCOLRGlyph`'s layer lookups to
  `cf.face.GlyphDataOutline`, which needs no lock. COLR layer glyphs are always
  outlines. It was left out here to keep this a pure race fix.
- Reviewer: check that each closure only assigns `gd`, and that no rasterization
  runs while the lock is held.
