# Plan 012: Block.Indent shrinks the wrap width and moves Line.Rect, with Pango hanging-indent semantics

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 6d6a018..HEAD -- layout_wrap_shared.go layout_wrap_shared_test.go layout_ft.go layout_wasm.go config.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (changes visible layout for every caller that sets `Indent`)
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `6d6a018`, 2026-11-13
- **Decision**: Pango indent semantics, confirmed by the maintainer on
  2026-11-13. Do not reopen it.

## Why this matters

`BlockStyle.Indent` is documented as "first-line indentation. Negative = hanging
indent" (`config.go:90-91`). Today it only shifts the glyphs of the layout's
first line:

1. **Wrapping ignores it.** A positive indent pushes line 0 past `Block.Width`
   by the indent amount. The text overflows its box.
2. **`Line.Rect.X` ignores it**, and so does `Layout.Width`. The caret,
   selection and hit-testing of an indented line are off by the indent.
3. **A negative indent draws line 0 left of the origin** (at x = indent < 0),
   outside the box, instead of a hanging indent.
4. Only the first line of the whole layout is affected, not the first line of
   each paragraph.

go-gui uses this field: `go-gui/gui/layout_pipeline.go:590-596` and
`go-gui/gui/view_rtf.go:151-158` pass `Indent: -HangingIndent` with
`Wrap: WrapWord`. go-gui documents `HangingIndent` (`view_rtf.go:25-29`) as "a
hanging indent pulls the first line left of the rest". Today go-gui's first line
is drawn left of the widget's box, and the continuation lines are not indented
at all. go-glyph began as a port of a Pango-based library, so the intended
meaning is Pango's: a **positive** indent indents the first line of each
paragraph, and a **negative** indent indents every line _except_ the first of
each paragraph by `|indent|`. After this plan both backends implement that
meaning, and the wrap width, `Line.Rect.X`, char rects and `Layout.Width` all
agree with where the glyphs are drawn.

## Current state

- `config.go:84-94` — `BlockStyle`:
  ```go
  	// Indent determines first-line indentation. Negative = hanging indent.
  	Indent float32
  ```
- `layout_wrap_shared.go` (tag
  `android || linux || darwin || windows || (js && wasm)`) — `lineInfo` and
  `wrapLines`, shared by both backends:
  ```go
  type lineInfo struct {
  	startChar, endChar int
  	width              float64
  }
  ...
  func wrapLines(dst []lineInfo, n int, mode WrapMode, wrapWidth float64,
  	width func(i int) float64, text func(i int) string,
  	canBreak []bool) []lineInfo {
  ```
  Inside the loop, a hard break appends `lineInfo{lineStart, i, lineW}` and
  starts a new line. The overflow test is
  `if wrapWidth > 0 && newW > wrapWidth && i > lineStart && mode != WrapNone {`.
  Soft wraps append `lineInfo{lineStart, lastSpace, lastSpaceW}`,
  `lineInfo{lineStart, lastBreak, lastBreakW}` or
  `lineInfo{lineStart, i, lineW}`. The final line is
  `append(dst, lineInfo{lineStart, n, lineW})`.
- Callers of `wrapLines`: `layout_ft.go:986`, `layout_wasm.go:355`,
  `layout_wrap_shared_test.go:10` (helper `wrapFixed`) and
  `layout_wrap_shared_test.go:79`.
- `layout_ft.go` (native; the same builder serves `LayoutText` and
  `LayoutRichText`). Wrap width is in pixels:
  ```go
  	wrapWidth := float64(-1)
  	if cfg.Block.Width > 0 {
  		wrapWidth = float64(cfg.Block.Width) * float64(ctx.scaleFactor)
  	}
  	lines := wrapLines(nil, len(chars), cfg.Block.Wrap, wrapWidth,
  		func(i int) float64 { return chars[i].width }, charText, canBreakBefore)
  ```
  The per-line loop (around lines 1046-1080):
  ```go
  		linePixelW := li.width
  		var alignOffset float64
  		if wrapWidth > 0 {
  			switch cfg.Block.Align {
  			case AlignCenter:
  				alignOffset = (wrapWidth - linePixelW) / 2
  			case AlignRight:
  				alignOffset = wrapWidth - linePixelW
  			}
  		}

  		indentPx := float64(0)
  		if lineIdx == 0 && cfg.Block.Indent != 0 {
  			indentPx = float64(cfg.Block.Indent) *
  				float64(ctx.scaleFactor)
  		}
  		...
  		cx := alignOffset + indentPx
  ```
  Glyphs and `CharRects` are placed from `cx`, so they already include the
  indent. The `Line` is built later (around lines 1290-1302) without it:
  ```go
  			Rect: Rect{
  				X:      float32(alignOffset * pixelScale),
  				Y:      float32(lineY * pixelScale),
  				Width:  float32(linePixelW * pixelScale),
  				Height: float32(lineHeight * pixelScale),
  			},
  		})

  		totalWidth = max(totalWidth, linePixelW)
  ```
  The same loop already detects a paragraph start for bidi:
  `if (lineIdx == 0 || isHardLineBreak(chars[li.startChar-1].text)) && ...`.
