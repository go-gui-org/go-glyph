package glyph

import "unicode/utf8"

// maxCompositionClauses limits how many clauses one composition keeps.
// Real IMEs send a few clauses. The limit stops a bad input source from
// growing the slice without bound.
const maxCompositionClauses = 256

// Clause represents a segment in multi-clause CJK composition.
type Clause struct {
	Start  int
	Length int
	Style  ClauseStyle
}

// ClauseRects holds clause index, rects, and style for rendering.
type ClauseRects struct {
	Rects     []Rect
	ClauseIdx int
	Style     ClauseStyle
}

// CompositionState tracks IME composition for preedit display.
//
// All offsets (PreeditStart, CursorOffset, Clause.Start and
// Clause.Length) are UTF-8 byte offsets. A platform bridge that gets
// UTF-16 offsets (for example macOS NSRange) must convert them first.
type CompositionState struct {
	PreeditText    string
	Clauses        []Clause
	Phase          CompositionPhase
	PreeditStart   int
	CursorOffset   int
	SelectedClause int
}

// NewCompositionState returns an initialized CompositionState.
func NewCompositionState() CompositionState {
	return CompositionState{
		Phase:          CompositionNone,
		SelectedClause: -1,
	}
}

// IsComposing returns true if composition is active.
func (cs *CompositionState) IsComposing() bool {
	return cs.Phase == CompositionStarted ||
		cs.Phase == CompositionUpdating
}

// Start begins composition at document cursor position.
// A negative position is clamped to 0; it comes from the platform
// bridge and must never place the preedit before the document.
func (cs *CompositionState) Start(cursorPos int) {
	cs.Phase = CompositionStarted
	cs.PreeditStart = max(0, cursorPos)
	cs.PreeditText = ""
	cs.CursorOffset = 0
	cs.Clauses = cs.Clauses[:0]
	cs.SelectedClause = -1
}

// SetMarkedText updates preedit from IME. cursorInPreedit is a byte
// offset into text. It is clamped to [0, len(text)] and moved back to
// the start of a rune, so the caret never lands in committed text or
// inside a multi-byte character.
func (cs *CompositionState) SetMarkedText(text string, cursorInPreedit int) {
	cs.PreeditText = text
	cs.CursorOffset = clampPreeditOffset(text, cursorInPreedit)
	cs.Phase = CompositionUpdating
}

// SetClauses updates clause segmentation from IME attributes.
// The clauses are copied, so the caller keeps ownership of its slice.
// Only the first maxCompositionClauses are kept. A selected index that
// is not a kept clause becomes -1.
func (cs *CompositionState) SetClauses(clauses []Clause, selected int) {
	// Copy into our own array. Keeping the caller's slice would let a
	// later ClearClauses + HandleClause write into the caller's memory.
	clauses = clauses[:min(len(clauses), maxCompositionClauses)]
	cs.Clauses = append(cs.Clauses[:0], clauses...)
	if selected < 0 || selected >= len(cs.Clauses) {
		selected = -1
	}
	cs.SelectedClause = selected
}

// Commit finalizes composition, returns text to insert.
func (cs *CompositionState) Commit() string {
	result := cs.PreeditText
	cs.Reset()
	return result
}

// Reset discards composition without inserting text.
func (cs *CompositionState) Reset() {
	cs.Phase = CompositionNone
	cs.PreeditText = ""
	cs.PreeditStart = 0
	cs.CursorOffset = 0
	cs.Clauses = cs.Clauses[:0]
	cs.SelectedClause = -1
}

// DocumentCursorPos returns absolute cursor position in document.
// CursorOffset is clamped again here because it is a public field and
// a caller can set it without SetMarkedText.
func (cs *CompositionState) DocumentCursorPos() int {
	return cs.PreeditStart + clampPreeditOffset(cs.PreeditText, cs.CursorOffset)
}

// clampPreeditOffset limits off to [0, len(text)] and moves it back to
// the first byte of the rune that contains it.
func clampPreeditOffset(text string, off int) int {
	off = max(0, min(off, len(text)))
	for off > 0 && off < len(text) && !utf8.RuneStart(text[off]) {
		off--
	}
	return off
}

// PreeditEnd returns byte offset where preedit ends in document.
func (cs *CompositionState) PreeditEnd() int {
	return cs.PreeditStart + len(cs.PreeditText)
}

// CompositionBounds returns bounding rect covering entire preedit.
// Returns ok=false if not composing or layout is nil. layout is a
// pointer, like the other layout query methods, to avoid copying the
// Layout struct on every call.
func (cs *CompositionState) CompositionBounds(layout *Layout) (Rect, bool) {
	if layout == nil || !cs.IsComposing() || len(cs.PreeditText) == 0 {
		return Rect{}, false
	}
	rects := layout.GetSelectionRects(cs.PreeditStart, cs.PreeditEnd())
	if len(rects) == 0 {
		return Rect{}, false
	}
	minX, minY := rects[0].X, rects[0].Y
	maxX := rects[0].X + rects[0].Width
	maxY := rects[0].Y + rects[0].Height
	for _, r := range rects[1:] {
		minX = min(minX, r.X)
		minY = min(minY, r.Y)
		maxX = max(maxX, r.X+r.Width)
		maxY = max(maxY, r.Y+r.Height)
	}
	return Rect{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}, true
}

