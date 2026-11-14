# Plan 017: Caret, selection and click-to-offset work on vertical layouts

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 4154f31..HEAD -- layout_types.go layout_query.go layout_ft.go layout_wasm.go draw_composition.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MEDIUM (adds a public struct field; caret drawing in consumers must
  read it)
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `4154f31`, 2026-11-14
- **API decision**: the maintainer approved the `CursorPosition.Width` design on
  2026-11-14

## Why this matters

`TextConfig.Orientation = OrientationVertical` lays text out as one column, top
to bottom. All the query functions assume horizontal lines, so on a vertical
layout:

- `GetCursorPos` returns a caret as tall as the whole column, at the column's
  top, for every index.
- `GetSelectionRects` returns one rectangle over the whole column, whatever the
  selection.
- `GetClosestOffset` (click to caret) returns 0 for any click.

Measured on 2026-11-14, native, `"漢字テスト"` at `Sans 20` (5 chars, each
CharRect `{X:0, Y:23·i, W:23, H:23}`):

```
GetCursorPos(3)          = {X:0  Y:0 Height:130.4}   want Y:23, horizontal bar
GetCursorPos(end)        = {X:23 Y:0 Height:130.4}   want Y:115
GetSelectionRects(0, 6)  = [{X:0 Y:0 W:23 H:130.4}]  want H:46 (2 chars)
GetClosestOffset(5, 50)  = 0                         want 6 (3rd char)
```

`HitTest` and `GetCharRect` already work. The per-char rects are correct.

A vertical caret needs to be a horizontal bar. `CursorPosition` has only `X`,
`Y` and `Height`, so this plan adds a `Width` field. `Width == 0` means what it
means today: a vertical bar at `X`, `Height` tall. `Width > 0` (only on vertical
layouts) means a horizontal bar at `Y`, `Width` wide, with `Height == 0`.

## Current state

`layout_types.go:38-43`:

```go
// CursorPosition represents the geometry for rendering a text cursor.
type CursorPosition struct {
	X      float32
	Y      float32
	Height float32
}
```

`layout_types.go:11-36` is the `Layout` struct. It has unexported fields after
`LogAttrs`, for example:

```go
	// charRTL[i] reports whether CharRects[i] is in a right-to-left bidi
	// run. nil when the layout has no RTL run (and on backends without
	// bidi), which means every char is LTR.
	charRTL []bool
```

There is only one `Layout` definition (the file has no build tag), and it is
shared by native and WASM.

Both vertical builders end with a `Layout{...}` literal: `layout_ft.go:1476`
(native `buildVerticalLayout`, starts at line 1356) and `layout_wasm.go:715`
(WASM `buildVerticalLayout`, starts at line 586). Each makes exactly one `Line`
that covers the whole text and the whole column:

```go
	lines := []Line{{
		StartIndex: 0,
		Length:     len(text),
		Rect: Rect{
			X: 0, Y: 0,
			Width:  float32(lineHeight * pixelScale),
			Height: float32(totalH * pixelScale),
		},
		IsParagraphStart: true,
	}}
```

The vertical builder appends one CharRect per grapheme cluster, in text order
(so `CharRects` is sorted by `Index`). It **skips** hard line breaks
(`if isHardLineBreak(cl.text) { continue }`), so a `\n` byte has no CharRect but
is still a cursor position (`appendHardLineBreakAttrs`). The end of text
(`len(text)`) is a cursor position with no CharRect.

`layout_query.go` (no build tag, shared). The functions to change:

- `GetClosestOffset` at line 120. It picks the closest line by Y, then calls
  `l.closestInLine(line, x)` (line 630), which compares X only.
- `appendSelectionRects` at line 158. Per line, it builds pieces from
  `minX`/`maxX` of the selected CharRects and uses `line.Rect.Y` and
  `line.Rect.Height` for every piece.
- `GetCursorPos` at line 199. After the validity block (lines 204-223), it takes
  the CharRect's X but `line.Rect.Y` and `line.Rect.Height` (lines 236-241).

`closestInLine` (lines 630-677) shows the caret-stop rule to copy:

```go
	haveStops := len(l.GetValidCursorPositions()) > 0
	isStop := func(byteIdx int) bool {
		return !haveStops || l.isCursorStop(byteIdx)
	}
```

