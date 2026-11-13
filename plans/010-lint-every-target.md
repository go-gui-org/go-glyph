# Plan 010: Lint runs for wasm, windows, darwin and android, not only the host OS

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 6d6a018..HEAD -- Makefile .github/workflows/ci.yml pango_wasm.go layout_attrs_wasm.go layout_iter_wasm.go cache.go bitmap_common.go layout_equivalence_test.go layout_bidi_shared_test.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `6d6a018`, 2026-11-13

## Why this matters

go-glyph swaps whole files by build tag: `*_wasm.go` for the browser,
`discover_windows.go` for Windows, and so on. golangci-lint only checks the
files that build for the `GOOS` it runs under. CI runs `make lint` once, on
Ubuntu, so code that builds only for wasm, Windows or darwin is never linted.
Running the pinned linter for other targets at `6d6a018` finds 9 issues on wasm
and 1 on Windows. Android, linux and darwin are clean. After this plan, a
`make lint-cross` target lints every target that builds without a C toolchain.
`make prepush` and CI both run it, and the 10 current issues are fixed.

## Current state

Measured at `6d6a018` with `.bin/golangci-lint` (the pinned build):

```
GOOS=js GOARCH=wasm CGO_ENABLED=0 golangci-lint run .
pango_wasm.go:17:1: ST1022: comment on exported const FTFaceFlagColor should be of the form "FTFaceFlagColor ..." (staticcheck)
pango_wasm.go:57:1: ST1022: comment on exported const SubpixelBins should be of the form "SubpixelBins ..." (staticcheck)
pango_wasm.go:60:1: ST1022: comment on exported const PangoScale should be of the form "PangoScale ..." (staticcheck)
bitmap_common.go:73:7: const maxNaturalGlyphDim is unused (unused)
cache.go:17:2: field age is unused (unused)
cache.go:21:2: field slot is unused (unused)
layout_attrs_wasm.go:10:6: type runAttributes is unused (unused)
layout_equivalence_test.go:20:6: func runLayoutEquivCases is unused (unused)
layout_iter_wasm.go:10:6: type runMetrics is unused (unused)

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 golangci-lint run . ./accessibility ./ime
layout_bidi_shared.go:66:6: func visualOrderForLine is unused (unused)

GOOS=android GOARCH=arm64 CGO_ENABLED=0 golangci-lint run . ./accessibility ./ime   → 0 issues.
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 golangci-lint run . ./accessibility ./ime    → 0 issues.
GOOS=js GOARCH=wasm CGO_ENABLED=0 golangci-lint run ./accessibility ./ime           → 0 issues.
```

What each one is:

- `pango_wasm.go:16-17`, `:56-57`, `:59-60` — group comments above single
  consts:
  ```go
  // FreeType face flags.
  const FTFaceFlagColor = 1 << 13
  ...
  // Subpixel positioning constants.
  const SubpixelBins = 4

  // Pango constants.
  const PangoScale = 1024
  ```
  Fix: reword so each comment starts with the const name, for example
  `// FTFaceFlagColor is the FreeType face flag for color fonts.` Do not change
  the values. They are exported API.
- `layout_attrs_wasm.go:10` `type runAttributes struct` and
  `layout_iter_wasm.go:10` `type runMetrics struct` — "WASM stubs" that nothing
  references (`grep -n 'runAttributes\|runMetrics' *.go` matches only their
  declarations). Fix: delete both types. If a file then holds only its
  `//go:build` line, `package glyph` and comments, delete the file.
- `cache.go:10-22` (untagged, shared by both renderers):
  ```go
  // Untagged file so both the FreeType and WASM Renderer structs share it.
  type cacheEntry struct {
  	CachedGlyph
  	age uint64
  	// slot is the entry's index in its page's pageKeys list, so removing
  	// one key is O(1) instead of a scan of every key on the page. Unused
  	// for entries with no page (Page < 0) and on WASM.
  	slot int
  }
  ```
  The native renderer reads both fields and the WASM renderer reads neither.
  Fix: add
  `//nolint:unused // read by the native renderer; WASM shares the struct` to
  the end of each field line. Do not move or delete the fields.
