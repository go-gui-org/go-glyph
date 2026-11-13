# Plan 008: Font discovery and AddFontFile open only regular files, so a FIFO cannot hang NewContext

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 6d6a018..HEAD -- context_puregoft.go discover_shared_puregoft_test.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug (robustness / DoS)
- **Planned at**: commit `6d6a018`, 2026-11-13

## Why this matters

On native platforms, font discovery walks every system and user font directory
(for example `~/.fonts`, `~/.local/share/fonts`). Each file whose name ends in
`.ttf`, `.otf` or `.ttc` is opened with `os.Open`. The walk skips only
directories. A named pipe (FIFO) called `x.ttf` in a font directory makes
`os.Open` block forever, because opening a FIFO for reading waits for a writer.
Discovery runs once per process inside a `sync.Once`, so the first `NewContext`
hangs, and so does every later `NewContext` that waits on that once. The same
thing happens with `AddFontFile` on a FIFO, or on a symlink that points to one.
After this plan, `describeFontFaces` stats the path first and rejects anything
that is not a regular file, before it calls `os.Open`.

## Current state

- `context_puregoft.go` (build tag `linux || darwin || windows`) — native font
  discovery and `Context.AddFontFile`.

`describeFontFaces`, `context_puregoft.go:250-265`. Its only callers are
`Context.AddFontFile` (line 175) and `fontScan.consider` (line 572):

```go
func describeFontFaces(path string) (faces []faceInfo) {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	// go-text can panic on defective tables. Discovery walks every system
	// font, so one broken file must drop only itself, not crash NewContext.
	defer func() {
		if recover() != nil {
			faces = nil
		}
	}()
	loaders, err := ot.NewLoaders(f)
```

`fontScan.walkDirs`, `context_puregoft.go:545-558`, checks only `d.IsDir()`:

```go
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			s.consider(path, aliases)
			return nil
		})
```

`filepath.WalkDir` does not follow symlinks, so `d.Type()` for a symlink is
`fs.ModeSymlink`, not the type of the target. Linux font directories often hold
symlinks to real fonts (fontconfig, distro packages). So the check must use
`os.Stat` (which follows symlinks), not `d.Type()`. A `d.Type().IsRegular()`
check in `walkDirs` would wrongly drop every symlinked font.

Test exemplar: `TestWalkDirsSkipsRelative` in
`discover_shared_puregoft_test.go:170-190`. It uses `newScanCtx()`,
`newFontScan(ctx).walkDirs(...)` and the helper `copyTestFont(t, dir)`
(`discover_shared_puregoft_test.go:152`), which copies a real system font into
`dir` and calls `t.Skip` if none is readable. Match its comment style: one
paragraph saying which bug the test is the regression test for.

## Commands you will need

| Purpose     | Command                                                         | Expected on success |
| ----------- | --------------------------------------------------------------- | ------------------- |
| Focus tests | `go test -run 'TestDescribeFontFaces\|TestWalkDirs' -count=1 .` | `ok`                |
| All tests   | `go test ./...`                                                 | all `ok`            |
| Lint        | `make lint`                                                     | `0 issues.`         |
| Full gate   | `make prepush`                                                  | exit 0              |

## Scope

**In scope**:

- `context_puregoft.go` — `describeFontFaces` only
- `discover_fifo_puregoft_test.go` (create; build tag `linux || darwin`)
- `CHANGELOG.md` — one bullet under `## [Unreleased]` → `### Fixed`

**Out of scope**:

- `fontScan.walkDirs` and `fontScan.consider`. The single check in
  `describeFontFaces` covers both callers.
- `discover_*.go` (per-OS directory lists).
- `parseFace` / `loadCachedFace` in `freetype_types_puregoft.go`. They only open
  paths that discovery or `AddFontFile` already accepted.
- The wasm backend. It has no filesystem fonts.

## Git workflow

- Branch: `advisor/008-font-discovery-regular-files`
- One commit, e.g.
  `fix: font discovery skips non-regular files so a FIFO cannot hang NewContext`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing test

Create `discover_fifo_puregoft_test.go` with `//go:build linux || darwin` (no
FIFOs on Windows) and `package glyph`. Use `syscall.Mkfifo(path, 0o600)` from
the standard library. Add two tests:

