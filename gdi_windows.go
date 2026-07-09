//go:build windows

package glyph

import (
	"log"
	"math"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Win32 GDI constants.
const (
	_TRANSPARENT         = 1
	_DEFAULT_CHARSET     = 1
	_OUT_TT_PRECIS       = 4
	_CLIP_DEFAULT        = 0
	_ANTIALIASED_QUALITY = 4
	_CLEARTYPE_QUALITY   = 5
	_DEFAULT_PITCH       = 0
	_FIXED_PITCH         = 1
	_FF_DONTCARE         = 0
	_FF_MODERN           = 48 // 0x30
	_TMPF_FIXED_PITCH    = 0x01
	_FW_NORMAL           = 400
	_FW_BOLD             = 700
	_DIB_RGB_COLORS      = 0
	_BI_RGB              = 0
	_SRCCOPY             = 0x00CC0020
	_GGO_METRICS         = 0
	_GGO_GRAY8_BITMAP    = 2
	_GGO_GLYPH_INDEX     = 0x0080
)

// Win32 structures.
type _TEXTMETRICW struct {
	TmHeight           int32
	TmAscent           int32
	TmDescent          int32
	TmInternalLeading  int32
	TmExternalLeading  int32
	TmAveCharWidth     int32
	TmMaxCharWidth     int32
	TmWeight           int32
	TmOverhang         int32
	TmDigitizedAspectX int32
	TmDigitizedAspectY int32
	TmFirstChar        uint16
	TmLastChar         uint16
	TmDefaultChar      uint16
	TmBreakChar        uint16
	TmItalic           byte
	TmUnderlined       byte
	TmStruckOut        byte
	TmPitchAndFamily   byte
	TmCharSet          byte
}

type _BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type _BITMAPINFO struct {
	BmiHeader _BITMAPINFOHEADER
	BmiColors [1]uint32 // RGBQUAD placeholder
}

type _GLYPHMETRICS struct {
	GmBlackBoxX     uint32
	GmBlackBoxY     uint32
	GmptGlyphOrigin _POINT
	GmCellIncX      int16
	GmCellIncY      int16
}

type _POINT struct {
	X int32
	Y int32
}

type _MAT2 struct {
	EM11 _FIXED
	EM12 _FIXED
	EM21 _FIXED
	EM22 _FIXED
}

type _FIXED struct {
	Fract uint16
	Value int16
}

type _SIZE struct {
	Cx int32
	Cy int32
}

type _ABC struct {
	AbcA int32
	AbcB uint32
	AbcC int32
}

// DLL and proc lazy loading.
var (
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC              = gdi32.NewProc("DeleteDC")
	procCreateFontW           = gdi32.NewProc("CreateFontW")
	procSelectObject          = gdi32.NewProc("SelectObject")
	procDeleteObject          = gdi32.NewProc("DeleteObject")
	procGetTextMetricsW       = gdi32.NewProc("GetTextMetricsW")
	procGetTextFaceW          = gdi32.NewProc("GetTextFaceW")
	procGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	procSetBkMode             = gdi32.NewProc("SetBkMode")
	procSetTextColor          = gdi32.NewProc("SetTextColor")
	procCreateDIBSection      = gdi32.NewProc("CreateDIBSection")
	procTextOutW              = gdi32.NewProc("TextOutW")
	procGetCharABCWidthsW     = gdi32.NewProc("GetCharABCWidthsW")
	procGetGlyphOutlineW      = gdi32.NewProc("GetGlyphOutlineW")
	procAddFontResourceExW    = gdi32.NewProc("AddFontResourceExW")
	procRemoveFontResourceExW = gdi32.NewProc("RemoveFontResourceExW")
	procGetDC                 = user32.NewProc("GetDC")
	procReleaseDC             = user32.NewProc("ReleaseDC")
	procRtlZeroMemory         = kernel32.NewProc("RtlZeroMemory")
)

// gdiContext holds a reusable GDI device context for text operations.
// It also owns the optional DirectWrite rasterizer used for COLR color
// glyph (emoji) rendering — classic GDI cannot draw color fonts.
type gdiContext struct {
	mu         sync.Mutex
	hdc        uintptr // memory DC
	screenDC   uintptr // screen DC for compatibility
	curFont    uintptr // currently selected HFONT
	curFontKey fontCacheKey
	fontCache  map[fontCacheKey]uintptr
	dwrite     *dwriteRasterizer // nil if DWrite init failed
}

type fontCacheKey struct {
	family string
	height int32
	weight int32
	italic bool
}

var (
	globalGDI   *gdiContext
	gdiInitOnce sync.Once
)

func getGDI() *gdiContext {
	gdiInitOnce.Do(func() {
		screenDC, _, _ := procGetDC.Call(0)
		var memDC uintptr
		if screenDC != 0 {
			memDC, _, _ = procCreateCompatibleDC.Call(screenDC)
		}
		if memDC != 0 {
			procSetBkMode.Call(memDC, _TRANSPARENT)
			procSetTextColor.Call(memDC, 0x00FFFFFF) // white
		}
		// Best-effort DirectWrite init. A nil rasterizer here just means
		// color emoji will render hollow via the GDI fallback below —
		// still a regression-free outcome for monochrome text.
		dw, _ := newDWriteRasterizer()
		globalGDI = &gdiContext{
			hdc:       memDC,
			screenDC:  screenDC,
			fontCache: make(map[fontCacheKey]uintptr),
			dwrite:    dw,
		}
	})
	return globalGDI
}

// CloseGDI releases the global GDI device context and cached fonts.
// This is optional — the OS reclaims GDI resources at process exit.
func CloseGDI() {
	if globalGDI != nil {
		globalGDI.close()
	}
}

func (g *gdiContext) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, hf := range g.fontCache {
		procDeleteObject.Call(hf)
	}
	g.fontCache = nil
	if g.hdc != 0 {
		procDeleteDC.Call(g.hdc)
		g.hdc = 0
	}
	if g.screenDC != 0 {
		procReleaseDC.Call(0, g.screenDC)
		g.screenDC = 0
	}
	if g.dwrite != nil {
		g.dwrite.Close()
		g.dwrite = nil
	}
}

