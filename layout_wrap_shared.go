//go:build android || linux || darwin || windows || (js && wasm)

package glyph

import "github.com/rivo/uniseg"

// lineInfo is one wrapped line: the half-open cluster range [startChar,
// endChar) and the line's width in the caller's units.
type lineInfo struct {
	startChar, endChar int
	width              float64
	// paraStart is true when the line is the first of its paragraph: the
	// layout's first line, or the line right after a hard break.
	paraStart bool
}

// lineIndent is the start offset of a line under BlockStyle.Indent, with
// Pango's meaning: a positive indent moves the first line of each
// paragraph right; a negative one moves every other line right by
// -indent (a hanging indent). The line's wrap width shrinks by the same
// amount.
func lineIndent(indent float64, paraStart bool) float64 {
	switch {
	case indent > 0 && paraStart:
		return indent
	case indent < 0 && !paraStart:
		return -indent
	}
	return 0
}

// wrapLines splits n measured clusters into lines and appends them to dst.
// Both backends call it, so they wrap the same text the same way.
//
// width(i) is cluster i's advance. text(i) is its text; hard line breaks and
// " " (a word-wrap point) are special. canBreak, when not nil,
// holds UAX #14 break opportunities: canBreak[i] means a line may start at
// cluster i. A wrapWidth of zero or less disables wrapping.
//
// A word wrap at a space ends the line just before the space and drops the
// space from both lines. The line's width is the width before the space.
//
// indent is BlockStyle.Indent in the same units as wrapWidth. It shrinks the
// wrap width of the lines that lineIndent offsets. Each line records whether
// it starts a paragraph in lineInfo.paraStart.
func wrapLines(dst []lineInfo, n int, mode WrapMode, wrapWidth, indent float64,
	width func(i int) float64, text func(i int) string,
	canBreak []bool) []lineInfo {

	lineStart := 0
	lineW := 0.0
	// paraStart is true while the current line starts a paragraph.
	paraStart := true
	// lastSpace is the last space on the current line and lastSpaceW the
	// line width before it. lastBreak and lastBreakW are the same for the
	// last UAX #14 opportunity that is not a space.
	lastSpace, lastBreak := -1, -1
	var lastSpaceW, lastBreakW float64

	// restart begins a new line at cluster from. The clusters from..upTo
	// are already on it, so their widths are summed again.
	restart := func(from, upTo int) {
		lineStart = from
		paraStart = false
		lineW = 0
		for j := from; j <= upTo; j++ {
			lineW += width(j)
		}
		lastSpace, lastBreak = -1, -1
	}

	for i := range n {
		t := text(i)
		if isHardLineBreak(t) {
			dst = append(dst, lineInfo{lineStart, i, lineW, paraStart})
			lineStart = i + 1
			paraStart = true
			lineW = 0
			lastSpace, lastBreak = -1, -1
			continue
		}
		if t == " " {
			lastSpace = i
			lastSpaceW = lineW
		} else if i > 0 && canBreak != nil && canBreak[i] {
			lastBreak = i
			lastBreakW = lineW
		}

		w := width(i)
		newW := lineW + w
		if wrapWidth > 0 && newW > wrapWidth-lineIndent(indent, paraStart) &&
			i > lineStart && mode != WrapNone {
			if mode == WrapWord || mode == WrapWordChar {
				if lastSpace >= lineStart {
					dst = append(dst, lineInfo{lineStart, lastSpace, lastSpaceW, paraStart})
					restart(lastSpace+1, i)
					continue
				}
				// Strictly greater: a break opportunity at the line's own
				// start (uniseg reports one after a hard newline) would emit
				// a zero-length line whose StartIndex duplicates the next
				// line's, and vertical caret motion then has a fixed point
				// it cannot move out of.
				if lastBreak > lineStart {
					dst = append(dst, lineInfo{lineStart, lastBreak, lastBreakW, paraStart})
					restart(lastBreak, i)
					continue
				}
			}
			if mode == WrapChar || mode == WrapWordChar {
				dst = append(dst, lineInfo{lineStart, i, lineW, paraStart})
				lineStart = i
				paraStart = false
				lineW = w
				lastSpace, lastBreak = -1, -1
				continue
			}
		}
		lineW = newW
	}
	return append(dst, lineInfo{lineStart, n, lineW, paraStart})
}