// GetClauseRects returns selection rects for each clause. Clauses are
// clipped to the preedit, so a bad clause cannot underline committed
// text. With no clauses, the whole preedit is one ClauseRaw clause.
func (cs *CompositionState) GetClauseRects(layout *Layout) []ClauseRects {
	if layout == nil {
		return nil
	}
	var result []ClauseRects
	cs.forEachClause(func(idx, start, end int, style ClauseStyle) {
		rects := layout.GetSelectionRects(start, end)
		if len(rects) > 0 {
			result = append(result, ClauseRects{
				ClauseIdx: idx,
				Rects:     rects,
				Style:     style,
			})
		}
	})
	return result
}

// forEachClause calls fn with the document byte range [start, end) of
// each non-empty clause, clipped to the preedit. It does nothing when
// not composing. With no clauses it reports the whole preedit as clause
// 0 with ClauseRaw. It is shared by GetClauseRects and the draw path so
// both clip in the same way; fn does not escape, so no allocation.
func (cs *CompositionState) forEachClause(
	fn func(idx, start, end int, style ClauseStyle)) {

	if !cs.IsComposing() {
		return
	}
	n := len(cs.PreeditText)
	if len(cs.Clauses) == 0 {
		if n > 0 {
			fn(0, cs.PreeditStart, cs.PreeditStart+n, ClauseRaw)
		}
		return
	}
	for i, c := range cs.Clauses {
		// Normalize a negative start by shrinking the length, so
		// [-3, 2) clips to [0, 2) instead of [0, 5). Lengths that
		// are empty after this cover nothing and are skipped.
		// Clipping stays in preedit-relative space, before adding
		// PreeditStart, so a huge Length cannot overflow into a
		// document range outside the preedit.
		s, l := c.Start, c.Length
		if l <= 0 {
			continue
		}
		if s < 0 {
			l += s
			if l <= 0 {
				continue
			}
			s = 0
		}
		s = min(s, n)
		e := s + min(l, n-s)
		if e <= s {
			continue
		}
		fn(i, cs.PreeditStart+s, cs.PreeditStart+e, c.Style)
	}
}

// HandleMarkedText processes setMarkedText from IME overlay.
// Empty text means the user deleted the whole preedit, so an active
// composition ends (same as HandleUnmarkText). Text that fails
// ValidateTextInput for any other reason (too long, bad UTF-8, NUL)
// returns that error and the current state is kept.
func (cs *CompositionState) HandleMarkedText(text string,
	cursorInPreedit, documentCursor int) error {

	if text == "" {
		// ValidateTextInput rejects "", but for an IME it is a real
		// event. Ignoring it left the old preedit on screen.
		if cs.IsComposing() {
			cs.Reset()
		}
		return nil
	}
	if err := ValidateTextInput(text, MaxTextLength,
		"HandleMarkedText"); err != nil {
		return err
	}
	if !cs.IsComposing() {
		cs.Start(documentCursor)
	}
	cs.SetMarkedText(text, cursorInPreedit)
	return nil
}

// HandleInsertText processes insertText from IME overlay and returns
// the text to insert. Text that fails ValidateTextInput returns "" and
// that error, and the composition state does not change.
func (cs *CompositionState) HandleInsertText(text string) (string, error) {
	if err := ValidateTextInput(text, MaxTextLength,
		"HandleInsertText"); err != nil {
		return "", err
	}
	if cs.IsComposing() {
		cs.Reset()
	}
	return text, nil
}

// HandleUnmarkText cancels composition without committing.
func (cs *CompositionState) HandleUnmarkText() {
	cs.Reset()
}

// HandleClause processes clause info from IME overlay. style uses the
// ClauseStyle values (0 raw, 1 converted, 2 selected); other values
// give ClauseRaw. A selected clause also sets SelectedClause. Negative
// ranges, empty ranges, and clauses past maxCompositionClauses are
// dropped.
func (cs *CompositionState) HandleClause(start, length, style int) {
	if start < 0 || length <= 0 ||
		len(cs.Clauses) >= maxCompositionClauses {
		return
	}
	clauseStyle := ClauseRaw
	if style >= int(ClauseRaw) && style <= int(ClauseSelected) {
		clauseStyle = ClauseStyle(style)
	}
	if clauseStyle == ClauseSelected {
		cs.SelectedClause = len(cs.Clauses)
	}
	cs.Clauses = append(cs.Clauses, Clause{
		Start:  start,
		Length: length,
		Style:  clauseStyle,
	})
}

