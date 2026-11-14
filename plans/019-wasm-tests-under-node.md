# Plan 019: The js/wasm tests run under node in CI

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 4154f31..HEAD -- Makefile .github/workflows/ci.yml draw_transform_test.go test_helpers_test.go validation_test.go context_wasm.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW (test and build files only; no library code changes)
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `4154f31`, 2026-11-14

## Why this matters

The WASM build has its own layout code (`layout_wasm.go`, `context_wasm.go`,
`draw_wasm.go`), and many shared files are tagged for it. CI's `test-wasm` job
only **builds** and **vets** for `GOOS=js GOARCH=wasm`. No WASM test runs
anywhere. A wasm-only layout bug (wrap, caret, bidi, indent) is found only when
someone opens the web demo.

Go can run js/wasm test binaries under node through
`$(go env GOROOT)/lib/wasm/go_js_wasm_exec`. The only thing node lacks is the
browser's Canvas2D. `NewContext` (`context_wasm.go:27-36`) uses
`document.createElement("canvas")`, or `OffscreenCanvas` when there is no
`document`. Layout calls only `measureText` (reading `width`,
`fontBoundingBoxAscent/Descent` and `actualBoundingBox*`) and sets `font`.
Grapheme splitting uses `Intl.Segmenter`, which node has.

This was tried on 2026-11-14 on a scratch copy of the repo (Go 1.27.4, node
v26). A 15-line `OffscreenCanvas` shim was preloaded with
`NODE_OPTIONS=--require`. The results:

- `./accessibility` and `./ime`: `ok`.
- Root package: **316 tests pass, 10 fail**. Each of the 10 asserts native-only
  behavior. None is a wasm layout bug:

| Test (file:line)                                                             | Failure                                     | Why it cannot hold on wasm                                 |
| ---------------------------------------------------------------------------- | ------------------------------------------- | ---------------------------------------------------------- |
| `TestTransformedBackgroundRotates` (`draw_transform_test.go:24`)             | `transformed fills = 0, want 1`             | wasm draws on the page canvas, not through `DrawBackend`   |
| `TestIdentityBackgroundUnchanged` (`draw_transform_test.go:63`)              | `fills = 0, want 1`                         | same                                                       |
| `TestTransformedDecorationRotates` (`draw_transform_test.go:87`)             | `transformed fills = 0, want 1 (underline)` | same                                                       |
| `TestTransformedFillFallbackWithoutExtension` (`draw_transform_test.go:175`) | `fallback rects = 0, want 1`                | same                                                       |
| `TestTextSystemDrawText` (`test_helpers_test.go:91`)                         | `no draw calls after DrawText + Commit`     | same                                                       |
| `TestTextSystemPurge` (`test_helpers_test.go:162`)                           | `glyph cache should have entries`           | wasm draws with `fillText` and never fills the glyph cache |
| `TestTextSystemAddFontFile` (`test_helpers_test.go:131`)                     | `expected error for empty path`             | wasm has no filesystem; `ValidateFontPath` is a no-op      |
| `TestValidatePathTraversal` (`validation_test.go:61`)                        | `expected traversal error, got: <nil>`      | same (`validation_wasm.go`)                                |
| `TestValidatePathNonexistent` (`validation_test.go:68`)                      | `expected not-accessible error`             | same                                                       |
| `TestValidatePathEmpty` (`validation_test.go:75`)                            | `expected empty error`                      | same                                                       |

After this plan, those 10 skip on js with a reason. The rest run in CI on every
PR, and locally with `make test-wasm`.

Maintainer decision (2026-11-14): CI only. Do **not** add `test-wasm` to
`make prepush`, so `prepush` does not need node.

## Current state

`context_wasm.go:27-41`:

```go
	doc := js.Global().Get("document")
	var canvas js.Value
	if !doc.IsUndefined() && !doc.IsNull() {
		canvas = doc.Call("createElement", "canvas")
		canvas.Set("width", 1)
		canvas.Set("height", 1)
	} else {
		// OffscreenCanvas for worker context.
		canvas = js.Global().Get("OffscreenCanvas").New(1, 1)
	}

	ctx2d := canvas.Call("getContext", "2d")
```

`validation_wasm.go`:

```go
// ValidateFontPath is a no-op under WASM (no filesystem).
func ValidateFontPath(_ string, _ string) error { return nil }
```

`draw_transform_test.go`, `test_helpers_test.go` and `validation_test.go` have
**no** build tag, so they compile for wasm.

`.github/workflows/ci.yml:162-194` is the `test-wasm` job. Its last step:

