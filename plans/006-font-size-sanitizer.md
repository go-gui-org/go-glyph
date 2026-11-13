# Plan 006: Every font size is clamped to [MinFontSize, MaxFontSize] by one resolver, and the shaping scale can never overflow

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- cssfont.go layout_shared.go freetype_types_puregoft.go validation_common.go bitmap_common.go fontname_test.go validation_test.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition. Plan 005 adds helpers to `freetype_types_puregoft.go`
> near `safeShape`. That change is expected and does not touch `openFTFont`.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW–MED (a deliberate behavior change for sizes outside 0.1–500)
- **Depends on**: none (if 005 has landed, rebase on it. Both touch
  `freetype_types_puregoft.go`, in different functions.)
- **Category**: security (robustness) + API contract
- **Planned at**: commit `7234b0b`, 2026-11-13
- **Maintainer decision (2026-11-13)**: enforce the exported
  `MinFontSize`/`MaxFontSize` bounds.

## Why this matters

The package exports `MinFontSize = 0.1` and `MaxFontSize = 500`, documented as
"the minimum/maximum font size in points", but no code enforces them. Sizes
reach the shaper through two different resolvers. `resolveFontSize`
(`cssfont.go`) rejects NaN and ±Inf, and `parseSizeFromStyle`
(`layout_shared.go`) returns `+Inf` unchanged. Neither one bounds a large finite
size. The native shaper then computes `int32(math.Round(size * 64))`
(`freetype_types_puregoft.go:382`). For sizes past about 33.5 million px, or for
Inf, that conversion is implementation-defined (a negative scale on amd64), and
Inf/NaN then leaks into the `Layout` geometry that go-gui feeds into its own
math. The exported `ValidateSize` also lets NaN through.

The maintainer decided to **enforce** the documented bounds. After this plan:

- one resolver handles every backend (native and wasm),
- every resolved size is **clamped** to `[MinFontSize, MaxFontSize]`,
- `openFTFont` defends its own int32 scale,
- `ValidateSize` rejects NaN.

**Clamp, not error.** The library already degrades invalid sizes (NaN, Inf, ≤0)
to a fallback instead of failing. An error from `DrawText` would make text
vanish. Clamping keeps text visible and bounded. Callers that want a hard
failure can call `ValidateSize` themselves.

## Current state

`layout_shared.go:7-17` (tag: native + `js && wasm`):

```go
// parseSizeFromStyle returns the effective font size from a TextStyle.
func parseSizeFromStyle(s TextStyle) float32 {
	if s.Size > 0 {
		return s.Size
	}
	sz := parseSizeFromFontName(s.FontName)
	if sz > 0 {
		return sz
	}
	return 16
}
```

Callers: `layout_ft.go:164` (fallback font size), `layout_ft.go:483` (native
rich-text subs/sups base size), `layout_wasm.go:72` (wasm subs/sups).

`cssfont.go:14-28` (no tag):

```go
// resolveFontSize returns the font size (logical px) for a style: the
// explicit Size, else the size embedded in a Pango font name, else 16.
// NaN, infinite, zero, and negative sizes count as "not set". ...
func resolveFontSize(style TextStyle) float32 {
	size := style.Size
	if !(size > 0) || math.IsInf(float64(size), 0) {
		size = parseSizeFromFontName(style.FontName)
	}
	if !(size > 0) {
		size = 16
	}
	return size
}
```

Callers: `cssFontSize` (wasm CSS strings and metrics) and `resolveFTFontParams`
(`freetype_types_puregoft.go:866-889`, native, which multiplies by
`scaleFactor`).

`validation_common.go:19-22, 43-51` (no tag):

```go
	// MinFontSize is the minimum font size in points.
	MinFontSize = float32(0.1)
	// MaxFontSize is the maximum font size in points.
	MaxFontSize = float32(500.0)
...
func ValidateSize(size, minVal, maxVal float32,
	name, location string) error {
	if size < minVal || size > maxVal {
```

`bitmap_common.go:55-67` (no tag): `validRenderSize` and
`const maxRenderSize = 1 << 20`, the pixel bound the rasterizer already
enforces.

`freetype_types_puregoft.go:369-386` (native only), `openFTFont`:

```go
func openFTFont(path string, size float64) ftFont {
	cf := loadCachedFace(path)
	...
		scale: int32(math.Round(size * 64)),
```

**Units note.** `TextStyle.Size` is documented as points, and the library uses
it directly as logical px (scale factor 1). The clamp applies to the value
`resolveFontSize` returns, before any `scaleFactor` multiply. With `scaleFactor`
up to its validated maximum, pixel sizes then stay far below `maxRenderSize`.
`openFTFont` still clamps against `maxRenderSize` as a final guard, because
`newFTFontFromPath` takes raw pixel sizes.

