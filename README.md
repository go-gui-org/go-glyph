# Go-Glyph

![Go version](https://img.shields.io/badge/go-1.26%2B-blue)
![License](https://img.shields.io/badge/license-MIT-blue)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/mike-ward/go-glyph)

High-performance text rendering for Go. Shaping, layout, rasterization, and
editing with platform-appropriate backends per OS.

| OS      | Shaper              | Rasterizer              |
| ------- | ------------------- | ----------------------- |
| Linux   | FreeType + HarfBuzz | FreeType                |
| macOS   | CoreText            | CoreText / CoreGraphics |
| Windows | GDI + DirectWrite   | GDI + DirectWrite       |
| Android | FreeType            | FreeType                |
| WASM    | Canvas2D            | Canvas2D                |

![screenshot](assets/a.png)

## Prerequisites

| Platform | Requirements                                                                                                  |
| -------- | ------------------------------------------------------------------------------------------------------------- |
| macOS    | None                                                                                                          |
| Windows  | None                                                                                                          |
| Linux   | None                                                                                  |
| Android  | NDK; run `./scripts/build_android_deps.sh`                                                                    |
| WASM     | None                                                                                                          |

Use system pkg-config instead of vendored static libs: `go build -tags glyph_system`.

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
ts.Commit()
```

See [package docs](https://pkg.go.dev/github.com/go-gui-org/go-glyph) for the
full API — text styling, layout queries, rich text, transforms, text mutation,
IME, and accessibility.

## License

See [LICENSE](LICENSE).
