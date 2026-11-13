# Plan 005: A malformed font cannot crash the process or leave a face lock held

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- context_puregoft.go bitmap_puregoft.go renderer_ft.go freetype_types_puregoft.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security (robustness / DoS)
- **Planned at**: commit `7234b0b`, 2026-11-13

## Why this matters

The native backend parses fonts with the pure-Go `go-text/typesetting` library.
It can panic on defective tables, and the repo already knows this: `parseFace`,
`parseCoverage`, `safeShape`, `glyphInkExtents` and `capEm` each recover. Three
places still do not:

1. `describeFontFaces` runs for every font file during system font discovery,
   and in `AddFontFile` / `AddFontBytes`. It parses with no recover, so one
   broken font in a user font directory crashes the first `NewContext`.
2. The renderer's glyph-load dispatch (`renderer_ft.go`) calls rasterizers that
   run `GlyphData`, `COLR.Search` and the shaper with no recover. A bad glyph
   crashes the render thread.
3. `renderColorGlyph` takes `cf.mu.Lock()` and releases it with a plain
   `cf.mu.Unlock()`, not `defer`. If `GlyphData`/`GlyphExtents` panics between
   them and the host recovers (GUI frame loops often do), the face mutex stays
   locked. Every later color-glyph render or ink measurement of that cached face
   then **deadlocks** in every Context, because faces are cached process-wide.

After this plan, each of these degrades to "font unusable" or "glyph not drawn"
(negative-cached), and the mutex is always released.

## Current state

Files (all native-only, build tag `linux || darwin || windows`; corrected during
review: these files exclude android, so the new test file uses the same tag):

- `context_puregoft.go` — font discovery. `describeFontFaces` at lines 240-268.
- `bitmap_puregoft.go` — rasterizers. `renderColorGlyph` at lines 612-~728.
- `renderer_ft.go` — renderer glyph cache. Load dispatch at lines ~180-207.
- `freetype_types_puregoft.go` — `cachedFace` (lines 34-60) and the existing
  recover patterns.

`context_puregoft.go:247-262` (no recover):

```go
func describeFontFaces(path string) []faceInfo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	loaders, err := ot.NewLoaders(f)
	if err != nil || len(loaders) == 0 {
		return nil
	}
	faces := make([]faceInfo, 0, len(loaders))
	for i, ld := range loaders {
		desc, _ := font.Describe(ld, nil)
		faces = append(faces, faceInfo{
```

`bitmap_puregoft.go:638-651` (lock without defer):

```go
	ppem := uint16(min(size+0.5, 65535))
	cf.mu.Lock()
	cf.face.SetPpem(ppem, ppem)
	gd := cf.face.GlyphData(gid)
	var pngData []byte
	if gb, ok := gd.(font.GlyphBitmap); ok && gb.Format == font.PNG &&
		len(gb.Data) > 0 && len(gb.Data) <= maxEmbeddedBitmapBytes {
		pngData = append([]byte(nil), gb.Data...)
	}
	// The extents depend on the strike SetPpem chose, ...
	ext, extOK := cf.face.GlyphExtents(gid)
	cf.mu.Unlock()
	if pngData == nil {
		return nil, false
	}
```

`renderer_ft.go:180-207` (dispatch; an error already becomes a negative cache
entry):

```go
	var result LoadGlyphResult
	var loadErr error
	switch {
	case isBox:
		result, loadErr = loadBoxGlyphFT(r.atlas, boxM)
	case byID:
		result, loadErr = loadGlyphByIDFT(r.atlas, item.FontPath,
			g.GlyphID, item, strokeWidth, bin, r.scaleFactor)
	case strokeWidth > 0:
		result, loadErr = loadStrokedGlyphFT(r.atlas, r.fontPaths, ch, runText,
			targetRuneIdx, item, strokeWidth, bin, r.scaleFactor)
	default:
		result, loadErr = loadGlyphFT(r.atlas, r.fontPaths, ch, runText,
			targetRuneIdx, item, bin, r.scaleFactor)
	}
	if loadErr != nil {
		// Negative-cache the failure (Page -1) so a broken glyph is not
		// re-rasterized every frame. ...
```

**Exemplar to match** — `ink_bounds_puregoft.go:93-108`, the right way to lock
and recover around a go-text call:

```go
	cf.mu.Lock()
	defer cf.mu.Unlock()
	defer func() {
		if recover() != nil {
			ext, ok = font.GlyphExtents{}, false
		}
	}()
	return cf.face.GlyphExtents(gid)
```

And `freetype_types_puregoft.go:486-494` (`safeShape`). The comment style is
plain English, and it says _why_ a recover is there: "can panic on defective
font tables … must degrade … rather than crash the render thread."

Errors use the `glyph: ` prefix (`context_puregoft.go:178`:
`fmt.Errorf("glyph: failed to parse font %q", path)`).

**There are no test fonts in the repo**, and no corrupt font reliably triggers a
go-text panic. The plan therefore adds small helpers with injectable functions
and unit-tests those helpers. It also adds one package-level seam for
`describeFontFaces`.

