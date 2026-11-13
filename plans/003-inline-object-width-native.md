# Plan 003: Inline objects keep their declared width on the native backend

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- layout_ft.go layout_review_ft_test.go` If
> any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug (backend parity)
- **Planned at**: commit `7234b0b`, 2026-11-13

## Why this matters

`LayoutRichText` supports inline objects, such as an image or a widget embedded
in text. The caller sets `StyleRun.Style.Object = &InlineObject{Width: …}` on a
placeholder run (usually `"\uFFFC"`). The object cluster must advance by
`InlineObject.Width`. On the native (go-text) backend, it only gets that width
when it is the **first cluster of a shaping run**. If the object run comes right
after text that uses the same font face and size (the normal case, because
object runs usually inherit the base font), the run-extension loop pulls the
object into the text's shaping run. Its width is then reset to the HarfBuzz
advance of the placeholder glyph. Text after the object then overlaps the
embedded element. The wasm backend measures each cluster on its own, so it is
correct, and the two backends disagree. The project requires the same behavior
on every backend.

## Current state

- `layout_ft.go` — native layout build (build tag
  `android || linux || darwin || windows`). Function `buildLayout` measures
  clusters around lines 835-930.
- `layout_wasm.go` — the wasm equivalent, which is correct. Do not change it.

Cluster records get their object flag at `layout_ft.go:812-823`:

```go
			if ov.style.Object != nil {
				isObject = true
				objWidth = float64(ov.style.Object.Width) * float64(ctx.scaleFactor)
			}
		}
		chars = append(chars, charInfo{
			text: cl.text, byteI: cl.byteI, byteL: cl.byteL,
			yShift: yShift, xPad: xPad, isColor: isColor,
			isObject: isObject, objWidth: objWidth,
			styleKey: styleKey,
		})