- `layout_wasm.go` (`js && wasm`) — same structure without `scaleFactor`:
  `wrapLines` at line 355, `indentPx` at lines 396-399
  (`if lineIdx == 0 && cfg.Block.Indent != 0 { indentPx = float64(cfg.Block.Indent) }`),
  `cx := alignOffset + indentPx`, `Rect{X: float32(alignOffset), ...}` near line
  540, `totalWidth = max(totalWidth, linePixelW)` at line 547.
- The vertical layout (`buildVerticalLayout`, `layout_ft.go:1357`) ignores
  `Indent`. Leave it.
- No test sets `Indent` today (`grep -rn Indent *_test.go` is empty).
- Test exemplars: pure wrap tests in `layout_wrap_shared_test.go` (`wrapFixed`
  gives every cluster width 10, see
  `TestWrapLines_WordWrapWidthExcludesNextWord`). Native layout tests in
  `layout_linebreak_ft_test.go` (`//go:build linux || darwin || windows`,
  `NewContext(1.0)`, `defer ctx.Free()`).

## Commands you will need

| Purpose     | Command                                                             | Expected on success |
| ----------- | ------------------------------------------------------------------- | ------------------- |
| Wrap tests  | `go test -run 'TestWrapLines' -count=1 .`                           | `ok`                |
| Indent      | `go test -run 'Indent' -count=1 -v .`                               | all `PASS`          |
| WASM vet    | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...`                    | exit 0              |
| Alloc guard | `go test -run '^$' -bench 'BenchmarkTerminalFrame1500' -benchmem .` | `0 allocs/op`       |
| Full gate   | `make prepush`                                                      | exit 0              |

## Scope

**In scope**:

- `layout_wrap_shared.go` — `lineInfo` gets a `paraStart bool` field.
  `wrapLines` gets an `indent float64` parameter. Add a new helper `lineIndent`.
- `layout_wrap_shared_test.go` — update the two call sites and add tests
- `layout_ft.go`, `layout_wasm.go` — pass the indent, use `li.paraStart`, and
  fix `Line.Rect.X` and `totalWidth`
- `layout_indent_ft_test.go` (create; `//go:build linux || darwin || windows`)
- `config.go` — the `Indent` doc comment only
- `CHANGELOG.md` — one bullet under `### Fixed`

**Out of scope**:

- `buildVerticalLayout`. Vertical text ignores `Indent`, and still will.
- go-gui. Do not edit `../go-gui`. See Maintenance notes for the follow-up
  check.
- `Block.Tabs` (does nothing; a separate finding).
- Changing how the layout cache key hashes `Indent` (`glyph.go:417`). It already
  includes it.

## Git workflow

- Branch: `advisor/012-block-indent-geometry`
- One commit, e.g.
  `fix(layout): Block.Indent shrinks wrap width and moves line rects; negative is a hanging indent`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Pure wrap tests (failing)

In `layout_wrap_shared_test.go`, change `wrapFixed` to take an `indent float64`
argument (after `wrapWidth`) and pass it to `wrapLines`. Pass `0` at every
existing `wrapFixed` call and at the direct call on line 79. Then add
`TestWrapLines_Indent` as a table test over the text `"a b c d"` (7 one-byte
clusters, each 10 wide), `WrapWord`, wrap width 45, `canBreak` nil:

| indent | want lines `{startChar, endChar, paraStart}` |
| ------ | -------------------------------------------- |
| 0      | `{0,3,true} {4,7,false}`                     |
| +20    | `{0,1,true} {2,5,false} {6,7,false}`         |
| -20    | `{0,3,true} {4,5,false} {6,7,false}`         |