- `bitmap_common.go:73` `const maxNaturalGlyphDim = 2048` — used only by native
  rasterization. Fix: the same kind of
  `//nolint:unused // native-only; this file also builds for wasm` comment.
- `layout_equivalence_test.go:20` `func runLayoutEquivCases` — called only from
  `layout_equiv_ft_test.go:17` (native). Fix: a `//nolint:unused` comment with
  the reason "native runner only; the wasm suite does not call it".
- `layout_bidi_shared.go:66` `visualOrderForLine` — a thin wrapper that only
  `layout_bidi_shared_test.go` calls. That test file has
  `//go:build android || linux || darwin`, so on Windows the function has no
  caller. The tests in that file are pure functions with no fonts
  (`TestIsPureLTR_*`, `TestVisualOrderForLine_*`). Fix: change that test file's
  build line to `//go:build android || linux || darwin || windows`, so it
  matches `layout_bidi_shared.go`. CI's `test-windows` job will then run those
  tests.

`Makefile` — the current lint target (lines 72-77) and the shared variable (line
13):

```make
LINT := GOWORK=off $(LINT_BIN)
...
## lint: lint the packages CI lints. Note the scope is narrower than the
## `check` target's ./... — CI passes this explicit list, and this target
## exists to reproduce CI exactly. Run `make check` for the wider sweep.
lint: $(LINT_BIN)
	$(LINT) run . ./accessibility ./backend/gpu ./ime
```

Do not change the `lint` target. `prepush` is at line 118:
`prepush: test-race vet lint nocgo cross-build build-modules`. `.PHONY` is at
lines 1-2.

`.github/workflows/ci.yml` — the `lint` job ends with:

```yaml
# Builds the linter pinned in tools/lint/go.mod, the same binary
# `make lint` runs locally.
- name: Run golangci-lint
  run: make lint
```

`backend/gpu` needs cgo (OpenGL, X11) on linux and Windows, so it stays out of
the cross-target lint. iOS needs an Xcode sysroot, so it stays out too.

## Commands you will need

| Purpose    | Command                                          | Expected on success         |
| ---------- | ------------------------------------------------ | --------------------------- |
| Build lint | `make lint-bin`                                  | `.bin/golangci-lint` exists |
| Host lint  | `make lint`                                      | `0 issues.`                 |
| Cross lint | `make lint-cross`                                | `0 issues.` ×4              |
| WASM build | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` | exit 0                      |
| Full gate  | `make prepush`                                   | exit 0                      |

## Scope

**In scope**:

- `Makefile` — new `lint-cross` target, added to `.PHONY` and `prepush`
- `.github/workflows/ci.yml` — one new step in the `lint` job
- `pango_wasm.go` — the three comments only
- `layout_attrs_wasm.go`, `layout_iter_wasm.go` — delete the unused types
  (delete a file if it ends up empty)
- `cache.go`, `bitmap_common.go`, `layout_equivalence_test.go` — `//nolint`
  comments only
- `layout_bidi_shared_test.go` — the build line only

**Out of scope**:

- Any lint fix that changes behavior or exported API. Only comments, the two
  unused wasm-only types, and one test build tag change.
- `.golangci.yml`. Do not disable linters or add exclusions.
- `backend/gpu` and `backend/ebitengine` cross-linting (they need cgo).
- Whether the WASM glyph cache should age entries (`cacheEntry.age` is unused
  there). Note it for the reviewer. Do not change it.

## Git workflow

- Branch: `advisor/010-lint-every-target`
- One commit, e.g. `build: lint wasm, windows, darwin and android targets`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add the `lint-cross` target and see it fail

Add to the `Makefile`, directly after the `lint` target, following its comment
style:

