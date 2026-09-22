package glyph

import (
	"slices"
	"sort"
)

// maxDistance is a sentinel for "no match found" distance comparisons.
const maxDistance float32 = 1e9

// buildPositionCaches pre-sorts cursor and word boundary positions.
// Called once after layout construction. One pass over the attribute map
// and one sort fill all three lists.
func (l *Layout) buildPositionCaches() {
	keys := make([]int, 0, len(l.LogAttrByIndex))
	nStarts, nEnds := 0, 0
	for byteIdx, attrIdx := range l.LogAttrByIndex {
		if attrIdx < 0 || attrIdx >= len(l.LogAttrs) {
			continue
		}
		a := l.LogAttrs[attrIdx]
		if a.IsWordStart {
			nStarts++
		}
		if a.IsWordEnd {
			nEnds++
		}
		keys = append(keys, byteIdx)
	}
	slices.Sort(keys)

	// Filter the sorted keys. The cursor list is compacted into keys
	// itself: it is written at w <= i, so no unread key is overwritten.
	starts := make([]int, 0, nStarts)
	ends := make([]int, 0, nEnds)
	w := 0
	for _, byteIdx := range keys {
		a := l.LogAttrs[l.LogAttrByIndex[byteIdx]]
		if a.IsWordStart {
			starts = append(starts, byteIdx)
		}
		if a.IsWordEnd {
			ends = append(ends, byteIdx)
		}
		if a.IsCursorPosition {
			keys[w] = byteIdx
			w++
		}
	}
	l.cursorPositions = keys[:w]
	l.wordStarts = starts
	l.wordEnds = ends
}

// lineRectRange returns the half-open range of CharRects that belong to
// line. CharRects are stored line by line, and a line's rects all have
// byte indices in [StartIndex, StartIndex+Length) — within the line they
// are in visual order, so not sorted — which makes "Index >= x" monotone
// across the whole slice for any line boundary x.
func (l *Layout) lineRectRange(line Line) (int, int) {
	lo := sort.Search(len(l.CharRects), func(i int) bool {
		return l.CharRects[i].Index >= line.StartIndex
	})
	end := line.StartIndex + line.Length
	hi := lo + sort.Search(len(l.CharRects)-lo, func(i int) bool {
		return l.CharRects[lo+i].Index >= end
	})
	return lo, hi
}

// rectIsRTL reports whether CharRects[ri] is in a right-to-left run.
func (l *Layout) rectIsRTL(ri int) bool {
	return ri >= 0 && ri < len(l.charRTL) && l.charRTL[ri]
}

// isCursorStop reports whether byteIdx is a caret position.
func (l *Layout) isCursorStop(byteIdx int) bool {
	ai, ok := l.LogAttrByIndex[byteIdx]
	return ok && ai >= 0 && ai < len(l.LogAttrs) && l.LogAttrs[ai].IsCursorPosition
}

// HitTestRect returns the bounding box of the character at (x, y)
// relative to the layout origin. Returns ok=false if no character
// is found.
func (l *Layout) HitTestRect(x, y float32) (Rect, bool) {
	for _, cr := range l.CharRects {
		if x >= cr.Rect.X && x <= cr.Rect.X+cr.Rect.Width &&
			y >= cr.Rect.Y && y <= cr.Rect.Y+cr.Rect.Height {
			return cr.Rect, true
		}
	}
	return Rect{}, false
}

// GetCharRect returns the bounding box for a character at byte
// index. Returns ok=false if index is not a valid character
// position.
func (l *Layout) GetCharRect(index int) (Rect, bool) {
	ri, ok := l.CharRectByIndex[index]
	if !ok {
		return Rect{}, false
	}
	return l.CharRects[ri].Rect, true
}

// HitTest returns the byte index of the character at (x, y)
// relative to origin. Returns -1 if no character is found.
func (l *Layout) HitTest(x, y float32) int {
	for _, cr := range l.CharRects {
		if x >= cr.Rect.X && x <= cr.Rect.X+cr.Rect.Width &&
			y >= cr.Rect.Y && y <= cr.Rect.Y+cr.Rect.Height {
			return cr.Index
		}
	}
	return -1
}

