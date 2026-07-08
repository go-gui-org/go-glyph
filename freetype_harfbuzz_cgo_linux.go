//go:build linux && !android && glyph_system

package glyph

/*
#cgo pkg-config: freetype2 harfbuzz

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
