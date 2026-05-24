module github.com/mike-ward/go-glyph/examples/showcase_android

go 1.26

require (
	github.com/mike-ward/go-glyph v1.4.1
	github.com/mike-ward/go-glyph/examples/showcase_sections v1.0.0
)

replace (
	github.com/mike-ward/go-glyph => ../..
	github.com/mike-ward/go-glyph/examples/showcase_sections => ../showcase_sections
)

require (
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/text v0.34.0 // indirect
)
