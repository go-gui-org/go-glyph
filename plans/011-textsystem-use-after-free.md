# Plan 011: TextSystem methods return ErrTextSystemFreed after Free instead of panicking

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
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `6d6a018`, 2026-11-13
- **Decision**: the exported name `ErrTextSystemFreed` was confirmed by the
  maintainer on 2026-11-13.

## Why this matters

`TextSystem.Free` sets `ts.ctx`, `ts.renderer` and `ts.cache` to nil. The draw
methods (`DrawLayout*`, `Commit`, `Purge`) already check `ts.renderer == nil`
and do nothing. Every method that reaches `ts.ctx` or writes `ts.cache` still
dereferences nil and panics: `DrawText`, `TextWidth`, `TextHeight`,
`LayoutText`, `LayoutTextCached`, `LayoutRichText`, `FontHeight`, `FontMetrics`,
`AddFontFile`, `AddFontBytes`, `ResolveFontName`, `ListFontFamilies`. GUI apps
hit this when a window closes while a queued callback still measures text. The
methods already return `error`, so a clear error is the expected behavior.
`Context.AddFontFile` already does this
(`return fmt.Errorf("glyph: AddFontFile on freed Context")`,
`context_puregoft.go:171-174`). After this plan, every method is safe after
`Free`: error-returning methods return `ErrTextSystemFreed`, and the rest do
nothing.

## Current state

- `glyph.go` (untagged; builds for native and wasm) — `TextSystem` and all its
  methods. There are no sentinel errors in the package yet. Errors use
  `fmt.Errorf("glyph: ...")`.

`Free`, `glyph.go:86-100`:

```go
func (ts *TextSystem) Free() {
	if ts.renderer != nil {
		ts.renderer.Free()
		ts.renderer = nil
	}
	if ts.ctx != nil {
		ts.ctx.Free()
		ts.ctx = nil
	}
	for _, p := range ts.tempFontFiles {
		_ = os.Remove(p)
	}
	ts.tempFontFiles = nil
	ts.cache = nil
}
```

The panicking paths:

- `getOrCreateLayout` (`glyph.go:314-351`) calls `ts.ctx.LayoutText` and writes
  `ts.cache[key] = item`. It serves `DrawText`, `TextWidth`, `TextHeight` and
  `LayoutTextCached`.
- Direct `ts.ctx.` calls: `FontHeight` (line 137), `FontMetrics` (142),
  `AddFontFile` (172), `AddFontBytes` (194, after it has already written a temp
  file at line 190), `ResolveFontName` (210), `ListFontFamilies` (221),
  `LayoutText` (229), `LayoutRichText` (243).

Already safe (leave as is): `Commit` (checks `ts.renderer == nil`, and
`pruneCache` handles an empty or nil map), `Purge` (`clear(nil)` is a no-op),
all `DrawLayout*` methods, `Renderer()`, `Context()`, and `Free` itself (calling
it twice is fine).

Test exemplar: `TestTextSystemDrawText` in `test_helpers_test.go:91-114`. It
uses `newRecordingBackend()` (same file, line 43) and `NewTextSystem(backend)`.

## Commands you will need

| Purpose     | Command                                                             | Expected on success |
| ----------- | ------------------------------------------------------------------- | ------------------- |
| Focus tests | `go test -run TestTextSystemAfterFree -count=1 .`                   | `ok`                |
| WASM vet    | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...`                    | exit 0              |
| Alloc guard | `go test -run '^$' -bench 'BenchmarkTerminalFrame1500' -benchmem .` | `0 allocs/op`       |
| Full gate   | `make prepush`                                                      | exit 0              |

## Scope

**In scope**:

- `glyph.go`
- `textsystem_free_test.go` (create; untagged)
- `CHANGELOG.md` — one bullet under `### Fixed`, and one under `### Added` for
  the new exported error (create the `### Added` heading under `## [Unreleased]`
  if it does not exist, above `### Changed`)

**Out of scope**:

- `Context` methods (`context_puregoft.go`, `context_wasm.go`). Guard at the
  `TextSystem` layer only.
- `Renderer` methods.
- Changing `Free` itself, other than nothing. It stays as is.

## Git workflow

- Branch: `advisor/011-textsystem-use-after-free`
- One commit, e.g.
  `fix: TextSystem methods return ErrTextSystemFreed after Free`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Failing test

Create `textsystem_free_test.go` (`package glyph`, no build tag). Add
`TestTextSystemAfterFree`: create a TextSystem with `newRecordingBackend()`,
call `ts.Free()`, then call every method listed below, each inside a subtest
(`t.Run(name, ...)`). Each subtest defers a `recover()` that fails the test with
`t.Errorf("%s panicked after Free: %v", name, r)`.

