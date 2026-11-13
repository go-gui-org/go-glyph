# Plan 002: The caret at a soft-wrap boundary is drawn on the line it moves to

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- layout_query.go layout_wrap_longword_test.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition. Plan 001 does not touch `layout_query.go`, so a clean
> drift check is expected even after 001.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none (run after 001 only to avoid working on two branches in
  parallel)
- **Category**: bug
- **Planned at**: commit `7234b0b`, 2026-11-13

## Why this matters

Some soft wraps do not consume a space: `WrapChar`, `WrapWordChar`, and UAX #14
breaks between CJK characters. At such a wrap, line k ends at byte `b` and line
k+1 starts at the same byte `b`. Caret _movement_ (`MoveCursorLineStart`,
`MoveCursorLineEnd`, `MoveCursorUp`, `MoveCursorDown`) treats `b` as being on
line k+1. Caret _drawing_ (`GetCursorPos`) takes X from the first character of
line k+1 but Y and Height from line k. The caret then shows at the **left edge
of the line above**. This happens every time the user moves to the start of a
wrapped line in CJK text or in char-wrapped text. It affects both the native and
wasm backends (shared code). After this plan, `GetCursorPos(b)` reports line
k+1's Y and Height, which matches the movement functions.

## Current state

- `layout_query.go` — cursor and hit-test queries on `Layout`, shared by all
  backends (no build tag).

`layout_query.go:299-307` — the helper with the bug:

```go
func (l *Layout) lineForByteIndex(byteIndex int) (Line, bool) {
	for _, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			return line, true
		}
	}
	return Line{}, false
}
```

Its only caller is `GetCursorPos` (`layout_query.go:224-247`):

```go
	if hardLineBreakLenAt(l.Text, byteIndex) == 0 {
		if ri, ok := l.CharRectByIndex[byteIndex]; ok {
			r := l.CharRects[ri].Rect
			...
			x := r.X
			if l.rectIsRTL(ri) {
				x += r.Width
			}
			if line, ok := l.lineForByteIndex(byteIndex); ok {
				return CursorPosition{
					X:      x,
					Y:      line.Rect.Y,
					Height: line.Rect.Height,
				}, true
			}
```

The convention that the rest of the file already uses, in `MoveCursorLineStart`
(`layout_query.go:399-413`):

```go
	for i, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			// At boundary: prefer next line if it starts here.
			if byteIndex == lineEnd && i+1 < len(l.Lines) &&
				l.Lines[i+1].StartIndex == lineEnd {
				continue
			}
			return line.StartIndex
		}
	}
```

`MoveCursorLineEnd`, `MoveCursorUp` and `MoveCursorDown` repeat the same "prefer
the next line" check. Copy it exactly.

**Why the line-based fallback further down in `GetCursorPos` (the
`// Fallback: find containing line.` loop) is NOT changed:** it only runs when
`byteIndex` has no CharRect, which means the end of the text, a hard line break,
or a byte that a wrap consumed. At those positions the caret correctly belongs
at the end of line k (see `TestCursorPosAtSoftWrapLineEnd` in
`layout_wrap_longword_test.go`).

Test conventions: `layout_wrap_longword_test.go` (no build tag, so it runs on
every native OS) builds real layouts through `NewContext(1.0)` +
`ctx.LayoutText` (helper `longWordLayout`, lines 10-24). Comments above each
test explain what regression it guards. Match that style.

## Commands you will need

| Purpose       | Command                                                   | Expected on success |
| ------------- | --------------------------------------------------------- | ------------------- |
| Focused tests | `go test -run 'CursorPos' .` then `go test -run 'Wrap' .` | `ok`                |
| Full tests    | `go test ./...`                                           | all `ok`            |
| WASM vet      | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .`              | exit 0              |
| Lint          | `make lint`                                               | `0 issues.`         |
| Full gate     | `make prepush`                                            | exit 0              |

## Scope

**In scope**:

- `layout_query.go` — only the `lineForByteIndex` function
- `layout_wrap_longword_test.go` — add tests
- `CHANGELOG.md` — one bullet under `## [Unreleased]` → `### Fixed`

**Out of scope**:

- The `// Fallback: find containing line.` loop and `lineEndPos` in
  `GetCursorPos`. Their end-of-line behavior is correct and tested.
- `MoveCursor*` functions. They are already correct.
- Vertical layouts (`OrientationVertical`). They have separate, known caret
  problems that are planned elsewhere.

## Git workflow

- Branch: `advisor/002-caret-at-soft-wrap-boundary`
- One commit, e.g. `fix: caret at a soft-wrap boundary draws on the next line`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing tests

Add to `layout_wrap_longword_test.go`:

