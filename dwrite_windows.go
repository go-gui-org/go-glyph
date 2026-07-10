//go:build windows

package glyph

import (
	"errors"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

// dwrite.dll is loaded lazily. All DirectWrite operations go through
// a single process-wide factory created once per process and
// serialized by mu — IDWriteGlyphRunAnalysis is not documented as
// thread-safe.
var dwriteDLL = syscall.NewLazyDLL("dwrite.dll")
var procDWriteCreateFactory = dwriteDLL.NewProc("DWriteCreateFactory")

// _GUID is a Windows GUID.
type _GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var _IID_IDWriteFactory4 = _GUID{
	0xA5D41E20, 0xF5FD, 0x4E95,
	[8]byte{0x87, 0xA0, 0x90, 0xE1, 0x3D, 0x16, 0x57, 0x76},
}

// DWrite constants.
const (
	_DWRITE_FACTORY_TYPE_SHARED = 0

	_DWRITE_FONT_WEIGHT_NORMAL  = 400
	_DWRITE_FONT_STRETCH_NORMAL = 5
	_DWRITE_FONT_STYLE_NORMAL   = 0

	_DWRITE_RENDERING_MODE_NATURAL_SYMMETRIC = 6
	_DWRITE_MEASURING_MODE_NATURAL           = 0
	_DWRITE_TEXTURE_CLEARTYPE_3x1            = 1

	_DWRITE_E_NOCOLOR = 0x8898500C

	_DWRITE_GLYPH_IMAGE_FORMATS_TRUETYPE               = 1
	_DWRITE_GLYPH_IMAGE_FORMATS_CFF                    = 2
	_DWRITE_GLYPH_IMAGE_FORMATS_COLR                   = 4
	_DWRITE_GLYPH_IMAGE_FORMATS_SVG                    = 8
	_DWRITE_GLYPH_IMAGE_FORMATS_PNG                    = 16
	_DWRITE_GLYPH_IMAGE_FORMATS_JPEG                   = 32
	_DWRITE_GLYPH_IMAGE_FORMATS_TIFF                   = 64
	_DWRITE_GLYPH_IMAGE_FORMATS_PREMULTIPLIED_B8G8R8A8 = 128
	_DWRITE_GLYPH_IMAGE_FORMATS_COLR_PAINT_TREE        = 256
)

// Windows struct types must match the binary layout of their C
// counterparts. Offsets are verified by the smoke test.

type _RECT struct {
	Left, Top, Right, Bottom int32
}

type _DWRITE_COLOR_F struct {
	R, G, B, A float32
}

type _DWRITE_GLYPH_OFFSET struct {
	AdvanceOffset, AscenderOffset float32
}

type _DWRITE_GLYPH_RUN struct {
	FontFace      unsafe.Pointer
	FontEmSize    float32
	GlyphCount    uint32
	GlyphIndices  *uint16
	GlyphAdvances *float32
	GlyphOffsets  *_DWRITE_GLYPH_OFFSET
	IsSideways    int32
	BidiLevel     uint32
}

// _DWRITE_COLOR_GLYPH_RUN1 mirrors the layout expected by the COM
// runtime.  baselineOriginX / baselineOriginY are included because
// MinGW's dwrite_3.h adds them after measuringMode; the binary
// layout and field accesses in the original C code depend on this.
type _DWRITE_COLOR_GLYPH_RUN1 struct {
	GlyphRun         _DWRITE_GLYPH_RUN
	GlyphImageFormat uint32
	MeasuringMode    uint32
	BaselineOriginX  float32
	BaselineOriginY  float32
	RunColor         _DWRITE_COLOR_F
	PaletteIndex     uint16
}

// _maxColorGlyphLayers prevents an unbounded allocation from a
// malformed font that reports an unbounded layer count.
const _maxColorGlyphLayers = 256

// vtblSlot returns the function pointer at the given index in a COM
// interface vtable.
func vtblSlot(p unsafe.Pointer, slot uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(p)
	return *(*uintptr)(unsafe.Add(vtbl, slot*unsafe.Sizeof(uintptr(0))))
}

// failed returns true when hr represents an HRESULT failure code.
func failed(hr uintptr) bool { return int32(hr) < 0 }

// ---------------------------------------------------------------------------
// COM interface method wrappers
// ---------------------------------------------------------------------------
// Each wrapper calls the corresponding vtable slot via syscall.SyscallN.
// Go 1.26's SyscallN copies args to both integer and XMM registers,
// satisfying the MS x64 calling convention for float parameters.

// IDWriteFactory (dwrite.h)
func iDWriteFactory_GetSystemFontCollection(factory unsafe.Pointer,
	collection *unsafe.Pointer, checkForUpdates int32) uintptr {
	fn := vtblSlot(factory, 3)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(factory),
		uintptr(unsafe.Pointer(collection)),
		uintptr(checkForUpdates))
	return r1
}

