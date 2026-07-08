//go:build linux && !android && !glyph_system

package glyph

/*
#cgo CFLAGS: -I${SRCDIR}/deps/include -I${SRCDIR}/deps/include/freetype2 -I${SRCDIR}/deps/include/harfbuzz
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_amd64 -lfreetype -lharfbuzz -lz -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_arm64 -lfreetype -lharfbuzz -lz -lm

#include <ft2build.h>
#include FT_FREETYPE_H
#include <stdlib.h>

static char* ftFaceFamilyName(FT_Face face) {
    if (!face || !face->family_name) return NULL;
    size_t len = strlen(face->family_name);
    char *buf = (char *)malloc(len + 1);
    if (!buf) return NULL;
    memcpy(buf, face->family_name, len + 1);
    return buf;
}
*/
import "C"
import "unsafe"

// Stub RAII types — no Pango on Linux.

type FTLibrary struct{}

func InitFreeType() (FTLibrary, error) { return FTLibrary{}, nil }
func (l *FTLibrary) Close()            {}

type FTFace struct{}

func (f *FTFace) FacePtr() unsafe.Pointer { return nil }

type FTStroker struct{}

func NewFTStroker(_ FTLibrary) (FTStroker, error) { return FTStroker{}, nil }
func (s *FTStroker) Close()                       {}

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

func getFontFamilyName(face unsafe.Pointer) string {
	if face == nil {
		return "Unknown"
	}
	cs := C.ftFaceFamilyName(C.FT_Face(face))
	if cs == nil {
		return "Unknown"
	}
	defer C.free(unsafe.Pointer(cs))
	return C.GoString(cs)
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
