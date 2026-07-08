//go:build linux && !android && !glyph_system

package glyph

/*
#cgo CFLAGS: -I${SRCDIR}/deps/include -I${SRCDIR}/deps/include/freetype2 -I${SRCDIR}/deps/include/harfbuzz
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_amd64 -lfreetype -lharfbuzz -lpng16 -lz -lbrotlidec -lbrotlicommon -lbz2 -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/deps/lib/linux_arm64 -lfreetype -lharfbuzz -lpng16 -lz -lbrotlidec -lbrotlicommon -lbz2 -lm

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