func iDWriteFactory_CreateGlyphRunAnalysis(factory unsafe.Pointer,
	glyphRun unsafe.Pointer,
	pixelsPerDip float32,
	transform unsafe.Pointer,
	renderingMode, measuringMode uint32,
	baselineOriginX, baselineOriginY float32,
	analysis *unsafe.Pointer,
) uintptr {
	fn := vtblSlot(factory, 23)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(factory),
		uintptr(glyphRun),
		uintptr(math.Float32bits(pixelsPerDip)),
		uintptr(transform),
		uintptr(renderingMode),
		uintptr(measuringMode),
		uintptr(math.Float32bits(baselineOriginX)),
		uintptr(math.Float32bits(baselineOriginY)),
		uintptr(unsafe.Pointer(analysis)),
	)
	return r1
}

// IDWriteFactory4 (dwrite_3.h)
func iDWriteFactory4_TranslateColorGlyphRun(factory unsafe.Pointer,
	baselineOriginX, baselineOriginY float32,
	glyphRun unsafe.Pointer,
	glyphRunDescription unsafe.Pointer,
	desiredFormats uint32,
	measuringMode uint32,
	transform unsafe.Pointer,
	paletteIndex uint32,
	colorEnum *unsafe.Pointer,
) uintptr {
	fn := vtblSlot(factory, 35)
	// baselineOrigin is a D2D1_POINT_2F (two float32, 8 bytes).
	bOrigin := uintptr(math.Float32bits(baselineOriginX)) |
		(uintptr(math.Float32bits(baselineOriginY)) << 32)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(factory),
		bOrigin,
		uintptr(glyphRun),
		uintptr(glyphRunDescription),
		uintptr(desiredFormats),
		uintptr(measuringMode),
		uintptr(transform),
		uintptr(paletteIndex),
		uintptr(unsafe.Pointer(colorEnum)),
	)
	return r1
}

// IDWriteFontCollection
func iDWriteFontCollection_FindFamilyName(collection unsafe.Pointer,
	familyName *uint16, index *uint32, exists *int32) uintptr {
	fn := vtblSlot(collection, 5)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(collection),
		uintptr(unsafe.Pointer(familyName)),
		uintptr(unsafe.Pointer(index)),
		uintptr(unsafe.Pointer(exists)),
	)
	return r1
}

func iDWriteFontCollection_GetFontFamily(collection unsafe.Pointer,
	index uint32, fontFamily *unsafe.Pointer) uintptr {
	fn := vtblSlot(collection, 4)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(collection),
		uintptr(index),
		uintptr(unsafe.Pointer(fontFamily)),
	)
	return r1
}

// IDWriteFontFamily
func iDWriteFontFamily_GetFirstMatchingFont(family unsafe.Pointer,
	weight, stretch, style uint32, font *unsafe.Pointer) uintptr {
	fn := vtblSlot(family, 7)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(family),
		uintptr(weight),
		uintptr(stretch),
		uintptr(style),
		uintptr(unsafe.Pointer(font)),
	)
	return r1
}

// IDWriteFont
func iDWriteFont_CreateFontFace(font unsafe.Pointer,
	fontFace *unsafe.Pointer) uintptr {
	fn := vtblSlot(font, 13)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(font),
		uintptr(unsafe.Pointer(fontFace)),
	)
	return r1
}

