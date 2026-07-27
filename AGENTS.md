# AGENTS.md — Go-Glyph

## Essential Commands

```bash
# Build all packages (native; requires CGO_ENABLED=1)
go build ./...

# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests on macOS (may need GOMEMLIMIT due to font parsing)
GOMEMLIMIT=2GiB go test -p 1 ./...

# Vet
go vet ./...

# Lint (requires golangci-lint)
golangci-lint run ./...

# Full check suite
go test ./... && go vet ./... && golangci-lint run ./...

# Generate docs and emoji table
go generate ./...

# Serve docs locally
pkgsite -open .

# Build for WASM
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build ./...

# Build for Android (no cgo, verify library compiles)
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build .

# Build for iOS (requires macOS with Xcode)
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 go build -tags ios ./...
```

## go.mod Gotcha

The root `go.mod` has a `replace` directive redirecting `github.com/go-text/typesetting` to a forked `github.com/go-gui-org/typesetting`. Any dependency changes must respect this. New dependencies in the `go-gui-org` namespace should be aware of the existing fork.

## Architecture

```
TextSystem (glyph.go)
├── Context — font loading, text shaping (HarfBuzz or Canvas2D)
├── Renderer — glyph rasterization, atlas management, draw call emission
├── Layout Cache — map[hash]→*cachedLayout, LRU eviction
└── UndoManager — undo/redo with coalescing
         │
         ▼
    DrawBackend (interface: backend.go)
    ├── backend/ebitengine  — Ebitengine
    ├── backend/gpu         — OpenGL 3.3 / Metal
    ├── backend/web         — HTML Canvas (WASM)
    ├── backend/android     — GLES
    └── backend/ios         — Metal
```

Data flow: `Text + Config → Context.LayoutText() → Layout → Renderer.DrawLayout() → DrawBackend → GPU`

**TextSystem** is the main entry point. It owns the Context, Renderer, and layout cache. Call `Free()` to release resources (including temp font files).

**Layout** is a pre-computed result that can be drawn repeatedly. Query methods (`HitTest`, `GetCharRect`, `GetCursorPos`, `MoveCursor*`, `GetWordAtIndex`, `GetSelectionRects`) all operate on a Layout.

**DrawText** is the one-shot convenience: it hashes text+config, looks up or creates a cached layout, draws it, and calls `Commit()`.

## Platform Matrix

| OS | Shaper | Rasterizer | Build Tag |
|---|---|---|---|
| Linux | HarfBuzz (go-text) | `x/image/vector` | `linux` |
| macOS | HarfBuzz (go-text) | `x/image/vector` | `darwin` |
| Windows | HarfBuzz (go-text) | `x/image/vector` | `windows` |
| Android | HarfBuzz (go-text) | `x/image/vector` | `android` |
| iOS | HarfBuzz (go-text) | `x/image/vector` | `ios` |
| WASM | Canvas2D measureText | Canvas2D fillText | `js && wasm` |

The native path (Linux/macOS/Windows/Android/iOS) uses the pure-Go `go-text/typesetting` stack — no cgo for text shaping. WASM uses the browser's Canvas2D API.

## Build Constraints (Critical)

Files are swapped by build tag, not by filename suffix alone. Look at the `//go:build` line at the top of every file:

- **`//go:build linux || darwin || windows`** — Native desktop Context/Renderer (e.g., `context_puregoft.go`, `renderer_common.go`)
- **`//go:build android || linux || darwin || windows`** — All non-WASM (e.g., `renderer_common.go`)
- **`//go:build js && wasm`** — WASM path (e.g., `context_wasm.go`, `renderer_wasm.go`, `layout_wasm.go`, `draw_wasm.go`)
- **`//go:build !js`** — Non-WASM (e.g., `validation.go`, `fontbytes.go`)
- **`//go:build darwin`** — macOS-specific font discovery

**Both `Context` and `Renderer` have two separate definitions** — one for native and one for WASM — with identical field names but different implementations. When adding fields, you must update both definitions.

Font discovery is platform-specific (`discover_darwin.go`, `discover_linux.go`, `discover_windows.go`, `discover_android.go`), but the rest of the pipeline is shared.

## Lint Configuration

`.golangci.yml` uses v2 format with: `errcheck`, `govet`, `staticcheck`, `unused` as linters, and `gofmt` + `goimports` as formatters. Examples are excluded from lint. The `errcheck` linter excludes `(*os.File).Close` and `defer .*Close` patterns.