// GetClosestOffset returns the byte index of the character closest
// to (x, y). Handles clicks outside bounds.
func (l *Layout) GetClosestOffset(x, y float32) int {
	if len(l.Lines) == 0 {
		return 0
	}

	// Find closest line vertically.
	closestLineIdx := 0
	minDistY := maxDistance
	for i, line := range l.Lines {
		var dist float32
		if y >= line.Rect.Y && y <= line.Rect.Y+line.Rect.Height {
			dist = 0
		} else {
			mid := line.Rect.Y + line.Rect.Height/2
			dist = absF32(y - mid)
		}
		if dist < minDistY {
			minDistY = dist
			closestLineIdx = i
		}
	}

	return l.closestInLine(l.Lines[closestLineIdx], x)
}

// GetSelectionRects returns rectangles covering [start, end).
func (l *Layout) GetSelectionRects(start, end int) []Rect {
	return l.appendSelectionRects(nil, start, end)
}

// appendSelectionRects appends the rectangles covering [start, end) to
// rects and returns the result. Draw paths pass a reused scratch slice
// so per-frame selection geometry does not allocate.
//
// Each line gives one rectangle per visually contiguous piece of the
// selection. In a bidi line, a logical range can be two or more pieces
// on screen; one rectangle over all of them would also paint the
// unselected text between them.
func (l *Layout) appendSelectionRects(rects []Rect, start, end int) []Rect {
	if start >= end || len(l.Lines) == 0 {
		return rects
	}
	for _, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if max(start, line.StartIndex) >= min(end, lineEnd) {
			continue
		}
		lo, hi := l.lineRectRange(line)
		inPiece := false
		var minX, maxX float32
		for ri := lo; ri < hi; ri++ {
			cr := l.CharRects[ri]
			if cr.Index < start || cr.Index >= end {
				if inPiece {
					rects = append(rects, Rect{X: minX, Y: line.Rect.Y,
						Width: maxX - minX, Height: line.Rect.Height})
					inPiece = false
				}
				continue
			}
			right := cr.Rect.X + cr.Rect.Width
			if !inPiece {
				minX, maxX = cr.Rect.X, right
				inPiece = true
				continue
			}
			minX = min(minX, cr.Rect.X)
			maxX = max(maxX, right)
		}
		if inPiece {
			rects = append(rects, Rect{X: minX, Y: line.Rect.Y,
				Width: maxX - minX, Height: line.Rect.Height})
		}
	}
	return rects
}

// GetCursorPos returns cursor geometry at byte_index.
// Returns ok=false if not a valid cursor position.
func (l *Layout) GetCursorPos(byteIndex int) (CursorPosition, bool) {
	if byteIndex < 0 {
		return CursorPosition{}, false
	}

	// Check valid cursor position via log attrs.
	attrIdx, ok := l.LogAttrByIndex[byteIndex]
	valid := ok || byteIndex == 0
	if ok && attrIdx >= 0 && attrIdx < len(l.LogAttrs) &&
		!l.LogAttrs[attrIdx].IsCursorPosition {
		valid = false
	}
	if !valid {
		// A soft wrap consumes the space it broke at, so that byte
		// belongs to no line and carries no log attr — yet it is
		// exactly where the caret sits at the end of the wrapped line,
		// and it is what findClosestIndexInLine returns for a click
		// past the line's right edge or for vertical motion with a
		// preferred x beyond it. Answer with the line's end geometry
		// rather than refusing a position the caller was handed here.
		if cp, endOK := l.lineEndPos(byteIndex); endOK {
			return cp, true
		}
		return CursorPosition{}, false
	}

	// Try exact char rect. Skip '\n' — its glyph rect is at the
	// start of the next line, but the cursor belongs at the end
	// of the current line (handled by the line-based fallback).
	if byteIndex >= len(l.Text) || l.Text[byteIndex] != '\n' {
		if ri, ok := l.CharRectByIndex[byteIndex]; ok {
			r := l.CharRects[ri].Rect
			// The caret before a char sits on its leading edge: the
			// left edge for LTR, the right edge for RTL.
			x := r.X
			if l.rectIsRTL(ri) {
				x += r.Width
			}
			if line, ok := l.lineForByteIndex(byteIndex); ok {
				return CursorPosition{
					X:      x,
					Y:      line.Rect.Y,
					Height: line.Rect.Height,
				}, true
			}
			return CursorPosition{
				X:      x,
				Y:      r.Y,
				Height: r.Height,
			}, true
		}
	}

	// Fallback: find containing line.
	for _, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			if byteIndex == lineEnd {
				return CursorPosition{
					X:      line.Rect.X + line.Rect.Width,
					Y:      line.Rect.Y,
					Height: line.Rect.Height,
				}, true
			}
			if byteIndex == line.StartIndex {
				return CursorPosition{
					X:      line.Rect.X,
					Y:      line.Rect.Y,
					Height: line.Rect.Height,
				}, true
			}
		}
	}

	// Ultimate fallback for position 0.
	if byteIndex == 0 && len(l.Lines) > 0 {
		first := l.Lines[0]
		return CursorPosition{
			X:      first.Rect.X,
			Y:      first.Rect.Y,
			Height: first.Rect.Height,
		}, true
	}
	return CursorPosition{}, false
}

