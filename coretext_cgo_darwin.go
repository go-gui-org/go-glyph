//go:build darwin && !glyph_pango

package glyph

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreText -framework CoreGraphics -framework CoreFoundation

#include <CoreText/CoreText.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

// getFontFamily returns the family name of a CTFont as a
// malloc'd C string. Caller must free.
static char* ctFontCopyFamilyNameC(CTFontRef font) {
    @autoreleasepool {
        CFStringRef name = CTFontCopyFamilyName(font);
        if (!name) return NULL;
        CFIndex len = CFStringGetLength(name);
        CFIndex maxSize = CFStringGetMaximumSizeForEncoding(len, kCFStringEncodingUTF8) + 1;
        char *buf = (char *)malloc(maxSize);
        if (!CFStringGetCString(name, buf, maxSize, kCFStringEncodingUTF8)) {
            free(buf);
            CFRelease(name);
            return NULL;
        }
        CFRelease(name);
        return buf;
    }
}
*/
import "C"
import "unsafe"

// Stub RAII types — no FreeType/Pango on iOS.

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

// getFontFamilyName returns "Unknown" on iOS (no FT_Face).
func getFontFamilyName(_ unsafe.Pointer) string { return "Unknown" }

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