`l.cursorPositions` is the sorted list of caret stops (built by
`buildPositionCaches`). It includes `len(text)`.

`draw_composition.go:66-73` draws the IME caret:

```go
	// Draw cursor at insertion point within preedit.
	if cp, ok := layout.GetCursorPos(cs.DocumentCursorPos()); ok {
		r.emitFillRect(Rect{
			X:      cp.X,
			Y:      cp.Y,
			Width:  2.0,
			Height: cp.Height,
		}, dimmed, x, y, combined, isIdentity)
	}
```

go-gui (`go-gui/gui/text_block_query.go`) builds `glyph.CursorPosition` with
named fields only. A new field does not break it. go-gui does not use
`OrientationVertical`.

Test conventions: `layout_review_ft_test.go` (tag
`android || linux || darwin || windows`) has `newReviewContext(t)` and
`reviewCfg` (`Sans 16`). `TestLayoutRichText_VerticalKeepsRunStyles` at line 255
is a vertical-layout test to copy. `recordingBackend` (`test_helpers_test.go`,
untagged) records fills in
`filledRects []filledRectCall{Dst Rect; Color Color}`.

Comment style: plain, short, present tense, no "we", wrapped at about 90
columns.

## Commands you will need

| Purpose      | Command                                                         | Expected on success |
| ------------ | --------------------------------------------------------------- | ------------------- |
| New tests    | `go test -run 'Vertical' -count=1 .`                            | `ok`                |
| Query tests  | `go test -run 'Cursor\|Selection\|Closest\|HitTest' -count=1 .` | `ok`                |
| Composition  | `go test -run 'DrawComposition' -count=1 .`                     | `ok`                |
| WASM compile | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .`                    | exit 0              |
| Alloc guard  | `go test -run 'NoAlloc\|NoAllocs\|Allocs' -count=1 ./...`       | all `ok`            |
| Full gate    | `make prepush`                                                  | exit 0              |

## Scope

**In scope**:

- `layout_types.go`: add `Width` to `CursorPosition` and add an unexported
  `vertical bool` to `Layout`.
- `layout_ft.go` and `layout_wasm.go`: set `vertical: true` in the `Layout`
  literal of `buildVerticalLayout` only.
- `layout_query.go`: vertical branches in `GetCursorPos`, `appendSelectionRects`
  and `GetClosestOffset`, plus three new helper methods.
- `draw_composition.go`: draw a horizontal caret when `cp.Width > 0`.
- `layout_vertical_query_test.go` (create).
- `CHANGELOG.md`: one bullet under `### Fixed` and one under `### Added`.

**Out of scope**:

- `MoveCursorUp`/`MoveCursorDown`. A vertical layout has one column, so up/down
  have nothing to move to. Callers map arrow keys to `MoveCursorLeft`/`Right`.
- Multi-column vertical text (hard breaks that start a new column). The builder
  does not do it today.
- The composition underline in `DrawComposition` (it is drawn under each
  selection rect, which is wrong for vertical text). That is a separate change.
- The column's `Height` including one extra font ascent past the last char
  (`totalH := penY`, and `penY` starts at `fontAscent`). Do not change layout
  geometry in this plan.
- Every horizontal code path. Horizontal layouts must give the exact same
  results as before, with `Width == 0`.

## Git workflow

- Branch: `advisor/017-vertical-caret-selection`
- One commit, e.g.
  `fix(layout): caret, selection and closest offset follow the column on vertical layouts`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing tests

Create `layout_vertical_query_test.go`:

```go
//go:build android || linux || darwin || windows

package glyph

import "testing"
```

Add a helper `verticalLayout(t, text)`. It calls
`newReviewContext(t).LayoutText(text, cfg)`, where `cfg := reviewCfg` and
`cfg.Orientation = OrientationVertical`. It fails the test on error and returns
the layout. Then add these tests. Compute every expected value from
`l.CharRects` and `l.Lines[0].Rect`. Do not hard-code pixel values, because font
metrics differ per OS.