**Consumer check done during planning:** no code in go-glyph, go-gui, go-term,
go-edit or go-charts uses a size outside 0.1–500. go-term's
`settings_test.go:71` uses exactly `Size: 500`, which stays valid.

Existing tests: `fontname_test.go:64-80` (`TestResolveFontSize`, a table test,
which is the pattern to extend), `validation_test.go:81-100`
(`TestValidateSize*`).

## Commands you will need

| Purpose       | Command                                          | Expected on success |
| ------------- | ------------------------------------------------ | ------------------- |
| Focused tests | `go test -run 'Size' .`                          | `ok`                |
| Full tests    | `go test ./...`                                  | all `ok`            |
| WASM vet      | `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` | exit 0              |
| Lint          | `make lint`                                      | `0 issues.`         |
| Full gate     | `make prepush`                                   | exit 0              |

## Scope

**In scope**:

- `cssfont.go` — `resolveFontSize`
- `layout_shared.go` — `parseSizeFromStyle`
- `freetype_types_puregoft.go` — `openFTFont` only
- `validation_common.go` — `ValidateSize` NaN handling, and the doc comments on
  `MinFontSize`/`MaxFontSize`
- `fontname_test.go`, `validation_test.go` — new cases
- `size_sanitize_test.go` (create, no build tag) and
  `size_sanitize_puregoft_test.go` (create, native tag
  `//go:build android || linux || darwin || windows`)
- `CHANGELOG.md` — bullet under `## [Unreleased]` → `### Changed` (it is a
  behavior change), not `### Fixed`

**Out of scope**:

- Changing the values of `MinFontSize`/`MaxFontSize`.
- Returning errors for out-of-range sizes from `LayoutText`/`DrawText`.
- `LetterSpacing`, `LineSpacing`, `Indent`, `CellWidth` and other numeric
  fields.
- `layout_ft.go` and `layout_wasm.go` call sites. They keep calling
  `parseSizeFromStyle`, and only its body changes.
- The subs/sups size computed as `baseSize * 0.58` from an already-clamped base.
  It can fall below 500 but never above. If the base is 0.1, the sub-size is
  0.058. Leave it; the rasterizer handles small sizes.

## Git workflow

- Branch: `advisor/006-font-size-sanitizer`
- One commit, e.g.
  `fix: enforce MinFontSize/MaxFontSize in one font-size resolver`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing tests

1. Extend the table in `TestResolveFontSize` (`fontname_test.go`) with:
   - `{TextStyle{Size: 1e30}, MaxFontSize}` (huge finite, clamped)
   - `{TextStyle{Size: 600}, MaxFontSize}`
   - `{TextStyle{Size: 500}, 500}` (the bound itself is kept)
   - `{TextStyle{Size: 0.01}, MinFontSize}` (tiny positive, clamped up)
   - `{TextStyle{FontName: "Sans 1e9"}, MaxFontSize}`. First check what
     `parseFontSize("1e9")` returns (the fast path accepts a leading digit, so
     it is probably 1e9). If it returns 0, expect 16 instead, and add a comment
     saying which one applies.
   - The existing rows (`20`, NaN→`11`, Inf→`16`, `-2`→`16`) must stay as they
     are.
2. Create `size_sanitize_test.go` (no build tag):
   `TestParseSizeFromStyleMatchesResolveFontSize`. For each style in
   `{Size: +Inf}`, `{Size: NaN, FontName: "Sans 11"}`, `{Size: 1e30}`,
   `{Size: 0.01}`, `{Size: 12}`, `{}`, assert
   `parseSizeFromStyle(s) == resolveFontSize(s)`.
3. `TestValidateSizeRejectsNaN` in `validation_test.go`:
   `ValidateSize(float32(math.NaN()), MinFontSize, MaxFontSize, "font size", "test")`
   must return a non-nil error.
4. Create `size_sanitize_puregoft_test.go` (native tag):
   - `TestOpenFTFontScaleBounded`: get any installed font path through
     `ctx, _ := NewContext(1); defer ctx.Free()` and the first value of
     `ctx.fontPaths` (`t.Skip` if empty). For sizes `1e12`, `math.Inf(1)` and
     `math.NaN()`, assert
     `f := openFTFont(path, size); f.scale > 0 && f.scale <= int32(maxRenderSize*64)`.
   - `TestLayoutTextSizeClamped`: lay out `"abc"` with
     `TextStyle{FontName: "Sans", Size: 600}` and with `Size: 500`. Assert both
     succeed and their `Height` values are equal (within 0.5). Also lay out with
     `Size: float32(math.Inf(1))` and with `Size: 1e30`, and assert `Width` and
     `Height` are finite.