// isHardLineBreak reports whether s is one hard line-break grapheme. Unicode
// grapheme segmentation keeps CRLF together, while lone CR and LF each form
// their own cluster.
func isHardLineBreak(s string) bool {
	return s == "\n" || s == "\r" || s == "\r\n"
}

// hardLineBreakLenAt returns the byte length of the hard line break starting
// at byte index i, or zero when i is not at one. CRLF is one break so callers
// never expose a caret stop between its two bytes.
//
// The function looks only forward. At the LF inside a CRLF it returns 1, so
// the result is a break start only when a linear scan reaches i by stepping
// over each earlier break with the returned length. A point probe at an
// arbitrary index must check text[i-1] == '\r' itself if the CRLF interior
// matters to it.
func hardLineBreakLenAt(text string, i int) int {
	if i < 0 || i >= len(text) {
		return 0
	}
	switch text[i] {
	case '\n':
		return 1
	case '\r':
		if i+1 < len(text) && text[i+1] == '\n' {
			return 2
		}
		return 1
	default:
		return 0
	}
}

// lineBreakOpportunities fills dst[i] with whether a UAX #14 line break is
// allowed before cluster i, for n clusters whose byte offsets byteI(i)
// increase. dst is reused when it is large enough.
//
// Segment end offsets and cluster offsets both increase, so a two-pointer
// merge maps each break to its cluster with no allocation. The string form
// of the uniseg segmenter avoids copying text into a []byte.
func lineBreakOpportunities(dst []bool, text string, n int,
	byteI func(i int) int) []bool {

	if cap(dst) < n {
		dst = make([]bool, n)
	} else {
		dst = dst[:n]
		clear(dst)
	}
	if n < 2 {
		return dst
	}
	rest := text
	consumed := 0
	state := -1
	ci := 0
	for len(rest) > 0 {
		var seg string
		seg, rest, _, state = uniseg.FirstLineSegmentInString(rest, state)
		if len(seg) == 0 {
			// uniseg guarantees progress on non-empty input; guard anyway
			// so a violated invariant cannot hang layout.
			break
		}
		consumed += len(seg)
		for ci < n && byteI(ci) < consumed {
			ci++
		}
		if ci < n && byteI(ci) == consumed {
			dst[ci] = true
		}
	}
	return dst
}

// appendHardLineBreakAttrs appends a caret stop at every hard line break.
// Break clusters are never visited by the per-line char loops: each line's
// [startChar, endChar) range stops just before its terminating break. CRLF is
// one grapheme and gets one stop at the CR, never one between CR and LF.
// Without these entries, clicks past a paragraph's last line land on its final
// character, arrow keys skip empty lines, and grapheme deletes swallow runs of
// line breaks.
func appendHardLineBreakAttrs(text string, logAttrs []LogAttr,
	logAttrByIndex map[int]int) []LogAttr {

	for i := 0; i < len(text); {
		size := hardLineBreakLenAt(text, i)
		if size == 0 {
			i++
			continue
		}
		logAttrByIndex[i] = len(logAttrs)
		logAttrs = append(logAttrs, LogAttr{
			IsCursorPosition: true,
			IsLineBreak:      true,
		})
		i += size
	}
	return logAttrs
}

// appendWrapSpaceAttrs appends a caret stop for each space that a soft wrap
// consumed. Such a space is on no line and has no CharRect, but its byte
// index is the end of the wrapped line: GetCursorPos answers it with the
// line's end geometry. Without a stop there, a forward delete before the
// space also deletes the space, and a word ending at the wrap has no end.
func appendWrapSpaceAttrs(lines []lineInfo, text func(i int) string,
	byteI func(i int) int, logAttrs []LogAttr,
	logAttrByIndex map[int]int) []LogAttr {

	for k := 0; k+1 < len(lines); k++ {
		gap := lines[k].endChar
		if lines[k+1].startChar != gap+1 || text(gap) != " " {
			continue
		}
		logAttrByIndex[byteI(gap)] = len(logAttrs)
		logAttrs = append(logAttrs, LogAttr{IsCursorPosition: true})
	}
	return logAttrs
}