```

The measuring loop, `layout_ft.go:837-886`:

```go
	for ri := 0; ri < len(chars); {
		ch := chars[ri]
		if isHardLineBreak(ch.text) {
			chars[ri].width = ch.xPad * scale
			ri++
			continue
		}
		if ch.isObject {
			chars[ri].width = ch.objWidth + ch.xPad*scale
			ri++
			continue
		}
		if ch.isColor {
			...
		}
		f := charFonts[ri]
		rj := ri + 1
		// A run is one font at one size. ...
		for rj < len(chars) && !chars[rj].isColor &&
			!isHardLineBreak(chars[rj].text) &&
			charFonts[rj].face == f.face && charFonts[rj].size == f.size {
			rj++
		}
```

The bug: the `for rj` condition stops at color clusters and hard breaks but
**not** at object clusters. Later in the same pass (`layout_ft.go:~905`), every
cluster in `[ri, rj)` gets `chars[k].width = chars[k].xPad * scale` and then the
HarfBuzz advances, so the object's `objWidth` is lost.

Trigger condition (corrected during review): the run font must **cover** the
placeholder. `applyRichScriptFallbacks` (`layout_ft.go:642-677`) gives an
uncovered cluster a script-fallback face, and the face check then ends the run
already. On macOS "Sans 16" does not cover U+FFFC, so a `"￼"` test passes before
the fix there. Fonts that cover U+FFFC (DejaVu Sans on Linux is one) hit the
bug, as does any covered placeholder text such as `"x"`. The regression test
uses `"x"`.

Object runs get their font from `ctx.createFTFont(merged)` (`layout_ft.go:479`),
where `merged = mergeStyles(cfg.Style, run.Style)`. Faces come from a shared
cache, so a same-name, same-size object run has the same `face` pointer as the
text before it.

Wasm reference behavior, `layout_wasm.go:240-247`: an object cluster takes
`w = objWidth` directly.

Test conventions: `layout_review_ft_test.go` (same native build tag) uses
`newReviewContext(t)` and
`reviewCfg = TextConfig{Style: TextStyle{FontName: "Sans 16"}}` (lines 13-23).
`TestLayoutRichText_EmptyRun` (line 75) is the pattern for a rich-text test.
`Layout.CharRects` holds per-cluster rects, and `l.GetCharRect(byteIndex)`
returns `(Rect, bool)`. Items carry `IsObject`/`ObjectID`
(`layout_types.go:124`, `:73`).

## Commands you will need

| Purpose       | Command                                        | Expected on success |
| ------------- | ---------------------------------------------- | ------------------- |
| Focused tests | `go test -run 'InlineObject' .`                | `ok`                |
| Full tests    | `go test ./...`                                | all `ok`            |
| WASM build    | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go build .` | exit 0              |
| Lint          | `make lint`                                    | `0 issues.`         |
| Full gate     | `make prepush`                                 | exit 0              |

## Scope

**In scope**:

- `layout_ft.go` — only the `for rj` run-extension condition and its comment
- `layout_review_ft_test.go` — new tests
- `CHANGELOG.md` — one bullet under `## [Unreleased]` → `### Fixed`

**Out of scope**:

- `layout_wasm.go`. It is already correct. (Note: wasm ignores an object with
  `Width <= 0`, and native gives it width 0. Do not try to fix that small
  difference here. Report it in your summary.)
- The vertical builder `buildVerticalLayout` in `layout_ft.go`.
- Any change to `charInfo` or to how `objWidth` is computed.

## Git workflow

- Branch: `advisor/003-inline-object-width-native`
- One commit, e.g.
  `fix(layout): inline objects keep their width after same-font text`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing tests

Add to `layout_review_ft_test.go`:

1. `TestLayoutRichText_InlineObjectAfterTextKeepsWidth`. Runs: `{Text: "ab"}`,
   `{Text: "\uFFFC", Style: TextStyle{Object: &InlineObject{ID: "img", Width: 50, Height: 20}}}`,
   `{Text: "cd"}`, with `reviewCfg`. The object starts at byte 2. Assert
   `r, ok := l.GetCharRect(2)` has `ok` and
   `math.Abs(float64(r.Width)-50) < 0.5`. Assert the CharRect at the byte after
   the object (`2 + len("\uFFFC")` = 5) has `X >= r.X + r.Width - 0.5`, which
   means it does not overlap.
2. `TestLayoutRichText_InlineObjectFirstKeepsWidth`. The object run comes first
   and `"cd"` follows. Same width assertion at byte 0. This already passes today
   and guards the existing path.
3. `TestLayoutRichText_TwoAdjacentObjects`. Two object runs in a row (widths 30
   and 40) between `"a"` and `"b"`. Assert both widths.

**Verify**: `go test -run 'InlineObject|TwoAdjacentObjects' .` → FAIL on test 1,
and on test 3's first object if it is pulled into the `"a"` run. Record the
reported widths in your summary.

### Step 2: Stop shaping runs at object clusters

In `layout_ft.go`, add `!chars[rj].isObject &&` to the `for rj` condition, next
to `!chars[rj].isColor`. Extend the comment above the loop with one sentence:
"Inline objects end a run too: an object takes its declared width, not the
placeholder glyph's shaped advance, so it must not be shaped with the text
around it."

**Verify**: `go test -run 'InlineObject|TwoAdjacentObjects' .` → `ok`

### Step 3: Full checks and changelog

Changelog bullet under `## [Unreleased]` → `### Fixed` (follow the `v1.26.5`
section's format): "Inline objects in `LayoutRichText` keep their declared
`Width` on native backends when they follow text in the same font. The
placeholder glyph's advance replaced it before, and following text overlapped
the object."

**Verify**:

- `go test ./...` → all `ok`
- `GOOS=js GOARCH=wasm CGO_ENABLED=0 go build .` → exit 0
- `make lint` → `0 issues.`
- `make prepush` → exit 0

## Test plan

- 3 new tests in `layout_review_ft_test.go` (native only, matching the file's
  build tag).
- Existing Arabic/ligature tests (`layout_ligature_test.go`) and the
  layout-equivalence tests (`layout_equiv_ft_test.go`,
  `layout_equivalence_test.go`) must still pass. They check that run shaping is
  unchanged for text without objects.

## Done criteria

- [ ] `go test ./...` exits 0. The 3 new tests exist and pass.
- [ ] `git diff 7234b0b -- layout_ft.go` touches only the run-extension
      condition and its comment (a handful of lines)
- [ ] `make lint` prints `0 issues.`
- [ ] Only in-scope files modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Test 1 passes **before** the fix. Then the object run does not share the
  text's face, and the bug needs a different trigger. Report the face pointers
  (`charFonts[i].face`) for the two runs, and do not change the code.
- After the fix, the object CharRect width is correct but the following CharRect
  still overlaps it. The pen advance is computed somewhere else (search
  `objWidth` in `layout_ft.go`). Report it.
- Any ligature or equivalence test fails.

## Maintenance notes

- Any new kind of fixed-width cluster (like color emoji and objects) must also
  end shaping runs in this loop. Otherwise shaping overwrites its width.
- Reviewers: check that the fix does not split runs of normal text. Only
  `isObject` clusters are affected.