// IDWriteFontFace
func iDWriteFontFace_GetGlyphIndices(fontFace unsafe.Pointer,
	codePoints *uint32, codePointCount uint32,
	glyphIndices *uint16) uintptr {
	fn := vtblSlot(fontFace, 11)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(fontFace),
		uintptr(unsafe.Pointer(codePoints)),
		uintptr(codePointCount),
		uintptr(unsafe.Pointer(glyphIndices)),
	)
	return r1
}

// IDWriteGlyphRunAnalysis
func iDWriteGlyphRunAnalysis_GetAlphaTextureBounds(analysis unsafe.Pointer,
	textureType uint32, rect *_RECT) uintptr {
	fn := vtblSlot(analysis, 3)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(analysis),
		uintptr(textureType),
		uintptr(unsafe.Pointer(rect)),
	)
	return r1
}

func iDWriteGlyphRunAnalysis_CreateAlphaTexture(analysis unsafe.Pointer,
	textureType uint32, rect *_RECT,
	alphaValues *byte, bufSize uint32) uintptr {
	fn := vtblSlot(analysis, 4)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(analysis),
		uintptr(textureType),
		uintptr(unsafe.Pointer(rect)),
		uintptr(unsafe.Pointer(alphaValues)),
		uintptr(bufSize),
	)
	return r1
}

// IDWriteColorGlyphRunEnumerator1
func iDWriteColorGlyphRunEnumerator1_MoveNext(enumerator unsafe.Pointer,
	hasRun *int32) uintptr {
	fn := vtblSlot(enumerator, 3)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(enumerator),
		uintptr(unsafe.Pointer(hasRun)),
	)
	return r1
}

func iDWriteColorGlyphRunEnumerator1_GetCurrentRun(enumerator unsafe.Pointer,
	colorGlyphRun *unsafe.Pointer) uintptr {
	fn := vtblSlot(enumerator, 4)
	r1, _, _ := syscall.SyscallN(fn,
		uintptr(enumerator),
		uintptr(unsafe.Pointer(colorGlyphRun)),
	)
	return r1
}

// viRelease calls Release (IUnknown slot 2) on a COM interface.
func viRelease(p unsafe.Pointer) {
	if p == nil {
		return
	}
	fn := vtblSlot(p, 2)
	syscall.SyscallN(fn, uintptr(p))
}

// ---------------------------------------------------------------------------
// dwriteRasterizer
// ---------------------------------------------------------------------------

// dwriteRasterizer renders color (COLR) glyphs via DirectWrite, which
// classic GDI cannot do. Used as the emoji rendering path on Windows.
//
// All DirectWrite operations go through a single process-wide factory
// and are serialized by mu.
type dwriteRasterizer struct {
	mu          sync.Mutex
	factory     unsafe.Pointer // IDWriteFactory4*
	emojiFace   unsafe.Pointer // IDWriteFontFace*
	emojiFaceOK bool
}

// errNoColorGlyph signals that the requested codepoint has no color
// glyph in the Segoe UI Emoji font. The caller should fall back to GDI
// monochrome rendering.
var errNoColorGlyph = errors.New("glyph: no color glyph for codepoint")