// ClearClauses resets clause array for fresh enumeration.
func (cs *CompositionState) ClearClauses() {
	cs.Clauses = cs.Clauses[:0]
	cs.SelectedClause = -1
}

// DeadKeyState tracks pending dead key for accent composition.
type DeadKeyState struct {
	Pending    rune
	HasPending bool
	PendingPos int
}

// TryCombine attempts to combine pending dead key with base char.
// Returns (result, wasCombined). A space gives the accent alone, as on
// common keyboard layouts. If invalid: returns both chars.
func (dks *DeadKeyState) TryCombine(base rune) (string, bool) {
	if !dks.HasPending {
		return "", false
	}
	dead := dks.Pending
	dks.Reset()

	if base == ' ' {
		return string(dead), true
	}

	if combined, ok := combineDeadKey(dead, base); ok {
		return string(combined), true
	}
	return string(dead) + string(base), false
}

// StartDeadKey records a dead key press.
func (dks *DeadKeyState) StartDeadKey(dead rune, pos int) {
	dks.Pending = dead
	dks.HasPending = true
	dks.PendingPos = pos
}

// Clear cancels pending dead key.
func (dks *DeadKeyState) Clear() {
	dks.Reset()
}

// Reset zeros all fields.
func (dks *DeadKeyState) Reset() {
	dks.Pending = 0
	dks.HasPending = false
	dks.PendingPos = 0
}

// IsDeadKey returns true if the rune can start a dead-key sequence in
// the table used by TryCombine. It does not know the keyboard layout:
// on a US layout ' " , : are plain characters. Call it only for keys
// the platform reports as dead keys.
func IsDeadKey(r rune) bool {
	switch r {
	case '`', '\'', '^', '~', '"', ':', ',':
		return true
	}
	return false
}

var deadKeyTable = map[[2]rune]rune{
	{'`', 'a'}: 0x00E0, {'`', 'e'}: 0x00E8, {'`', 'i'}: 0x00EC,
	{'`', 'o'}: 0x00F2, {'`', 'u'}: 0x00F9,
	{'`', 'A'}: 0x00C0, {'`', 'E'}: 0x00C8, {'`', 'I'}: 0x00CC,
	{'`', 'O'}: 0x00D2, {'`', 'U'}: 0x00D9,

	{'\'', 'a'}: 0x00E1, {'\'', 'e'}: 0x00E9, {'\'', 'i'}: 0x00ED,
	{'\'', 'o'}: 0x00F3, {'\'', 'u'}: 0x00FA,
	{'\'', 'A'}: 0x00C1, {'\'', 'E'}: 0x00C9, {'\'', 'I'}: 0x00CD,
	{'\'', 'O'}: 0x00D3, {'\'', 'U'}: 0x00DA,
	{'\'', 'y'}: 0x00FD, {'\'', 'Y'}: 0x00DD,

	{'^', 'a'}: 0x00E2, {'^', 'e'}: 0x00EA, {'^', 'i'}: 0x00EE,
	{'^', 'o'}: 0x00F4, {'^', 'u'}: 0x00FB,
	{'^', 'A'}: 0x00C2, {'^', 'E'}: 0x00CA, {'^', 'I'}: 0x00CE,
	{'^', 'O'}: 0x00D4, {'^', 'U'}: 0x00DB,

	{'~', 'a'}: 0x00E3, {'~', 'n'}: 0x00F1, {'~', 'o'}: 0x00F5,
	{'~', 'A'}: 0x00C3, {'~', 'N'}: 0x00D1, {'~', 'O'}: 0x00D5,

	{'"', 'a'}: 0x00E4, {'"', 'e'}: 0x00EB, {'"', 'i'}: 0x00EF,
	{'"', 'o'}: 0x00F6, {'"', 'u'}: 0x00FC, {'"', 'y'}: 0x00FF,
	{'"', 'A'}: 0x00C4, {'"', 'E'}: 0x00CB, {'"', 'I'}: 0x00CF,
	{'"', 'O'}: 0x00D6, {'"', 'U'}: 0x00DC, {'"', 'Y'}: 0x0178,

	{':', 'a'}: 0x00E4, {':', 'e'}: 0x00EB, {':', 'i'}: 0x00EF,
	{':', 'o'}: 0x00F6, {':', 'u'}: 0x00FC, {':', 'y'}: 0x00FF,
	{':', 'A'}: 0x00C4, {':', 'E'}: 0x00CB, {':', 'I'}: 0x00CF,
	{':', 'O'}: 0x00D6, {':', 'U'}: 0x00DC, {':', 'Y'}: 0x0178,

	{',', 'c'}: 0x00E7, {',', 'C'}: 0x00C7,
}

// combineDeadKey returns combined character or ok=false.
func combineDeadKey(dead, base rune) (rune, bool) {
	r, ok := deadKeyTable[[2]rune{dead, base}]
	return r, ok
}
