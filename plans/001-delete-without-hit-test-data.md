# Plan 001: Layout-driven deletes never wipe text when the layout has no hit-test data

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- layout_mutation.go layout_mutation_test.go layout_query.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `7234b0b`, 2026-11-13

## Why this matters

`DeleteBackward(text, layout, cursor)` (Backspace) asks the layout for the
previous caret stop. A layout built with `TextConfig.NoHitTesting` (and every
layout that `DrawText`/`TextWidth` cache) has `LogAttrs == nil`. For such a
layout `MoveCursorLeft` returns `0`, so Backspace deletes **everything from
offset 0 to the caret** in one keystroke. `DeleteToWordBoundary` has the same
problem. This is silent data loss. After this plan, deletes on a layout without
hit-test data still remove exactly one grapheme cluster (Backspace / Delete) or
do nothing (word / line deletes). They never remove a whole prefix.

## Current state

- `layout_mutation.go` — pure text-mutation helpers. It has no build tag, so it
  compiles on every platform (native and `js && wasm`).
- `layout_query.go` — cursor queries on `Layout`. **Do not change its behavior**
  (see Out of scope).

`layout_query.go:320-331` today:

```go
// MoveCursorLeft returns the previous valid cursor position.
func (l *Layout) MoveCursorLeft(byteIndex int) int {
	if byteIndex <= 0 || len(l.LogAttrs) == 0 {
		return 0
	}
```

`layout_query.go:364-368` — `MoveCursorWordLeft` has the same
`len(l.LogAttrs) == 0 → return 0` early exit. `layout_query.go:334-337` —
`MoveCursorRight` returns `byteIndex` (does not move) when there are no
LogAttrs. `MoveCursorLineStart` (`layout_query.go:399-413`) loops over `l.Lines`
and returns `0` when no line matches. A `NoHitTesting` layout still has `Lines`,
but a zero `Layout{}` has none.

`layout_mutation.go:35-52` today:

```go
// DeleteBackward removes one grapheme cluster before cursor
// (Backspace). Uses layout.MoveCursorLeft for grapheme boundary.
func DeleteBackward(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	// Clamp what the layout returns too: a layout built for another
	// (longer) text can name offsets past the end of this one. The same
	// holds for every layout-driven delete below.
	prev := clampIndex(layout.MoveCursorLeft(c), len(text))
	return deleteRange(text, prev, c, c)
}

// DeleteForward removes one grapheme cluster after cursor (Delete).
func DeleteForward(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	next := clampIndex(layout.MoveCursorRight(c), len(text))
	return deleteRange(text, c, next, c)
}
```

`layout_mutation.go:80-103`: `DeleteToWordBoundary` uses
`layout.MoveCursorWordLeft(c)`. `DeleteToLineStart` uses
`layout.MoveCursorLineStart(c)`. Both go through `deleteRange`, which does
nothing when `start >= end`.

`config.go:10-16` documents the lean layout:

```go
	// NoHitTesting builds the layout without hit-test data: CharRects,
	// LogAttrs, their index maps and the cursor and word caches stay nil, so
	// cursor, selection and hit-test queries find nothing.
```

**Why the fix goes in `layout_mutation.go` and not `layout_query.go`:** the main
consumer (go-gui, `gui/text_block_query.go:327-336`, `chunkStopAt0`) depends on
`MoveCursorLeft` returning `0` for a layout without LogAttrs. It treats such a
layout as "every byte is a stop". Changing the query's return value would break
caret movement there. The mutation layer must detect missing data itself.

Conventions: comments are full sentences in plain English and explain _why_.
Match the comment density of the existing functions in `layout_mutation.go`.
Tests in `layout_mutation_test.go` are plain `func TestXxx(t *testing.T)` with
`t.Errorf`. They build layouts with `testLayout()` (from
`test_helpers_test.go:211`, text `"Hello\nWorld"`, full LogAttrs) or with a
literal `Layout{...}`. `github.com/rivo/uniseg v0.4.7` is already a direct
dependency (used by `grapheme_ft.go` and `layout_wrap_shared.go`).