1. `TestVerticalCursorPosIsHorizontalBar`, text `"abcde"`. For each CharRect
   `cr`, `GetCursorPos(cr.Index)` returns `ok == true`, `X == col.X`,
   `Y == cr.Rect.Y`, `Width == col.Width` and `Height == 0`. Here
   `col := l.Lines[0].Rect`. `GetCursorPos(len(text))` returns
   `Y == last.Rect.Y + last.Rect.Height`, where `last` is the last CharRect.
2. `TestVerticalCursorPosAtHardBreak`, text `"ab\ncd"`. `GetCursorPos(2)` (the
   `\n`, which has no CharRect) returns `ok == true` and `Y` equal to the Y of
   the CharRect with `Index == 3`.
3. `TestVerticalSelectionRects`, text `"abcde"`. `GetSelectionRects(1, 3)`
   returns exactly 1 rect: `X == col.X`, `Width == col.Width`,
   `Y == CharRects[1].Rect.Y`, and
   `Y+Height == CharRects[2].Rect.Y + CharRects[2].Rect.Height`.
   `GetSelectionRects(0, len(text))` returns 1 rect from the first char's top to
   the last char's bottom.
4. `TestVerticalClosestOffsetUsesY`, text `"abcde"`. For each CharRect `cr` at
   position i: `GetClosestOffset(col.X+1, cr.Rect.Y+1) == cr.Index`, and
   `GetClosestOffset(col.X+1, cr.Rect.Y+cr.Rect.Height-1)` equals the next
   CharRect's Index, or `len(text)` for the last one.
   `GetClosestOffset(col.X+1, -50) == 0`.
   `GetClosestOffset(col.X+1, l.Height+50) == len(text)`.
5. `TestHorizontalCursorPosWidthZero`: a horizontal `"abc"` layout
   (`reviewCfg`). `GetCursorPos(1)` has `Width == 0` and `Height > 0`.
6. `TestDrawCompositionVerticalCaret`: build `l := verticalLayout(t, "abcde")`.
   Make `r, err := NewRenderer(newRecordingBackend(), 1.0)` (fatal on error,
   `defer r.Free()`), and keep the backend in a variable. Then
   `cs := NewCompositionState(); cs.Start(0); cs.SetMarkedText("ab", 1)` and
   `r.DrawComposition(l, 0, 0, &cs, Color{0, 0, 0, 255})`. The caret is at
   document index 1 (`Start(0)` + preedit cursor 1). Assert that
   `backend.filledRects` has a `Dst` exactly equal to
   `Rect{X: col.X, Y: l.CharRects[1].Rect.Y, Width: col.Width, Height: 2}`, and
   none with `Height == l.Height`. Match the exact `Y`: the clause underline is
   also a full-width rect, but its `Y` is at the bottom of a char, not the top.

**Verify**: `go test -run 'Vertical|HorizontalCursorPosWidthZero' -count=1 .` →
compile error (`Width` does not exist on `CursorPosition`). That is the expected
failure for now. After Step 2 adds the field, tests 1-4 and 6 must FAIL and test
5 must PASS. If tests 1-4 pass after Step 2, STOP.

### Step 2: Add the fields

In `layout_types.go`:

```go
// CursorPosition represents the geometry for rendering a text cursor.
//
// On a horizontal layout, Width is 0: the caret is a vertical bar at X
// from Y to Y+Height. On a vertical layout (OrientationVertical), the
// caret is a horizontal bar at Y from X to X+Width, and Height is 0.
type CursorPosition struct {
	X      float32
	Y      float32
	Height float32
	Width  float32
}
```

In `Layout`, after `charRTL`, add:

```go
	// vertical is set by buildVerticalLayout. The layout is one column,
	// top to bottom, so caret, selection and closest-offset queries use
	// the Y axis.
	vertical bool
```

In both `buildVerticalLayout` result literals (`layout_ft.go` and
`layout_wasm.go`), add `vertical: true,`.

**Verify**: `go build ./... && GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .` →
exit 0. `go test -run 'Vertical|HorizontalCursorPosWidthZero' -count=1 .` →
tests 1-4 and 6 FAIL, test 5 PASSES.

### Step 3: Vertical caret

In `layout_query.go`, add this method next to `lineEndPos`:

