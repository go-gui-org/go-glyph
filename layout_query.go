package glyph

import "slices"

// maxDistance is a sentinel for "no match found" distance comparisons.
const maxDistance float32 = 1e9

// buildPositionCaches pre-sorts cursor and word boundary positions.
// Called once after layout construction.
func (l *Layout) buildPositionCaches() {
	l.cursorPositions = l.collectPositions(func(a LogAttr) bool { return a.IsCursorPosition })
	l.wordStarts = l.collectPositions(func(a LogAttr) bool { return a.IsWordStart })
	l.wordEnds = l.collectPositions(func(a LogAttr) bool { return a.IsWordEnd })
}

func (l *Layout) collectPositions(pred func(LogAttr) bool) []int {
	positions := make([]int, 0, len(l.LogAttrByIndex))
	for byteIdx, attrIdx := range l.LogAttrByIndex {
		if attrIdx >= 0 && attrIdx < len(l.LogAttrs) {
			if pred(l.LogAttrs[attrIdx]) {
				positions = append(positions, byteIdx)
			}
		}
	}
	slices.Sort(positions)
	return positions
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

	targetLine := l.Lines[closestLineIdx]
	lineEnd := targetLine.StartIndex + targetLine.Length

	// Find closest char in line.
	closestCharIdx := targetLine.StartIndex
	minDistX := maxDistance
	foundAny := false

	for i := targetLine.StartIndex; i < lineEnd; i++ {
		ri, ok := l.CharRectByIndex[i]
		if !ok {
			continue
		}
		cr := l.CharRects[ri]
		mid := cr.Rect.X + cr.Rect.Width/2
		dist := absF32(x - mid)
		if dist < minDistX {
			minDistX = dist
			closestCharIdx = i
			foundAny = true
		}
	}

	// If x is past rightmost character, return line end.
	if foundAny {
		lastRight := -maxDistance
		for i := targetLine.StartIndex; i < lineEnd; i++ {
			ri, ok := l.CharRectByIndex[i]
			if !ok {
				continue
			}
			cr := l.CharRects[ri]
			right := cr.Rect.X + cr.Rect.Width
			lastRight = max(lastRight, right)
		}
		if lastRight > 0 && x > lastRight {
			if _, ok := l.LogAttrByIndex[lineEnd]; ok {
				return lineEnd
			}
		}
	}

	if !foundAny {
		return targetLine.StartIndex
	}
	return closestCharIdx
}

// GetSelectionRects returns rectangles covering [start, end).
func (l *Layout) GetSelectionRects(start, end int) []Rect {
	if start >= end || len(l.Lines) == 0 {
		return nil
	}
	s := start
	if s < 0 {
		s = 0
	}

	var rects []Rect
	for _, line := range l.Lines {
		lineEnd := line.StartIndex + line.Length
		overlapStart := max(s, line.StartIndex)
		overlapEnd := min(end, lineEnd)
		if overlapStart >= overlapEnd {
			continue
		}

		minX := maxDistance
		maxX := -maxDistance
		found := false
		for i := overlapStart; i < overlapEnd; i++ {
			ri, ok := l.CharRectByIndex[i]
			if !ok {
				continue
			}
			cr := l.CharRects[ri]
			minX = min(minX, cr.Rect.X)
			right := cr.Rect.X + cr.Rect.Width
			maxX = max(maxX, right)
			found = true
		}
		if found {
			rects = append(rects, Rect{
				X:      minX,
				Y:      line.Rect.Y,
				Width:  maxX - minX,
				Height: line.Rect.Height,
			})
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
	if !ok && byteIndex != 0 {
		return CursorPosition{}, false
	}
	if ok && attrIdx >= 0 && attrIdx < len(l.LogAttrs) {
		if !l.LogAttrs[attrIdx].IsCursorPosition {
			return CursorPosition{}, false
		}
	}

	// Try exact char rect. Skip '\n' — its glyph rect is at the
	// start of the next line, but the cursor belongs at the end
	// of the current line (handled by the line-based fallback).
	if byteIndex >= len(l.Text) || l.Text[byteIndex] != '\n' {
		if r, ok := l.GetCharRect(byteIndex); ok {
			if line, ok := l.lineForByteIndex(byteIndex); ok {
				return CursorPosition{
					X:      r.X,
					Y:      line.Rect.Y,
					Height: line.Rect.Height,
				}, true
			}
			return CursorPosition{
				X:      r.X,
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
	for i := len(positions) - 1; i >= 0; i-- {
		if positions[i] < byteIndex {
			return positions[i]
		}
	}
	return 0
}

// MoveCursorRight returns the next valid cursor position.
func (l *Layout) MoveCursorRight(byteIndex int) int {
	if len(l.LogAttrs) == 0 {
		return byteIndex
	}
	positions := l.GetValidCursorPositions()
	for _, pos := range positions {
		if pos > byteIndex {
			return pos
		}
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
	lineEnd := line.StartIndex + line.Length
	closestIdx := line.StartIndex
	minDist := maxDistance

	for i := line.StartIndex; i < lineEnd; i++ {
		ri, ok := l.CharRectByIndex[i]
		if !ok {
			continue
		}
		cr := l.CharRects[ri]
		mid := cr.Rect.X + cr.Rect.Width/2
		dist := absF32(targetX - mid)
		if dist < minDist {
			minDist = dist
			closestIdx = i
		}
	}

	// Check if closer to end of line.
	endX := line.Rect.X + line.Rect.Width
	if absF32(targetX-endX) < minDist {
		return lineEnd
	}
	return closestIdx
}

func absF32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