// winFontParams extracts GDI font parameters from a TextStyle.
func winFontParams(style TextStyle, scaleFactor float32) (string, int32, int32, bool) {
	family, size := parseFontDesc(style)
	heightPx := -int32(math.Round(float64(size) * float64(scaleFactor)))
	if heightPx == 0 {
		heightPx = -int32(12 * scaleFactor)
	}
	weight := int32(_FW_NORMAL)
	italic := false
	switch style.Typeface {
	case TypefaceBold:
		weight = _FW_BOLD
	case TypefaceItalic:
		italic = true
	case TypefaceBoldItalic:
		weight = _FW_BOLD
		italic = true
	}
	return family, heightPx, weight, italic
}

// selectFont selects a font matching the given parameters into the DC.
// Returns the HFONT handle.
const maxFontCacheEntries = 64

func (g *gdiContext) selectFont(family string, heightPx int32, weight int32, italic bool) uintptr {
	key := fontCacheKey{family: family, height: heightPx, weight: weight, italic: italic}
	hf, ok := g.fontCache[key]
	if !ok {
		if len(g.fontCache) >= maxFontCacheEntries {
			g.evictFonts()
		}
		hf = createFontW(family, heightPx, weight, italic, _DEFAULT_PITCH|_FF_DONTCARE)
		if hf == 0 {
			return g.curFont // Keep current font on failure.
		}
		// Detect silent GDI font substitution.
		prevFont := g.curFont
		procSelectObject.Call(g.hdc, hf)
		actual := g.actualFaceName()
		if !strings.EqualFold(actual, family) {
			tm := g.getTextMetrics()
			if tm.TmPitchAndFamily&_TMPF_FIXED_PITCH == 0 {
				// GDI gave us a proportional substitute.
				// Recreate with FIXED_PITCH so terminal grids
				// don't misalign.
				procSelectObject.Call(g.hdc, prevFont)
				procDeleteObject.Call(hf)
				hf = createFontW("Consolas", heightPx, weight,
					italic, _FIXED_PITCH|_FF_MODERN)
				if hf != 0 {
					log.Printf("glyph: GDI substituted proportional font for %q; falling back to %q",
						family, "Consolas")
				}
			}
			// If GDI substituted a fixed-pitch face: keep it. It
			// won't break grids.
		}
		procSelectObject.Call(g.hdc, prevFont)
		if hf == 0 {
			return g.curFont
		}
		g.fontCache[key] = hf
	}
	if hf != g.curFont {
		procSelectObject.Call(g.hdc, hf)
		g.curFont = hf
		g.curFontKey = key
	}
	return hf
}

// evictFonts removes all cached fonts except the currently selected one.
func (g *gdiContext) evictFonts() {
	for key, hf := range g.fontCache {
		if hf != g.curFont {
			procDeleteObject.Call(hf)
			delete(g.fontCache, key)
		}
	}
}

