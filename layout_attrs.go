//go:build !js && !android && !windows && (!darwin || glyph_pango)

package glyph

/*
#include <pango/pango.h>
#include <pango/pangoft2.h>
#include <stdlib.h>
#include <string.h>

// C callbacks for pango_attr_shape_new_with_data so Pango
// copies/frees the null-terminated object ID string correctly.
gpointer shape_data_copy(gconstpointer src) {
    return g_strdup((const gchar *)src);
}
void shape_data_destroy(gpointer data) {
    g_free(data);
}
*/
import "C"
import (
	"fmt"
	"strings"
	"unsafe"
)

// runAttributes holds parsed visual properties from Pango run attributes.
type runAttributes struct {
	Color            Color
	BgColor          Color
	HasBgColor       bool
	HasUnderline     bool
	HasStrikethrough bool
	IsObject         bool
	ObjectID         string
}

// parseRunAttributes extracts visual properties from a PangoItem's
// attribute list in a single pass.
func parseRunAttributes(pangoItem *C.PangoItem) runAttributes {
	attrs := runAttributes{
		// Default transparent = "no color attribute".
		Color:   Color{0, 0, 0, 0},
		BgColor: Color{0, 0, 0, 0},
	}

	node := pangoItem.analysis.extra_attrs
	for node != nil {
		attr := (*C.PangoAttribute)(node.data)
		attrType := attr.klass._type

		switch attrType {
		case C.PANGO_ATTR_FOREGROUND:
			ca := (*C.PangoAttrColor)(unsafe.Pointer(attr))
			attrs.Color = Color{
				R: uint8(ca.color.red >> 8),
				G: uint8(ca.color.green >> 8),
				B: uint8(ca.color.blue >> 8),
				A: 255,
			}
		case C.PANGO_ATTR_BACKGROUND:
			ca := (*C.PangoAttrColor)(unsafe.Pointer(attr))
			attrs.HasBgColor = true
			attrs.BgColor = Color{
				R: uint8(ca.color.red >> 8),
				G: uint8(ca.color.green >> 8),
				B: uint8(ca.color.blue >> 8),
				A: 255,
			}
		case C.PANGO_ATTR_UNDERLINE:
			ia := (*C.PangoAttrInt)(unsafe.Pointer(attr))
			if ia.value != C.int(C.PANGO_UNDERLINE_NONE) {
				attrs.HasUnderline = true
			}
		case C.PANGO_ATTR_STRIKETHROUGH:
			ia := (*C.PangoAttrInt)(unsafe.Pointer(attr))
			if ia.value != 0 {
				attrs.HasStrikethrough = true
			}
		case C.PANGO_ATTR_SHAPE:
			sa := (*C.PangoAttrShape)(unsafe.Pointer(attr))
			if sa.data != nil {
				attrs.IsObject = true
				attrs.ObjectID = C.GoString((*C.char)(sa.data))
			}
		}

		node = node.next
	}
	return attrs
}

// applyRichTextStyle inserts per-run Pango attributes into list
// for the byte range [start, end). Caller retains ownership of list.
func applyRichTextStyle(ctx *Context, list PangoAttrListW, style TextStyle,
	start, end int, clonedIDs *[]string) {

	// Foreground color.
	if style.Color.A > 0 {
		attr := C.pango_attr_foreground_new(
			C.guint16(uint16(style.Color.R)<<8),
			C.guint16(uint16(style.Color.G)<<8),
			C.guint16(uint16(style.Color.B)<<8))
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)
		C.pango_attr_list_insert(list.ptr, attr)
	}

	// Background color.
	if style.BgColor.A > 0 {
		attr := C.pango_attr_background_new(
			C.guint16(uint16(style.BgColor.R)<<8),
			C.guint16(uint16(style.BgColor.G)<<8),
			C.guint16(uint16(style.BgColor.B)<<8))
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)
		C.pango_attr_list_insert(list.ptr, attr)
	}

	// Underline.
	if style.Underline {
		attr := C.pango_attr_underline_new(C.PANGO_UNDERLINE_SINGLE)
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)
		C.pango_attr_list_insert(list.ptr, attr)
	}

	// Strikethrough.
	if style.Strikethrough {
		attr := C.pango_attr_strikethrough_new(C.TRUE)
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)
		C.pango_attr_list_insert(list.ptr, attr)
	}

	// Font description (name, size, typeface, variations).
	if style.FontName != "" || style.Size > 0 || style.Typeface != TypefaceRegular {
		var desc PangoFontDescW
		if style.FontName != "" {
			desc = NewPangoFontDescFromString(style.FontName)
		} else {
			desc = PangoFontDescNew()
		}
		if desc.ptr != nil {
			if style.FontName != "" {
				fam := PangoFontDescGetFamily(desc.ptr)
				resolved := resolveFamilyAlias(fam, isMonospaceName(fam))
				PangoFontDescSetFamily(desc.ptr, resolved)
			}
			applyTypeface(desc.ptr, style.Typeface)

			if style.Features != nil && len(style.Features.VariationAxes) > 0 {
				var sb strings.Builder
				for i, a := range style.Features.VariationAxes {
					if i > 0 {
						sb.WriteByte(',')
					}
					fmt.Fprintf(&sb, "%s=%g", a.Tag, a.Value)
				}
				desc.SetVariations(sb.String())
			}
			if style.Size > 0 {
				desc.SetSize(int(style.Size * float32(PangoScale)))
			}

			attr := PangoAttrFontDescNew(desc.ptr)
			attr.start_index = C.guint(start)
			attr.end_index = C.guint(end)
			C.pango_attr_list_insert(list.ptr, attr)
			desc.Close()
		}
	}

	// OpenType features.
	if style.Features != nil && len(style.Features.OpenTypeFeatures) > 0 {
		var sb strings.Builder
		for i, f := range style.Features.OpenTypeFeatures {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s=%d", f.Tag, f.Value)
		}
		cs := C.CString(sb.String())
		attr := C.pango_attr_font_features_new(cs)
		C.free(unsafe.Pointer(cs))
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)
		C.pango_attr_list_insert(list.ptr, attr)
	}

	// Letter spacing.
	if style.LetterSpacing != 0 {
		spacing := int(style.LetterSpacing * ctx.scaleFactor * float32(PangoScale))
		attr := C.pango_attr_letter_spacing_new(C.int(spacing))
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)
		C.pango_attr_list_insert(list.ptr, attr)
	}

	// Inline objects.
	if style.Object != nil {
		obj := style.Object
		w := C.int(obj.Width * ctx.scaleFactor * float32(PangoScale))
		h := C.int(obj.Height * ctx.scaleFactor * float32(PangoScale))
		offset := C.int(obj.Offset * ctx.scaleFactor * float32(PangoScale))

		logicalRect := C.PangoRectangle{
			x:      0,
			y:      -h - offset,
			width:  w,
			height: h,
		}
		inkRect := logicalRect

		// Allocate a C null-terminated copy of the ID and
		// register copy/destroy callbacks so Pango manages
		// the lifetime across attribute copies.
		var cData unsafe.Pointer
		if obj.ID != "" {
			cData = unsafe.Pointer(C.CString(obj.ID))
		}

		attr := C.pango_attr_shape_new_with_data(
			&inkRect, &logicalRect,
			C.gpointer(cData),
			C.PangoAttrDataCopyFunc(C.shape_data_copy),
			C.GDestroyNotify(C.shape_data_destroy),
		)
		attr.start_index = C.guint(start)
		attr.end_index = C.guint(end)

		C.pango_attr_list_insert(list.ptr, attr)
	}
}
