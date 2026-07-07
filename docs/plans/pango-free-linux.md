# Plan: Pango-free Linux text backend (FreeType+HarfBuzz)

Tracks go-gui-org/go-glyph#10.

## Goal

Make FT+HB the default Linux text backend. Plain `go build ./...` on Linux
links only libc/libm/libz — no pango/glib/fontconfig. Pango becomes opt-in
via `-tags glyph_pango` (symmetric with darwin).

## Current state

All FT+HB code lives in `*_android.go` files (`//go:build android`). The
pipeline has zero JNI/NDK coupling — Android is "Linux without pango." The
only Android-specific pieces are:

- `freetype_harfbuzz_cgo_android.go` — LDFLAGS hardcode `deps/lib/arm64-v8a`
- `freetype_types_android.go` — `resolveFontFamilyAndroid()` maps generic
  names to Android system font families
- `context_android.go` — font discovery reads `/system/etc/fonts.xml`

Everything else (`bitmap`, `draw`, `grapheme`, `layout*`, `renderer*`) is
portable.

Current pango tag (active on Linux by default):

```v ignore
//go:build !js && !android && !windows && (!darwin || glyph_pango)
```

## Phase 1 — Build tags + pango demotion (1 day, low risk)

### 1a. Extend eight portable files to `android || linux`

Change `//go:build android` to `//go:build android || linux` in:

- `bitmap_android.go`
- `draw_android.go`
- `grapheme_android.go`
- `layout_android.go`
- `layout_attrs_android.go`
- `layout_iter_android.go`
- `renderer_android.go`
- `renderer_load_android.go`

### 1b. Demote pango to opt-in on Linux

In `pango.go` and `pango_cgo.go`, change:

```v ignore
//go:build !js && !android && !windows && (!darwin || glyph_pango)
```

to:

```v ignore
//go:build !js && !android && !windows && (!darwin || glyph_pango) &&
    (!linux || glyph_pango)
```

### 1c. New file: `freetype_harfbuzz_cgo_linux.go`

`//go:build linux && !glyph_pango`

Identical to `freetype_harfbuzz_cgo_android.go` except LDFLAGS:

```c
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_amd64
    -lfreetype -lharfbuzz -lz -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_arm64
    -lfreetype -lharfbuzz -lz -lm
```

## Phase 2 — Linux deps build script (0.5 day, low risk)

Create `scripts/build_linux_deps.sh`. No cross-compilation — builds for host:

- Same FreeType + HarfBuzz versions as `scripts/build_android_deps.sh`
- Output `deps/lib/linux_amd64/` or `deps/lib/linux_arm64/` based on
  `$(uname -m)`
- Add those paths to `.gitignore` (not committed; built locally)

## Phase 3 — Linux font type resolution (0.5 day, low risk)

Create `freetype_types_linux.go` (`//go:build linux && !glyph_pango`):

Copy of `freetype_types_android.go` with `resolveFontFamilyAndroid` renamed
to `resolveFontFamilyLinux`. Default fallback map targets common Linux font
packages:

| Generic name | Linux family |
|---|---|
| sans-serif | DejaVu Sans |
| serif | DejaVu Serif |
| monospace | DejaVu Sans Mono |
| cursive | URW Chancery L |
| fantasy | Impact |

Fallback chain: DejaVu → Liberation → Noto → first discovered font.

## Phase 4 — Linux context + font discovery (1 day, medium risk)

Create `context_linux.go` (`//go:build linux && !glyph_pango`).

Replaces the Android `parseSystemFonts()`/`populateDefaultFontPaths()` logic
with a pure-Go directory walk:

    Font search dirs (in priority order):
    1. ~/.local/share/fonts
    2. ~/.fonts
    3. /usr/local/share/fonts
    4. /usr/share/fonts

For each `.ttf`/`.otf` file found, call `FT_New_Face` + `ftFaceFamilyName`
(already available via CGo) to extract the family name, then store
`familyName → path` in `ctx.fontPaths`. No fontconfig dependency.

Also add `AddFontBytes(data []byte) error` for distroless containers that
`go:embed` a fallback font.