func createFontW(family string, height int32, weight int32, italic bool, pitchAndFamily uintptr) uintptr {
	name := utf16.Encode([]rune(family))
	if len(name) > 31 {
		name = name[:31]
	}
	var faceName [32]uint16
	copy(faceName[:], name)

	var ital uintptr
	if italic {
		ital = 1
	}

	hf, _, _ := procCreateFontW.Call(
		uintptr(height),    // nHeight (negative = character height)
		0,                  // nWidth
		0,                  // nEscapement
		0,                  // nOrientation
		uintptr(weight),    // fnWeight
		ital,               // fdwItalic
		0,                  // fdwUnderline
		0,                  // fdwStrikeOut
		_DEFAULT_CHARSET,   // fdwCharSet
		_OUT_TT_PRECIS,     // fdwOutputPrecision
		_CLIP_DEFAULT,      // fdwClipPrecision
		_CLEARTYPE_QUALITY, // fdwQuality
		pitchAndFamily,     // fdwPitchAndFamily
		uintptr(unsafe.Pointer(&faceName[0])),
	)
	return hf
}

// getTextMetrics returns TEXTMETRICW for the currently selected font.
func (g *gdiContext) getTextMetrics() _TEXTMETRICW {
	var tm _TEXTMETRICW
	procGetTextMetricsW.Call(g.hdc, uintptr(unsafe.Pointer(&tm)))
	return tm
}

// actualFaceName returns the face name of the font currently selected
// into the DC via GetTextFaceW.
func (g *gdiContext) actualFaceName() string {
	var buf [32]uint16
	procGetTextFaceW.Call(g.hdc, 32, uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf[:])
}

// measureString returns the width and height of a string in pixels.
func (g *gdiContext) measureString(s string) (int32, int32) {
	if len(s) == 0 {
		return 0, 0
	}
	u16 := utf16.Encode([]rune(s))
	var sz _SIZE
	procGetTextExtentPoint32W.Call(
		g.hdc,
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(u16)),
		uintptr(unsafe.Pointer(&sz)),
	)
	return sz.Cx, sz.Cy
}

// getCharABC returns the ABC widths for a character.
func (g *gdiContext) getCharABC(ch rune) _ABC {
	var abc _ABC
	procGetCharABCWidthsW.Call(
		g.hdc,
		uintptr(ch),
		uintptr(ch),
		uintptr(unsafe.Pointer(&abc)),
	)
	return abc
}

