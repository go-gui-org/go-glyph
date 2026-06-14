//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

/*
#cgo pkg-config: freetype2 harfbuzz pango pangoft2 gobject-2.0 glib-2.0 fontconfig
#include <pango/pango.h>
#include <pango/pangoft2.h>
#include <glib-object.h>
#include <glib.h>
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_STROKER_H
#include FT_BITMAP_H
#include FT_GLYPH_H
#include <fontconfig/fontconfig.h>
*/
import "C"

// FreeType pixel modes.
const (
	FTPixelModeNone = C.FT_PIXEL_MODE_NONE
	FTPixelModeMono = C.FT_PIXEL_MODE_MONO
	FTPixelModeGray = C.FT_PIXEL_MODE_GRAY
	FTPixelModeLCD  = C.FT_PIXEL_MODE_LCD
	FTPixelModeBGRA = C.FT_PIXEL_MODE_BGRA
	FTPixelModeLCDV = C.FT_PIXEL_MODE_LCD_V
)

// FTFaceFlagColor is the FreeType face flag for color glyphs.
const FTFaceFlagColor = C.FT_FACE_FLAG_COLOR

// FreeType load flags.
const (
	FTLoadDefault       = C.FT_LOAD_DEFAULT
	FTLoadNoScale       = C.FT_LOAD_NO_SCALE
	FTLoadNoHinting     = C.FT_LOAD_NO_HINTING
	FTLoadRender        = C.FT_LOAD_RENDER
	FTLoadNoBitmap      = C.FT_LOAD_NO_BITMAP
	FTLoadForceAutohint = C.FT_LOAD_FORCE_AUTOHINT
	FTLoadMonochrome    = C.FT_LOAD_MONOCHROME
	FTLoadNoAutohint    = C.FT_LOAD_NO_AUTOHINT
	FTLoadTargetNormal  = C.FT_LOAD_TARGET_NORMAL
	FTLoadTargetLight   = C.FT_LOAD_TARGET_LIGHT
	FTLoadTargetMono    = C.FT_LOAD_TARGET_MONO
	FTLoadTargetLCD     = C.FT_LOAD_TARGET_LCD
)

// FreeType render modes.
const (
	FTRenderModeNormal = C.FT_RENDER_MODE_NORMAL
	FTRenderModeLight  = C.FT_RENDER_MODE_LIGHT
	FTRenderModeMono   = C.FT_RENDER_MODE_MONO
	FTRenderModeLCD    = C.FT_RENDER_MODE_LCD
)

// FreeType stroker constants.
const (
	FTStrokerLineCapRound  = C.FT_STROKER_LINECAP_ROUND
	FTStrokerLineJoinRound = C.FT_STROKER_LINEJOIN_ROUND
)

// FreeType 26.6 fixed-point constants.
const (
	FTFixedPointShift = 6
	FTFixedPointUnit  = 64 // 1 pixel in 26.6 format.
	FTSubpixelUnit    = 16 // 0.25 pixels in 26.6.
)

// SubpixelBins is the number of subpixel positioning bins.
const SubpixelBins = 4

// PangoScale is the Pango scale factor (1024 units per em).
const PangoScale = C.PANGO_SCALE
