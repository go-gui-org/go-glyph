package glyph

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
func (cs *CompositionState) Start(cursorPos int) {
	cs.Phase = CompositionStarted
	cs.PreeditStart = cursorPos
	cs.PreeditText = ""
	cs.CursorOffset = 0
	cs.Clauses = cs.Clauses[:0]
	cs.SelectedClause = -1
}

// SetMarkedText updates preedit from IME.
func (cs *CompositionState) SetMarkedText(text string, cursorInPreedit int) {
	cs.PreeditText = text
	cs.CursorOffset = cursorInPreedit
	cs.Phase = CompositionUpdating
}

// SetClauses updates clause segmentation from IME attributes.
func (cs *CompositionState) SetClauses(clauses []Clause, selected int) {
	cs.Clauses = clauses
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
func (cs *CompositionState) DocumentCursorPos() int {
	return cs.PreeditStart + cs.CursorOffset
}

// PreeditEnd returns byte offset where preedit ends in document.
func (cs *CompositionState) PreeditEnd() int {
	return cs.PreeditStart + len(cs.PreeditText)
}

// CompositionBounds returns bounding rect covering entire preedit.
// Returns ok=false if not composing.
func (cs *CompositionState) CompositionBounds(layout Layout) (Rect, bool) {
	if !cs.IsComposing() || len(cs.PreeditText) == 0 {
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

// GetClauseRects returns selection rects for each clause.
func (cs *CompositionState) GetClauseRects(layout Layout) []ClauseRects {
	if !cs.IsComposing() {
		return nil
	}
	if len(cs.Clauses) == 0 && len(cs.PreeditText) > 0 {
		rects := layout.GetSelectionRects(cs.PreeditStart, cs.PreeditEnd())
		if len(rects) > 0 {
			return []ClauseRects{{
				ClauseIdx: 0,
				Rects:     rects,
				Style:     ClauseRaw,
			}}
		}
		return nil
	}
	var result []ClauseRects
	for i, clause := range cs.Clauses {
		clauseStart := cs.PreeditStart + clause.Start
		clauseEnd := clauseStart + clause.Length
		rects := layout.GetSelectionRects(clauseStart, clauseEnd)
		if len(rects) > 0 {
			result = append(result, ClauseRects{
				ClauseIdx: i,
				Rects:     rects,
				Style:     clause.Style,
			})
		}
	}
	return result
}

// HandleMarkedText processes setMarkedText from IME overlay.
func (cs *CompositionState) HandleMarkedText(text string,
	cursorInPreedit, documentCursor int) {

	if err := ValidateTextInput(text, MaxTextLength,
		"HandleMarkedText"); err != nil {
		return
	}
	if !cs.IsComposing() {
		cs.Start(documentCursor)
	}
	cs.SetMarkedText(text, cursorInPreedit)
}

// HandleInsertText processes insertText from IME overlay.
func (cs *CompositionState) HandleInsertText(text string) string {
	if err := ValidateTextInput(text, MaxTextLength,
		"HandleInsertText"); err != nil {
		return ""
	}
	if cs.IsComposing() {
		cs.Reset()
	}
	return text
}

// HandleUnmarkText cancels composition without committing.
func (cs *CompositionState) HandleUnmarkText() {
	cs.Reset()
}

// HandleClause processes clause info from IME overlay.
func (cs *CompositionState) HandleClause(start, length, style int) {
	if start < 0 || length < 0 {
		return
	}
	clauseStyle := ClauseRaw
	switch style {
	case 2:
		clauseStyle = ClauseSelected
	case 1:
		clauseStyle = ClauseConverted
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
// Returns (result, wasCombined). If invalid: returns both chars.
func (dks *DeadKeyState) TryCombine(base rune) (string, bool) {
	if !dks.HasPending {
		return "", false
	}
	dead := dks.Pending
	dks.Reset()

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

// IsDeadKey returns true if the rune is a dead key accent starter.
func IsDeadKey(r rune) bool {
	switch r {
	case '`', '\'', '^', '~', '"', ':', ',':
		return true
	}
	return false
}

// combineDeadKey returns combined character or ok=false.
func combineDeadKey(dead, base rune) (rune, bool) {
	switch dead {
	case '`':
		switch base {
		case 'a':
			return 0x00E0, true
		case 'e':
			return 0x00E8, true
		case 'i':
			return 0x00EC, true
		case 'o':
			return 0x00F2, true
		case 'u':
			return 0x00F9, true
		case 'A':
			return 0x00C0, true
		case 'E':
			return 0x00C8, true
		case 'I':
			return 0x00CC, true
		case 'O':
			return 0x00D2, true
		case 'U':
			return 0x00D9, true
		}
	case '\'':
		switch base {
		case 'a':
			return 0x00E1, true
		case 'e':
			return 0x00E9, true
		case 'i':
			return 0x00ED, true
		case 'o':
			return 0x00F3, true
		case 'u':
			return 0x00FA, true
		case 'A':
			return 0x00C1, true
		case 'E':
			return 0x00C9, true
		case 'I':
			return 0x00CD, true
		case 'O':
			return 0x00D3, true
		case 'U':
			return 0x00DA, true
		}
	case '^':
		switch base {
		case 'a':
			return 0x00E2, true
		case 'e':
			return 0x00EA, true
		case 'i':
			return 0x00EE, true
		case 'o':
			return 0x00F4, true
		case 'u':
			return 0x00FB, true
		case 'A':
			return 0x00C2, true
		case 'E':
			return 0x00CA, true
		case 'I':
			return 0x00CE, true
		case 'O':
			return 0x00D4, true
		case 'U':
			return 0x00DB, true
		}
	case '~':
		switch base {
		case 'a':
			return 0x00E3, true
		case 'n':
			return 0x00F1, true
		case 'o':
			return 0x00F5, true
		case 'A':
			return 0x00C3, true
		case 'N':
			return 0x00D1, true
		case 'O':
			return 0x00D5, true
		}
	case '"', ':':
		switch base {
		case 'a':
			return 0x00E4, true
		case 'e':
			return 0x00EB, true
		case 'i':
			return 0x00EF, true
		case 'o':
			return 0x00F6, true
		case 'u':
			return 0x00FC, true
		case 'y':
			return 0x00FF, true
		case 'A':
			return 0x00C4, true
		case 'E':
			return 0x00CB, true
		case 'I':
			return 0x00CF, true
		case 'O':
			return 0x00D6, true
		case 'U':
			return 0x00DC, true
		}
	case ',':
		switch base {
		case 'c':
			return 0x00E7, true
		case 'C':
			return 0x00C7, true
		}
	}
	return 0, false
}