```yaml
- name: Vet WASM packages
  env:
    GOOS: js
    GOARCH: wasm
    CGO_ENABLED: "0"
  run: go vet ./...
```

`Makefile`: the first line is the `.PHONY` list. The `test-race` target is the
style to copy (a `## name: description` comment, then the recipe). `prepush` is
the last target:

```make
prepush: test-race vet lint lint-cross nocgo cross-build build-modules
```

The comment block above `prepush` lists what it runs. Its "Omissions vs CI, by
design" list says what it leaves out compared with CI.

`scripts/` exists (it holds `gen-docs.sh`).

## Commands you will need

| Purpose      | Command              | Expected on success |
| ------------ | -------------------- | ------------------- |
| WASM tests   | `make test-wasm`     | 3 `ok` lines        |
| Native tests | `go test -count=1 .` | `ok`                |
| Node present | `node --version`     | a version           |
| Full gate    | `make prepush`       | exit 0              |

## Scope

**In scope**:

- `scripts/wasm-canvas-shim.cjs` (create).
- `skip_wasm_test.go` (create): one helper.
- `draw_transform_test.go`, `test_helpers_test.go`, `validation_test.go`: add
  one `skipOnWASM(...)` line at the top of each of the 10 tests in the table.
- `Makefile`: the `test-wasm` target, `.PHONY`, and one new bullet in the
  "Omissions vs CI" comment above `prepush`. The `prepush:` line itself does not
  change.
- `.github/workflows/ci.yml`: one new step at the end of the `test-wasm` job.
- `CLAUDE.md`: in the `## Essential Commands` code block, add `make test-wasm`
  with a one-line comment. In `## CI`, say that `test-wasm` also runs the tests
  under node.

**Out of scope**:

- Any non-test `.go` file. If a wasm test fails for a reason not in the table,
  it is a finding to report, not something to fix here.
- `backend/web` and the other modules. They need a real browser.
- Making the shim's metrics realistic. Tests must not depend on exact pixel
  values from the shim.
- The unused WASM glyph cache (`Renderer.cache` in `renderer_wasm.go` is never
  written). That is a separate cleanup.

## Git workflow

- Branch: `advisor/019-wasm-tests-under-node`
- One commit, e.g. `test: run the js/wasm tests under node with a Canvas2D shim`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add the shim

Create `scripts/wasm-canvas-shim.cjs`:

```js
// Canvas2D stand-in for running the js/wasm tests under node.
//
// node has no DOM, so NewContext (context_wasm.go) falls back to
// OffscreenCanvas. This file defines it before the Go test binary starts
// (NODE_OPTIONS=--require). Metrics are fixed and simple: every code point
// is 0.6em wide, ascent is 0.8em and descent is 0.2em. Tests check layout
// logic, not real font metrics.
"use strict";

class Ctx2D {
  constructor() {
    this.font = "10px sans-serif";
  }
  px() {
    const m = /(\d+(?:\.\d+)?)px/.exec(this.font);
    return m ? parseFloat(m[1]) : 10;
  }
  measureText(s) {
    const px = this.px();
    const w = [...String(s)].length * px * 0.6;
    return {
      width: w,
      fontBoundingBoxAscent: px * 0.8,
      fontBoundingBoxDescent: px * 0.2,
      actualBoundingBoxAscent: px * 0.7,
      actualBoundingBoxDescent: px * 0.1,
      actualBoundingBoxLeft: 0,
      actualBoundingBoxRight: w,
    };
  }
}

globalThis.OffscreenCanvas = class {
  constructor(w, h) {
    this.width = w;
    this.height = h;
  }
  getContext() {
    return new Ctx2D();
  }
};
```

**Verify**:
`node -e 'require("./scripts/wasm-canvas-shim.cjs"); console.log(new OffscreenCanvas(1,1).getContext("2d").measureText("ab").width)'`
→ `12`.

### Step 2: Add the `test-wasm` target and see the 10 failures

In `Makefile`, add `test-wasm` to the `.PHONY` list. Add this target after
`test-race`:

```make
## test-wasm: run the root, accessibility and ime tests as js/wasm under
## node. go_js_wasm_exec (from GOROOT/lib/wasm) runs each test binary in
## node; scripts/wasm-canvas-shim.cjs stands in for the browser's Canvas2D,
## so the wasm layout path runs without a page. Needs node on PATH.
test-wasm:
	@command -v node >/dev/null || { echo "test-wasm: node is not on PATH"; exit 1; }
	PATH="$$(go env GOROOT)/lib/wasm:$$PATH" \
	  NODE_OPTIONS="--require $(CURDIR)/scripts/wasm-canvas-shim.cjs" \
	  GOOS=js GOARCH=wasm CGO_ENABLED=0 go test -count=1 . ./accessibility ./ime
```

