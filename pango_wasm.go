//go:build js && wasm

package glyph

// Constants matching pango_cgo.go for WASM builds.

// FreeType pixel modes.
const (
	FTPixelModeNone = 0
	FTPixelModeMono = 1
	FTPixelModeGray = 2
	FTPixelModeLCD  = 5
	FTPixelModeBGRA = 7
	FTPixelModeLCDV = 6
)

// FTFaceFlagColor is the FreeType face flag for color fonts.
const FTFaceFlagColor = 1 << 13

// FreeType load flags.
const (
	FTLoadDefault       = 0
	FTLoadNoScale       = 1 << 0
	FTLoadNoHinting     = 1 << 1
	FTLoadRender        = 1 << 2
	FTLoadNoBitmap      = 1 << 3
	FTLoadForceAutohint = 1 << 5
	FTLoadMonochrome    = 1 << 12
	FTLoadNoAutohint    = 1 << 15
	FTLoadTargetNormal  = 0
	FTLoadTargetLight   = 1 << 16
	FTLoadTargetMono    = 2 << 16
	FTLoadTargetLCD     = 3 << 16
)

// FreeType render modes.
const (
	FTRenderModeNormal = 0
	FTRenderModeLight  = 1
	FTRenderModeMono   = 2
	FTRenderModeLCD    = 3
)

// FreeType stroker constants.
const (
	FTStrokerLineCapRound  = 1
	FTStrokerLineJoinRound = 1
)

// FreeType 26.6 fixed-point constants.
const (
	FTFixedPointShift = 6
	FTFixedPointUnit  = 64
	FTSubpixelUnit    = 16
)

// SubpixelBins is the number of subpixel positioning bins.
const SubpixelBins = 4

// PangoScale is the number of Pango units in one point.
const PangoScale = 1024