## Commands you will need

| Purpose       | Command                                                   | Expected on success |
| ------------- | --------------------------------------------------------- | ------------------- |
| Focused tests | `go test -run 'Guard' .` then `go test -run 'Describe' .` | `ok`                |
| Full tests    | `go test ./...`                                           | all `ok`            |
| Race          | `go test -race -run 'Guard' .`                            | `ok`                |
| Android vet   | `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet .`        | exit 0              |
| Lint          | `make lint`                                               | `0 issues.`         |
| Full gate     | `make prepush`                                            | exit 0              |

## Scope

**In scope**:

- `context_puregoft.go` — `describeFontFaces` only
- `bitmap_puregoft.go` — `renderColorGlyph` only
- `renderer_ft.go` — the load dispatch only
- `freetype_types_puregoft.go` — add the two helpers next to `safeShape`
- `font_guard_puregoft_test.go` (create, with the same build tag as the files
  above)
- `CHANGELOG.md` — one bullet under `## [Unreleased]` → `### Fixed`

**Out of scope**:

- Any `*_wasm.go` file. The browser parses fonts there, not go-text.
- Bounding table sizes inside go-text, which is a separate problem with its own
  plan. Do not edit `go.mod` or vendor go-text.
- `parseFace`, `parseCoverage`, `glyphInkExtents`, `capEm`. They are already
  guarded.
- Changing what `loadGlyphFT` etc. return in non-panic cases.

## Git workflow

- Branch: `advisor/005-font-panic-guards`
- One commit, e.g.
  `fix: recover go-text panics in font discovery and glyph loads; always release face lock`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add the helpers and their tests (test first)

Create `font_guard_puregoft_test.go` with build tag
`//go:build android || linux || darwin || windows`. Write:

1. `TestLockedFaceCallReleasesLockOnPanic`: `cf := &cachedFace{}`.
   `ok := cf.lockedCall(func() { panic("bad table") })`. Assert `ok == false`
   and `cf.mu.TryLock() == true` (then `cf.mu.Unlock()`).
2. `TestLockedFaceCallOK`: a non-panicking fn runs once (counter == 1) and
   `ok == true`.
3. `TestGuardGlyphLoadConvertsPanic`:
   `res, err := guardGlyphLoad(func() (LoadGlyphResult, error) { panic("x") })`.
   Assert `err != nil`, `strings.HasPrefix(err.Error(), "glyph: ")`, and
   `res == LoadGlyphResult{}`.
4. `TestGuardGlyphLoadPassesThrough`: a func that returns a known error returns
   that same error (`errors.Is`).

**Verify**: `go test -run 'LockedFaceCall|GuardGlyphLoad' .` → build failure
(the helpers do not exist yet). That is the expected "red".

Then add to `freetype_types_puregoft.go`, directly below `safeShape`:

```go
// lockedCall runs fn with cf.mu held and reports whether it finished.
// go-text can panic on defective font tables; the deferred Unlock runs
// even then, so one bad glyph cannot leave the shared face locked (every
// later color render or ink measurement of the face would deadlock).
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

// guardGlyphLoad runs one glyph rasterization and turns a go-text panic
// into an error, which the renderer negative-caches like any other load
// failure instead of crashing the render thread.
func guardGlyphLoad(load func() (LoadGlyphResult, error)) (
	res LoadGlyphResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = LoadGlyphResult{}, fmt.Errorf("glyph: rasterizer panic: %v", r)
		}
	}()
	return load()
}
```

Add the `fmt` import if the file lacks it.

**Verify**: `go test -run 'LockedFaceCall|GuardGlyphLoad' .` → `ok` (4 tests)

### Step 2: Use `lockedCall` in `renderColorGlyph`

Replace the `cf.mu.Lock()` … `cf.mu.Unlock()` block (excerpt above) with:

```go
	var gd font.GlyphData
	var ext font.GlyphExtents
	var extOK bool
	if !cf.lockedCall(func() {
		cf.face.SetPpem(ppem, ppem)
		gd = cf.face.GlyphData(gid)
		if gb, ok := gd.(font.GlyphBitmap); ok && gb.Format == font.PNG &&
			len(gb.Data) > 0 && len(gb.Data) <= maxEmbeddedBitmapBytes {
			pngData = append([]byte(nil), gb.Data...)
		}
		ext, extOK = cf.face.GlyphExtents(gid)
	}) {
		return nil, false
	}
```

Keep the existing comments about why SetPpem + GlyphData + the copy run under
the lock. Move them above the call. Declare `var pngData []byte` before it.
Check the real type name of the `GlyphData` return value in the go-text `font`
package (`go doc github.com/go-text/typesetting/font Face.GlyphData`) and use
that name. If `gd` is not needed after the closure, keep it local inside the
closure instead.

**Verify**: `grep -nE 'cf.mu.(Lock|Unlock)' bitmap_puregoft.go` → no output.
Then `go test ./...` → all `ok`.

### Step 3: Guard the renderer load dispatch