## Commands you will need

| Purpose       | Command                                                       | Expected on success |
| ------------- | ------------------------------------------------------------- | ------------------- |
| Focused tests | `go test -run 'Delete' .`                                     | `ok`                |
| Fuzz smoke    | `go test -run '^$' -fuzz FuzzDeleteSelection -fuzztime 10s .` | `ok`                |
| Full tests    | `go test ./...`                                               | all `ok`            |
| WASM compile  | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .`                  | exit 0              |
| Lint          | `make lint`                                                   | `0 issues.`         |
| Format        | `gofmt -l .`                                                  | no output           |
| Full gate     | `make prepush`                                                | exit 0              |

## Scope

**In scope** (the only files you should modify):

- `layout_mutation.go`
- `layout_mutation_test.go`
- `CHANGELOG.md` (one bullet under the top `## [Unreleased]` section; create
  that heading if absent, matching the existing heading style)

**Out of scope** (do NOT touch):

- `layout_query.go`. `MoveCursorLeft`, `MoveCursorWordLeft` and
  `MoveCursorLineStart` keep their current return values. go-gui depends on them
  (see above).
- `undo.go`, and anything outside the root package.

## Git workflow

- Branch: `advisor/001-delete-without-hit-test-data`
- One commit. Message style from `git log`: `fix: <summary>`, e.g.
  `fix: deletes on a layout without hit-test data remove one grapheme`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Write failing tests

Append to `layout_mutation_test.go`:

1. `TestDeleteBackwardWithoutHitTestData`: `l := Layout{Text: "héllo"}` (no
   LogAttrs). `DeleteBackward("héllo", l, 6)` must give `NewText == "héll"`,
   `CursorPos == 5`, `DeletedText == "o"`. Then `DeleteBackward("héllo", l, 3)`
   (the caret is after the 2-byte `é`) must give `"hllo"` and `CursorPos == 1`.
2. `TestDeleteBackwardWithoutHitTestDataGraphemeCluster`: text written in Go
   source as the escaped literal `"a\u0301b"` (ASCII a, then U+0301 COMBINING
   ACUTE ACCENT, then b; 4 bytes total, not the precomposed U+00E1).
   `DeleteBackward(text, Layout{}, 3)` must delete the whole 3-byte cluster
   `"a\u0301"` and give `"b"` with `CursorPos == 0`.
3. `TestDeleteForwardWithoutHitTestData`: `DeleteForward("héllo", Layout{}, 1)`
   gives `"hllo"` and `DeletedText == "é"`.
4. `TestDeleteToWordBoundaryWithoutHitTestData`:
   `DeleteToWordBoundary("hello world", Layout{}, 11)` returns
   `NewText == "hello world"` (unchanged), `CursorPos == 11` and
   `DeletedText == ""`.
5. `TestDeleteToLineStartWithoutLines`:
   `DeleteToLineStart("hello", Layout{}, 5)` returns the text unchanged with
   `CursorPos == 5`.
6. Regression guard, which must pass both before and after the fix:
   `DeleteBackward("Hello\nWorld", testLayout(), 0)` leaves the text unchanged
   (an existing test, `TestDeleteBackwardAtStart`, covers part of this; keep
   it).

**Verify**: `go test -run 'WithoutHitTestData|WithoutLines' .` → FAIL. Tests 1–5
fail (the text is wiped to the prefix or the cursor jumps to 0).

### Step 2: Add a grapheme fallback and no-data guards

In `layout_mutation.go`:

- Add `import "github.com/rivo/uniseg"`.
- Add an unexported helper `hasCaretData(l Layout) bool` that returns
  `len(l.LogAttrs) > 0`. Add a comment that says why it exists: `NoHitTesting`
  layouts and the layouts cached by draw and measure calls carry no LogAttrs,
  and the layout cursor queries then answer 0 (left) or "unchanged" (right).