- These must return an error where `errors.Is(err, ErrTextSystemFreed)`:
  `DrawText`, `TextWidth`, `TextHeight`, `LayoutText`, `LayoutTextCached`,
  `LayoutRichText` (pass `RichText{Runs: []StyleRun{{Text: "a"}}}`, after you
  check the field names in `config.go`), `FontHeight`, `FontMetrics`,
  `AddFontFile("/nonexistent.ttf")`, `AddFontBytes([]byte{1})`,
  `ResolveFontName("Sans")`.
- `ListFontFamilies()` must return nil.
- These must not panic: `Commit()`, `Purge()`, `DrawLayout(Layout{}, 0, 0)`, and
  a second `Free()`.
- For `AddFontBytes`, also check that no temp file was created. Note
  `os.TempDir()` entries before and after, or check that `ts.tempFontFiles` is
  still empty.

The test must reference `ErrTextSystemFreed`, so it will not compile yet. That
counts as failing.

**Verify**: `go test -run TestTextSystemAfterFree -count=1 .` → build fails with
`undefined: ErrTextSystemFreed`.

### Step 2: Add the error and the guards

In `glyph.go`, above `type TextSystem`:

```go
// ErrTextSystemFreed is returned by TextSystem methods called after Free.
// Draw methods and Commit do nothing instead, since they return no error.
var ErrTextSystemFreed = errors.New("glyph: TextSystem used after Free")
```

Add `"errors"` to the imports. Then:

- At the very top of `getOrCreateLayout`, before `ValidateTextInput`:
  `if ts.ctx == nil { return nil, ErrTextSystemFreed }`.
- At the top of `FontHeight`, `FontMetrics`, `AddFontFile` (before
  `ValidateFontPath`), `AddFontBytes` (before the empty-data check and before
  `writeTempFontFile`), `ResolveFontName`, `LayoutText` and `LayoutRichText`:
  return the zero value and `ErrTextSystemFreed` when `ts.ctx == nil`.
- `ListFontFamilies`: `if ts.ctx == nil { return nil }`.

Use one nil check on `ts.ctx` everywhere. Do not add a `freed bool` field:
`Free` already sets `ctx` to nil, and one source of truth cannot drift. Add a
short comment at the first guard in `getOrCreateLayout` that points to Free.

**Verify**: `go test -run TestTextSystemAfterFree -count=1 -v .` → all subtests
`PASS`. Then `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` → exit 0.

### Step 3: No hot-path cost

The guard is one pointer compare on `DrawText`, the per-frame path. Confirm that
steady-state drawing still allocates nothing.

**Verify**:
`go test -run '^$' -bench 'BenchmarkTerminalFrame1500|BenchmarkDrawLayoutPlacedCached' -benchmem -count=1 .`
→ both report `0 allocs/op`.

### Step 4: Changelog and gate

- `### Added`: "**`ErrTextSystemFreed`.** Returned by `TextSystem` methods
  called after `Free`."
- `### Fixed`: "**`TextSystem` no longer panics after `Free`.** Measuring,
  layout and font methods return `ErrTextSystemFreed`, and `ListFontFamilies`
  returns nil. Before, they dereferenced the freed context."

**Verify**: `make prepush` → exit 0.

## Test plan

- `TestTextSystemAfterFree`: every exported method after `Free` either returns
  `ErrTextSystemFreed` or does nothing, never panics, and `AddFontBytes` writes
  no temp file.
- Existing `TestTextSystem*` tests keep passing (normal, non-freed behavior is
  unchanged).

## Done criteria

- [ ] `go test -run TestTextSystemAfterFree -count=1 .` → `ok`
- [ ] `grep -c 'ts.ctx == nil' glyph.go` prints at least `9`
- [ ] `grep -n 'ErrTextSystemFreed = errors.New' glyph.go` prints 1 line
- [ ] Both benchmarks in Step 3 report `0 allocs/op`
- [ ] `make prepush` exits 0
- [ ] Only `glyph.go`, `textsystem_free_test.go`, `CHANGELOG.md` changed (plus
      `plans/README.md`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- A method listed as "already safe" panics in the test. Report which method and
  the panic. Do not add guards outside `glyph.go`.
- A guard seems to need changes in `Context` or `Renderer`.
- Step 3 shows any `allocs/op` above 0.
- `glyph.go` does not build for wasm after the change.

## Maintenance notes

- Any new `TextSystem` method that touches `ts.ctx` or `ts.cache` needs the same
  `ts.ctx == nil` guard. `TestTextSystemAfterFree` should get a subtest for it.
- `ErrTextSystemFreed` is the package's first exported sentinel error. If more
  sentinels follow, keep the `"glyph: ..."` message prefix.
