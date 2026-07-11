# Shaped-glyph-stream refactor — formal implementation plan (pure-Go FT backend)

## Locked decisions
1. **Gate on `GlyphID != 0`.** One `Glyph` struct. FT layout populates `GlyphID`;
   renderer takes the by-GID path when set, else falls through to today's text path.
   WASM never sets `GlyphID` → unchanged.
2. **Carets at ligature boundaries only.** No fractional interpolation inside a
   ligature. A ligature run's advance sits on its first (logical) cluster; absorbed
   clusters keep zero-width `CharRect` but are NOT cursor positions (LogAttr).
3. **Keep `computeRunText`.** It is the `GlyphID == 0` fallback (color emoji, missing
   font, malformed index). Not deleted.
4. Color emoji (`renderColorGlyph`/`renderCOLRGlyph`) stay on the shape path — 1
   cluster → 1 glyph, `GlyphID` left 0 so they route to the fallback.

## Goal
Shape each font-run once at layout time (buffer already built at layout_ft.go:375),
emit `Layout.Glyphs` directly from the HarfBuzz output with real `GlyphID`, `XOffset`,
`YOffset`, `XAdvance`. Renderer rasterizes by GID — no second shaping pass. Fixes
variable-glyph-count and cross-cluster mark positioning; caret-in-ligature resolved by
making absorbed clusters non-cursor positions.

## Data model
`Glyph` (layout_types.go:105) — no struct change. Semantics per emitted HB glyph on the
FT path:
- `GlyphID` = HB glyph id (nonzero for real glyphs; 0 → fallback/notdef).
- `Index` = byteI of the **owning cluster** (HB `Info[i].Cluster` mapped to byte
  offset). Many glyphs may share one `Index` (marks on a base); a ligature glyph's
  `Index` is its first cluster.
- `Codepoint` = byte length of the owning cluster (kept for `glyphText` fallback).
- `XOffset/YOffset` = HB `Pos` offsets (device px), added to today's `xPad`/`yShift`.
- `XAdvance/YAdvance` = HB advances (device px).

## Phase 1 — render by GID (bitmap_puregoft.go, renderer_ft.go)
- Add `renderGlyphByID(path, size, strokeWidth, subpixelShift float64, gid uint16)
  (*rasterResult, bool)`: load cached face, `cf.face.GlyphData(font.GID(gid))`,
  reuse existing outline/stroke rasterization (extract the mapPt + rasterize block from
  `renderRun` into a shared helper taking `[]placedGlyph`). No HB call.
- `getOrLoadGlyph`: when `g.GlyphID != 0`, key on
  `GID + size + style + bin + strokeWidth` (drop `runText`/`ch` from key) and call the
  new path. Else unchanged (`computeRunText` → `renderRun`).
- `loadGlyphFT`/`loadStrokedGlyphFT` (renderer_load_ft.go, bitmap_puregoft.go:60/121):
  add GID-aware entry or branch; keep text entry for fallback.
- Verify sub-pixel bin + stroke growth identical to `renderRun`.

## Phase 2 — emit shaped glyphs in layout (layout_ft.go buildLayout)
Current width loop (341-405) shapes each run and distributes advances onto clusters,
producing per-cluster `chars[k].width`. Restructure into a run-shaping pass that keeps
`chars[].width` (still needed for wrap + CharRect) AND records the shaped-glyph stream
per run:
- For each run, after `buf := f.shape(...)`, build `[]shapedGlyph{gid, xAdv, yAdv,
  xOff, yOff, clusterByteI, clusterByteL}` keyed back to clusters via the existing
  `startRune`/cluster-owner logic (layout_ft.go:384-403).
- Cluster width (for wrap/CharRect) = sum of member glyph advances (unchanged result).
- Mark first cluster of a multi-cluster ligature as the width owner; absorbed clusters
  width 0 (already true) **and** flag them non-cursor.
- Store the run's shaped glyphs so the line-assembly loop (646-716) can emit them in
  visual order instead of one Glyph per cluster.

Line assembly (646-716): replace the per-cluster `allGlyphs = append(Glyph{Index:byteI…})`
with emission of each cluster's shaped glyphs (in run/visual order), carrying `GlyphID`
and HB offsets. `Item.GlyphStart/GlyphCount` now count shaped glyphs, not clusters.
Item splitting (color/styleKey/face) unchanged. `CharRects`/`LogAttrs` stay per cluster.

## Phase 3 — caret at ligature boundary (layout_ft.go LogAttr build)
- When a cluster is ligature-absorbed (width 0, not the run's owner), set its
  `LogAttr.IsCursorPosition = false`. Keep its `CharRect` (zero width) for hit-test
  mapping. `GetValidCursorPositions`/`MoveCursorLeft/Right` (layout_query.go) already
  filter on `IsCursorPosition` → carets land only at ligature boundaries. Verify
  `MoveCursorLeft/Right` skip the zero-width interior cleanly.

## Phase 4 — placement/query audit (no behavior change intended)
- `GlyphPositions()` (layout_types.go:134) and `DrawLayoutPlaced` accumulate `cx` by
  `XAdvance`; with the true stream, advances sum to the same run width. Confirm
  `PangoGlyphUnknownFlag` still applies (set it on `GlyphID==0` notdef so hit-test
  skips). `DrawLayoutPlaced` requires `len(placements)==len(Glyphs)` — callers that
  build placements per-glyph (text-on-curve) now see the real glyph count; document.
- `draw_atlas.go` (79-117) iterates `Glyphs`; verify GID path renders.

## Phase 5 — cleanup + tests
- Extract shared rasterize helper; remove any now-dead branches (keep `renderRun` for
  fallback).
- Tests:
  - `layout_equiv_ft_test.go`: advances/line widths unchanged for Latin.
  - Golden/behavior: Arabic `سلام` joined; lam-alef (`لا`) = 1 glyph, 2 clusters, caret
    skips interior; Arabic base+fatha and Latin `e`+combining acute → mark y/x offset
    nonzero and glyph count = 2; mixed `abcسلام` one line; emoji still color
    (GlyphID==0 fallback).
  - `renderer_ft_test.go`: GID path cache key stable; fallback still exercised.
- `gofmt` + `golangci-lint run ./...` clean.

## File-change map
- `bitmap_puregoft.go` — new `renderGlyphByID`, shared rasterize helper.
- `renderer_ft.go` — `getOrLoadGlyph` GID branch + key; keep `computeRunText`.
- `renderer_load_ft.go` — GID-aware load entry.
- `layout_ft.go` — run-shaping emits shaped-glyph stream; LogAttr ligature flag.
- `layout_query.go` — verify cursor filters (likely no change).
- `layout_types.go` — verify `PangoGlyphUnknownFlag`/placement math (likely no change).
- Tests as above. **WASM/CGo/other backends untouched.**

## Risks
- Atlas key substring→GID: no cross-font collision (key includes font path+size). Verify
  cache hit-rate unchanged for repeated glyphs.
- HB Y-up vs device Y-down for `YOffset` — single sign flip; cover with mark test.
- Bidi visual order of the emitted stream must match today's `visualOrderForLine`.
- `DrawLayoutPlaced` glyph-count change is a caller-visible contract shift for
  text-on-curve placement builders — call out in changelog.

## Rollback
`GlyphID` gating means reverting is `GlyphID=0` in layout → old path fully intact.

## Remaining open question
- COLR/CBDT emoji stay on fallback now; migrating them into the stream (COLR layers as
  multiple GID glyphs) is a later, separable step — out of scope here. Confirm OK.