- Add `prevGraphemeStart(text string, c int) int` and
  `nextGraphemeEnd(text string, c int) int`. They walk `text` from byte 0 with
  `uniseg.FirstGraphemeClusterInString(rest, state)`, accumulate cluster byte
  lengths, and return the start of the cluster that ends at or after `c` (prev)
  or the end of the cluster that contains `c` or starts at it (next; a caret
  inside a cluster deletes only the rest of that cluster, never the next one —
  corrected during review). A comment must state that the walk is O(len(text)).
  That is acceptable because it only runs on the fallback path for one
  keystroke. When `c <= 0`, `prevGraphemeStart` returns 0. When
  `c >= len(text)`, `nextGraphemeEnd` returns `len(text)`.
- `DeleteBackward`: if `!hasCaretData(layout)`, set
  `prev = prevGraphemeStart(text, c)`. Otherwise keep the existing line.
- `DeleteForward`: if `!hasCaretData(layout)`, set
  `next = nextGraphemeEnd(text, c)`.
- `DeleteToWordBoundary` and `DeleteToWordEnd`: if `!hasCaretData(layout)`,
  return `MutationResult{NewText: text, CursorPos: c}` (no-op). Without word
  data, no safe boundary is known.
- `DeleteToLineStart` and `DeleteToLineEnd`: if `len(layout.Lines) == 0`, return
  the same no-op result. A `NoHitTesting` layout keeps `Lines`, so line deletes
  still work on it.
- Update the doc comment of `DeleteBackward` and `DeleteForward` to say that a
  layout without hit-test data falls back to Unicode grapheme segmentation.

**Verify**: `go test -run 'Delete' .` → `ok`. All new and existing delete tests
pass.

### Step 3: Check all platforms and the gate

**Verify**:

- `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .` → exit 0
- `go test -run '^$' -fuzz FuzzDeleteSelection -fuzztime 10s .` → `ok`
- `gofmt -l .` → no output
- `make lint` → `0 issues.`

### Step 4: Changelog

Add under `## [Unreleased]` → `### Fixed` in `CHANGELOG.md`: "Backspace, Delete
and word/line deletes no longer remove the whole text before the caret when the
layout was built without hit-test data (`NoHitTesting`, or a layout cached by
`DrawText`/`TextWidth`). Backspace and Delete fall back to grapheme
segmentation, and word/line deletes do nothing." Check the top of `CHANGELOG.md`
first and copy its exact heading format.

**Verify**: `make prepush` → exit 0.

## Test plan

- The 5 new tests from Step 1, plus the unchanged existing tests in
  `layout_mutation_test.go`.
- Structural pattern: `TestDeleteBackward` in `layout_mutation_test.go:25-37`.
- `go test ./...` → all `ok`, with 5 new tests.

## Done criteria

- [ ] `go test ./...` exits 0. The tests from Step 1 exist and pass.
- [ ] `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .` exits 0
- [ ] `make lint` prints `0 issues.`
- [ ] `git diff --stat 7234b0b -- layout_query.go` prints nothing
- [ ] `git status` shows changes only in `layout_mutation.go`,
      `layout_mutation_test.go`, `CHANGELOG.md`, `plans/README.md`
- [ ] `plans/README.md` status row updated

## STOP conditions

- `layout_mutation.go` already imports a grapheme helper or already handles
  missing LogAttrs (the code has drifted).
- An existing test in `layout_mutation_test.go` or
  `layout_paragraph_cursor_test.go` fails after Step 2. That means some caller
  depends on the old no-data behavior. Report which test.
- The fix seems to need a change to `layout_query.go`.

## Maintenance notes

- go-gui (`gui/text_block_query.go:721`) calls `glyph.DeleteBackward` with
  per-chunk layouts. After this lands, Backspace on a chunk whose layout has no
  LogAttrs deletes one grapheme instead of the chunk's prefix. That is strictly
  safer and needs no go-gui change.
- If `MoveCursorLeft` is ever changed to return `byteIndex` for missing data,
  go-gui's `chunkStopAt0` must change at the same time.
- Reviewers: check that `prevGraphemeStart` never returns a value greater than
  `c`, and that the cursor stays on a rune boundary.
