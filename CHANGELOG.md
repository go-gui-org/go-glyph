# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Box-drawing and block-element characters are now drawn procedurally at cell
  size.** Rendering U+2500–257F and U+2580–259F through the font gives visibly
  uneven TUI frames, and three separate causes stack. The cell origin's
  sub-pixel phase renders a 1px stem crisp in bin 0 and split across two columns
  in the others, so a fractional cell advance walks the bins and the border
  alternates between crisp and half-lit. A font's box glyph need not span the
  full advance or line box, so neighbouring cells gap at the join — and by how
  much is font-dependent. And the stem thickness comes from the font's design at
  that ppem, so it is not an integer number of pixels. Snapping at the caller
  fixes only the first. These codepoints now bypass the font entirely: every
  dimension is in whole physical pixels, stems are centered by integer division
  so the same weight lands at the same offset in every cell, and the drawn
  extent is the full cell box so neighbours abut. Shades (U+2591–2593) are
  uniform coverage fills rather than the font's stipple, so they stay flat under
  scaling. kitty, Alacritty, WezTerm and Ghostty all do this, for the same
  reasons (issue #99).

  On by default. `TextStyle.NoBuiltinBoxGlyphs` keeps the font's own glyphs;
  stroked runs (`StrokeWidth` > 0) always use the font, since a built-in fill
  inside a font-derived outline would not match. The cell defaults to
  `ceil(advance × scale)` by the run's ascent+descent, which fixes the weight
  and the phase but leaves a sub-pixel overlap wherever the advance is
  fractional — grid callers should set `TextStyle.CellWidth` and `CellHeight` to
  their real cell, which is what makes the joins exact. The atlas key is the
  cell geometry alone: the sub-pixel bin is deliberately absent (four identical
  copies would reintroduce the phase variation) and so is the font identity, so
  two faces at one cell size share the entry and a frame looks the same
  whichever monospace face the user picked.

- **Powerline separators U+E0B0–E0B3 are drawn when the font has none.** These
  are private-use codepoints, so a patched Nerd Font legitimately owns them and
  draws them to match the rest of its E0Bx family. Synthesis is therefore gated
  on an authoritative `.notdef` from the item's own face: the alternative in
  that case is tofu, not a different look.

- **The built-in box glyphs now reach WASM too.** The web backend has no
  rasterizer — it redraws text with Canvas2D `fillText` every frame — so there
  was no atlas for the geometry to land in, and box art in the browser kept all
  three artifacts the feature exists to remove. The drawing code now emits
  through a small sink: native backends fill a coverage buffer for the atlas as
  before, and WASM replays the identical geometry as `fillRect` calls and, for
  the arcs (U+256D–2570), diagonals (U+2571–2573) and Powerline chevrons, as
  stroked paths. Nothing is uploaded or cached per glyph, and the result is
  resolution-independent. A layout drawn under a rotation or skew keeps the
  font's glyphs, since snapping a cell to the pixel grid means nothing once the
  cell is no longer on it; U+E0B0–E0B3 also stay font-driven on WASM, because
  Canvas2D cannot report a `.notdef` (issue #101).

### Changed

- **The Canvas2D backend works at the device pixel ratio.** `BeginFrame` leaves
  the frame's base transform at the backend's `dpiScale` rather than the
  identity, so callers keep drawing in logical units on a canvas backed by
  physical pixels — which is what gives the built-in box glyphs a real pixel
  grid to snap to on a HiDPI display. A transformed layout composes with that
  base instead of replacing it. Pass the display's `devicePixelRatio` to
  `web.New` and size the canvas' drawing buffer accordingly, as
  `examples/showcase_web` now does; passing 1.0 keeps the previous behavior
  exactly.

### Fixed

- **WASM layout at a scale factor other than 1.0.** The Canvas2D layout divided
  every metric — advances, ascent, descent, item positions, wrap width — by the
  context's scale factor, on the assumption that `measureText` had returned
  physical pixels. It never did: the measurement font is built at the logical
  CSS size. Any scale but 1.0 therefore shrank the advances while leaving the
  drawn glyphs at full size, and the text piled up on itself. The layout is now
  logical throughout, and the device pixel ratio lives in the canvas transform
  where the browser applies it.

- **WASM layout items carry their `TextStyle`.** The Canvas2D layout left
  `Item.Style` zero and passed only the CSS font string along, so anything the
  renderer reads off the style was invisible to it — including
  `NoBuiltinBoxGlyphs`, `CellWidth` and `CellHeight`, which meant a grid
  caller's declared cell was ignored and the box-glyph opt-out never took.
  `LayoutRichText` assigns the run's merged style per item.

## [v1.19.0] - 2026-08-06

### Added

- **Fallback glyphs match the primary font's cap height.** Fallback faces were
  opened at the primary font's pixel size, which equalizes the em, not the
  visible letter size: a CJK or icon face reserves its em for other shapes, so
  its Latin and symbol glyphs read as "small" next to the surrounding text.
  Fallback faces now open at a size scaled by the cap-height ratio (what kitty
  and WezTerm do), so fallback letters are as tall as the primary's.
  `fallbackFitScale` measures the cap height lazily per face (memoized,
  panic-recovered like the rest of the font path) and clamps the correction to
  [0.8, 1.3] with a 0.02 deadband. Items split on size as well as face, and the
  fit rides to the renderer on `Item.FontScale`. PUA clusters are additionally
  re-ranked by Nerd Font probe coverage (`rankIconFallbacks`), dropping
  candidates that cover no probe rune so an unrelated glyph does not silently
  stand in for a missing icon.

### Fixed

- **Bare family key prefers the upright face on a weight tie.** When a family
  name resolves to several faces of equal weight, the resolver now picks the
  upright one instead of leaving the choice to registration order.
- **Scratch `CachedGlyph` slices are reused across draw calls.** The terminal
  steady-state frame path no longer allocates ~3000 slices per frame (issue
  #92); `BenchmarkTerminalFrame1500` pins the target at 0 allocs/op.

## [v1.18.3] - 2026-08-06

### Fixed

- **Atlas uploads batch to the frame boundary (per-call upload storm).**
  `drawLayoutImpl` and `DrawLayoutPlaced` defer dirty-page uploads to `Commit`
  instead of uploading the whole atlas page per draw call. A terminal frame
  issues hundreds of per-glyph text calls, so the v1.18.2 per-call upload
  multiplied the 4 MiB page transfer into GB-scale main-thread traffic whenever
  a frame rasterized any new glyph (measured: 1500 fresh glyphs cost 355 ms /
  5.86 GB of uploads; frame-boundary batching costs 17 ms / 43 MB). The
  issue-#89 guarantee is preserved — hosts call `Commit` after their draw pass
  and before the render pass samples the textures, so the upload still lands
  before the quads' first appearance.

## [v1.18.2] - 2026-08-05

### Fixed

- **Glyph atlas uploads now precede textured draws (one-frame lag).**
  `drawLayoutImpl` and `DrawLayoutPlaced` resolve every glyph in the layout,
  upload dirty atlas pages once, then emit quads. Previously the first
  appearance of a glyph sampled CPU-staging texels and rendered blank for one
  frame — most visible as text popping in one frame late.

## [v1.18.1] - 2026-08-05

### Fixed

- **Newline bytes are caret stops for paragraph cursor movement.** The caret now
  stops at `\n` inside a paragraph when moving left/right through text, so
  editing a multi-line paragraph behaves like an editor instead of skipping to
  the paragraph start/end.

## [v1.18.0] - 2026-07-26

### Added

- **Memory reclamation after a full clear.** `TextSystem.Purge` drops the layout
  cache and glyph cache and resets atlas pages without tearing down the
  `TextSystem`, so callers can reclaim heap and atlas memory mid-session — e.g.
  a terminal handling `CSI 3 J`. Supporting entry points: `GlyphAtlas.Reset` and
  `Renderer.PurgeGlyphCache` (both the common and wasm renderers).

### Changed

- **Dependencies: `go-gui-org/typesetting` pseudo-version bumped.**

## [v1.17.3] - 2026-07-19

### Changed

- **Layout: fewer allocations across the shaping pipeline.** Scratch buffers are
  reused across layout calls, grapheme segmentation runs once per layout,
  glyph-to-cluster assignment uses binary search, and the bidi pass is skipped
  for pure-LTR lines.
- **Fonts: parseFace opens lazily.** Font files are no longer fully slurped at
  parse time; the face opens on-demand, matching parseCoverage behavior.
- **Raster: scratch buffers reused.** Rasterizes and color paths share pooled
  scratch buffers.
- **Renderer: sampled glyph-cache eviction.** Cache ages are folded into
  glyph-cache values and eviction uses sampling rather than a full sweep.

## [v1.17.2] - 2026-07-18

### Added

- **Background cache warming.** The CJK fallback coverage cache is warmed
  asynchronously at `NewContext` time, reducing first-shaping latency without
  any API change.

## [v1.17.1] - 2026-07-17

### Changed

- **Struct field alignment.** `ftFont` and `Glyph` fields are reordered to
  minimize padding on 64-bit targets, shrinking per-instance memory with no API
  change.

## [v1.17.0] - 2026-07-17

### Added

- **CJK fallback follows the session locale.** The CJK fallback tier is
  reordered so the family matching the session locale (`LC_ALL` > `LC_CTYPE` >
  `LANG`) leads — zh-Hans/zh-Hant/ja/ko resolve to PingFang SC/TC, Hiragino/Yu
  Gothic, Apple SD Gothic/Malgun, etc. — so Han-unified ideographs render in the
  reader's regional shapes, matching how native terminals route CJK. Non-CJK or
  unset locales keep discovery order.
- **Font family enumeration.** New `(*TextSystem).ListFontFamilies()` returns
  the collected font family names (system discovery plus
  `RegisterAppFont`/`AddFontFile`), sorted case-insensitively and de-duplicated
  by case fold (first-seen display case wins). Leading-`"."` private names and
  generic aliases are excluded by construction. Not safe for concurrent use —
  call from the main/UI thread.

### Fixed

- **Fallback text no longer renders bold or italic.** Fallback tiers kept the
  first-walked face per family, and lexical walk order puts `-Bold` ahead of
  `-Regular`, so CJK/script fallback text could render in the wrong weight. The
  face closest to Regular weight (upright preferred on ties) now wins regardless
  of filename order.
- **Render-side text fallback matches layout selection.** The text rasterization
  path re-derived fallback from the raw tier list (color and emoji fonts first,
  no coverage filter), so a cluster rasterized by text could pick a different
  font than layout chose. Both now share one selection policy: monochrome fonts
  in fallback order, color fonts only as a last resort.
- **Default-ignorable coverage completed.** `isDefaultIgnorable` now covers the
  full Unicode 17.0 `Default_Ignorable_Code_Point` set (CGJ, Arabic letter mark,
  Hangul fillers, Mongolian variation selectors, bidi embeddings,
  shorthand/musical format controls, …), so clusters carrying these code points
  are not forced to a spurious fallback or tofu.

## [v1.16.2] - 2026-07-14

### Fixed

- **Text symbols across many blocks no longer render as tofu, and default-text
  emoji render as text.** v1.16.1 fixed the Misc Technical block one codepoint
  set at a time; the same over-broad classification remained elsewhere.
  `clusterIsEmoji` matched whole Unicode blocks (U+2600–U+27BF, U+1F000–U+1F0FF,
  U+1F300–U+1FAFF) as emoji. Two failures resulted: (1) the ~760 plain text
  symbols interleaved in those blocks — heavy asterisk U+2731 ✱, mahjong tiles,
  playing cards, alchemical and chess symbols, Supplemental Arrows-C — took the
  color-only path, found no color glyph, and rendered as the base font's
  `.notdef` box; (2) default-text emoji such as U+2733 ✳ (eight-spoked
  asterisk), ❄ ❤ ☀ ✂ — which carry the Emoji property but NOT Emoji_Presentation
  — were forced to their color form instead of the monochrome text glyph Unicode
  specifies. Replaced the block heuristics with a generated `emojiBaseRanges`
  table built from the Unicode **Emoji_Presentation** property, consulted by
  binary search. Now only default-emoji codepoints take the color path; text
  symbols keep their monochrome text-font fallback, and a default-text emoji
  goes color only when an explicit VS16 (U+FE0F) requests it — matching Core
  Text and Ghostty. The text-presentation fallback also now prefers a monochrome
  font over a color-emoji font that merely happens to cover the codepoint (and
  sorts earlier in the fallback list), falling back to color only when nothing
  monochrome covers it — so a reclassified glyph such as ✳ actually resolves to
  Menlo rather than Apple Color Emoji.
- **Rich text items propagate InlineObject metadata.** `IsObject` and `ObjectID`
  now survive layout so callers can identify object runs post-shape.
- **Script fallback prevents symbol tofu in rich text.** `LayoutRichText` falls
  back through script-specific shapers when the primary font lacks a glyph.
- **VS15 (U+FE0E) honored as a text-presentation request.** The variation
  selector now forces text presentation, matching VS16's color override.

### Changed

- Added `internal/genemoji` (wired into `go generate`) that regenerates
  `emoji_table.go` from a pinned Unicode `emoji-data.txt` release (currently
  17.0.0). Bump the version constant and re-generate to adopt newer emoji.
- **Face cache capacity raised** from 128 to 512 to prevent parse thrash under
  many-font workloads.
- **CI: macOS tests** probe fallback fonts via cmap cache to avoid OOM.

### Notes

- This release replaces the upstream `go-text/typesetting` dependency with
  `go-gui-org/typesetting` (PR #267, branch `fix/cff2-index-offsize-oob-alloc`)
  via a `replace` directive pending upstream merge. Consumers should not be
  affected.

## [v1.16.1] - 2026-07-12

### Fixed

- **Text symbols no longer render as tofu.** Codepoints such as U+23F5 ⏵ (the
  media triangles ⏴⏵⏶⏷) fell through to the base font's `.notdef` box instead of
  a real glyph. Two causes: `clusterIsEmoji` matched the whole Misc Technical
  block (U+2300–U+23FF), forcing a color-only fallback the triangles have no
  glyph for; and Apple's `.LastResort` font — which maps every codepoint to a
  placeholder box — shadowed real fonts (STIX) in the general fallback tier.
  Narrowed the emoji classification to the codepoints Unicode marks Emoji and
  excluded `.LastResort` from the fallback set, so such symbols now resolve to a
  real fallback glyph, matching Core Text.

## [v1.16.0] - 2026-07-12

### Added

- **Recommended line height.** `TextMetrics.LineHeight` reports the
  baseline-to-baseline advance a caller should use when stacking lines:
  ascent+descent+leading (the font's own line-gap hint), floored to 1.15×em so
  fonts with a zero line-gap hint — or synthesized fallback metrics — do not
  render cramped. Prefer it over `Height` for line stacking.

### Changed

- Multi-line layout now uses the recommended line height instead of
  ascent+descent, so the font's line-gap leading is no longer discarded and
  wrapped text stacks with correct spacing.
- **Shaped-glyph-stream rendering (pure-Go text backend).** Layout builds
  `Layout.Glyphs` directly from HarfBuzz output — each glyph carries its
  resolved glyph id, advances, and mark-positioning offsets — and the renderer
  rasterizes by glyph id, dropping the second per-glyph shaping pass. This
  positions combining marks across grapheme clusters, and makes the glyph count
  independent of the grapheme count (ligatures collapse clusters, marks add
  glyphs). Text that is unshaped, color emoji, or from a missing font falls back
  to the previous text-based rasterization.
- Carets now stop only at ligature boundaries, not inside a ligature (a
  ligature-absorbed cluster is no longer a cursor position).
- `Glyph.GlyphID` widened from `uint16` to `uint32` (fonts can exceed 65535
  glyphs).

### Fixed

- Combining marks are now x/y-offset relative to their base instead of advancing
  past it.

### Notes

- `DrawLayoutPlaced` callers must size the placement slice to
  `len(layout.Glyphs)` and index it by `GlyphInfo.Index` — the glyph count is no
  longer the grapheme count. The package doc example shows the pattern.

## [v1.15.0] - 2026-07-11

### Changed

- **Pure-Go text backends on all non-Darwin platforms.** Linux, Android, macOS,
  and Windows replace the cgo FreeType+HarfBuzz stack with `go-text/typesetting`
  (shaping, font parsing, discovery) and `golang.org/x/image/vector`
  (rasterization). Color emoji decode CBDT/sbix bitmaps; stroked text uses a
  pure-Go path stroker. The library now builds with `CGO_ENABLED=0` on these
  platforms — no `libfreetype`/`libharfbuzz`, no vendored static libs, no system
  packages. Android shares the Linux engine, differing only in font discovery
  (`/system/fonts` via go-text); example apps still use cgo for their platform
  shells. (#31, #32, #33, #35, #36, #37)

### Fixed

- Rich-text emoji rendering and baseline alignment. (#38)
- Increase subscript/superscript yShift for improved positioning. (#40)
- Render COLR v0 color emoji and stop the whitespace-dot fallback. (#41)

### Removed

- Vendored FreeType+HarfBuzz static libraries and headers (all of `deps/`), the
  Android/Docker dep-build scripts, and the `glyph_system` build tag — the
  library no longer links any C text library on the pure-Go platforms. (#31)

## [2.0.0] - 2026-07-08

### Added

- **FreeType+HarfBuzz text backend (Linux default).** Pango/CGo code path
  removed; Linux now uses statically-linked FreeType+HarfBuzz with color emoji
  support. Links only OS-default libraries (libc, libm, libz, libstdc++).
  Includes bidi, CJK line-wrap, pure-Go font discovery, and `AddFontBytes`.
  (#10, #16)
- **Native GLX backend on Linux.** `backend/gpu` now uses native GLX for OpenGL
  context and window handling — no SDL2 required. Links only X11, GL, and GLX.
  (#11, #18)
- **Native WGL backend on Windows.** `backend/gpu` now uses native WGL for
  OpenGL context and window handling — no SDL2 required. Links only `opengl32`,
  `gdi32`, `user32`. (#17, #19)
- **Vendored static libraries.** FreeType+HarfBuzz static libs are vendored in
  the repository; no build script required on Linux.
- **Darwin ASCII monospace fast-path.** Pure-Go shaping for ASCII content in
  monospace fonts skips CoreText, yielding ~500× speedup for cached layouts (68
  ns/op, 0 allocs). (#13, #15)

### Removed

- **Pango** text backend removed entirely. FreeType+HarfBuzz is now the sole
  Linux text path.
- **SDL2 backend** deleted (`backend/sdl2` module and `demo_sdl2` example). All
  GPU backends (macOS, Linux, Windows) now use native OS windowing.
- **ROADMAP.md** retired; planning moves to GitHub issues and project board.
- Stale BSD/MSYS2 install instructions removed from README.

### Changed

- **Breaking (Linux GPU):** `gpu.New()` parameter is now
  `*gpu.X11Handle{Display, Window}` instead of `SDL_Window*`. `WindowFlag()` and
  `WindowDrawableSize()` removed from the Linux backend.
- **Breaking (Windows GPU):** `gpu.New()` parameter is now
  `*gpu.Win32Handle{HWND}` instead of `SDL_Window*`. `WindowFlag()` and
  `WindowDrawableSize()` removed from the Windows backend.
- Linux: system pkg-config FreeType+HarfBuzz used by default (static vendored
  libs remain as fallback for systems without pkg-config).
- README rewritten for conciseness; expanded godoc for public API.

### Fixed

- `uniseg` used for wasm grapheme fallback, fixing emoji sequence rendering in
  wasm builds. (#8, #9)
- Layout items now split at rich-text run boundaries respecting per-run
  appearance; Pango markup demo dropped. (#22)
- PNG support enabled in vendored FreeType for color bitmap glyphs.
- CI modernized: removed stale Pango/SDL2 dependencies from Linux, Windows, and
  macOS jobs; lint now uses filtered package list with direct golangci-lint
  download.

## [1.14.0] - 2026-07-10

### Changed

- **Windows:** DirectWrite backend rewritten from CGo to pure syscall bindings,
  removing the C compiler requirement for the DirectWrite path. (#28)

### Fixed

- **Windows:** color emoji rendering in DirectWrite fixed after CGo→syscall
  port. (#29)
- Docs: stale SDL2 prerequisite description corrected in CONTRIBUTING.md.

## [1.13.1] - 2026-07-09

### Fixed

- **Windows:** detect proportional font substitution when the configured font
  family is unavailable and fall back to Consolas, preserving fixed-advance grid
  rendering. (#25)

### Changed

- Refactor: deduplicate draw and renderer logic between FreeType (Linux) and
  CoreText (Darwin) paths. (#23)

## [1.12.0] - 2026-06-28

### Added

- **`TextStyle.EmojiBoxWidth`:** grid callers (e.g. terminals) can set a target
  cell-box width (logical px) so color/emoji glyphs scale to fill their reserved
  cells at any DPI, instead of the font's narrower natural emoji advance.
  Implemented for the CoreText (Metal), pango (Linux) and Android draw paths;
  Windows carries a documented TODO; 0 keeps the previous sizing.

### Fixed

- **Emoji ZWJ ligatures:** couple/kiss sequences that CoreText decomposes into a
  zero-advance lead fragment plus a trailing glyph are now coalesced before
  per-cluster re-rasterization, so the whole grapheme reforms its ligature
  instead of rendering components (e.g. a standalone heart overflowing the
  cell).

## [1.11.0] - 2026-06-21

### Added

- **GPU backend tests:** batch vertex ordering, multi-quad offset, reset
  capacity retention, nil-window error guard, stub-platform DPI clamping and
  NaN/Inf handling (`backend/gpu/backend_test.go`,
  `backend/gpu/backend_stub_test.go`).

### Changed

- **`backend/sdl2` extracted to separate Go module** at
  `github.com/go-gui-org/go-glyph/backend/sdl2`. Root module no longer depends
  on `github.com/veandco/go-sdl2`. Downstream users who do not use the SDL2
  backend will no longer pull the binding.
- **`backend/gpu` Metal path no longer requires SDL2 C headers on macOS.**
  `metalInit` accepts `CAMetalLayer*` directly instead of `SDL_Window*`. Removed
  `-I/opt/homebrew/include/SDL2` and `-I/usr/local/include/SDL2` CGO flags. The
  caller (e.g. go-gui) owns window and layer creation.
- **Breaking (macOS):** `gpu.New()` parameter is now `CAMetalLayer*` instead of
  `SDL_Window*`. `WindowFlag()` and `WindowDrawableSize()` removed from macOS
  Metal backend (still present on Linux/Windows OpenGL).
- **DPI guard hardened:** `!(dpiScale > 0)` catches NaN, ±Inf, zero, and
  negative in one IEEE-754 expression. Removed `dpiScale` round-trip through CGo
  — C init functions never used it.
- Updated `demo_gpu` and `showcase_gpu` examples with platform-split helpers for
  the new API.
- CI: bumped GitHub Actions to latest major versions.

### Security

- `gpu.New()` nil-window guard returns error before CGo instead of passing a nil
  pointer to C.
- `metalInit` NULL guard in C returns early before any allocation.
- `backend/gpu` Metal path links only Apple frameworks (Metal, QuartzCore,
  Foundation) — no third-party C libraries.

## [1.10.0] - 2026-06-14

### Added

- **Phase B regression tests** (9 new test files, 15 fuzz targets, 1,378 LOC):
  cache-key regression, layout equivalence across Pango/Darwin, backend contract
  tests, accessibility manager tests, layout mutation and query fuzzing.
- **Phase C benchmarks:** `BenchmarkLayoutText`, `BenchmarkLayoutTextCached`,
  `BenchmarkLayoutRichText` in `context_darwin_test.go`. Baseline on Apple M5:
  cached layout 68 ns/op (0 allocs, ~500× faster than uncached 33.7 µs).
- Platform matrix in `doc.go` and `README.md` (shaper/rasterizer per OS). All
  six backends documented: ebitengine, gpu, sdl2, web, android, ios.
- **`GradientDiagonal`** direction for gradient text fills (top-left to
  bottom-right). De-duplicated `gradientColorForGlyph` across all draw backends.

### Changed

- **Phase D de-duplication:** Extracted `parseSizeFromStyle` and `mergeStyles`
  to `layout_shared.go` (pure Go, no build tags). Removed duplicates from
  `layout_darwin.go`, `layout_wasm.go`, `layout_android.go`.
- Prerequisites in `README.md` now list platform-specific requirements (macOS
  and Windows need no native C libraries for the root package).
- Module path: `github.com/mike-ward/go-glyph` →
  `github.com/go-gui-org/go-glyph`.
- Migrated `.golangci.yml` to v2 schema.
- README audited for stale sections: expanded backend table (6 backends),
  examples table (8 entries), clarified macOS prerequisites, removed false
  go.mod claim.

### Fixed

- **Glyph cache key collision:** cache keys now hash text (plus features, with
  GlyphID as ligature tiebreaker) — `GlyphID` alone was not unique across fonts
  or sizes.
- FreeType download: added SourceForge fallback and XZ validation.
- CGo export comments: added Go-style comments to single exported consts in
  `pango_cgo.go` for staticcheck compliance.
- Ebitengine CI: fixed headless environment detection in `examples/gpu`.
- Build tag on `layout_shared.go` and `layout_shared_test.go` (was breaking
  non-Darwin builds).
- `parseSizeFromFontName` stub for Linux and unsupported platforms.
- CONTRIBUTING license corrected to MIT (was incorrectly PolyForm
  Noncommercial).

## [1.8.1] - 2026-06-03

### Changed

- Reorder struct fields for optimal memory alignment across 10 files; reduces
  struct sizes by eliminating inter-field padding

## [1.8.0] - 2026-05-24

### Added

- Darwin: shaped glyph rendering via CTLine cluster shaping with OpenType
  calt/liga support
- Darwin: paragraph-level bidirectional reordering via
  `golang.org/x/text/unicode/bidi`
- Context: monospace-aware fallback font families with deduplication

### Fixed

- Darwin: duplicate-glyph bug in RTL shapeTextClusters

### Changed

- examples: add `golang.org/x/text` to go.mod/go.sum for all example modules

## [1.7.1] - 2026-05-16

### Fixed

- Darwin: wrap CGo entry points in `@autoreleasepool` to drain Apple-framework
  autoreleases
- Darwin: memoize font-name parsing; wire metrics cache to eliminate per-miss
  CGo allocs

### Changed

- examples/showcase_gpu: discard `Destroy`/`EndFrame` errors explicitly
  (errcheck)

## [1.7.0] - 2026-04-30

### Added

- Darwin: CoreText backend is now the default; legacy Pango path moved behind
  the `glyph_pango` build tag
- Darwin: arbitrary OpenType feature tags forwarded to CoreText
- Darwin: font variation axes and inline-object placeholders
- Darwin: per-run style by splitting per-line Items at run boundaries

### Fixed

- Darwin: preserve RGB channels for color emoji
- Darwin: pass sub/sup OpenType features through to CoreText
- Darwin: restore sub/sup size-scaling fallback
- README.md formatting

### Changed

- Darwin: drop dead types, gate metrics cache helpers behind build tag

## [1.6.5] - 2026-04-13

### Changed

- Modernize codebase with Go 1.26 idioms: min/max builtins, for-range loops,
  clear(), variadic max(), deleted redundant helpers

## [1.6.4] - 2026-04-08

### Added

- DirectWrite color emoji support on Windows
- Claude automation prompts and configuration

### Fixed

- Windows DPI handling in DirectWrite backend

### Changed

- Tidy example module dependencies to match root go.mod

## [1.6.3] - 2026-04-05

### Changed

- Update dependencies: ebiten v2.9.9, uniseg v0.4.7, purego v0.10.0

## [1.6.2] - 2026-04-05

### Fixed

- Correctness and robustness issues from adversarial code review

### Changed

- Windows CI: native CGo job with MSYS2, dynamic path resolution

## [1.6.1] - 2026-04-02

### Fixed

- Windows: `AddFontFile` now registers fonts via `AddFontResourceExW` instead of
  silently succeeding as a no-op
- Windows: grapheme clusters now render full cluster text instead of only the
  first rune (fixes emoji sequences and combining marks)
- Windows: malformed Pango markup returns error and falls back to plain text
  instead of silently truncating content

### Changed

- README: description and architecture reflect multi-platform backends (GDI on
  Windows, CoreText on iOS)