1. `TestCursorPosAtCharWrapBoundaryUsesNextLine` (unit test with a literal
   layout, so no fonts are needed). Build a `Layout` with `Text: "abcdef"`, two
   lines
   `{StartIndex: 0, Length: 3, Rect: Rect{X: 0, Y: 0, Width: 30, Height: 20}}`
   and
   `{StartIndex: 3, Length: 3, Rect: Rect{X: 0, Y: 20, Width: 30, Height: 20}}`,
   six CharRects (index i at `X: float32(i%3)*10`, `Y` 0 for i<3 and 20 for i≥3,
   width 10, height 20), `CharRectByIndex` mapping 0..5 to 0..5, and seven
   `LogAttr{IsCursorPosition: true}` with `LogAttrByIndex` mapping 0..6. Use
   `testLayout()` in `test_helpers_test.go:211` as the shape example. Assert
   `GetCursorPos(3)` returns `ok`, `X == 0`, `Y == 20`, `Height == 20`. Also
   assert `GetCursorPos(6)` (end of text) still gives `Y == 20`, `X == 30`, and
   `GetCursorPos(0)` gives `Y == 0`.
2. `TestCursorPosAtCharWrapBoundaryRealLayout` (integration). Build a layout
   with
   `ctx.LayoutText(strings.Repeat("x", 60), TextConfig{Style: TextStyle{FontName: "Sans 20"}, Block: BlockStyle{Width: 120, Wrap: WrapChar}})`
   (same `NewContext(1.0)` setup as `longWordLayout`; write a small helper or
   generalize `longWordLayout` to take a `BlockStyle`). Fatal if
   `len(l.Lines) < 2`. For each line `k >= 1` where
   `Lines[k].StartIndex == Lines[k-1].StartIndex + Lines[k-1].Length`, assert
   `GetCursorPos(Lines[k].StartIndex)` has `Y == Lines[k].Rect.Y`.

**Verify**: `go test -run 'CharWrapBoundary' .` → FAIL. Both tests report `Y`
from the previous line.

### Step 2: Fix `lineForByteIndex`

Change the loop to `for i, line := range l.Lines` and add the same boundary
check that `MoveCursorLineStart` uses: when `byteIndex == lineEnd`, a next line
exists, and `l.Lines[i+1].StartIndex == byteIndex`, `continue`. Add a doc
comment: "At a soft-wrap boundary that consumed no space, the later line is
preferred, matching MoveCursorLineStart/End/Up/Down; the caret's X already comes
from that line's first character."

**Verify**: `go test -run 'CharWrapBoundary|SoftWrap|CursorPos' .` → `ok`

### Step 3: Full checks and changelog

Add under `## [Unreleased]` → `### Fixed` in `CHANGELOG.md` (create the
`### Fixed` subheading if absent, following the format of the `v1.26.5`
section): "The caret at a soft wrap that consumes no space (CJK, `WrapChar`) is
drawn at the start of the next line instead of the left edge of the line above."

**Verify**:

- `go test ./...` → all `ok`
- `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .` → exit 0
- `make lint` → `0 issues.`
- `make prepush` → exit 0

## Test plan

- 2 new tests (unit + integration) in `layout_wrap_longword_test.go`.
- The existing `TestCursorPosAtSoftWrapLineEnd`, the paragraph cursor tests
  (`layout_paragraph_cursor_test.go`) and the fuzzers in
  `layout_query_fuzz_test.go` must still pass unchanged.

## Done criteria

- [ ] `go test ./...` exits 0. Both new tests exist and pass.
- [ ] `git diff 7234b0b -- layout_query.go` changes only `lineForByteIndex`
- [ ] `make lint` prints `0 issues.`
- [ ] `git status` shows only in-scope files modified
- [ ] `plans/README.md` status row updated

## STOP conditions

- Any existing test fails after Step 2. A test that expects the old Y is a sign
  that some caller depends on "earlier line" affinity. Report its name and do
  not change its expectation.
- With `WrapChar` at width 120, the integration layout has no line pair that
  shares a boundary byte (the wrap consumed bytes, or did not wrap). Report the
  line table (`t.Logf` StartIndex/Length per line).
- `lineForByteIndex` has gained more callers than `GetCursorPos`
  (`grep -n lineForByteIndex *.go`).

## Maintenance notes

- Any new query that maps a byte to a line must use the same "prefer the next
  line at a shared boundary" rule. Five functions now repeat it. A follow-up
  could extract `lineIndexForByte(b) int` and use it in all of them. This plan
  does not do that, so the diff stays small.
- Reviewers: confirm RTL lines still take X from the trailing edge. The fix
  changes only Y and Height.