```make
## lint-cross: lint the targets the host never compiles. Files are swapped
## by build tag (*_wasm.go, discover_windows.go, ...), and golangci-lint
## only sees files that build for its GOOS, so `make lint` on one OS leaves
## the others unlinted. backend/gpu needs cgo and iOS needs an Xcode
## sysroot, so neither is here.
lint-cross: $(LINT_BIN)
	GOOS=js GOARCH=wasm CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
```

Add `lint-cross` to `.PHONY`.

**Verify**: `make lint-cross` → fails on the first (wasm) line with the 9 issues
listed in "Current state". If the list differs, compare it with "Current state".
New issues in in-scope files are fine to fix the same way. Issues in other files
are a STOP condition.

### Step 2: Fix the wasm issues

Apply the fixes from "Current state" to `pango_wasm.go`, `layout_attrs_wasm.go`,
`layout_iter_wasm.go`, `cache.go`, `bitmap_common.go` and
`layout_equivalence_test.go`. Every `//nolint` must name the linter
(`//nolint:unused`) and give a reason after `//`.

**Verify**:
`GOWORK=off GOOS=js GOARCH=wasm CGO_ENABLED=0 .bin/golangci-lint run . ./accessibility ./ime`
→ `0 issues.` Also `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` → exit 0.

### Step 3: Fix the Windows issue

Change line 1 of `layout_bidi_shared_test.go` to
`//go:build android || linux || darwin || windows`.

**Verify**:
`GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 .bin/golangci-lint run . ./accessibility ./ime`
→ `0 issues.` Also `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet .` → exit 0.
Then `make lint-cross` → exit 0.

### Step 4: Wire into prepush and CI

- `Makefile`: change `prepush` to
  `prepush: test-race vet lint lint-cross nocgo cross-build build-modules`. Add
  one line to the prepush comment block that says it now lints every cgo-free
  target.
- `.github/workflows/ci.yml`: in the `lint` job, add after "Run golangci-lint":

```yaml
# Lints the GOOS targets the Ubuntu host never compiles (wasm,
# windows, darwin, android), with the same pinned linter.
- name: Run golangci-lint for other targets
  run: make lint-cross
```

**Verify**: `make lint` → `0 issues.` `make prepush` → exit 0.

## Test plan

No new Go tests. The checks are `make lint-cross` reaching `0 issues.` on all 4
targets, and the existing suite passing (`make prepush`). The `TestIsPureLTR_*`
and `TestVisualOrderForLine_*` tests now also build for Windows, and CI's
`test-windows` job runs them there.

## Done criteria

- [ ] `make lint-cross` exits 0
- [ ] `grep -n 'lint-cross' Makefile` shows the target, `.PHONY` and `prepush`
- [ ] `grep -n 'make lint-cross' .github/workflows/ci.yml` prints 1 line
- [ ] `grep -rn 'runAttributes\|runMetrics' --include='*.go' .` prints nothing
- [ ] `grep -c 'nolint:unused' cache.go bitmap_common.go layout_equivalence_test.go`
      → 2, 1, 1
- [ ] `make prepush` exits 0
- [ ] Only in-scope files changed (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- `make lint-cross` reports issues in files not listed in Scope. Report the
  list. Do not widen the scope.
- A fix for an `unused` finding seems to need deleting or moving code that the
  native build uses. Native `make lint` must stay at `0 issues.`, and native
  tests must pass.
- Deleting `runAttributes` / `runMetrics` breaks `GOOS=js go vet ./...`. That
  would mean something references them. Report the reference.
- The Windows build tag change makes `GOOS=windows go vet .` fail.

## Maintenance notes

- New `//nolint:unused` comments are a smell. If a shared file collects many
  native-only items, move them into a native-tagged file instead.
- Reviewer: the WASM renderer never reads `cacheEntry.age`. Check whether the
  WASM glyph cache evicts at all. That is a possible follow-up finding, not part
  of this plan.
- `lint-cross` roughly doubles lint time. If CI gets slow, run the four targets
  as a matrix.