Recipe lines start with a TAB.

**Verify**: `make test-wasm 2>&1 | grep -c '^--- FAIL'` → `10`. Each failing
name is in the table above. `accessibility` and `ime` print `ok`. If the count
is not 10, or a name is not in the table, STOP.

### Step 3: Skip the 10 native-only tests on js

Create `skip_wasm_test.go` (no build tag):

```go
package glyph

import (
	"runtime"
	"testing"
)

// skipOnWASM skips a test that checks native-only behavior when it runs as
// js/wasm (make test-wasm). The wasm build draws through the page canvas with
// fillText, not through DrawBackend or the glyph cache, and has no file system.
func skipOnWASM(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("js/wasm: " + why)
	}
}
```

Add one line as the first statement of each test:

- The 4 tests in `draw_transform_test.go` and `TestTextSystemDrawText`:
  `skipOnWASM(t, "draws on the page canvas, not through DrawBackend")`
- `TestTextSystemPurge`:
  `skipOnWASM(t, "fillText drawing never fills the glyph cache")`
- `TestTextSystemAddFontFile` and the 3 `TestValidatePath*` tests:
  `skipOnWASM(t, "no file system; ValidateFontPath is a no-op")`

**Verify**: `make test-wasm` → 3 lines starting with `ok`. `go test -count=1 .`
(native) → `ok`. The 10 tests still run and pass natively.

### Step 4: Run it in CI (not in `prepush`)

`Makefile`: leave the `prepush:` line unchanged. In the comment above it, under
"Omissions vs CI, by design", add this bullet (match the other bullets' `##   -`
prefix):

```make
##   - the js/wasm tests under node (`make test-wasm`), so prepush does not
##     need node; CI's test-wasm job runs them
```

`.github/workflows/ci.yml`: at the end of the `test-wasm` job, after
`Vet WASM packages`, add (same indentation as the other steps):

```yaml
- name: Test WASM packages under node
  run: make test-wasm
```

`ubuntu-latest` has node installed. Do not add a `setup-node` step unless the CI
run shows `node is not on PATH`.

`CLAUDE.md`: update the two places listed in Scope. Then run
`prettier --prose-wrap always --print-width 80 --write CLAUDE.md`.

**Verify**: `make prepush` → exit 0.
`grep -n 'make test-wasm' .github/workflows/ci.yml` → 1 line.
`prettier --prose-wrap always --print-width 80 --check CLAUDE.md` → passes.

## Test plan

No new test logic. The plan turns on about 316 existing tests for js/wasm: the
shared layout, wrap, cursor, selection and bidi tests in untagged and
`(js && wasm)`-tagged files, plus `accessibility` and `ime`. The 10 tests that
check native-only behavior skip on js with a stated reason.

## Done criteria

- [ ] `make test-wasm` exits 0 and prints 3 `ok` lines
- [ ] `make test-wasm 2>&1 | grep -c '^--- FAIL'` → `0`
- [ ] `grep -c 'skipOnWASM(t' draw_transform_test.go test_helpers_test.go validation_test.go`
      → `4`, `3`, `3`
- [ ] `grep -n 'make test-wasm' .github/workflows/ci.yml` prints 1 line
- [ ] `grep -n '^prepush:.*test-wasm' Makefile` prints nothing (CI only)
- [ ] `grep -n 'make test-wasm' Makefile` prints 1 line (the omissions note)
- [ ] `make prepush` exits 0
- [ ] Only in-scope files changed (`git status`, plus `plans/README.md`)

## STOP conditions

- Step 2 shows a failure that is not in the table. It may be a real wasm bug.
  Report the test name and its message. Do not skip it.
- `$(go env GOROOT)/lib/wasm/go_js_wasm_exec` does not exist (Go older than 1.24
  kept it in `misc/wasm`). Report `go version`.
- A test hangs or node crashes. Report the last 30 lines of output.
- The fix seems to need a change to a non-test `.go` file.

## Maintenance notes

- A new test that checks `DrawBackend` calls, the glyph cache or file paths must
  call `skipOnWASM`, or have a `!js` build tag. CI's `test-wasm` job catches the
  ones that forget. `prepush` does not, so run `make test-wasm` locally when a
  change touches shared test files.
- The shim's metrics are not real. A wasm test must not assert exact pixel
  widths. It can compare widths with each other (for example, wrapped line width
  ≤ the wrap width).
- If `context_wasm.go` starts to call another Canvas2D API, add it to the shim,
  or `make test-wasm` fails with `undefined`.