In `renderer_ft.go`, wrap the `switch` in `guardGlyphLoad`:

```go
	result, loadErr := guardGlyphLoad(func() (LoadGlyphResult, error) {
		switch {
		case isBox:
			return loadBoxGlyphFT(r.atlas, boxM)
		...
		default:
			return loadGlyphFT(...same args...)
		}
	})
```

Remove the now-redundant `var result` / `var loadErr` declarations. Keep the
`if loadErr != nil` negative-cache block unchanged. Add a one-line comment: "A
go-text panic on a defective font becomes a load error and is negative-cached
below."

**Verify**: `go test ./...` → all `ok`. Then
`go test -run '^$' -bench 'TerminalFrame' -benchmem -count 1 .` → `0 allocs/op`
for `BenchmarkTerminalFrame1500` and `BenchmarkTerminalFrameBoxDrawing`. The
cache-hit path returns before this code, so steady-state allocations must
stay 0. If they are no longer 0, STOP.

### Step 4: Guard `describeFontFaces`

1. Add a package-level seam just above the function:

   ```go
   // describeFace is font.Describe; a variable so tests can inject a panic.
   var describeFace = font.Describe
   ```

   Use `describeFace(ld, nil)` in the loop.

2. Change the signature to a named result `(faces []faceInfo)` and add, right
   after `defer f.Close()`:

   ```go
   	// go-text can panic on defective tables. Discovery walks every system
   	// font, so one broken file must drop only itself, not crash NewContext.
   	defer func() {
   		if recover() != nil {
   			faces = nil
   		}
   	}()
   ```

   Keep `faces := make(...)` as an assignment (`faces = make(...)`), so it does
   not shadow the named result.

3. Add the test `TestDescribeFontFacesRecoversPanic` to
   `font_guard_puregoft_test.go`. Find one installed font path:
   `ctx, _ := NewContext(1); defer ctx.Free()`, then take any value from
   `ctx.fontPaths`, or `t.Skip` if empty. Save `describeFace`, set it to a func
   that panics, and restore it with `t.Cleanup`. Assert
   `describeFontFaces(path) == nil` with no panic. Also assert that
   `ctx.AddFontFile(path)` returns a non-nil error whose text starts with
   `"glyph: "`.

**Verify**: `go test -run 'Describe|Collection' .` → `ok`. The existing
`TestCollectionFacesRegistered` in `facepath_puregoft_test.go` must still pass.

### Step 5: Full checks and changelog

Changelog bullet: "A font whose tables make the parser panic no longer crashes
font discovery, `AddFontFile`/`AddFontBytes`, or glyph rendering. The font or
glyph is skipped. A panic inside the color-glyph path no longer leaves the
shared face locked."

**Verify**:

- `go test ./...` → all `ok`
- `go test -race -run 'LockedFaceCall|GuardGlyphLoad|Describe' .` → `ok`
- `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet .` → exit 0
- `make lint` → `0 issues.`
- `make prepush` → exit 0

## Test plan

- 5 new tests in `font_guard_puregoft_test.go` (4 helper tests + 1 describe seam
  test).
- Existing color-emoji tests (`emoji_render_linux_test.go`,
  `emoji_metrics_darwin_test.go`, `color_glyph_box_test.go`) prove that
  `renderColorGlyph` still works.
- The terminal-frame benchmarks prove there is no steady-state allocation
  regression.

## Done criteria

- [ ] `go test ./...` exits 0. The 5 new tests pass.
- [ ] `grep -n 'cf.mu.Lock()' bitmap_puregoft.go` prints nothing
- [ ] `grep -n 'guardGlyphLoad' renderer_ft.go` prints 1 line
- [ ] `grep -n 'recover()' context_puregoft.go` prints at least 1 line inside
      `describeFontFaces`
- [ ] `BenchmarkTerminalFrame1500` still reports `0 allocs/op`
- [ ] `make lint` prints `0 issues.`
- [ ] Only in-scope files modified
- [ ] `plans/README.md` status row updated

## STOP conditions

- `cachedFace.mu` is no longer a `sync.Mutex` (for example, it became an
  `RWMutex`), or a second `cf.mu.Lock()` site has appeared in
  `bitmap_puregoft.go`. Report it, and do not guess the locking design.
- Wrapping the dispatch in a closure makes the steady-state benchmark allocate.
- `renderer_ft.go`'s dispatch has gained cases that call into code outside the
  root package.

## Maintenance notes

- Every new direct go-text call on a shared `cachedFace` that needs the lock
  must go through `cf.lockedCall`. Every new rasterizer entry is already covered
  by `guardGlyphLoad` if it is called from the dispatch.
- A recovered panic leaves no log. If diagnosing bad fonts becomes necessary,
  add a debug hook here. Do not log from the render path by default.
- Related, not covered here: go-text allocates table buffers sized from
  untrusted header fields, so a tiny file can request about 4 GiB. That is an
  OOM, not a panic, and `recover` cannot catch it. It needs a size check before
  `ot.NewLoaders`.