**Verify**: `go test -run 'Size' .` → FAIL. Record which tests fail.

### Step 2: Clamp in `resolveFontSize`, and make `parseSizeFromStyle` delegate

- `resolveFontSize`: after the existing logic, add
  `size = min(max(size, MinFontSize), MaxFontSize)`. Update the doc comment:
  "The result is clamped to [MinFontSize, MaxFontSize]: invalid sizes fall back
  as above, out-of-range sizes are clamped rather than rejected, so text stays
  visible and every caller stays far inside the range the shaper's int32 26.6
  scale can hold."
- `parseSizeFromStyle`: replace the body with `return resolveFontSize(s)`. Keep
  the function, since it has three call sites. Its comment says it is the same
  resolver as `resolveFontSize`, kept under this name for the layout call sites.

**Verify**: `go test -run 'ResolveFontSize|ParseSizeFromStyleMatches' .` → `ok`

### Step 3: Make `openFTFont` bound its own scale

At the top of `openFTFont`, before `loadCachedFace`:

```go
	// The 26.6 scale is int32(size*64): a NaN, infinite or huge size would
	// make that conversion implementation-defined (a negative scale on
	// amd64). Callers pass resolved sizes, but newFTFontFromPath takes raw
	// pixels, so clamp to the bound the rasterizer enforces.
	if !(size > 0) {
		size = 16
	} else if size > maxRenderSize {
		size = maxRenderSize
	}
```

**Verify**: `go test -run 'OpenFTFontScale|SizeClamped' .` → `ok`

### Step 4: `ValidateSize` rejects NaN, and the bound constants document the clamp

- `ValidateSize`: change the condition to
  `if !(size >= minVal && size <= maxVal)`. Add a comment that the negated form
  also rejects NaN.
- Doc comments: `MinFontSize`: "MinFontSize is the smallest font size in points.
  Smaller sizes are clamped up to it." `MaxFontSize`: "MaxFontSize is the
  largest font size in points. Larger sizes are clamped down to it."

**Verify**: `go test -run 'ValidateSize' .` → `ok`, with all 4 tests passing

### Step 5: Full checks and changelog

Changelog, under `## [Unreleased]` → `### Changed`: "Font sizes are now clamped
to `MinFontSize` (0.1) – `MaxFontSize` (500) on every backend. This applies to
`TextStyle.Size` and to sizes in font names. Infinite and NaN sizes still fall
back to the font-name size or 16. Before, sizes were unbounded, and the native
shaper could compute an out-of-range scale and return infinite layout geometry.
`ValidateSize` now rejects NaN."

**Verify**:

- `go test ./...` → all `ok`
- `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` → exit 0
- `make lint` → `0 issues.`
- `make prepush` → exit 0

## Test plan

- New: 5 table rows, 1 shared test, 1 validation test, 2 native tests.
- Pattern: `TestResolveFontSize` (`fontname_test.go:64`).
- Existing subs/sups and rich-text tests must pass unchanged
  (`go test -run RichText .`).

## Done criteria

- [ ] `go test ./...` exits 0. All new tests exist and pass.
- [ ] `grep -n 's.Size > 0' layout_shared.go` prints nothing
- [ ] `grep -n 'MaxFontSize' cssfont.go` prints at least 1 line
- [ ] `grep -n 'maxRenderSize' freetype_types_puregoft.go` prints at least 1
      line
- [ ] `GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...` exits 0
- [ ] `make lint` prints `0 issues.`
- [ ] Only in-scope files modified
- [ ] `plans/README.md` status row updated

## STOP conditions

- Any existing test lays out text with a size above 500 or below 0.1 and now
  fails. Report the test name. Do not raise the bounds, and do not edit the
  test.
- `MinFontSize`, `MaxFontSize` or `maxRenderSize` are not in untagged files, so
  the wasm build cannot see them. Report it, and do not duplicate the constants.
- `TestLayoutTextSizeClamped` still produces Inf/NaN geometry after Steps 2–3.
  Then another numeric path is unbounded. Report the field.

## Maintenance notes

- Any new code that needs a font size must call `resolveFontSize` (or
  `parseSizeFromStyle`). Never read `TextStyle.Size` directly.
- This is an observable behavior change. Mention it in the release notes for
  go-gui and go-term, because a caller that asked for 600 now gets 500.
- Other numeric `TextConfig` fields (spacing, indent, cell sizes) still have no
  shared sanitizer. A fuzz target over `LayoutText` with arbitrary floats would
  find the next one.