These come from these rules: the first line of a paragraph may use
`45 - max(indent, 0)`, and every other line may use `45 - max(-indent, 0)`. Add
one hard-break case: text `"a b\nc d"` with indent +20 must give
`paraStart == true` for the line that starts at cluster 4 (after `\n`).

**Verify**: `go test -run 'TestWrapLines' -count=1 .` → the build fails
(`wrapLines` has no indent parameter, and `lineInfo` has no `paraStart`). That
counts as failing.

### Step 2: Indent-aware `wrapLines`

In `layout_wrap_shared.go`:

1. Add `paraStart bool` to `lineInfo`, with a comment: true when the line is the
   first of its paragraph (the layout's first line, or the line right after a
   hard break).
2. Add:
   ```go
   // lineIndent is the start offset of a line under BlockStyle.Indent, with
   // Pango's meaning: a positive indent moves the first line of each
   // paragraph right; a negative one moves every other line right by
   // -indent (a hanging indent). The line's wrap width shrinks by the same
   // amount.
   func lineIndent(indent float64, paraStart bool) float64 {
   	switch {
   	case indent > 0 && paraStart:
   		return indent
   	case indent < 0 && !paraStart:
   		return -indent
   	}
   	return 0
   }
   ```
3. Change the signature to
   `wrapLines(dst []lineInfo, n int, mode WrapMode, wrapWidth, indent float64, width ..., text ..., canBreak []bool)`,
   and update its doc comment.
4. Track `paraStart := true` for the current line. Set it to `true` after a hard
   break, and to `false` after every soft wrap (all three soft-wrap `append`s,
   and the `restart` paths). Every `append` records the current line's
   `paraStart`.
5. Replace `newW > wrapWidth` in the overflow test with
   `newW > wrapWidth-lineIndent(indent, paraStart)`. Keep `i > lineStart` as is.
   It already guarantees at least one cluster per line, even when the indent is
   as large as the wrap width.

**Verify**: `go test -run 'TestWrapLines' -count=1 .` → the build still fails,
in `layout_ft.go` / `layout_wasm.go` callers only. Fix those in Step 3.

### Step 3: Use it in both backends

In `layout_ft.go`:

- Pass `float64(cfg.Block.Indent) * float64(ctx.scaleFactor)` as `indent` to
  `wrapLines`. Store it in a local `indentUnits` so the line loop uses the same
  value.
- Replace the `indentPx` block with
  `indentPx := lineIndent(indentUnits, li.paraStart)`.
- Change alignment so it works inside the available width. When `wrapWidth > 0`,
  compute `avail := wrapWidth - indentPx`, then center
  `alignOffset = (avail - linePixelW) / 2` and right
  `alignOffset = avail - linePixelW`. Keep `cx := alignOffset + indentPx`.
- `Line.Rect.X`: `float32((alignOffset + indentPx) * pixelScale)`.
- `totalWidth = max(totalWidth, indentPx+linePixelW)`.

In `layout_wasm.go` make the same changes, with
`indentUnits := float64(cfg.Block.Indent)` (no scale factor, matching how wasm's
`wrapWidth` is computed).

**Verify**: `go test -run 'TestWrapLines' -count=1 .` → `ok`. Then
`GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` → exit 0. Then `go test ./...`
→ all `ok`. Existing tests do not set `Indent`, so their output must not change.
If any existing test fails, that is a STOP condition.

### Step 4: Native layout tests

Create `layout_indent_ft_test.go` (`//go:build linux || darwin || windows`). Use
`NewContext(1.0)` and the text
`"alpha beta gamma delta epsilon zeta eta theta iota kappa"` with
`TextConfig{Style: TextStyle{FontName: "Sans 16"}, Block: BlockStyle{Wrap: WrapWord, Width: 200, Indent: ind}}`.
Write these tests (allow `1e-3` float slack):

- `TestLayoutIndentPositive` (`ind = 30`): `len(l.Lines) >= 2`.
  `l.Lines[0].Rect.X == 30`, and every later line has `Rect.X == 0`. For every
  line, `Rect.X + Rect.Width <= 200`. `l.Width <= 200`. The first `CharRect` of
  line 0 has `X >= 30`.
