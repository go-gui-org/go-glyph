//go:build android || linux || darwin || windows || (js && wasm)

package glyph

import "github.com/rivo/uniseg"

// lineInfo is one wrapped line: the half-open cluster range [startChar,
// endChar) and the line's width in the caller's units.
type lineInfo struct {
	startChar, endChar int
	width              float64
}

// wrapLines splits n measured clusters into lines and appends them to dst.
// Both backends call it, so they wrap the same text the same way.
//
// width(i) is cluster i's advance. text(i) is its text; only "\n" (a hard
// break) and " " (a word-wrap point) are special. canBreak, when not nil,
// holds UAX #14 break opportunities: canBreak[i] means a line may start at
// cluster i. A wrapWidth of zero or less disables wrapping.
//
// A word wrap at a space ends the line just before the space and drops the
// space from both lines. The line's width is the width before the space.
func wrapLines(dst []lineInfo, n int, mode WrapMode, wrapWidth float64,
	width func(i int) float64, text func(i int) string,
	canBreak []bool) []lineInfo {

	lineStart := 0
	lineW := 0.0
	// lastSpace is the last space on the current line and lastSpaceW the
	// line width before it. lastBreak and lastBreakW are the same for the
	// last UAX #14 opportunity that is not a space.
	lastSpace, lastBreak := -1, -1
	var lastSpaceW, lastBreakW float64

	// restart begins a new line at cluster from. The clusters from..upTo
	// are already on it, so their widths are summed again.
	restart := func(from, upTo int) {
		lineStart = from
		lineW = 0
		for j := from; j <= upTo; j++ {
			lineW += width(j)
		}
		lastSpace, lastBreak = -1, -1
	}

	for i := range n {
		t := text(i)
		if t == "\n" {
			dst = append(dst, lineInfo{lineStart, i, lineW})
			lineStart = i + 1
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
		if wrapWidth > 0 && newW > wrapWidth && i > lineStart && mode != WrapNone {
			if mode == WrapWord || mode == WrapWordChar {
				if lastSpace >= lineStart {
					dst = append(dst, lineInfo{lineStart, lastSpace, lastSpaceW})
					restart(lastSpace+1, i)
					continue
				}
				// Strictly greater: a break opportunity at the line's own
				// start (uniseg reports one after a hard newline) would emit
				// a zero-length line whose StartIndex duplicates the next
				// line's, and vertical caret motion then has a fixed point
				// it cannot move out of.
				if lastBreak > lineStart {
					dst = append(dst, lineInfo{lineStart, lastBreak, lastBreakW})
					restart(lastBreak, i)
					continue
				}
			}
			if mode == WrapChar || mode == WrapWordChar {
				dst = append(dst, lineInfo{lineStart, i, lineW})
				lineStart = i
				lineW = w
				lastSpace, lastBreak = -1, -1
				continue
			}
		}
		lineW = newW
	}
	return append(dst, lineInfo{lineStart, n, lineW})
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

// appendNewlineAttrs appends a caret stop for every '\n' byte of text.
// Newline bytes are never visited by the per-line char loops — each line's
// [startChar, endChar) range stops just before its terminating '\n' — but
// they are still caret stops: the byte index of a '\n' is the end of the
// current line, and in a "\n\n" run it is also the empty line separating
// paragraphs. Without an entry, clicks past a paragraph's last line land on
// its final character, arrow keys skip from the end of a paragraph straight
// to the next paragraph (never visiting the empty line), and grapheme
// deletes swallow the newline run.
func appendNewlineAttrs(text string, logAttrs []LogAttr,
	logAttrByIndex map[int]int) []LogAttr {

	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			logAttrByIndex[i] = len(logAttrs)
			logAttrs = append(logAttrs, LogAttr{
				IsCursorPosition: true,
				IsLineBreak:      true,
			})
		}
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