```go
// verticalCursorPos returns the caret on a vertical layout: a horizontal
// bar across the column at the top of the char at byteIndex. A byte with
// no CharRect (a hard line break, which the vertical builder skips, or the
// end of the text) gets the top of the next char, or the bottom of the
// last char.
func (l *Layout) verticalCursorPos(byteIndex int) CursorPosition {
	col := Rect{Width: l.Width}
	if len(l.Lines) > 0 {
		col = l.Lines[0].Rect
	}
	at := func(y float32) CursorPosition {
		return CursorPosition{X: col.X, Y: y, Width: col.Width}
	}
	if ri, ok := l.CharRectByIndex[byteIndex]; ok {
		return at(l.CharRects[ri].Rect.Y)
	}
	for _, cr := range l.CharRects {
		if cr.Index > byteIndex {
			return at(cr.Rect.Y)
		}
	}
	if n := len(l.CharRects); n > 0 {
		last := l.CharRects[n-1].Rect
		return at(last.Y + last.Height)
	}
	return at(col.Y)
}
```

In `GetCursorPos`, insert this directly after the `if !valid { ... }` block
(after line 223), before the "Try exact char rect" comment:

```go
	if l.vertical {
		return l.verticalCursorPos(byteIndex), true
	}
```

**Verify**:
`go test -run 'TestVerticalCursorPos|HorizontalCursorPosWidthZero' -count=1 .` →
`ok`.

### Step 4: Vertical selection

Add:

```go
// appendVerticalSelectionRects is appendSelectionRects for a vertical
// layout: one rect across the column per run of selected chars, from the
// first char's top to the last char's bottom. CharRects are in text order
// on a vertical layout.
func (l *Layout) appendVerticalSelectionRects(rects []Rect, start, end int) []Rect {
	col := l.Lines[0].Rect
	in := false
	var minY, maxY float32
	for _, cr := range l.CharRects {
		if cr.Index < start || cr.Index >= end {
			if in {
				rects = append(rects, Rect{X: col.X, Y: minY, Width: col.Width,
					Height: maxY - minY})
				in = false
			}
			continue
		}
		bottom := cr.Rect.Y + cr.Rect.Height
		if !in {
			minY, maxY, in = cr.Rect.Y, bottom, true
			continue
		}
		maxY = max(maxY, bottom)
	}
	if in {
		rects = append(rects, Rect{X: col.X, Y: minY, Width: col.Width,
			Height: maxY - minY})
	}
	return rects
}
```

In `appendSelectionRects`, directly after the
`if start >= end || len(l.Lines) == 0 { return rects }` check, add:

```go
	if l.vertical {
		return l.appendVerticalSelectionRects(rects, start, end)
	}
```

**Verify**: `go test -run 'TestVerticalSelectionRects' -count=1 .` → `ok`.

### Step 5: Vertical closest offset

Add:

```go
// closestInColumn is GetClosestOffset for a vertical layout. It finds the
// char nearest to y, then returns its index, or the next caret stop when y
// is in the char's lower half.
func (l *Layout) closestInColumn(y float32) int {
	haveStops := len(l.cursorPositions) > 0
	best, bestDist := -1, maxDistance
	for ri, cr := range l.CharRects {
		if haveStops && !l.isCursorStop(cr.Index) {
			continue
		}
		var dist float32
		switch top, bottom := cr.Rect.Y, cr.Rect.Y+cr.Rect.Height; {
		case y < top:
			dist = top - y
		case y > bottom:
			dist = y - bottom
		}
		if dist < bestDist {
			best, bestDist = ri, dist
		}
	}
	if best < 0 {
		return 0
	}
	cr := l.CharRects[best]
	if y <= cr.Rect.Y+cr.Rect.Height/2 {
		return cr.Index
	}
	// The next caret stop after this char. cursorPositions is sorted.
	if i := sort.SearchInts(l.cursorPositions, cr.Index+1); i < len(l.cursorPositions) {
		return l.cursorPositions[i]
	}
	return len(l.Text)
}
```

`sort` is already imported in `layout_query.go` (used by `lineRectRange`). In
`GetClosestOffset`, directly after the `if len(l.Lines) == 0 { return 0 }`
check, add:

```go
	if l.vertical {
		return l.closestInColumn(y)
	}
```

