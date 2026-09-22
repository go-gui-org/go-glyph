//go:build android || linux || darwin || windows

package glyph

import (
	"unicode/utf8"

	xbidi "golang.org/x/text/unicode/bidi"
)

// charBidiInfo holds the byte range of a grapheme cluster used by
// visualOrderForLine. It is the minimal set of fields shared between
// the CoreText (darwin) and FreeType (android/linux) backends.
type charBidiInfo struct {
	byteI int // byte offset of the cluster in the full text
	byteL int // byte length of the cluster
}

// isPureLTR returns true when every rune in text is below U+0590.
// No RTL script exists below Hebrew (U+0590), so the bidi machinery can
// be skipped entirely for such text — the visual order is sequential.
func isPureLTR(text string) bool {
	for _, r := range text {
		if r >= 0x0590 {
			return false
		}
	}
	return true
}

// paragraphDirection returns the base direction of the paragraph that
// starts at text: the direction of its first strong character (UAX #9 rules
// P2 and P3), or LeftToRight when it has none. The scan stops at the first
// '\n', which ends the paragraph. Isolate content is not skipped, which is a
// simplification of P2.
func paragraphDirection(text string) xbidi.Direction {
	for _, r := range text {
		if r == '\n' {
			break
		}
		if r < 0x0590 {
			// Fast path: below Hebrew, only letters can be strong, and
			// they are all L.
			if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
				return xbidi.LeftToRight
			}
		}
		props, _ := xbidi.LookupRune(r)
		switch props.Class() {
		case xbidi.L:
			return xbidi.LeftToRight
		case xbidi.R, xbidi.AL:
			return xbidi.RightToLeft
		}
	}
	return xbidi.LeftToRight
}

// lrm is U+200E LEFT-TO-RIGHT MARK. Put first in a line, it forces the line's
// base direction to left-to-right.
const lrm = "\u200e"

// visualOrderForLine returns char indices for one line in visual order,
// with the base direction taken from the line itself. Within an RTL bidi
// run, char clusters are emitted in reverse logical order.
func visualOrderForLine(text string, chars []charBidiInfo, startChar, endChar int) []int {
	order, _ := visualOrderForLineDir(text, chars, startChar, endChar, xbidi.Neutral, nil)
	return order
}

