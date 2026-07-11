//go:build linux || darwin

package glyph

import "unsafe"

// This file preserves the exported types and constants that the cgo
// FreeType/HarfBuzz backend used to provide, so the pure-Go Linux/Android
// backend keeps the same public API surface as the other platforms
// (backend parity). None of these carry behavior on the pure-Go path —
// shaping and rasterization are handled by go-text/typesetting and
// x/image/vector (see freetype_types_puregoft.go and bitmap_puregoft.go).

// FTLibrary is a no-op handle retained for API parity. The pure-Go
// backend keeps no FreeType library state.
type FTLibrary struct{}

func InitFreeType() (FTLibrary, error) { return FTLibrary{}, nil }
func (l *FTLibrary) Close()            {}

type FTFace struct{}

func (f *FTFace) FacePtr() unsafe.Pointer { return nil }

type FTStroker struct{}

func NewFTStroker(_ FTLibrary) (FTStroker, error) { return FTStroker{}, nil }
func (s *FTStroker) Close()                       {}

// Pango stub types — Linux never used Pango; these exist only so the
// exported symbol set matches the Pango-backed platforms.

type PangoFontMapW struct{}

func NewPangoFT2FontMap() PangoFontMapW              { return PangoFontMapW{} }
func (m PangoFontMapW) SetResolution(_, _ float64)   {}
func (m PangoFontMapW) CreateContext() PangoContextW { return PangoContextW{} }
func (m *PangoFontMapW) Close()                      {}

type PangoContextW struct{}

func (c *PangoContextW) Close() {}

type PangoLayoutW struct{}

func (l *PangoLayoutW) Close() {}

type PangoFontDescW struct{}

func (d *PangoFontDescW) Close()                {}
func (d PangoFontDescW) SetSize(_ int)          {}
func (d PangoFontDescW) SetWeight(_ int)        {}
func (d PangoFontDescW) SetStyle(_ int)         {}
func (d PangoFontDescW) SetVariations(_ string) {}

type PangoAttrListW struct{}

func NewPangoAttrList() PangoAttrListW { return PangoAttrListW{} }
func (a *PangoAttrListW) Close()       {}

type PangoLayoutIterW struct{}

func (it *PangoLayoutIterW) Close() {}

type PangoTabArrayW struct{}

func NewPangoTabArray(_ int) PangoTabArrayW { return PangoTabArrayW{} }
func (t PangoTabArrayW) SetTab(_, _ int)    {}
func (t *PangoTabArrayW) Close()            {}

type PangoFontW struct{}

func (f *PangoFontW) Close() {}

type PangoFontMetricsW struct{}

func (m *PangoFontMetricsW) Close() {}

// getFontFamilyName is queried by layout_query.go via item.ftFace.
// The pure-Go path never populates ftFace, so this always reports the
// unknown sentinel (matching the previous behavior for a nil face).
func getFontFamilyName(face unsafe.Pointer) string {
	if face == nil {
		return "Unknown"
	}
	return "Unknown"
}

const (
	FTPixelModeNone = 0
	FTPixelModeMono = 1
	FTPixelModeGray = 2
	FTPixelModeLCD  = 5
	FTPixelModeBGRA = 7
	FTPixelModeLCDV = 6
)

const FTFaceFlagColor = 1 << 13

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

const (
	FTRenderModeNormal = 0
	FTRenderModeLight  = 1
	FTRenderModeMono   = 2
	FTRenderModeLCD    = 3
)

const (
	FTStrokerLineCapRound  = 1
	FTStrokerLineJoinRound = 1
)

const (
	FTFixedPointShift = 6
	FTFixedPointUnit  = 64
	FTSubpixelUnit    = 16
)

const SubpixelBins = 4
const PangoScale = 1024