**Implemented (differs from original sketch):** exposed on the public
`TextSystem` (matching `AddFontFile`), not per-`Context`, so it works on
every backend with one implementation. The bytes are persisted to a private
temp file (`writeTempFontFile`, `fontbytes.go` / no-op `fontbytes_wasm.go`)
and registered via the existing `AddFontFile` path, so the font flows
through the full, already-tested render/shape/measure pipeline with zero
hot-path changes. Temp files are pinned for the `TextSystem` lifetime and
removed in `Free`. Requires a writable `os.TempDir` (present in distroless).

Rationale for not using `FT_New_Memory_Face`: every FT C entry point
(`ftRenderGlyph` per glyph, `ftCreateFont`, metrics) re-opens the face from
a path, so pure in-memory faces would require rewriting the per-glyph hot
path on both linux and android — high regression risk on a freshly
stabilized backend. Left as a possible follow-up if a filesystem-free temp
dir becomes a hard requirement.

## Phase 5 — Bidi + CJK wrap (3–4 days, high risk)

### 5a. Lift bidi helpers to shared file

Create `layout_bidi_shared.go` (no build tag, pure Go):

- Move `buildByteToRuneIndexSlice` and `visualOrderForLine` from
  `layout_darwin.go` into this file
- Move `buildByteToRuneIndexSlice` tests from `layout_darwin_test.go` to
  `layout_bidi_shared_test.go`
- `layout_darwin.go` calls the shared functions (no behaviour change)
- `layout_android.go` `buildLayout` gains a `visualOrderForLine` call at
  the end of each line to reorder RTL runs (mirrors darwin usage at
  `layout_darwin.go:1108`)

### 5b. CJK line-wrap in `buildLayout`

Replace the Android whitespace-only wrap loop with `uniseg` break
opportunities (already vendored `github.com/rivo/uniseg v0.4.7`):

Current loop finds wrap points only at ASCII spaces. Replace with
`uniseg.StepString` to honour UAX #14 line-break opportunities — required
for CJK, Thai, and other scripts without space delimiters.

The new loop:

    for each uniseg break opportunity at byte position p:
        measure advance from lineStart to p
        if advance > wrapWidth: emit line break before p

### 5c. Validation

Extend `layout_equivalence_test.go` with:

- Arabic paragraph (RTL, bidi reorder)
- Hebrew mixed LTR/RTL line
- CJK wrap (Chinese paragraph, narrow width)
- Thai wrap (no spaces)

Gate: FT+HB output must match pango output (`-tags glyph_pango`) for each
sample.

## File summary

| Action | File |
|---|---|
| Tag change (×8) | `bitmap/draw/grapheme/layout/layout_attrs/layout_iter/renderer/renderer_load _android.go` |
| Tag narrow (×2) | `pango.go`, `pango_cgo.go` |
| New | `freetype_harfbuzz_cgo_linux.go` |
| New | `freetype_types_linux.go` |
| New | `context_linux.go` |
| New | `layout_bidi_shared.go` |
| New | `layout_bidi_shared_test.go` |
| New | `scripts/build_linux_deps.sh` |
| Modify | `layout_android.go` (`buildLayout` bidi + uniseg wrap) |
| Modify | `layout_darwin.go` (remove lifted functions) |
| Modify | `layout_darwin_test.go` (remove moved tests) |
| Modify | `layout_equivalence_test.go` (add bidi/CJK cases) |
| Modify | `context_android.go` (add `AddFontBytes`) |

## Acceptance criteria

- Plain `go build ./...` on Linux compiles with no pango/glib symbols
- `ldd` on the resulting binary lists only libc/libm/libz
- `-tags glyph_pango` still works (pango opt-in preserved)
- Bidi and CJK wrap match pango output in `layout_equivalence_test.go`
- Pure-Go font discovery resolves system fonts on stock Ubuntu/Fedora with
  no fontconfig installed
- `AddFontBytes` allows a distroless binary with `go:embed`'d font to render

## Unresolved questions

1. Commit `deps/lib/linux_amd64/*.a` or gitignore + build locally? (Check
   whether Android `.a` files are committed.)
2. Default Linux fallback font order: DejaVu vs Liberation vs Noto —
   which is most universally present across distros?
3. `layout_equivalence_test.go` — does it run on Linux CI today? If not,
   add a CI step before Phase 5 lands.
