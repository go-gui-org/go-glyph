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

// buildByteToRuneIndexSlice maps each byte offset to its rune index.
// Non-rune-start positions are -1. Index len(text) holds the total
// rune count.
func buildByteToRuneIndexSlice(text string) []int {
	m := make([]int, len(text)+1)
	for i := range m {
		m[i] = -1
	}
	runeIdx := 0
	for i := 0; i < len(text); {
		m[i] = runeIdx
		_, sz := utf8.DecodeRuneInString(text[i:])
		i += sz
		runeIdx++
	}
	m[len(text)] = runeIdx
	return m
}

// visualOrderForLine returns char indices for one line in visual order.
// Within an RTL bidi run, char clusters are emitted in reverse logical
// order.
func visualOrderForLine(text string, chars []charBidiInfo, startChar, endChar int) []int {
	if startChar < 0 || endChar > len(chars) || startChar >= endChar {
		return nil
	}

	startByte := chars[startChar].byteI
	endByte := chars[endChar-1].byteI + chars[endChar-1].byteL
	if startByte < 0 || endByte < startByte || endByte > len(text) {
		return nil
	}
	lineText := text[startByte:endByte]
	if isPureLTR(lineText) {
		return nil
	}
	runeMap := buildByteToRuneIndexSlice(lineText)

	type span struct {
		charIndex          int
		runeStart, runeEnd int
	}
	spans := make([]span, 0, endChar-startChar)
	for i := startChar; i < endChar; i++ {
		relStart := chars[i].byteI - startByte
		relEnd := relStart + chars[i].byteL
		if relStart < 0 || relEnd < relStart ||
			relStart >= len(runeMap) || relEnd >= len(runeMap) {
			return nil
		}
		rs, re := runeMap[relStart], runeMap[relEnd]
		if rs < 0 || re < 0 {
			return nil
		}
		spans = append(spans, span{
			charIndex: i,
			runeStart: rs,
			runeEnd:   re,
		})
	}

	var para xbidi.Paragraph
	if _, err := para.SetString(lineText); err != nil {
		return nil
	}
	ordering, err := para.Order()
	if err != nil || ordering.NumRuns() == 0 {
		return nil
	}

	order := make([]int, 0, len(spans))
	used := make([]bool, len(spans))
	runChars := make([]int, 0, len(spans))
	runSpanIdx := make([]int, 0, len(spans))
	for ri := 0; ri < ordering.NumRuns(); ri++ {
		run := ordering.Run(ri)
		runStart, runEndInclusive := run.Pos()
		runEnd := runEndInclusive + 1

		runChars = runChars[:0]
		runSpanIdx = runSpanIdx[:0]
		for si, sp := range spans {
			if used[si] {
				continue
			}
			if sp.runeStart >= runStart && sp.runeEnd <= runEnd {
				runChars = append(runChars, sp.charIndex)
				runSpanIdx = append(runSpanIdx, si)
			}
		}
		if len(runChars) == 0 {
			continue
		}

		if run.Direction() == xbidi.RightToLeft {
			for i := len(runChars) - 1; i >= 0; i-- {
				order = append(order, runChars[i])
				used[runSpanIdx[i]] = true
			}
		} else {
			order = append(order, runChars...)
			for _, si := range runSpanIdx {
				used[si] = true
			}
		}
	}

	for si, sp := range spans {
		if !used[si] {
			order = append(order, sp.charIndex)
		}
	}
	return order
}
