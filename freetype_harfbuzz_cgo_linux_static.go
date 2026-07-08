//go:build linux && !android && !glyph_system

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