// visualOrderForLineDir returns the char indices of one line in visual
// order, and for each char (indexed by char-startChar) whether it is in a
// right-to-left run. dir is the paragraph's base direction; Neutral takes
// it from the line itself. A wrapped line must use its paragraph's
// direction, not its own: an RTL paragraph whose second line starts with a
// Latin word is still RTL. rtl is reused when it is large enough.
//
// A nil order means the visual order is the logical order and every char
// is LTR (the returned rtl is then nil too).
func visualOrderForLineDir(text string, chars []charBidiInfo, startChar, endChar int,
	dir xbidi.Direction, rtl []bool) ([]int, []bool) {

	if startChar < 0 || endChar > len(chars) || startChar >= endChar {
		return nil, nil
	}
	startByte := chars[startChar].byteI
	endByte := chars[endChar-1].byteI + chars[endChar-1].byteL
	if startByte < 0 || endByte < startByte || endByte > len(text) {
		return nil, nil
	}
	lineText := text[startByte:endByte]
	if isPureLTR(lineText) && dir != xbidi.RightToLeft {
		return nil, nil
	}

	// Resolve the base direction. Neutral takes it from the line itself.
	if dir != xbidi.RightToLeft && dir != xbidi.LeftToRight {
		dir = paragraphDirection(lineText)
	}
	rtlBase := dir == xbidi.RightToLeft

	// Force the base direction. RTL has an option; LTR does not (its option
	// only sets the default for text with no strong character), so a line
	// whose own first strong character is RTL gets a leading LRM. The mark
	// shifts every rune index by one.
	src := lineText
	shift := 0
	var opts []xbidi.Option
	if rtlBase {
		opts = append(opts, xbidi.DefaultDirection(xbidi.RightToLeft))
	} else if paragraphDirection(lineText) == xbidi.RightToLeft {
		src = lrm + lineText
		shift = 1
	}

	// Rune range of each char, counted as the chars are walked. Chars are
	// in logical order, so the spans are sorted by runeStart.
	type span struct {
		runeStart, runeEnd int
	}
	n := endChar - startChar
	spans := make([]span, n)
	acc := shift
	prevEnd := startByte
	for k := range n {
		c := chars[startChar+k]
		if c.byteI < prevEnd || c.byteL < 0 || c.byteI+c.byteL > endByte ||
			!utf8.RuneStart(text[c.byteI]) {
			return nil, nil
		}
		acc += utf8.RuneCountInString(text[prevEnd:c.byteI]) // gap, normally 0
		rs := acc
		acc += utf8.RuneCountInString(text[c.byteI : c.byteI+c.byteL])
		spans[k] = span{rs, acc}
		prevEnd = c.byteI + c.byteL
	}

	var para xbidi.Paragraph
	if _, err := para.SetString(src, opts...); err != nil {
		return nil, nil
	}
	ordering, err := para.Order()
	if err != nil || ordering.NumRuns() == 0 {
		return nil, nil
	}

	// Embedding level of each char. x/text resolves the levels but exposes
	// only their parity: its runs are the maximal same-parity spans, in
	// logical order, and it does not reorder them (UAX #9 rule L2). The
	// levels are rebuilt from the parity here. Without explicit embeddings
	// only levels 0 to 2 occur:
	//   - RTL base (level 1): RTL chars are 1; LTR chars and numbers are 2
	//     (rule I2).
	//   - LTR base (level 0): RTL chars are 1; LTR chars are 0, except a
	//     number that follows RTL text, which is 2 (rules W7 and I1).
	// A char that spans two runs takes the level of the run it starts in.
	levels := make([]int8, n)
	ri := 0
	lastStrongRTL := rtlBase // sos: the base direction
	for k := range n {
		for ri+1 < ordering.NumRuns() {
			run := ordering.Run(ri)
			if _, end := run.Pos(); end >= spans[k].runeStart {
				break
			}
			ri++
		}
		run := ordering.Run(ri)
		cls := firstClass(text[chars[startChar+k].byteI:])
		switch cls {
		case xbidi.L:
			lastStrongRTL = false
		case xbidi.R, xbidi.AL:
			lastStrongRTL = true
		}
		switch {
		case run.Direction() == xbidi.RightToLeft:
			levels[k] = 1
		case rtlBase:
			levels[k] = 2
		case cls == xbidi.AN || cls == xbidi.EN && lastStrongRTL:
			levels[k] = 2
		}
	}
	if !rtlBase {
		raiseNumberSeparators(levels, func(k int) xbidi.Class {
			return firstClass(text[chars[startChar+k].byteI:])
		})
	}

	// Rule L2: from the highest level down to the lowest odd level, reverse
	// every maximal sequence of chars at that level or higher.
	order := make([]int, n)
	for k := range order {
		order[k] = startChar + k
	}
	var maxLevel int8
	for _, lv := range levels {
		maxLevel = max(maxLevel, lv)
	}
	for lv := maxLevel; lv >= 1; lv-- {
		for i := 0; i < n; {
			if levels[order[i]-startChar] < lv {
				i++
				continue
			}
			j := i
			for j < n && levels[order[j]-startChar] >= lv {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}

	if cap(rtl) < n {
		rtl = make([]bool, n)
	} else {
		rtl = rtl[:n]
	}
	for k, lv := range levels {
		rtl[k] = lv%2 == 1
	}
	return order, rtl
}

// firstClass returns the bidi class of the first rune of s.
func firstClass(s string) xbidi.Class {
	props, _ := xbidi.LookupString(s)
	return props.Class()
}

// raiseNumberSeparators puts number separators and terminators at the
// level of the numbers they belong to (UAX #9 rules W4 and W5), so that
// "1,000" after RTL text stays one number: a single ES or CS between two
// level-2 numbers, and a sequence of ET next to one, move to level 2.
// class(k) is the bidi class of char k.
func raiseNumberSeparators(levels []int8, class func(k int) xbidi.Class) {
	num := func(k int) bool {
		return k >= 0 && k < len(levels) && levels[k] == 2 &&
			(class(k) == xbidi.EN || class(k) == xbidi.AN)
	}
	for k := range levels {
		if levels[k] != 0 {
			continue
		}
		switch class(k) {
		case xbidi.ES, xbidi.CS:
			if num(k-1) && num(k+1) {
				levels[k] = 2
			}
		case xbidi.ET:
			// Extend over the ET sequence and check both ends.
			j := k
			for j < len(levels) && levels[j] == 0 && class(j) == xbidi.ET {
				j++
			}
			if num(k-1) || num(j) {
				for m := k; m < j; m++ {
					levels[m] = 2
				}
			}
		}
	}
}
