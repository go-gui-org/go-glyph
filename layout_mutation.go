package glyph

import "github.com/rivo/uniseg"

// MutationResult contains the result of applying a text mutation.
//
// RangeStart and RangeEnd are byte offsets. For a pure deletion,
// [RangeStart, RangeEnd) is the removed range in the old text. For an
// insertion or a replacement, it is the inserted range in the new text.
// UndoManager depends on this convention.
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

// hasCaretData reports whether the layout can answer cursor queries.
// NoHitTesting layouts and the layouts cached by draw and measure calls carry
// no LogAttrs. The layout cursor queries then answer 0 (left) or "unchanged"
// (right). A delete must not read that answer as a real boundary, or it would
// remove the whole text before the caret.
func hasCaretData(l Layout) bool {
	return len(l.LogAttrs) > 0
}

// prevGraphemeStart returns the start of the grapheme cluster that ends at or
// after byte offset c, so it never returns a value greater than c. The walk
// starts at byte 0 and is O(len(text)). That is acceptable because it runs only
// on the fallback path, once per keystroke.
func prevGraphemeStart(text string, c int) int {
	if c <= 0 {
		return 0
	}
	pos, state := 0, -1
	rest := text
	for len(rest) > 0 {
		cluster, r, _, ns := uniseg.FirstGraphemeClusterInString(rest, state)
		if pos+len(cluster) >= c {
			return pos
		}
		pos += len(cluster)
		rest, state = r, ns
	}
	return pos
}

// nextGraphemeEnd returns the end of the grapheme cluster that contains byte
// offset c or starts at it, so it never returns a value less than or equal to c
// when c < len(text). The walk starts at byte 0 and is O(len(text)); see
// prevGraphemeStart for why that is acceptable.
func nextGraphemeEnd(text string, c int) int {
	if c >= len(text) {
		return len(text)
	}
	pos, state := 0, -1
	rest := text
	for len(rest) > 0 {
		cluster, r, _, ns := uniseg.FirstGraphemeClusterInString(rest, state)
		if pos+len(cluster) > c {
			return pos + len(cluster)
		}
		pos += len(cluster)
		rest, state = r, ns
	}
	return len(text)
}

// DeleteBackward removes one grapheme cluster before cursor
// (Backspace). Uses layout.MoveCursorLeft for grapheme boundary. A layout
// without hit-test data falls back to Unicode grapheme segmentation.
func DeleteBackward(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	// Clamp what the layout returns too: a layout built for another
	// (longer) text can name offsets past the end of this one. The same
	// holds for every layout-driven delete below.
	var prev int
	if hasCaretData(layout) {
		prev = clampIndex(layout.MoveCursorLeft(c), len(text))
	} else {
		prev = prevGraphemeStart(text, c)
	}
	return deleteRange(text, prev, c, c)
}

// DeleteForward removes one grapheme cluster after cursor (Delete). A layout
// without hit-test data falls back to Unicode grapheme segmentation.
func DeleteForward(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	var next int
	if hasCaretData(layout) {
		next = clampIndex(layout.MoveCursorRight(c), len(text))
	} else {
		next = nextGraphemeEnd(text, c)
	}
	return deleteRange(text, c, next, c)
}

// deleteRange removes text[start:end] and puts the cursor at cursor. An
// empty or inverted range changes nothing. Both bounds must already be
// clamped to the text.
func deleteRange(text string, start, end, cursor int) MutationResult {
	if start >= end {
		return MutationResult{NewText: text, CursorPos: cursor}
	}
	return MutationResult{
		NewText:     text[:start] + text[end:],
		CursorPos:   start,
		DeletedText: text[start:end],
		RangeStart:  start,
		RangeEnd:    end,
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
	c := clampIndex(cursor, len(text))
	// Without word data no safe boundary is known, so do nothing.
	if !hasCaretData(layout) {
		return MutationResult{NewText: text, CursorPos: c}
	}
	wordStart := clampIndex(layout.MoveCursorWordLeft(c), len(text))
	return deleteRange(text, wordStart, c, c)
}

// DeleteToWordEnd removes text from cursor to next word boundary
// (Option+Delete).
func DeleteToWordEnd(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	// Without word data no safe boundary is known, so do nothing.
	if !hasCaretData(layout) {
		return MutationResult{NewText: text, CursorPos: c}
	}
	wordEnd := clampIndex(layout.MoveCursorWordRight(c), len(text))
	return deleteRange(text, c, wordEnd, c)
}

// DeleteToLineStart removes text from cursor to line start
// (Cmd+Backspace).
func DeleteToLineStart(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	// A layout with no lines has no line start to delete to. NoHitTesting
	// layouts keep their Lines, so this guard does not affect them.
	if len(layout.Lines) == 0 {
		return MutationResult{NewText: text, CursorPos: c}
	}
	lineStart := clampIndex(layout.MoveCursorLineStart(c), len(text))
	return deleteRange(text, lineStart, c, c)
}

// DeleteToLineEnd removes text from cursor to line end
// (Cmd+Delete).
func DeleteToLineEnd(text string, layout Layout, cursor int) MutationResult {
	c := clampIndex(cursor, len(text))
	// Same guard as DeleteToLineStart.
	if len(layout.Lines) == 0 {
		return MutationResult{NewText: text, CursorPos: c}
	}
	lineEnd := clampIndex(layout.MoveCursorLineEnd(c), len(text))
	return deleteRange(text, c, lineEnd, c)
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
		RangeEnd:    selEnd,
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
