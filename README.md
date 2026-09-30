# Go-Glyph

![Go version](https://img.shields.io/badge/go-1.26%2B-blue)
![License](https://img.shields.io/badge/license-MIT-blue)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/mike-ward/go-glyph)

High-performance text rendering for Go. Shaping, layout, rasterization, and
editing live in one package. Text shaping and rasterization are pure Go on all
native platforms. No C libraries or system text APIs are required for the core
library.

| OS      | Shaper             | Rasterizer     |
| ------- | ------------------ | -------------- |
| Linux   | HarfBuzz (go-text) | x/image/vector |
| macOS   | HarfBuzz (go-text) | x/image/vector |
| iOS     | HarfBuzz (go-text) | x/image/vector |
| Windows | HarfBuzz (go-text) | x/image/vector |
| Android | HarfBuzz (go-text) | x/image/vector |
| WASM    | Canvas2D           | Canvas2D       |

The `glyph` package builds with `CGO_ENABLED=0` on all platforms. CGo is only
used by the optional GPU rendering backends (`backend/gpu`, `backend/ios`,
`backend/android`) to reach native graphics APIs (Metal, OpenGL, GLES).

![screenshot](assets/a.png)

## Install

```sh
go get github.com/go-gui-org/go-glyph@latest
```

## Quick Start

```go
ts, err := glyph.NewTextSystem(backend)
if err != nil {
    log.Fatal(err)
}
defer ts.Free()

ts.DrawText(x, y, "Hello, World!", glyph.TextConfig{
    Style: glyph.TextStyle{FontName: "Sans 24", Color: glyph.Color{A: 255}},
})
ts.Commit() // Call once per frame, after all draw calls.
```

For repeated draws, shape once with `LayoutText` or `LayoutRichText`. Then draw
the cached `Layout` each frame. See the
[package docs](https://pkg.go.dev/github.com/go-gui-org/go-glyph) for the full
API.

## Features

**Shaping and layout**

- HarfBuzz shaping with ligatures and combining-mark positioning.
- Bidirectional text (UAX #9) with per-paragraph direction.
- Word segmentation by rune class, with standalone helpers that need no
  `Layout`.
- Line wrap (word, char, word-char), CJK breaks (UAX #14), alignment, indent,
  line spacing, tab stops.
- Vertical text flow for CJK.
- OpenType feature tags and variable-font axes.
- Pango markup (`<b>`, `<i>`, `<span>`, and more).
- Rich text with per-run styles, plus inline-object placeholders.

**Rendering**

- Glyph atlas with sub-pixel bins, dirty-rect uploads, and a glyph cache.
- N-stop gradients (horizontal, vertical, diagonal).
- Strokes, underlines, strikethroughs, background highlights.
- Color emoji (CBDT/sbix bitmaps, COLR v0) with text/color presentation rules
  (VS15/VS16).
- Procedural box-drawing and block elements for gap-free TUI frames. Grid
  callers set `CellWidth`/`CellHeight` for exact joins.
- Affine transforms (rotate, skew, scale) and per-glyph placements for text on a
  path.
- `InkBounds` reports the painted box for centering icons and marks.

**Editing and input**

- Grapheme-aware mutation helpers (`InsertText`, `DeleteBackward`,
  `DeleteForward`, selection ops).
- `UndoManager` with time-based coalescing.
- IME preedit state (`CompositionState`) with clause styling and cursor
  rendering, plus transformed variants that follow rotated text.
- Hit testing, cursor geometry, word/paragraph queries, selection rects, and
  cursor motion (char, word, line, vertical with preferred x).

## Backends

| Backend              | Target             | Module          |
| -------------------- | ------------------ | --------------- |
| `backend/ebitengine` | Ebitengine         | Separate module |
| `backend/gpu`        | OpenGL 3.3 / Metal | Main module     |
| `backend/web`        | HTML Canvas (WASM) | Main module     |
| `backend/android`    | GLES               | Main module     |
| `backend/ios`        | Metal              | Main module     |

`backend/ebitengine` keeps its own `go.mod` so the root module stays free of the
Ebiten dependency. The import path is unchanged. Custom backends implement
`DrawBackend` (plus the optional `RectTextureUpdater` and
`TransformedFillBackend` extensions).

## Fonts

System fonts load per OS (macOS, Linux with XDG dirs, Windows with
`%SystemRoot%` fallback, Android `/system/fonts`). Use `AddFontFile` or
`AddFontBytes` (go:embed friendly) for app fonts, including `.ttc` collections.
`ResolveFontName` shows the resolved family. `ListFontFamilies` enumerates
registered families. Fallback faces scale by cap-height ratio so CJK and icon
glyphs match the primary size. On WASM, use the browser FontFace API instead of
`AddFontBytes`.

## DPI and Memory

Pair `(*TextSystem).SetDPIScale` with `SetDPIScale` on the backend when the
window moves to a display with a new scale factor. The call is a no-op when the
value is unchanged or out of range, so resize handlers can call it each frame.
Call `Purge` after a full clear to reclaim layout, glyph, and atlas memory
without tearing down the `TextSystem`.

## Thread Safety

`Context`, `Renderer`, `TextSystem`, `GlyphAtlas`, and `UndoManager` are not
safe for concurrent use. Call all glyph methods from the main/render goroutine.
The package performs no internal locking by design.

## Examples

Each directory under `examples/` is its own module: `demo`, `demo_gpu`,
`showcase_gpu`, `showcase_web`, `showcase_android`, `showcase_ios`,
`showcase_sections`.

## Docs

- [Package API](https://pkg.go.dev/github.com/go-gui-org/go-glyph)
- [Generated API reference](docs/api/API.md)
- [Contributing](CONTRIBUTING.md)

## License

See [LICENSE](LICENSE).