- `TestLayoutIndentHanging` (`ind = -30`): `l.Lines[0].Rect.X == 0`. Every later
  line has `Rect.X == 30`. For every line, `Rect.X + Rect.Width <= 200`. No
  `CharRect` has `X < 0`.
- `TestLayoutIndentCaretFollowsLine` (`ind = -30`):
  `l.GetCursorPos(l.Lines[1].StartIndex)` returns `X` within 1 px of 30.
- `TestLayoutIndentPerParagraph` (`ind = 30`, text `"one\ntwo"`, no width): both
  lines have `Rect.X == 30`.
- `TestLayoutIndentZeroUnchanged`: with `ind = 0`, `Lines` and `Width` equal the
  same layout built without setting `Indent` (`reflect.DeepEqual` on `l.Lines`).

**Verify**: `go test -run 'Indent' -count=1 -v .` → all `PASS`.

### Step 5: Doc comment, alloc guard, changelog, gate

- `config.go`: replace the `Indent` comment with:
  ```go
  	// Indent offsets lines from the block's left edge, in points, with
  	// Pango's meaning. Positive: the first line of each paragraph starts
  	// Indent to the right. Negative: every line except the first of each
  	// paragraph starts -Indent to the right (a hanging indent). The offset
  	// line's wrap width shrinks by the same amount. Vertical layouts ignore it.
  	Indent float32
  ```
- `CHANGELOG.md` `### Fixed`: "**`Block.Indent` geometry.** Indented lines now
  wrap inside `Block.Width`, and `Line.Rect.X`, the caret and `Layout.Width`
  include the indent. A negative indent is now a real hanging indent: the first
  line of each paragraph starts at 0 and the following lines start at `-Indent`.
  Before, the first line was drawn left of the box. A positive indent now
  applies to the first line of every paragraph, not only the layout's first
  line."

**Verify**:
`go test -run '^$' -bench 'BenchmarkTerminalFrame1500' -benchmem -count=1 .` →
`0 allocs/op`. `make prepush` → exit 0.

## Test plan

- `TestWrapLines_Indent` (pure): exact line splits and `paraStart` for indent 0,
  +20, -20, and a hard-break paragraph.
- `layout_indent_ft_test.go` (native): line rects stay inside `Block.Width`,
  rect X matches the indent rule, no glyph sits left of 0, the caret follows the
  line, each paragraph is indented, and indent 0 is unchanged.
- WASM: compile check (`go vet` for js/wasm). There is no wasm test runner in CI
  for this path, so the shared `wrapLines` tests carry the logic.

## Done criteria

- [ ] `go test -run 'TestWrapLines|Indent' -count=1 .` → `ok`
- [ ] `grep -n 'lineIdx == 0 && cfg.Block.Indent' layout_ft.go layout_wasm.go`
      prints nothing
- [ ] `grep -c 'lineIndent(' layout_ft.go layout_wasm.go layout_wrap_shared.go`
      → each at least 1
- [ ] `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` exits 0
- [ ] `BenchmarkTerminalFrame1500` reports `0 allocs/op`
- [ ] `make prepush` exits 0
- [ ] Only in-scope files changed (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- The Step 1 expected splits do not match what the rules produce, and you can
  show the rules (not your code) disagree with the table. Report your trace. Do
  not edit the table to fit your code.
- Any existing test changes output in Step 3. Callers that never set `Indent`
  must be byte-for-byte unchanged.
- `Line.Rect` is built in more places than the two noted (for example, a
  rich-text or inline-object path that sets `Rect.X` separately). Report where.
  The fix must cover all horizontal paths, or none.
- `BenchmarkTerminalFrame1500` shows allocations above 0.

## Maintenance notes

- **go-gui follow-up (needed after release):** go-gui's RTF view passes
  `Indent: -HangingIndent`. After this lands, its continuation lines move right
  by `HangingIndent` and wrap narrower. That matches go-gui's own doc comment,
  but go-gui's RTF layout tests and any golden images may need updating. Run
  `go test ./...` in go-gui against this branch (with a temporary `go.work`)
  before tagging a go-glyph release.
- A reviewer should check the alignment change: for `AlignCenter` /
  `AlignRight`, the indent narrows the available width. It is not added on top
  of a full-width alignment offset.
- `IsParagraphStart` on `Line` (set in `layout_ft.go`) and `lineInfo.paraStart`
  now say the same thing. A later cleanup could derive one from the other.
