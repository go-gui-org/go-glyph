//go:build !js && !android && !windows && (!darwin || glyph_pango) && (!linux || glyph_pango)

package glyph

/*
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#include <pango/pango.h>
#include <pango/pangoft2.h>
#include <pango/pangofc-font.h>
#include <pango/pangofc-fontmap.h>
#include <glib-object.h>
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_STROKER_H
#include FT_GLYPH_H
#include <fontconfig/fontconfig.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// --- FreeType RAII wrappers ---

// FTLibrary wraps FT_Library with explicit Close.
type FTLibrary struct {
	ptr C.FT_Library
}

// InitFreeType initializes a new FreeType library instance.
func InitFreeType() (FTLibrary, error) {
	var lib C.FT_Library
	if C.FT_Init_FreeType(&lib) != 0 {
		return FTLibrary{}, fmt.Errorf("FT_Init_FreeType failed")
	}
	return FTLibrary{ptr: lib}, nil
}

// Close releases the FreeType library.
func (l *FTLibrary) Close() {
	if l.ptr != nil {
		C.FT_Done_FreeType(l.ptr)
		l.ptr = nil
	}
}

// Ptr returns the raw C pointer.
func (l *FTLibrary) Ptr() C.FT_Library { return l.ptr }

// FTFace wraps FT_Face.
type FTFace struct {
	ptr C.FT_Face
}

// FacePtr returns the raw FT_Face as unsafe.Pointer for storage in Item.
func (f *FTFace) FacePtr() unsafe.Pointer { return unsafe.Pointer(f.ptr) }

// FTStroker wraps FT_Stroker with explicit Close.
type FTStroker struct {
	ptr C.FT_Stroker
}

// NewFTStroker creates a new FreeType stroker.
func NewFTStroker(lib FTLibrary) (FTStroker, error) {
	var s C.FT_Stroker
	if C.FT_Stroker_New(lib.ptr, &s) != 0 {
		return FTStroker{}, fmt.Errorf("FT_Stroker_New failed")
	}
	return FTStroker{ptr: s}, nil
}

// Close releases the stroker.
func (s *FTStroker) Close() {
	if s.ptr != nil {
		C.FT_Stroker_Done(s.ptr)
		s.ptr = nil
	}
}

// --- Pango RAII wrappers ---

// PangoFontMapW wraps *PangoFontMap (the W suffix avoids conflict
// with the C type).
type PangoFontMapW struct {
	ptr *C.PangoFontMap
}

// NewPangoFT2FontMap creates a new Pango FT2 font map.
func NewPangoFT2FontMap() PangoFontMapW {
	return PangoFontMapW{ptr: C.pango_ft2_font_map_new()}
}

// SetResolution sets the DPI resolution on the font map.
func (m PangoFontMapW) SetResolution(dpiX, dpiY float64) {
	C.pango_ft2_font_map_set_resolution(
		(*C.PangoFT2FontMap)(unsafe.Pointer(m.ptr)),
		C.double(dpiX), C.double(dpiY),
	)
}

// CreateContext creates a PangoContext from this font map.
func (m PangoFontMapW) CreateContext() PangoContextW {
	return PangoContextW{ptr: C.pango_font_map_create_context(m.ptr)}
}

// Close unrefs the font map.
func (m *PangoFontMapW) Close() {
	if m.ptr != nil {
		C.g_object_unref(C.gpointer(unsafe.Pointer(m.ptr)))
		m.ptr = nil
	}
}

// PangoContextW wraps *PangoContext.
type PangoContextW struct {
	ptr *C.PangoContext
}

// Close unrefs the Pango context.
func (c *PangoContextW) Close() {
	if c.ptr != nil {
		C.g_object_unref(C.gpointer(unsafe.Pointer(c.ptr)))
		c.ptr = nil
	}
}

// Ptr returns the raw C pointer.
func (c *PangoContextW) Ptr() *C.PangoContext { return c.ptr }

// PangoLayoutW wraps *PangoLayout with RAII.
type PangoLayoutW struct {
	ptr *C.PangoLayout
}

// NewPangoLayout creates a new PangoLayout for the given context.
func NewPangoLayout(ctx PangoContextW) PangoLayoutW {
	return PangoLayoutW{ptr: C.pango_layout_new(ctx.ptr)}
}

// SetText sets the layout text.
func (l PangoLayoutW) SetText(text string) {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.pango_layout_set_text(l.ptr, cs, C.int(len(text)))
}

// SetMarkup sets the layout text with Pango markup.
func (l PangoLayoutW) SetMarkup(text string) {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.pango_layout_set_markup(l.ptr, cs, C.int(len(text)))
}

// SetWidth sets the layout width in Pango units (-1 = no wrap).
func (l PangoLayoutW) SetWidth(width int) {
	C.pango_layout_set_width(l.ptr, C.int(width))
}

// SetWrap sets the wrap mode.
func (l PangoLayoutW) SetWrap(mode WrapMode) {
	C.pango_layout_set_wrap(l.ptr, C.PangoWrapMode(mode))
}

// SetAlignment sets horizontal alignment.
func (l PangoLayoutW) SetAlignment(align Alignment) {
	C.pango_layout_set_alignment(l.ptr, C.PangoAlignment(align))
}

// SetIndent sets the first-line indent in Pango units.
func (l PangoLayoutW) SetIndent(indent int) {
	C.pango_layout_set_indent(l.ptr, C.int(indent))
}

// SetFontDescription sets the font for the layout.
func (l PangoLayoutW) SetFontDescription(desc PangoFontDescW) {
	C.pango_layout_set_font_description(l.ptr, desc.ptr)
}

// SetAttributes sets the attribute list.
func (l PangoLayoutW) SetAttributes(attrs PangoAttrListW) {
	C.pango_layout_set_attributes(l.ptr, attrs.ptr)
}

// SetTabs sets custom tab stops.
func (l PangoLayoutW) SetTabs(tabs PangoTabArrayW) {
	C.pango_layout_set_tabs(l.ptr, tabs.ptr)
}

// GetIter returns a layout iterator.
func (l PangoLayoutW) GetIter() PangoLayoutIterW {
	return PangoLayoutIterW{ptr: C.pango_layout_get_iter(l.ptr)}
}

// GetExtents returns ink and logical extents.
func (l PangoLayoutW) GetExtents() (ink, logical C.PangoRectangle) {
	C.pango_layout_get_extents(l.ptr, &ink, &logical)
	return
}

// Ptr returns the raw C pointer.
func (l PangoLayoutW) Ptr() *C.PangoLayout { return l.ptr }

// Close unrefs the layout.
func (l *PangoLayoutW) Close() {
	if l.ptr != nil {
		C.g_object_unref(C.gpointer(unsafe.Pointer(l.ptr)))
		l.ptr = nil
	}
}

// PangoFontDescW wraps *PangoFontDescription.
type PangoFontDescW struct {
	ptr *C.PangoFontDescription
}

// NewPangoFontDescFromString parses a Pango font description string.
func NewPangoFontDescFromString(desc string) PangoFontDescW {
	cs := C.CString(desc)
	defer C.free(unsafe.Pointer(cs))
	return PangoFontDescW{ptr: C.pango_font_description_from_string(cs)}
}

// SetSize sets the font size in Pango units.
func (d PangoFontDescW) SetSize(size int) {
	C.pango_font_description_set_size(d.ptr, C.gint(size))
}

// SetWeight sets the font weight.
func (d PangoFontDescW) SetWeight(weight int) {
	C.pango_font_description_set_weight(d.ptr, C.PangoWeight(weight))
}

// SetStyle sets the font style (normal/italic/oblique).
func (d PangoFontDescW) SetStyle(style int) {
	C.pango_font_description_set_style(d.ptr, C.PangoStyle(style))
}

// SetVariations sets font variation axes string.
func (d PangoFontDescW) SetVariations(variations string) {
	cs := C.CString(variations)
	defer C.free(unsafe.Pointer(cs))
	C.pango_font_description_set_variations(d.ptr, cs)
}

// Close frees the font description.
func (d *PangoFontDescW) Close() {
	if d.ptr != nil {
		C.pango_font_description_free(d.ptr)
		d.ptr = nil
	}
}

// PangoAttrListW wraps *PangoAttrList.
type PangoAttrListW struct {
	ptr *C.PangoAttrList
}

// NewPangoAttrList creates a new attribute list.
func NewPangoAttrList() PangoAttrListW {
	return PangoAttrListW{ptr: C.pango_attr_list_new()}
}

// Insert adds an attribute to the list. The list takes ownership.
func (a PangoAttrListW) Insert(attr *C.PangoAttribute) {
	C.pango_attr_list_insert(a.ptr, attr)
}

// Close unrefs the attribute list.
func (a *PangoAttrListW) Close() {
	if a.ptr != nil {
		C.pango_attr_list_unref(a.ptr)
		a.ptr = nil
	}
}

// PangoLayoutIterW wraps *PangoLayoutIter.
type PangoLayoutIterW struct {
	ptr *C.PangoLayoutIter
}

// NextRun advances to the next run. Returns false at end.
func (it PangoLayoutIterW) NextRun() bool {
	return C.pango_layout_iter_next_run(it.ptr) != 0
}

// GetRunReadonly returns the current run (may be nil for empty runs).
func (it PangoLayoutIterW) GetRunReadonly() *C.PangoGlyphItem {
	return C.pango_layout_iter_get_run_readonly(it.ptr)
}

// GetBaseline returns the current baseline in Pango units.
func (it PangoLayoutIterW) GetBaseline() int {
	return int(C.pango_layout_iter_get_baseline(it.ptr))
}

// GetRunExtents returns ink and logical extents of the current run.
func (it PangoLayoutIterW) GetRunExtents() (ink, logical C.PangoRectangle) {
	C.pango_layout_iter_get_run_extents(it.ptr, &ink, &logical)
	return
}

// NextLine advances to the next line.
func (it PangoLayoutIterW) NextLine() bool {
	return C.pango_layout_iter_next_line(it.ptr) != 0
}

// GetLineReadonly returns the current layout line.
func (it PangoLayoutIterW) GetLineReadonly() *C.PangoLayoutLine {
	return C.pango_layout_iter_get_line_readonly(it.ptr)
}

// GetLineExtents returns ink and logical extents of the current line.
func (it PangoLayoutIterW) GetLineExtents() (ink, logical C.PangoRectangle) {
	C.pango_layout_iter_get_line_extents(it.ptr, &ink, &logical)
	return
}

// NextChar advances to the next character.
func (it PangoLayoutIterW) NextChar() bool {
	return C.pango_layout_iter_next_char(it.ptr) != 0
}

// GetIndex returns the byte index of the current iterator position.
func (it PangoLayoutIterW) GetIndex() int {
	return int(C.pango_layout_iter_get_index(it.ptr))
}

// GetCharExtents returns the logical extents of the current character.
func (it PangoLayoutIterW) GetCharExtents() C.PangoRectangle {
	var r C.PangoRectangle
	C.pango_layout_iter_get_char_extents(it.ptr, &r)
	return r
}

// Close frees the iterator.
func (it *PangoLayoutIterW) Close() {
	if it.ptr != nil {
		C.pango_layout_iter_free(it.ptr)
		it.ptr = nil
	}
}

// PangoTabArrayW wraps *PangoTabArray.
type PangoTabArrayW struct {
	ptr *C.PangoTabArray
}

// NewPangoTabArray creates a tab array with the given number of stops.
func NewPangoTabArray(size int) PangoTabArrayW {
	return PangoTabArrayW{ptr: C.pango_tab_array_new(C.gint(size), C.TRUE)}
}

// SetTab sets a tab stop position in pixels.
func (t PangoTabArrayW) SetTab(index, position int) {
	C.pango_tab_array_set_tab(t.ptr, C.gint(index), C.PANGO_TAB_LEFT, C.gint(position))
}

// Close frees the tab array.
func (t *PangoTabArrayW) Close() {
	if t.ptr != nil {
		C.pango_tab_array_free(t.ptr)
		t.ptr = nil
	}
}

// --- FontConfig helpers ---

// FCConfigAppFontAddFile registers a font file with FontConfig.
func FCConfigAppFontAddFile(path string) bool {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	config := C.FcConfigGetCurrent()
	return C.FcConfigAppFontAddFile(config, (*C.FcChar8)(unsafe.Pointer(cs))) != 0
}

// FCConfigAppFontAddDir registers a font directory with FontConfig.
func FCConfigAppFontAddDir(dir string) bool {
	cs := C.CString(dir)
	defer C.free(unsafe.Pointer(cs))
	config := C.FcConfigGetCurrent()
	return C.FcConfigAppFontAddDir(config, (*C.FcChar8)(unsafe.Pointer(cs))) != 0
}

// PangoFCFontMapConfigChanged notifies the font map that config changed.
func PangoFCFontMapConfigChanged(fontMap PangoFontMapW) {
	C.pango_fc_font_map_config_changed((*C.PangoFcFontMap)(unsafe.Pointer(fontMap.ptr)))
}

// PangoFCFontLockFace extracts the FT_Face from a PangoFont.
// Caller must call PangoFCFontUnlockFace when done.
func PangoFCFontLockFace(font *C.PangoFont) unsafe.Pointer {
	face := C.pango_fc_font_lock_face((*C.PangoFcFont)(unsafe.Pointer(font)))
	return unsafe.Pointer(face)
}

// PangoFCFontUnlockFace releases the FT_Face lock.
func PangoFCFontUnlockFace(font *C.PangoFont) {
	C.pango_fc_font_unlock_face((*C.PangoFcFont)(unsafe.Pointer(font)))
}

// --- Pango attribute constructors ---

// PangoAttrForegroundNew creates a foreground color attribute.
// Colors are 16-bit (0–65535).
func PangoAttrForegroundNew(r, g, b uint16) *C.PangoAttribute {
	return C.pango_attr_foreground_new(C.guint16(r), C.guint16(g), C.guint16(b))
}

// PangoAttrBackgroundNew creates a background color attribute.
func PangoAttrBackgroundNew(r, g, b uint16) *C.PangoAttribute {
	return C.pango_attr_background_new(C.guint16(r), C.guint16(g), C.guint16(b))
}

// PangoAttrUnderlineNew creates an underline attribute.
func PangoAttrUnderlineNew(style int) *C.PangoAttribute {
	return C.pango_attr_underline_new(C.PangoUnderline(style))
}

// PangoAttrStrikethroughNew creates a strikethrough attribute.
func PangoAttrStrikethroughNew(strikethrough bool) *C.PangoAttribute {
	b := C.gboolean(0)
	if strikethrough {
		b = C.gboolean(1)
	}
	return C.pango_attr_strikethrough_new(b)
}

// PangoAttrLetterSpacingNew creates a letter-spacing attribute (Pango units).
func PangoAttrLetterSpacingNew(spacing int) *C.PangoAttribute {
	return C.pango_attr_letter_spacing_new(C.int(spacing))
}

// PangoAttrFontFeaturesNew creates an OpenType font features attribute.
func PangoAttrFontFeaturesNew(features string) *C.PangoAttribute {
	cs := C.CString(features)
	defer C.free(unsafe.Pointer(cs))
	return C.pango_attr_font_features_new(cs)
}

// --- Pango font helpers ---

// PangoFontW wraps *PangoFont.
type PangoFontW struct {
	ptr *C.PangoFont
}

// Close unrefs the font.
func (f *PangoFontW) Close() {
	if f.ptr != nil {
		C.g_object_unref(C.gpointer(unsafe.Pointer(f.ptr)))
		f.ptr = nil
	}
}

// PangoFontMetricsW wraps *PangoFontMetrics.
type PangoFontMetricsW struct {
	ptr *C.PangoFontMetrics
}

// Close unrefs the metrics.
func (m *PangoFontMetricsW) Close() {
	if m.ptr != nil {
		C.pango_font_metrics_unref(m.ptr)
		m.ptr = nil
	}
}

// PangoContextLoadFont loads a font matching the description.
func PangoContextLoadFont(ctx *C.PangoContext, desc *C.PangoFontDescription) PangoFontW {
	return PangoFontW{ptr: C.pango_context_load_font(ctx, desc)}
}

// PangoFontGetMetrics returns metrics for the font and language.
func PangoFontGetMetrics(font *C.PangoFont, lang *C.PangoLanguage) PangoFontMetricsW {
	return PangoFontMetricsW{ptr: C.pango_font_get_metrics(font, lang)}
}

// PangoGetDefaultLanguage returns the default language.
func PangoGetDefaultLanguage() *C.PangoLanguage {
	return C.pango_language_get_default()
}

// PangoFontMetricsGetAscent returns ascent in Pango units.
func PangoFontMetricsGetAscent(m *C.PangoFontMetrics) int {
	return int(C.pango_font_metrics_get_ascent(m))
}

// PangoFontMetricsGetDescent returns descent in Pango units.
func PangoFontMetricsGetDescent(m *C.PangoFontMetrics) int {
	return int(C.pango_font_metrics_get_descent(m))
}

// PangoFontDescGetSize returns the size in Pango units.
func PangoFontDescGetSize(desc *C.PangoFontDescription) int {
	return int(C.pango_font_description_get_size(desc))
}

// PangoFT2FontGetFace returns the FT_Face from a PangoFont.
// Deprecated but needed for cache key generation.
func PangoFT2FontGetFace(font *C.PangoFont) C.FT_Face {
	return C.pango_ft2_font_get_face(font)
}

// PangoFontDescNew creates a new empty font description.
func PangoFontDescNew() PangoFontDescW {
	return PangoFontDescW{ptr: C.pango_font_description_new()}
}

// PangoFontDescGetFamily returns the font family name.
func PangoFontDescGetFamily(desc *C.PangoFontDescription) string {
	fam := C.pango_font_description_get_family(desc)
	if fam == nil {
		return ""
	}
	return C.GoString(fam)
}

// PangoFontDescSetFamily sets the font family.
func PangoFontDescSetFamily(desc *C.PangoFontDescription, family string) {
	cs := C.CString(family)
	defer C.free(unsafe.Pointer(cs))
	C.pango_font_description_set_family(desc, cs)
}

// PangoContextSetBaseGravity sets the base gravity for the context.
func PangoContextSetBaseGravity(ctx *C.PangoContext) {
	C.pango_context_set_base_gravity(ctx, C.PANGO_GRAVITY_SOUTH)
	C.pango_context_set_gravity_hint(ctx, C.PANGO_GRAVITY_HINT_NATURAL)
	C.pango_context_set_matrix(ctx, nil)
	C.pango_context_changed(ctx)
}

// PangoAttrListCopy copies a PangoAttrList.
func PangoAttrListCopy(list *C.PangoAttrList) PangoAttrListW {
	return PangoAttrListW{ptr: C.pango_attr_list_copy(list)}
}

// PangoLayoutGetAttributes returns the attribute list from a layout.
func PangoLayoutGetAttributes(layout *C.PangoLayout) *C.PangoAttrList {
	return C.pango_layout_get_attributes(layout)
}

// PangoLayoutGetFontDescription returns the font description of a layout.
func PangoLayoutGetFontDescription(layout *C.PangoLayout) *C.PangoFontDescription {
	return C.pango_layout_get_font_description(layout)
}

// PangoLayoutGetContext returns the context from a layout.
func PangoLayoutGetContext(layout *C.PangoLayout) *C.PangoContext {
	return C.pango_layout_get_context(layout)
}

// PangoContextGetMetrics returns metrics for a font description.
func PangoContextGetMetrics(ctx *C.PangoContext, desc *C.PangoFontDescription, lang *C.PangoLanguage) *C.PangoFontMetrics {
	return C.pango_context_get_metrics(ctx, desc, lang)
}

// PangoFontMetricsGetStrikethroughPosition returns position in Pango units.
func PangoFontMetricsGetStrikethroughPosition(m *C.PangoFontMetrics) int {
	return int(C.pango_font_metrics_get_strikethrough_position(m))
}

// PangoFontMetricsGetStrikethroughThickness returns thickness in Pango units.
func PangoFontMetricsGetStrikethroughThickness(m *C.PangoFontMetrics) int {
	return int(C.pango_font_metrics_get_strikethrough_thickness(m))
}

// PangoFontMetricsGetUnderlinePosition returns position in Pango units.
func PangoFontMetricsGetUnderlinePosition(m *C.PangoFontMetrics) int {
	return int(C.pango_font_metrics_get_underline_position(m))
}

// PangoFontMetricsGetUnderlineThickness returns thickness in Pango units.
func PangoFontMetricsGetUnderlineThickness(m *C.PangoFontMetrics) int {
	return int(C.pango_font_metrics_get_underline_thickness(m))
}

// PangoLayoutGetLineCount returns the number of lines.
func PangoLayoutGetLineCount(layout *C.PangoLayout) int {
	return int(C.pango_layout_get_line_count(layout))
}

// PangoLayoutGetLogAttrsReadonly returns readonly log attrs and count.
func PangoLayoutGetLogAttrsReadonly(layout *C.PangoLayout) (*C.PangoLogAttr, int) {
	var n C.gint
	ptr := C.pango_layout_get_log_attrs_readonly(layout, &n)
	return ptr, int(n)
}

// PangoAttrFontDescNew creates a font description attribute.
func PangoAttrFontDescNew(desc *C.PangoFontDescription) *C.PangoAttribute {
	return C.pango_attr_font_desc_new(desc)
}

// PangoAttrShapeNew creates a shape attribute (for inline objects).
func PangoAttrShapeNew(ink, logical *C.PangoRectangle) *C.PangoAttribute {
	return C.pango_attr_shape_new(ink, logical)
}

// getFontFamilyName extracts the family name from an FT_Face
// stored as unsafe.Pointer.
func getFontFamilyName(facePtr unsafe.Pointer) string {
	if facePtr == nil {
		return "Unknown"
	}
	face := (C.FT_Face)(facePtr)
	if face.family_name == nil {
		return "Unknown"
	}
	return C.GoString(face.family_name)
}