1. `TestDescribeFontFacesSkipsFIFO`: make `t.TempDir()`, create a FIFO named
   `pipe.ttf` in it, and call `describeFontFaces` on it in a goroutine that
   closes a `done` channel. `select` on `done` and on
   `time.After(2 * time.Second)`. On timeout,
   `t.Fatal("describeFontFaces blocked on a FIFO")`. When it finishes, the
   result must be `nil`. Then repeat with a symlink `link.ttf` → `pipe.ttf`
   (`os.Symlink`), which must also return `nil` without blocking.
2. `TestWalkDirsSkipsFIFO`: in a temp dir, call `copyTestFont(t, dir)` (so one
   real font exists) and create a FIFO `pipe.ttf`. Run
   `newFontScan(ctx).walkDirs([]string{dir}, nil)` with `ctx := newScanCtx()` in
   a goroutine with the same 2 s timeout. After it returns, `len(ctx.fontPaths)`
   must be > 0 (the real font was still found).

On timeout the blocked goroutine leaks. That is acceptable in a failing test.
Before the fix, open the FIFO for writing in the test cleanup
(`os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)`) so the stuck reader
unblocks and the test binary can exit. Wrap that cleanup open in a comment that
explains why it is there.

**Verify**:
`go test -run 'TestDescribeFontFacesSkipsFIFO|TestWalkDirsSkipsFIFO' -count=1 .`
→ FAIL with "blocked on a FIFO" (or the walk timeout message) for both tests.

### Step 2: Reject non-regular files in `describeFontFaces`

At the top of `describeFontFaces`, before `os.Open`:

```go
	// Only regular files can be fonts. os.Open on a FIFO blocks until a
	// writer appears, so one named pipe called "x.ttf" in a font dir would
	// hang discovery (and every NewContext waiting on its sync.Once) forever.
	// os.Stat follows symlinks: symlinked fonts, common on Linux, still pass.
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
		return nil
	}
```

This leaves a small race: the file can be swapped between `Stat` and `Open`.
That is acceptable, because the attacker would need write access to the font
directory. Say so in one comment line.

**Verify**: the Step 1 command → `ok`. Then
`go test -run 'TestWalkDirs|TestDiscover|TestAddFontFile' -count=1 .` → `ok`.

### Step 3: Changelog and gate

Add under `## [Unreleased]` → `### Fixed`:

"**Font discovery skips non-regular files.** A named pipe or other special file
with a font extension in a font directory no longer hangs `NewContext`.
`AddFontFile` on such a path now returns an error instead of blocking."

**Verify**: `make prepush` → exit 0.

## Test plan

- `TestDescribeFontFacesSkipsFIFO`: a FIFO path and a symlink to a FIFO both
  return `nil` within 2 s. Before the fix, both block.
- `TestWalkDirsSkipsFIFO`: a directory with a FIFO and a real font finishes, and
  it still registers the real font.
- Existing discovery tests (`discover_shared_puregoft_test.go`,
  `discover_fallback_test.go`) keep passing. They show that symlinked or regular
  fonts are not rejected.

## Done criteria

- [ ] `go test -run 'SkipsFIFO' -count=1 -v .` shows 2 tests `PASS`
- [ ] `grep -n 'IsRegular' context_puregoft.go` prints 1 line, inside
      `describeFontFaces`
- [ ] `make prepush` exits 0
- [ ] `git status` shows changes only in `context_puregoft.go`,
      `discover_fifo_puregoft_test.go`, `CHANGELOG.md` (and `plans/README.md`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- `syscall.Mkfifo` is not available, or the FIFO test cannot be made to fail
  before the fix (for example, `os.Open` does not block on this OS). Report what
  you saw. Do not commit the fix without a test that failed first.
- Any existing discovery test fails after Step 2. That would mean a real font
  path is not a regular file after `os.Stat`. Report the path.
- The fix seems to need changes in `walkDirs`, `consider` or `discover_*.go`.

## Maintenance notes

- Any new caller that opens font paths directly (for example, a future zero-copy
  mmap loader, see `docs/font-memory-plan.md`) must go through
  `describeFontFaces` or repeat the regular-file check.
- A reviewer should confirm the check uses `os.Stat`, not `os.Lstat` or
  `d.Type()`. Both of those would drop symlinked fonts.
