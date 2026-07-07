//go:build linux && !android && !glyph_pango

package glyph

/*
#cgo CFLAGS: -I${SRCDIR}/deps/include -I${SRCDIR}/deps/include/freetype2 -I${SRCDIR}/deps/include/harfbuzz
// -lpng16 lets FreeType decode PNG-compressed color-emoji bitmaps
// (CBDT/CBLC, e.g. Noto Color Emoji), linked statically from the bundled
// deps so the executable stays self-contained. -lbrotlidec/-lbrotlicommon
// (WOFF2) and -lbz2 satisfy a system FreeType built with those enabled;
// they contribute nothing when the archive does not reference them.
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_amd64 -lfreetype -lharfbuzz -lpng16 -lz -lbrotlidec -lbrotlicommon -lbz2 -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_arm64 -lfreetype -lharfbuzz -lpng16 -lz -lbrotlidec -lbrotlicommon -lbz2 -lm

#include <ft2build.h>
#include FT_FREETYPE_H
#include <stdlib.h>

// getFontFamilyNameFT returns the family name of an FT_Face as a
// malloc'd C string. Caller must free.
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

// FTLibrary is a no-op stub.
type FTLibrary struct{}

func InitFreeType() (FTLibrary, error) { return FTLibrary{}, nil }
func (l *FTLibrary) Close()            {}

// FTFace is a no-op stub.
type FTFace struct{}

func (f *FTFace) FacePtr() unsafe.Pointer { return nil }

// FTStroker is a no-op stub.
type FTStroker struct{}

func NewFTStroker(_ FTLibrary) (FTStroker, error) { return FTStroker{}, nil }
func (s *FTStroker) Close()                       {}

// PangoFontMapW is a no-op stub.
type PangoFontMapW struct{}

func NewPangoFT2FontMap() PangoFontMapW              { return PangoFontMapW{} }
func (m PangoFontMapW) SetResolution(_, _ float64)   {}
func (m PangoFontMapW) CreateContext() PangoContextW { return PangoContextW{} }
func (m *PangoFontMapW) Close()                      {}

// PangoContextW is a no-op stub.
type PangoContextW struct{}

func (c *PangoContextW) Close() {}

// PangoLayoutW is a no-op stub.
type PangoLayoutW struct{}

func (l *PangoLayoutW) Close() {}

// PangoFontDescW is a no-op stub.
type PangoFontDescW struct{}

func (d *PangoFontDescW) Close()                {}
func (d PangoFontDescW) SetSize(_ int)          {}
func (d PangoFontDescW) SetWeight(_ int)        {}
func (d PangoFontDescW) SetStyle(_ int)         {}
func (d PangoFontDescW) SetVariations(_ string) {}

// PangoAttrListW is a no-op stub.
type PangoAttrListW struct{}

func NewPangoAttrList() PangoAttrListW { return PangoAttrListW{} }
func (a *PangoAttrListW) Close()       {}

// PangoLayoutIterW is a no-op stub.
type PangoLayoutIterW struct{}

func (it *PangoLayoutIterW) Close() {}

// PangoTabArrayW is a no-op stub.
type PangoTabArrayW struct{}

func NewPangoTabArray(_ int) PangoTabArrayW { return PangoTabArrayW{} }
func (t PangoTabArrayW) SetTab(_, _ int)    {}
func (t *PangoTabArrayW) Close()            {}

// PangoFontW is a no-op stub.
type PangoFontW struct{}

func (f *PangoFontW) Close() {}

// PangoFontMetricsW is a no-op stub.
type PangoFontMetricsW struct{}

func (m *PangoFontMetricsW) Close() {}

// getFontFamilyName returns the family name from an FT_Face pointer.
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

// FreeType-equivalent constants (pure Go values).
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