// lineEndPos returns the caret geometry for a byte index that sits
// exactly at the end of a line. Only a byte the wrap consumed reaches
// it: every other line end is a cursor position in its own right and is
// answered before this.
func (l *Layout) lineEndPos(byteIndex int) (CursorPosition, bool) {
	for _, line := range l.Lines {
		if byteIndex == line.StartIndex+line.Length {
			return CursorPosition{
				X:      line.Rect.X + line.Rect.Width,
				Y:      line.Rect.Y,
				Height: line.Rect.Height,
			}, true
		}
	}
	return CursorPosition{}, false
}

func (l *Layout) lineForByteIndex(byteIndex int) (Line, bool) {
	for _, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			return line, true
		}
	}
	return Line{}, false
}

// GetValidCursorPositions returns sorted byte indices that are
// valid cursor positions. Uses pre-built cache.
func (l *Layout) GetValidCursorPositions() []int {
	if l.cursorPositions == nil {
		l.buildPositionCaches()
	}
	return l.cursorPositions
}

// MoveCursorLeft returns the previous valid cursor position.
func (l *Layout) MoveCursorLeft(byteIndex int) int {
	if byteIndex <= 0 || len(l.LogAttrs) == 0 {
		return 0
	}
	positions := l.GetValidCursorPositions()
	// Largest position strictly below byteIndex.
	if i, _ := slices.BinarySearch(positions, byteIndex); i > 0 {
		return positions[i-1]
	}
	return 0
}

// MoveCursorRight returns the next valid cursor position.
func (l *Layout) MoveCursorRight(byteIndex int) int {
	if len(l.LogAttrs) == 0 {
		return byteIndex
	}
	positions := l.GetValidCursorPositions()
	// Smallest position strictly above byteIndex.
	i, found := slices.BinarySearch(positions, byteIndex)
	if found {
		i++
	}
	if i < len(positions) {
		return positions[i]
	}
	if len(positions) > 0 {
		return positions[len(positions)-1]
	}
	return byteIndex
}

// getWordStarts returns sorted byte indices that are word starts.
// Uses pre-built cache.
func (l *Layout) getWordStarts() []int {
	if l.wordStarts == nil {
		l.buildPositionCaches()
	}
	return l.wordStarts
}

// getWordEnds returns sorted byte indices that are word ends.
// Uses pre-built cache.
func (l *Layout) getWordEnds() []int {
	if l.wordEnds == nil {
		l.buildPositionCaches()
	}
	return l.wordEnds
}

// MoveCursorWordLeft returns the previous word start.
func (l *Layout) MoveCursorWordLeft(byteIndex int) int {
	if byteIndex <= 0 || len(l.LogAttrs) == 0 {
		return 0
	}
	starts := l.getWordStarts()
	// Largest start strictly below byteIndex. BinarySearch returns the
	// insertion point, so an exact hit must step back one as well.
	if i, _ := slices.BinarySearch(starts, byteIndex); i > 0 {
		return starts[i-1]
	}
	return 0
}

// MoveCursorWordRight returns the next word start.
func (l *Layout) MoveCursorWordRight(byteIndex int) int {
	if len(l.LogAttrs) == 0 {
		return byteIndex
	}
	starts := l.getWordStarts()
	// Smallest start strictly above byteIndex.
	if i, _ := slices.BinarySearch(starts, byteIndex+1); i < len(starts) {
		return starts[i]
	}
	positions := l.GetValidCursorPositions()
	if len(positions) > 0 {
		return positions[len(positions)-1]
	}
	return byteIndex
}

