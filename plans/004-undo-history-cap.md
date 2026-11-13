# Plan 004: UndoManager enforces its history limit on every push and ignores no-op mutations

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- undo.go undo_test.go` If any in-scope file
> changed since this plan was written, compare the "Current state" excerpts
> against the live code before proceeding; on a mismatch, treat it as a STOP
> condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `7234b0b`, 2026-11-13

## Why this matters

`NewUndoManager(maxHistory)` promises a bounded history, but the cap is checked
only in `FlushPending` and `Redo`. Normal typing goes through `RecordMutation`.
Each op that does not coalesce (a pause of more than 1 s, a switch between
insert and delete, a jump to another place) pushes the previous pending op onto
`undoStack` **with no limit check**. In a long editing session the undo stack,
and every deleted and inserted string it holds, grows without bound. Second
problem: `RecordMutation` also records mutations that changed nothing (for
example Backspace at offset 0, where `DeletedText == ""` and nothing was
inserted). Each one becomes an empty undo step and **clears the redo stack**.
Pressing Backspace at the start of a field after an undo therefore destroys the
redo history. After this plan the stack never holds more than `maxHistory` ops,
and no-op mutations leave both stacks unchanged.

## Current state

- `undo.go` — `UndoManager` (no build tag, all platforms). There are no other
  files in scope.

`undo.go:116-136` — `RecordMutation`:

```go
func (um *UndoManager) RecordMutation(result MutationResult,
	inserted string, cursorBefore, anchorBefore int) {

	now := time.Now().UnixMilli()
	op := MutationToUndoOp(result, inserted, cursorBefore, anchorBefore)

	if um.shouldCoalesce(op, now) {
		um.coalesceOperation(op)
		um.lastMutationTime = now
	} else {
		if um.coalescableOp != nil {
			um.undoStack = append(um.undoStack, *um.coalescableOp)
			um.coalescableOp = nil
		}
		cp := op
		um.coalescableOp = &cp
		um.lastMutationTime = now
		um.redoStack = nil
	}
}
```

`undo.go:138-147` — `FlushPending`, the only push that trims:

```go
func (um *UndoManager) FlushPending() {
	if um.coalescableOp != nil {
		if len(um.undoStack) >= um.maxHistory {
			um.undoStack = um.undoStack[1:]
		}
		um.undoStack = append(um.undoStack, *um.coalescableOp)
		um.coalescableOp = nil
	}
}
```

`undo.go:219-222` (inside `Redo`) repeats the same trim-then-append.

Note on `um.undoStack[1:]`: re-slicing drops the oldest op from the slice view,
but the backing array still references its strings until `append` reallocates.
The new helper must shift with `copy` and clear the vacated last element, so
dropped ops can be garbage-collected. The maintainer prefers fewer heap
allocations and less retention.

`MutationToUndoOp` (`undo.go:46-70`) classifies a result with no deletion and no
insertion as `OpDelete`.

Test conventions: `undo_test.go`. Tests force non-coalescing by setting
`um.lastMutationTime = 0` (see `TestUndoManagerHistoryLimit`,
`undo_test.go:114-127`) and inspect `len(um.undoStack)` directly (same package).

## Commands you will need

| Purpose       | Command                 | Expected on success |
| ------------- | ----------------------- | ------------------- |
| Focused tests | `go test -run 'Undo' .` | `ok`                |
| Full tests    | `go test ./...`         | all `ok`            |
| Lint          | `make lint`             | `0 issues.`         |
| Full gate     | `make prepush`          | exit 0              |

## Scope

**In scope**: `undo.go`, `undo_test.go`, `CHANGELOG.md` (one bullet under
`## [Unreleased]` → `### Fixed`).

**Out of scope**:

- Coalescing rules (`shouldCoalesce`, `coalesceOperation`). Do not change when
  ops merge.
- The public API. No new exported identifiers.
- `layout_mutation.go`.

## Git workflow

- Branch: `advisor/004-undo-history-cap`
- One commit, e.g.
  `fix(undo): cap history on every push; ignore no-op mutations`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing tests

Add to `undo_test.go`:

