package glyph

// MutationResult contains the result of applying a text mutation.
type MutationResult struct {
	NewText     string
	DeletedText string
	CursorPos   int
	RangeStart  int
	RangeEnd    int
}

// TextChange captures mutation info for undo support and events.
type TextChange struct {
	NewText    string
	OldText    string
	RangeStart int
	RangeEnd   int
}

// ToChange converts a MutationResult to a TextChange.
func (m MutationResult) ToChange(inserted string) TextChange {
	return TextChange{
		RangeStart: m.RangeStart,
		RangeEnd:   m.RangeEnd,
		NewText:    inserted,
		OldText:    m.DeletedText,
	}
}

// DeleteBackward removes one grapheme cluster before cursor
// (Backspace). Uses layout.MoveCursorLeft for grapheme boundary.
func DeleteBackward(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	if c == 0 {
		return MutationResult{NewText: text, CursorPos: 0}
	}
	prev := layout.MoveCursorLeft(c)
	return MutationResult{
		NewText:     text[:prev] + text[c:],
		CursorPos:   prev,
		DeletedText: text[prev:c],
		RangeStart:  prev,
		RangeEnd:    c,
	}
}

// DeleteForward removes one grapheme cluster after cursor (Delete).
func DeleteForward(text string, layout Layout, cursor int) MutationResult {
	next := layout.MoveCursorRight(cursor)
	if next == cursor {
		return MutationResult{NewText: text, CursorPos: cursor}
	}
	return MutationResult{
		NewText:     text[:cursor] + text[next:],
		CursorPos:   cursor,
		DeletedText: text[cursor:next],
		RangeStart:  cursor,
		RangeEnd:    next,
	}
}

// InsertText inserts a string at cursor position.
func InsertText(text string, cursor int, insert string) MutationResult {
	c := clampIndex(cursor, len(text))
	return MutationResult{
		NewText:    text[:c] + insert + text[c:],
		CursorPos:  c + len(insert),
		RangeStart: c,
		RangeEnd:   c + len(insert),
	}
}

// DeleteToWordBoundary removes text from cursor to previous word
// boundary (Option+Backspace).
func DeleteToWordBoundary(text string, layout Layout, cursor int) MutationResult {
	if cursor == 0 {
		return MutationResult{NewText: text, CursorPos: 0}
	}
	wordStart := layout.MoveCursorWordLeft(cursor)
	return MutationResult{
		NewText:     text[:wordStart] + text[cursor:],
		CursorPos:   wordStart,
		DeletedText: text[wordStart:cursor],
		RangeStart:  wordStart,
		RangeEnd:    cursor,
	}
}

// DeleteToWordEnd removes text from cursor to next word boundary
// (Option+Delete).
func DeleteToWordEnd(text string, layout Layout, cursor int) MutationResult {
	wordEnd := layout.MoveCursorWordRight(cursor)
	if wordEnd == cursor {
		return MutationResult{NewText: text, CursorPos: cursor}
	}
	return MutationResult{
		NewText:     text[:cursor] + text[wordEnd:],
		CursorPos:   cursor,
		DeletedText: text[cursor:wordEnd],
		RangeStart:  cursor,
		RangeEnd:    wordEnd,
	}
}

// DeleteToLineStart removes text from cursor to line start
// (Cmd+Backspace).
func DeleteToLineStart(text string, layout Layout, cursor int) MutationResult {
	lineStart := layout.MoveCursorLineStart(cursor)
	if lineStart == cursor {
		return MutationResult{NewText: text, CursorPos: cursor}
	}
	return MutationResult{
		NewText:     text[:lineStart] + text[cursor:],
		CursorPos:   lineStart,
		DeletedText: text[lineStart:cursor],
		RangeStart:  lineStart,
		RangeEnd:    cursor,
	}
}

// DeleteToLineEnd removes text from cursor to line end
// (Cmd+Delete).
func DeleteToLineEnd(text string, layout Layout, cursor int) MutationResult {
	lineEnd := layout.MoveCursorLineEnd(cursor)
	if lineEnd == cursor {
		return MutationResult{NewText: text, CursorPos: cursor}
	}
	return MutationResult{
		NewText:     text[:cursor] + text[lineEnd:],
		CursorPos:   cursor,
		DeletedText: text[cursor:lineEnd],
		RangeStart:  cursor,
		RangeEnd:    lineEnd,
	}
}

// DeleteSelection removes text between cursor and anchor.
func DeleteSelection(text string, cursor, anchor int) MutationResult {
	c := clampIndex(cursor, len(text))
	a := clampIndex(anchor, len(text))
	if c == a {
		return MutationResult{NewText: text, CursorPos: c}
	}
	selStart, selEnd := c, a
	if selStart > selEnd {
		selStart, selEnd = selEnd, selStart
	}
	return MutationResult{
		NewText:     text[:selStart] + text[selEnd:],
		CursorPos:   selStart,
		DeletedText: text[selStart:selEnd],
		RangeStart:  selStart,
		RangeEnd:    selStart,
	}
}

// InsertReplacingSelection inserts text, replacing any selection.
func InsertReplacingSelection(text string, cursor, anchor int, insert string) MutationResult {
	c := clampIndex(cursor, len(text))
	a := clampIndex(anchor, len(text))
	if c == a {
		return InsertText(text, c, insert)
	}
	selStart, selEnd := c, a
	if selStart > selEnd {
		selStart, selEnd = selEnd, selStart
	}
	return MutationResult{
		NewText:     text[:selStart] + insert + text[selEnd:],
		CursorPos:   selStart + len(insert),
		DeletedText: text[selStart:selEnd],
		RangeStart:  selStart,
		RangeEnd:    selStart + len(insert),
	}
}

// GetSelectedText returns the text between cursor and anchor.
func GetSelectedText(text string, cursor, anchor int) string {
	c := clampIndex(cursor, len(text))
	a := clampIndex(anchor, len(text))
	if c == a {
		return ""
	}
	selStart, selEnd := c, a
	if selStart > selEnd {
		selStart, selEnd = selEnd, selStart
	}
	return text[selStart:selEnd]
}

// CutSelection removes selected text and returns it for clipboard.
func CutSelection(text string, cursor, anchor int) (string, MutationResult) {
	if cursor == anchor {
		return "", MutationResult{NewText: text, CursorPos: cursor}
	}
	cutText := GetSelectedText(text, cursor, anchor)
	result := DeleteSelection(text, cursor, anchor)
	return cutText, result
}

func clampIndex(val, hi int) int {
	return max(0, min(val, hi))
}