// MoveCursorLineStart returns the start of the current line.
// At a soft-wrap boundary the later line is preferred.
func (l *Layout) MoveCursorLineStart(byteIndex int) int {
	for i, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			// At boundary: prefer next line if it starts here.
			if byteIndex == lineEnd && i+1 < len(l.Lines) &&
				l.Lines[i+1].StartIndex == lineEnd {
				continue
			}
			return line.StartIndex
		}
	}
	return 0
}

// MoveCursorLineEnd returns the end of the current line.
// At a soft-wrap boundary the later line is preferred.
func (l *Layout) MoveCursorLineEnd(byteIndex int) int {
	for i, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			// At boundary: prefer next line if it starts here.
			if byteIndex == lineEnd && i+1 < len(l.Lines) &&
				l.Lines[i+1].StartIndex == lineEnd {
				continue
			}
			return lineEnd
		}
	}
	return byteIndex
}

// MoveCursorUp returns byte index on previous line at similar x.
// Pass preferredX < 0 to use cursor's current x.
func (l *Layout) MoveCursorUp(byteIndex int, preferredX float32) int {
	if len(l.Lines) == 0 {
		return byteIndex
	}
	currentLineIdx := -1
	targetX := preferredX
	for i, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			// At boundary: prefer line that starts here.
			if byteIndex == lineEnd && i+1 < len(l.Lines) &&
				l.Lines[i+1].StartIndex == byteIndex {
				continue
			}
			currentLineIdx = i
			if targetX < 0 {
				if pos, ok := l.GetCursorPos(byteIndex); ok {
					targetX = pos.X
				} else {
					targetX = line.Rect.X
				}
			}
			break
		}
	}
	if currentLineIdx <= 0 {
		return byteIndex
	}
	return l.findClosestIndexInLine(l.Lines[currentLineIdx-1], targetX)
}

// MoveCursorDown returns byte index on next line at similar x.
func (l *Layout) MoveCursorDown(byteIndex int, preferredX float32) int {
	if len(l.Lines) == 0 {
		return byteIndex
	}
	currentLineIdx := -1
	targetX := preferredX
	for i, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		if byteIndex >= line.StartIndex && byteIndex <= lineEnd {
			// At boundary: prefer line that starts here.
			if byteIndex == lineEnd && i+1 < len(l.Lines) &&
				l.Lines[i+1].StartIndex == byteIndex {
				continue
			}
			currentLineIdx = i
			if targetX < 0 {
				if pos, ok := l.GetCursorPos(byteIndex); ok {
					targetX = pos.X
				} else {
					targetX = line.Rect.X
				}
			}
			break
		}
	}
	if currentLineIdx < 0 || currentLineIdx >= len(l.Lines)-1 {
		return byteIndex
	}
	return l.findClosestIndexInLine(l.Lines[currentLineIdx+1], targetX)
}

// GetWordAtIndex returns the [start, end) byte range of the word
// containing byteIndex. Words are class runs — see layout_words.go — so a
// punctuation run is a word of its own.
//
// An index that falls between two words is in a whitespace run, and the
// whitespace run itself is returned. That matches the platform convention
// (double-clicking a run of spaces selects the run) and keeps the function
// total: every index yields a meaningful range.
func (l *Layout) GetWordAtIndex(byteIndex int) (int, int) {
	if len(l.LogAttrs) == 0 {
		return byteIndex, byteIndex
	}
	wordStarts := l.getWordStarts()
	wordEnds := l.getWordEnds()
	if len(wordStarts) == 0 || len(wordStarts) != len(wordEnds) {
		return byteIndex, byteIndex
	}

	// The caret position one past the end of the text belongs to the last
	// run, so that double-clicking there selects the final word instead of
	// an empty range. Word boundaries are rune-aligned, so comparing
	// against len-1 is safe even mid-rune.
	if byteIndex >= len(l.Text) && len(l.Text) > 0 {
		byteIndex = len(l.Text) - 1
	}

	// Starts and ends strictly alternate, so word k is
	// [wordStarts[k], wordEnds[k]). Find the last word that begins at or
	// before byteIndex; byteIndex is inside it when it also ends after.
	si, ok := slices.BinarySearch(wordStarts, byteIndex)
	if !ok {
		si--
	}
	if si >= 0 && byteIndex < wordEnds[si] {
		return wordStarts[si], wordEnds[si]
	}

	// Not inside a word: byteIndex sits in the gap between word si and
	// word si+1, which is exactly the whitespace run. A missing neighbour
	// means the gap runs to the edge of the text.
	gapStart := 0
	if si >= 0 {
		gapStart = wordEnds[si]
	}
	gapEnd := len(l.Text)
	if si+1 < len(wordStarts) {
		gapEnd = wordStarts[si+1]
	}
	if gapStart > gapEnd {
		return byteIndex, byteIndex
	}
	return gapStart, gapEnd
}

