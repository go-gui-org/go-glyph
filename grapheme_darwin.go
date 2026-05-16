//go:build darwin && !glyph_pango

package glyph

/*
#include <CoreFoundation/CoreFoundation.h>

// cfSegmentGraphemes iterates composed character ranges for UAX #29
// grapheme cluster segmentation. Returns cluster count.
static int cfSegmentGraphemes(const char *text, int textLen,
    int *outByteOffsets, int *outByteLengths, int maxClusters) {
    @autoreleasepool {
        CFStringRef str = CFStringCreateWithBytes(NULL,
            (const UInt8 *)text, textLen,
            kCFStringEncodingUTF8, false);
        if (!str) return 0;

        CFIndex len = CFStringGetLength(str);
        int count = 0;
        CFIndex utf16Idx = 0;
        int byteOffset = 0;

        while (utf16Idx < len && count < maxClusters) {
            CFRange r = CFStringGetRangeOfComposedCharactersAtIndex(
                str, utf16Idx);

            CFIndex byteLen = 0;
            CFStringGetBytes(str, r, kCFStringEncodingUTF8,
                '?', false, NULL, 0, &byteLen);

            outByteOffsets[count] = byteOffset;
            outByteLengths[count] = (int)byteLen;
            count++;

            byteOffset += (int)byteLen;
            utf16Idx = r.location + r.length;
        }

        CFRelease(str);
        return count;
    }
}
*/
import "C"
import "unsafe"

// graphemeCluster represents one user-perceived character.
type graphemeCluster struct {
	text  string
	byteI int
	byteL int
}

// segmentGraphemes splits text into grapheme clusters using
// Core Foundation composed character ranges (UAX #29).
func segmentGraphemes(text string) []graphemeCluster {
	if len(text) == 0 {
		return nil
	}

	maxClusters := len(text)
	offsets := make([]C.int, maxClusters)
	lengths := make([]C.int, maxClusters)

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	n := C.cfSegmentGraphemes(cText, C.int(len(text)),
		&offsets[0], &lengths[0], C.int(maxClusters))

	clusters := make([]graphemeCluster, int(n))
	for i := range int(n) {
		off := int(offsets[i])
		ln := int(lengths[i])
		clusters[i] = graphemeCluster{
			text:  text[off : off+ln],
			byteI: off,
			byteL: ln,
		}
	}
	return clusters
}