**Verify**: `go test -run 'TestVerticalClosestOffsetUsesY' -count=1 .` → `ok`.

### Step 6: Horizontal IME caret for vertical text

In `draw_composition.go`, replace the caret block with:

```go
	// Draw cursor at insertion point within preedit. A vertical layout
	// gives a caret with Width > 0: a horizontal bar across the column.
	if cp, ok := layout.GetCursorPos(cs.DocumentCursorPos()); ok {
		caret := Rect{X: cp.X, Y: cp.Y, Width: 2.0, Height: cp.Height}
		if cp.Width > 0 {
			caret = Rect{X: cp.X, Y: cp.Y, Width: cp.Width, Height: 2.0}
		}
		r.emitFillRect(caret, dimmed, x, y, combined, isIdentity)
	}
```

**Verify**: `go test -run 'DrawComposition' -count=1 .` → `ok` (this includes
`TestDrawCompositionNoAllocs` and the new vertical test).

### Step 7: Regression sweep, changelog, gate

Add to `CHANGELOG.md` under `## [Unreleased]`, matching the nearby bullets and
wrapped at 80 columns:

- `### Fixed`: "**Caret, selection and click-to-caret work on vertical
  layouts.** On `OrientationVertical`, `GetCursorPos` returned a caret as tall
  as the whole column, `GetSelectionRects` covered the whole column, and
  `GetClosestOffset` returned 0. They now follow the chars in the column."
- `### Added`: "**`CursorPosition.Width`.** It is 0 on horizontal layouts. On a
  vertical layout the caret is a horizontal bar `Width` wide at `Y`, with
  `Height` 0. Code that draws its own caret must check `Width`."

**Verify**:

- `go test -run 'Cursor|Selection|Closest|HitTest|Vertical' -count=1 .` → `ok`
- `go test -run 'NoAlloc|NoAllocs|Allocs' -count=1 ./...` → all `ok`
- `make prepush` → exit 0

## Test plan

The six tests in Step 1. Model them on
`TestLayoutRichText_VerticalKeepsRunStyles` (`layout_review_ft_test.go:255`).
They use `"abcde"` and `"ab\ncd"` so that no CJK font is needed. The vertical
builder makes square cells of `lineHeight` for any glyph. Existing horizontal
query tests (`layout_review_*_test.go`, `layout_paragraph_cursor_test.go`,
`layout_wrap_longword_test.go`) must keep passing unchanged. They prove that
`Width == 0` and that the horizontal geometry did not change.

## Done criteria

- [ ] `go test -run 'Vertical|HorizontalCursorPosWidthZero' -count=1 -v . 2>&1 | grep -c '^--- PASS'`
      prints at least `6`, and nothing prints `FAIL`
- [ ] `grep -c 'vertical: *true' layout_ft.go layout_wasm.go` → `1` and `1`
- [ ] `grep -n 'Width  float32\|Width float32' layout_types.go` shows the new
      `CursorPosition` field
- [ ] `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet .` exits 0
- [ ] `go test -run 'NoAlloc|NoAllocs|Allocs' -count=1 ./...` → all `ok`
- [ ] `make prepush` exits 0
- [ ] Only in-scope files changed (`git status`, plus `plans/README.md`)

## STOP conditions

- After Step 2, tests 1-4 pass. That would mean vertical queries already work on
  this commit. Report the output.
- A horizontal query test that passed before this plan fails. The vertical
  branches must not touch horizontal layouts. Report the test name and diff.
- `buildVerticalLayout` makes more than one `Line` (drift). The helpers assume
  one column. Report it.
- `TestDrawCompositionNoAllocs` fails after Step 6.

## Maintenance notes

- `CursorPosition.Width > 0` is now the signal for "horizontal caret". go-gui's
  caret drawing (`go-gui/gui/render_text.go`, near line 589) only reads
  `Height`. If go-gui adds vertical text, it must read `Width`. Say this in the
  PR text.
- If multi-column vertical text is added later (hard breaks that start a new
  column), the helpers must find the column for each char, like the horizontal
  code finds the line. `MoveCursorUp/Down` would then move between columns.
- The composition underline on vertical text is still drawn under each selection
  rect. Fix it in the same place if vertical IME input matters.