// GetParagraphAtIndex returns (start, end) byte indices for
// paragraph containing index. Paragraph = text between \n\n.
// text is normally l.Text; the parameter is kept for API compatibility.
func (l *Layout) GetParagraphAtIndex(byteIndex int, text string) (int, int) {
	if len(text) == 0 {
		return 0, 0
	}
	idx := max(0, min(byteIndex, len(text)))

	// Scan backwards for paragraph start.
	paraStart := 0
	for i := idx - 1; i >= 1; i-- {
		if text[i] == '\n' && text[i-1] == '\n' {
			paraStart = i + 1
			break
		}
	}

	// Scan forwards for paragraph end.
	paraEnd := len(text)
	for i := idx; i < len(text)-1; i++ {
		if text[i] == '\n' && text[i+1] == '\n' {
			paraEnd = i
			break
		}
	}
	return paraStart, paraEnd
}

// GetFontNameAtIndex returns the font family name at byte index.
func (l *Layout) GetFontNameAtIndex(index int) string {
	for _, item := range l.Items {
		if index >= item.StartIndex && index < item.StartIndex+item.Length {
			if item.ftFace != nil {
				return getFontFamilyName(item.ftFace)
			}
		}
	}
	return "Unknown"
}

// findClosestIndexInLine returns the byte index closest to
// targetX within the given line.
func (l *Layout) findClosestIndexInLine(line Line, targetX float32) int {
	return l.closestInLine(line, targetX)
}

// closestInLine returns the caret position in line nearest to x. It finds
// the char under x (or the nearest one), then picks the char's leading
// edge or its trailing edge by which half of the char x is in. For an RTL
// char the trailing half is the left one. A trailing hit gives the next
// caret stop in logical order, or the line end after the line's last char,
// which is also where a click past the line's last char lands.
func (l *Layout) closestInLine(line Line, x float32) int {
	lineEnd := line.StartIndex + line.Length
	lo, hi := l.lineRectRange(line)
	// A char absorbed into a ligature is not a caret stop. Only layouts
	// that carry caret-stop attrs can say so; a hand-built layout without
	// them treats every char as a stop.
	haveStops := len(l.GetValidCursorPositions()) > 0
	isStop := func(byteIdx int) bool {
		return !haveStops || l.isCursorStop(byteIdx)
	}
	best := -1
	bestDist := maxDistance
	for ri := lo; ri < hi; ri++ {
		cr := l.CharRects[ri]
		if !isStop(cr.Index) {
			continue
		}
		var dist float32
		switch left, right := cr.Rect.X, cr.Rect.X+cr.Rect.Width; {
		case x < left:
			dist = left - x
		case x > right:
			dist = x - right
		}
		if dist < bestDist {
			best, bestDist = ri, dist
		}
	}
	if best < 0 {
		return line.StartIndex
	}
	cr := l.CharRects[best]
	mid := cr.Rect.X + cr.Rect.Width/2
	trailing := x > mid
	if l.rectIsRTL(best) {
		trailing = x < mid
	}
	if !trailing {
		return cr.Index
	}
	// The next caret stop is the smallest stop index above this char's.
	next := lineEnd
	for ri := lo; ri < hi; ri++ {
		if i := l.CharRects[ri].Index; i > cr.Index && i < next && isStop(i) {
			next = i
		}
	}
	return next
}

func absF32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