## Concurrency

**None of the core types are safe for concurrent use.** `Context`, `Renderer`, `TextSystem`, and `GlyphAtlas` must be called from a single goroutine (the main/render thread). This is by design — no locking overhead.

## Thread Safety

`Context`, `Renderer`, `TextSystem`, `GlyphAtlas`, and `UndoManager` are not goroutine-safe. Call all glyph methods from the main/render goroutine. No locking is performed internally — this is a deliberate design choice for performance.

## Key Types

- `TextConfig` → `TextStyle` (font, color, stroke, decorations, letter spacing) + `BlockStyle` (wrap, align, indent, line spacing) + `GradientConfig` + `UseMarkup` + `Orientation` + `NoHitTesting`
- `Layout` → `Items` (runs of glyphs sharing a font), `Glyphs` (individual glyph positions), `Lines`, `CharRects`, `LogAttrs`
- `Glyph` → `Index` (glyph index in the font), `GlyphID` (resolved CGGlyph/HB glyph ID), `Shaped` flag
- `PangoGlyphUnknownFlag` = `0x10000000` — bit set on glyph indices that couldn't be mapped to the font
- `DrawBackend` interface: `NewTexture`, `UpdateTexture`, `DeleteTexture`, `DrawTexturedQuad`, `DrawFilledRect`, `DrawTexturedQuadTransformed`, `DPIScale`

## Testing

Tests use mock backends (`test_helpers_test.go` provides `mockBackend` and `recordingBackend`). `backend_contract_test.go` tests the `DrawBackend` interface contract.

The ebitengine backend tests are skipped on CI (`backend/ebitengine` is excluded from `go test` due to requiring a display).

On macOS CI, tests use `GOMEMLIMIT=2GiB -p 1` to prevent OOM from font parsing.

## Sub-packages

- `accessibility/` — Screen-reader tree management (`Backend` interface + `Manager`)
- `ime/` — IME bridge (`Bridge` interface: macOS NSTextInputClient, Linux IBus, stub)
- `backend/` — Five `DrawBackend` implementations
- `internal/genemoji/` — Code generator for the emoji name table
- `tools/` — Tool dependency tracking (for gomarkdoc)
- `examples/` — Each example has its own `go.mod` (multi-module repo)

## Emoji

The `emoji_table.go` file is generated by `go generate` (via `internal/genemoji/main.go`). It maps emoji codepoints to human-readable names. The `emoji_render_linux_test.go` and `emoji_render_windows_test.go` files test emoji rendering on specific platforms.

## Code Generation

`go generate ./...` runs two generators:
1. `scripts/gen-docs.sh` — generates `docs/api/API.md` using gomarkdoc
2. `internal/genemoji/main.go` — generates `emoji_table.go` from Unicode data

## File Naming Conventions

- `*_ft.go` — FreeType/HarfBuzz path (native desktop)
- `*_wasm.go` — WASM/Canvas2D path
- `*_puregoft.go` — Pure Go FreeType implementation
- `*_shared.go` — Code shared between native and WASM (e.g., `layout_shared.go`, `layout_bidi_shared.go`)
- `*_test.go` — Tests (standard Go convention)
- `*_stub.go` — Stub/no-op implementations (e.g., `backend_stub.go`)
- `discover_<os>.go` — Platform-specific font discovery
- `resolve_font_family_<os>_test.go` — Platform-specific font resolution tests

## Examples with Own go.mod

Examples are separate modules with their own `go.mod` files. CI builds them independently. When changing the root module's API, all example modules must also build.

## CI

GitHub Actions tests on: Ubuntu (Linux + WASM + Android cross-compile), macOS, Windows. Each platform installs its own dependencies (SDL2, font packages, etc.). The ebitengine backend is excluded from test runs because it requires a display.

## Key Dependencies

- `github.com/go-text/typesetting` (replaced with `github.com/go-gui-org/typesetting`) — text shaping
- `github.com/ebitengine/purego` — C library loading without cgo
- `github.com/hajimehoshi/ebiten/v2` — Ebitengine backend
- `github.com/rivo/uniseg` — Unicode grapheme cluster segmentation
- `golang.org/x/image` — vector rasterizer
- `github.com/princjef/gomarkdoc` — documentation generation