1. `TestUndoManagerHistoryLimitWithoutFlush`. `um := NewUndoManager(3)`. Record
   10 inserts at increasing offsets (`InsertText(s, len(s), "x")`, updating
   `s`). Set `um.lastMutationTime = 0` before each call so none coalesce, and
   **never** call `BreakCoalescing` or `FlushPending`. Assert
   `len(um.undoStack) <= 3` after the loop.
2. `TestUndoManagerNoOpMutationKeepsRedo`. Insert `"x"` into `""`, then
   `FlushPending`, then `Undo("x")` → redo stack has 1 op. Then record
   `DeleteBackward("", Layout{}, 0)` (a no-op result) with `inserted == ""`.
   Assert `um.CanRedo()` is still true and `um.UndoDepth() == 0`.
3. `TestUndoManagerTrimReleasesOldest`. `NewUndoManager(2)`. Push 3 ops via
   `RecordMutation` + `FlushPending`, with inserted texts `"a"`, `"b"`, `"c"`.
   Assert the stack's `InsertedText` values are `["b", "c"]`, which checks order
   after the shift.

**Verify**:
`go test -run 'HistoryLimitWithoutFlush|NoOpMutationKeepsRedo|TrimReleasesOldest' .`
→ tests 1 and 2 FAIL. Test 3 may already pass.

### Step 2: Add one push helper and use it everywhere

In `undo.go` add:

```go
// pushUndo appends op to the undo stack, dropping the oldest entry
// first when the stack is at maxHistory. The shift uses copy and clears
// the vacated slot so a dropped op's strings are not kept alive by the
// backing array.
func (um *UndoManager) pushUndo(op UndoOperation) {
	if len(um.undoStack) >= um.maxHistory {
		n := copy(um.undoStack, um.undoStack[1:])
		um.undoStack[n] = UndoOperation{}
		um.undoStack = um.undoStack[:n]
	}
	um.undoStack = append(um.undoStack, op)
}
```

Replace the three push sites with `um.pushUndo(...)`: `RecordMutation`'s
`append`, `FlushPending`'s trim + `append`, and `Redo`'s trim + `append`.

**Verify**: `go test -run 'Undo' .` → `ok` except test 2.

### Step 3: Ignore no-op mutations

At the top of `RecordMutation`, before calling `time.Now()`, return early when
`result.DeletedText == "" && inserted == ""`. Comment: "A mutation that neither
removed nor inserted text (Backspace at offset 0, a delete with an empty range)
changes nothing to undo; recording it would push an empty step and clear the
redo stack."

**Verify**: `go test -run 'Undo' .` → `ok`, with all 3 new tests passing.

### Step 4: Full checks and changelog

Changelog bullet: "`UndoManager` now enforces `maxHistory` while typing, not
only on flush and redo, and no longer records mutations that changed nothing.
Before, such a mutation cleared the redo stack."

**Verify**: `go test ./...` → `ok`. `make lint` → `0 issues.`. `make prepush` →
exit 0.

## Test plan

- 3 new tests in `undo_test.go`. All 15 existing undo tests stay green and
  unchanged.
- Pattern: `TestUndoManagerHistoryLimit` (`undo_test.go:114`).

## Done criteria

- [ ] `go test ./...` exits 0. The 3 new tests exist and pass.
- [ ] `grep -n 'undoStack = append' undo.go` prints exactly 1 line (inside
      `pushUndo`)
- [ ] `grep -n 'undoStack = um.undoStack\[1:\]' undo.go` prints nothing (the
      `copy(um.undoStack, um.undoStack[1:])` shift in `pushUndo` is expected)
- [ ] `make lint` prints `0 issues.`
- [ ] Only in-scope files modified
- [ ] `plans/README.md` status row updated

## STOP conditions

- An existing undo test fails after Step 3. Some test may expect a no-op
  mutation to be recorded. Report which one, and do not change its expectation.
- `UndoManager` has gained a field or method that also pushes to `undoStack`
  (search `undoStack` in `undo.go`) that this plan does not list.

## Maintenance notes

- Any future code path that pushes to `undoStack` must call `pushUndo`.
- go-gui calls `RecordMutation` from its text fields. After this lands,
  Backspace at the start of a field no longer wipes redo there.
- A replace that swaps text for identical text (`DeletedText == inserted`,
  non-empty) is still recorded. That case is rare and harmless, so it is
  deliberately left alone.
