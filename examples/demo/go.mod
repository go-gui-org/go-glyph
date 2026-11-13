module github.com/go-gui-org/go-glyph/examples/demo

go 1.26.0

require (
	github.com/go-gui-org/go-glyph v1.4.1
	github.com/go-gui-org/go-glyph/backend/ebitengine v0.0.0
	github.com/hajimehoshi/ebiten/v2 v2.10.6
)

replace github.com/go-gui-org/go-glyph => ../..

replace github.com/go-gui-org/go-glyph/backend/ebitengine => ../../backend/ebitengine

require (
	github.com/ebitengine/gomobile v0.0.0-20260820040257-d11f821a26a6 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/image v0.48.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)