// newDWriteRasterizer initializes the DirectWrite factory and preloads
// the Segoe UI Emoji font face. Returns an error on any failure; the
// caller should fall back to GDI-only rendering in that case.
func newDWriteRasterizer() (*dwriteRasterizer, error) {
	var factory unsafe.Pointer
	hr, _, _ := procDWriteCreateFactory.Call(
		_DWRITE_FACTORY_TYPE_SHARED,
		uintptr(unsafe.Pointer(&_IID_IDWriteFactory4)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if failed(hr) || factory == nil {
		return nil, errors.New("glyph: DirectWrite initialization failed")
	}

	var sysCollection unsafe.Pointer
	hr = iDWriteFactory_GetSystemFontCollection(factory, &sysCollection, 0)
	if failed(hr) || sysCollection == nil {
		viRelease(factory)
		return nil, errors.New("glyph: DirectWrite initialization failed")
	}
	defer viRelease(sysCollection)

	emojiName := syscall.StringToUTF16("Segoe UI Emoji")
	var idx uint32
	var exists int32
	hr = iDWriteFontCollection_FindFamilyName(sysCollection,
		&emojiName[0], &idx, &exists)
	if failed(hr) || exists == 0 {
		viRelease(factory)
		return nil, errors.New("glyph: DirectWrite initialization failed")
	}

	var fam unsafe.Pointer
	hr = iDWriteFontCollection_GetFontFamily(sysCollection, idx, &fam)
	if failed(hr) || fam == nil {
		viRelease(factory)
		return nil, errors.New("glyph: DirectWrite initialization failed")
	}
	defer viRelease(fam)

	var font unsafe.Pointer
	hr = iDWriteFontFamily_GetFirstMatchingFont(fam,
		_DWRITE_FONT_WEIGHT_NORMAL,
		_DWRITE_FONT_STRETCH_NORMAL,
		_DWRITE_FONT_STYLE_NORMAL,
		&font)
	if failed(hr) || font == nil {
		viRelease(factory)
		return nil, errors.New("glyph: DirectWrite initialization failed")
	}
	defer viRelease(font)

	var face unsafe.Pointer
	hr = iDWriteFont_CreateFontFace(font, &face)
	if failed(hr) || face == nil {
		viRelease(factory)
		return nil, errors.New("glyph: DirectWrite initialization failed")
	}

	return &dwriteRasterizer{
		factory:     factory,
		emojiFace:   face,
		emojiFaceOK: true,
	}, nil
}

// Close releases all DirectWrite COM objects.
func (d *dwriteRasterizer) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.emojiFaceOK {
		viRelease(d.emojiFace)
		d.emojiFaceOK = false
	}
	if d.factory != nil {
		viRelease(d.factory)
		d.factory = nil
	}
}

// RenderColorGlyph rasterizes a single color glyph. emSizePx is the
// font em size in physical pixels (already scaled by DPI). The
// returned Bitmap holds RGBA premultiplied pixels and uses the same
// bearing convention as the FreeType/GDI paths (left = pen-X left
// edge, top = baseline upward, positive = above baseline).
func (d *dwriteRasterizer) RenderColorGlyph(
	emSizePx float32, codepoint rune,
) (Bitmap, int, int, error) {
	if d == nil || !d.emojiFaceOK || d.factory == nil {
		return Bitmap{}, 0, 0, errors.New("glyph: DirectWrite not initialized")
	}
	if emSizePx <= 0 || math.IsNaN(float64(emSizePx)) {
		return Bitmap{}, 0, 0, errNoColorGlyph
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	cp := uint32(codepoint)
	var glyphIndex uint16
	hr := iDWriteFontFace_GetGlyphIndices(d.emojiFace, &cp, 1, &glyphIndex)
	if failed(hr) || glyphIndex == 0 {
		return Bitmap{}, 0, 0, errNoColorGlyph
	}

	var advance float32
	glyphRun := _DWRITE_GLYPH_RUN{
		FontFace:      d.emojiFace,
		FontEmSize:    emSizePx,
		GlyphCount:    1,
		GlyphIndices:  &glyphIndex,
		GlyphAdvances: &advance,
		GlyphOffsets:  &_DWRITE_GLYPH_OFFSET{},
		BidiLevel:     0,
	}

	desiredFormats := uint32(
		_DWRITE_GLYPH_IMAGE_FORMATS_TRUETYPE |
			_DWRITE_GLYPH_IMAGE_FORMATS_CFF |
			_DWRITE_GLYPH_IMAGE_FORMATS_COLR |
			_DWRITE_GLYPH_IMAGE_FORMATS_SVG |
			_DWRITE_GLYPH_IMAGE_FORMATS_PNG |
			_DWRITE_GLYPH_IMAGE_FORMATS_JPEG |
			_DWRITE_GLYPH_IMAGE_FORMATS_TIFF |
			_DWRITE_GLYPH_IMAGE_FORMATS_PREMULTIPLIED_B8G8R8A8)

	var colorEnum unsafe.Pointer
	hr = iDWriteFactory4_TranslateColorGlyphRun(d.factory,
		0, 0, // baselineOrigin
		unsafe.Pointer(&glyphRun), nil,
		desiredFormats,
		_DWRITE_MEASURING_MODE_NATURAL,
		nil, 0,
		&colorEnum)
	if hr == _DWRITE_E_NOCOLOR {
		return Bitmap{}, 0, 0, errNoColorGlyph
	}
	if failed(hr) || colorEnum == nil {
		return Bitmap{}, 0, 0, errors.New("glyph: DirectWrite rasterization failed")
	}
	defer viRelease(colorEnum)

	// Layer accumulation.
	type layer struct {
		analysis unsafe.Pointer // IDWriteGlyphRunAnalysis*
		bounds   _RECT
		color    _DWRITE_COLOR_F
	}
	var layers []layer
	var unionSet bool
	var unionL, unionT, unionR, unionB int32

	for {
		var hasRun int32
		hr = iDWriteColorGlyphRunEnumerator1_MoveNext(colorEnum, &hasRun)
		if failed(hr) || hasRun == 0 {
			break
		}

		var colorRun unsafe.Pointer
		hr = iDWriteColorGlyphRunEnumerator1_GetCurrentRun(colorEnum, &colorRun)
		if failed(hr) || colorRun == nil {
			continue
		}
		cr := (*_DWRITE_COLOR_GLYPH_RUN1)(colorRun)

		if cr.GlyphRun.GlyphCount == 0 {
			continue
		}

		// Skip bitmap / SVG / paint-tree layers we cannot rasterise
		// via CreateGlyphRunAnalysis.
		fmt := cr.GlyphImageFormat
		if fmt&(_DWRITE_GLYPH_IMAGE_FORMATS_PNG|
			_DWRITE_GLYPH_IMAGE_FORMATS_JPEG|
			_DWRITE_GLYPH_IMAGE_FORMATS_TIFF|
			_DWRITE_GLYPH_IMAGE_FORMATS_PREMULTIPLIED_B8G8R8A8|
			_DWRITE_GLYPH_IMAGE_FORMATS_SVG|
			_DWRITE_GLYPH_IMAGE_FORMATS_COLR_PAINT_TREE) != 0 {
			continue
		}

		var ana unsafe.Pointer
		grp := unsafe.Pointer(&cr.GlyphRun)
		hr = iDWriteFactory_CreateGlyphRunAnalysis(d.factory,
			grp,
			1.0, // pixelsPerDip
			nil, // transform
			_DWRITE_RENDERING_MODE_NATURAL_SYMMETRIC,
			_DWRITE_MEASURING_MODE_NATURAL,
			cr.BaselineOriginX,
			cr.BaselineOriginY,
			&ana)
		if failed(hr) || ana == nil {
			continue
		}

		var rc _RECT
		hr = iDWriteGlyphRunAnalysis_GetAlphaTextureBounds(ana,
			_DWRITE_TEXTURE_CLEARTYPE_3x1, &rc)
		if failed(hr) || rc.Right <= rc.Left || rc.Bottom <= rc.Top {
			viRelease(ana)
			continue
		}

		layers = append(layers, layer{analysis: ana, bounds: rc, color: cr.RunColor})
		if len(layers) >= _maxColorGlyphLayers {
			break
		}

		if !unionSet {
			unionL, unionT, unionR, unionB = rc.Left, rc.Top, rc.Right, rc.Bottom
			unionSet = true
		} else {
			if rc.Left < unionL {
				unionL = rc.Left
			}
			if rc.Top < unionT {
				unionT = rc.Top
			}
			if rc.Right > unionR {
				unionR = rc.Right
			}
			if rc.Bottom > unionB {
				unionB = rc.Bottom
			}
		}
	}

	if !unionSet || len(layers) == 0 {
		for _, l := range layers {
			viRelease(l.analysis)
		}
		return Bitmap{}, 0, 0, errNoColorGlyph
	}

	outW := int(unionR - unionL)
	outH := int(unionB - unionT)
	if outW <= 0 || outH <= 0 || outW > 4096 || outH > 4096 {
		for _, l := range layers {
			viRelease(l.analysis)
		}
		return Bitmap{}, 0, 0, errors.New("glyph: DirectWrite rasterization failed")
	}

	pixels := make([]byte, outW*outH*4)

	// Composite each layer onto the accumulator in premultiplied BGRA.
	for _, L := range layers {
		lw := int(L.bounds.Right - L.bounds.Left)
		lh := int(L.bounds.Bottom - L.bounds.Top)
		if lw <= 0 || lh <= 0 {
			continue
		}

		bufSize := uint32(lw * lh * 3)
		alphaBuf := make([]byte, bufSize)

		hr = iDWriteGlyphRunAnalysis_CreateAlphaTexture(L.analysis,
			_DWRITE_TEXTURE_CLEARTYPE_3x1, &L.bounds,
			&alphaBuf[0], bufSize)
		if failed(hr) {
			continue
		}

		cr := L.color.R
		cg := L.color.G
		cb := L.color.B
		ca := L.color.A
		if ca <= 0 && cr <= 0 && cg <= 0 && cb <= 0 {
			cr, cg, cb, ca = 1, 1, 1, 1
		}
		if ca < 0 || ca > 1 || math.IsNaN(float64(ca)) || math.IsInf(float64(ca), 0) {
			ca = 1
		}
		if math.IsNaN(float64(cr)) || math.IsInf(float64(cr), 0) {
			cr = 0
		}
		if math.IsNaN(float64(cg)) || math.IsInf(float64(cg), 0) {
			cg = 0
		}
		if math.IsNaN(float64(cb)) || math.IsInf(float64(cb), 0) {
			cb = 0
		}

		offX := int(L.bounds.Left - unionL)
		offY := int(L.bounds.Top - unionT)

		for py := 0; py < lh; py++ {
			dy := py + offY
			if dy < 0 || dy >= outH {
				continue
			}
			srcRow := alphaBuf[py*lw*3:]
			dstRow := pixels[(dy*outW+offX)*4:]
			for px := 0; px < lw; px++ {
				a8 := (uint32(srcRow[px*3+0]) +
					uint32(srcRow[px*3+1]) +
					uint32(srcRow[px*3+2])) / 3
				if a8 == 0 {
					continue
				}

				pa := (float32(a8) / 255.0) * ca
				srcA := int(pa*255.0 + 0.5)
				srcB := int(pa*cb*255.0 + 0.5)
				srcG := int(pa*cg*255.0 + 0.5)
				srcR := int(pa*cr*255.0 + 0.5)
				if srcA > 255 {
					srcA = 255
				}
				if srcB > 255 {
					srcB = 255
				}
				if srcG > 255 {
					srcG = 255
				}
				if srcR > 255 {
					srcR = 255
				}

				dp := dstRow[px*4:]
				invA := 255 - srcA
				nB := srcB + (int(dp[0])*invA+127)/255
				nG := srcG + (int(dp[1])*invA+127)/255
				nR := srcR + (int(dp[2])*invA+127)/255
				nA := srcA + (int(dp[3])*invA+127)/255
				if nB > 255 {
					nB = 255
				}
				if nG > 255 {
					nG = 255
				}
				if nR > 255 {
					nR = 255
				}
				if nA > 255 {
					nA = 255
				}
				dp[0] = byte(nB)
				dp[1] = byte(nG)
				dp[2] = byte(nR)
				dp[3] = byte(nA)
			}
		}
	}

	for _, L := range layers {
		viRelease(L.analysis)
	}

	// Swizzle BGRA -> RGBA for the consumer.
	data := make([]byte, len(pixels))
	for i := 0; i < len(pixels); i += 4 {
		data[i+0] = pixels[i+2] // R
		data[i+1] = pixels[i+1] // G
		data[i+2] = pixels[i+0] // B
		data[i+3] = pixels[i+3] // A
	}

	return Bitmap{
		Width:    outW,
		Height:   outH,
		Channels: 4,
		Data:     data,
	}, int(unionL), -int(unionT), nil
}