// renderGlyphBitmap renders a single character to an RGBA bitmap using
// 2x supersampling for improved anti-aliasing quality.
// Returns the bitmap, left bearing, and top bearing.
func (g *gdiContext) renderGlyphBitmap(ch string, heightPx int) (Bitmap, int, int) {
	// Get 1x metrics from the currently selected font.
	tm := g.getTextMetrics()
	ascent1x := int(tm.TmAscent)

	u16 := utf16.Encode([]rune(ch))
	if len(u16) == 0 {
		return Bitmap{}, 0, 0
	}

	// Select a 2x font for supersampled rendering.
	origFont := g.curFont
	origKey := g.curFontKey
	ssKey := fontCacheKey{
		family: origKey.family,
		height: origKey.height * 2,
		weight: origKey.weight,
		italic: origKey.italic,
	}
	ssFont, ok := g.fontCache[ssKey]
	if !ok {
		ssFont = createFontW(ssKey.family, ssKey.height, ssKey.weight, ssKey.italic, _DEFAULT_PITCH|_FF_DONTCARE)
		g.fontCache[ssKey] = ssFont
	}

	// Measure with the 2x font.
	procSelectObject.Call(g.hdc, ssFont)
	var sz _SIZE
	procGetTextExtentPoint32W.Call(
		g.hdc,
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(u16)),
		uintptr(unsafe.Pointer(&sz)),
	)
	// Restore 1x font on the main DC.
	procSelectObject.Call(g.hdc, origFont)
	g.curFont = origFont
	g.curFontKey = origKey

	w := int(sz.Cx)
	h := int(sz.Cy)
	if w <= 0 || h <= 0 {
		return Bitmap{}, 0, 0
	}

	// Cap glyph size (2x bitmap, will be halved after downscale).
	if w > MaxGlyphSize*2 {
		w = MaxGlyphSize * 2
	}
	if h > MaxGlyphSize*2 {
		h = MaxGlyphSize * 2
	}

	stride := w * 4
	totalBytes := stride * h

	bmi := _BITMAPINFO{
		BmiHeader: _BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(_BITMAPINFOHEADER{})),
			BiWidth:       int32(w),
			BiHeight:      -int32(h), // top-down
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: _BI_RGB,
		},
	}

	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(
		g.hdc,
		uintptr(unsafe.Pointer(&bmi)),
		_DIB_RGB_COLORS,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbmp == 0 || bits == nil {
		return Bitmap{}, 0, 0
	}
	defer procDeleteObject.Call(hbmp)

	tmpDC, _, _ := procCreateCompatibleDC.Call(g.hdc)
	if tmpDC == 0 {
		return Bitmap{}, 0, 0
	}
	defer procDeleteDC.Call(tmpDC)

	oldBmp, _, _ := procSelectObject.Call(tmpDC, hbmp)
	procSelectObject.Call(tmpDC, ssFont) // Use the 2x font for rendering.
	procSetBkMode.Call(tmpDC, _TRANSPARENT)
	procSetTextColor.Call(tmpDC, 0x00FFFFFF)

	// Clear to black and render at 2x. GDI font linking handles emoji fallback.
	procRtlZeroMemory.Call(uintptr(bits), uintptr(totalBytes))
	procTextOutW.Call(tmpDC, 0, 0,
		uintptr(unsafe.Pointer(&u16[0])), uintptr(len(u16)))

	src := unsafe.Slice((*byte)(bits), totalBytes)
	data := make([]byte, totalBytes)

	// Convert BGRA → RGBA with alpha from max channel.
	for i := 0; i < totalBytes; i += 4 {
		b := src[i+0]
		gg := src[i+1]
		r := src[i+2]

		a := max(r, gg, b)

		if a < 4 {
			continue
		}

		data[i+0] = 255
		data[i+1] = 255
		data[i+2] = 255
		data[i+3] = a
	}

	procSelectObject.Call(tmpDC, oldBmp)

	hiBmp := Bitmap{
		Width:    w,
		Height:   h,
		Channels: 4,
		Data:     data,
	}

	// Downsample 2x → 1x with box filter for smooth anti-aliasing.
	bmp := downsample2x(hiBmp)

	// Apply gamma correction to the downsampled alpha.
	for i := 3; i < len(bmp.Data); i += 4 {
		if bmp.Data[i] > 0 {
			bmp.Data[i] = gammaTable[bmp.Data[i]]
		}
	}

	return bmp, 0, ascent1x
}

// downsample2x reduces a bitmap by half using a box filter on the alpha channel.
func downsample2x(src Bitmap) Bitmap {
	dstW := src.Width / 2
	dstH := src.Height / 2
	if dstW == 0 || dstH == 0 {
		return src // Too small to downsample.
	}

	dst := make([]byte, dstW*dstH*4)
	srcStride := src.Width * 4

	for y := range dstH {
		sy := y * 2
		for x := range dstW {
			sx := x * 2

			// Average alpha from 2x2 block.
			i00 := sy*srcStride + sx*4 + 3
			i10 := sy*srcStride + (sx+1)*4 + 3
			i01 := (sy+1)*srcStride + sx*4 + 3
			i11 := (sy+1)*srcStride + (sx+1)*4 + 3

			sum := uint32(src.Data[i00]) + uint32(src.Data[i10]) +
				uint32(src.Data[i01]) + uint32(src.Data[i11])
			a := byte((sum + 2) / 4) // Rounded average.

			if a > 0 {
				di := (y*dstW + x) * 4
				dst[di+0] = 255
				dst[di+1] = 255
				dst[di+2] = 255
				dst[di+3] = a
			}
		}
	}

	return Bitmap{
		Width:    dstW,
		Height:   dstH,
		Channels: 4,
		Data:     dst,
	}
}

// isEmojiRune returns true if the rune is likely an emoji character.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F300 && r <= 0x1F5FF: // Misc Symbols and Pictographs
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols
		return true
	case r >= 0x1FA00 && r <= 0x1FAFF: // Symbols Extended-A
		return true
	case r >= 0x2600 && r <= 0x26FF: // Misc Symbols
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // Variation Selectors
		return true
	case r == 0x200D: // ZWJ
		return true
	case r >= 0x2300 && r <= 0x23FF: // Misc Technical (includes ⌚⌛)
		return true
	case r == 0x2764 || r == 0x2763: // Hearts
		return true
	case r >= 0x1F000 && r <= 0x1F02F: // Mahjong/Dominos
		return true
	}
	return false
}
