module github.com/go-gui-org/go-glyph/examples/demo_sdl2

go 1.26

require (
	github.com/go-gui-org/go-glyph v1.11.0
	github.com/go-gui-org/go-glyph/backend/sdl2 v1.11.0
	github.com/veandco/go-sdl2 v0.4.40
)

replace (
	github.com/go-gui-org/go-glyph => ../..
	github.com/go-gui-org/go-glyph/backend/sdl2 => ../../backend/sdl2
)

require (
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/text v0.34.0 // indirect
)
