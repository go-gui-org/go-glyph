package glyph

import "testing"

func TestCompositionStateLifecycle(t *testing.T) {
	cs := NewCompositionState()
	if cs.IsComposing() {
		t.Error("should not be composing initially")
	}

	cs.Start(10)
	if !cs.IsComposing() {
		t.Error("should be composing after Start")
	}
	if cs.PreeditStart != 10 {
		t.Errorf("PreeditStart = %d, want 10", cs.PreeditStart)
	}

	cs.SetMarkedText("abc", 2)
	if cs.PreeditText != "abc" {
		t.Errorf("PreeditText = %q", cs.PreeditText)
	}
	if cs.CursorOffset != 2 {
		t.Errorf("CursorOffset = %d", cs.CursorOffset)
	}

	result := cs.Commit()
	if result != "abc" {
		t.Errorf("Commit = %q, want 'abc'", result)
	}
	if cs.IsComposing() {
		t.Error("should not be composing after Commit")
	}
}

func TestCompositionStateReset(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(5)
	cs.SetMarkedText("test", 3)
	cs.Reset()

	if cs.IsComposing() {
		t.Error("should not be composing after Reset")
	}
	if cs.PreeditText != "" {
		t.Errorf("PreeditText = %q after Reset", cs.PreeditText)
	}
}

func TestCompositionDocumentCursorPos(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(10)
	cs.SetMarkedText("hello", 3)
	if got := cs.DocumentCursorPos(); got != 13 {
		t.Errorf("DocumentCursorPos = %d, want 13", got)
	}
}

func TestCompositionPreeditEnd(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(10)
	cs.SetMarkedText("hello", 5)
	if got := cs.PreeditEnd(); got != 15 {
		t.Errorf("PreeditEnd = %d, want 15", got)
	}
}

func TestCompositionBoundsNotComposing(t *testing.T) {
	cs := NewCompositionState()
	_, ok := cs.CompositionBounds(&Layout{})
	if ok {
		t.Error("should not have bounds when not composing")
	}
}

func TestCompositionClauses(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("test", 4)

	cs.HandleClause(0, 2, 0) // raw
	cs.HandleClause(2, 2, 2) // selected

	if len(cs.Clauses) != 2 {
		t.Fatalf("clause count = %d, want 2", len(cs.Clauses))
	}
	if cs.Clauses[0].Style != ClauseRaw {
		t.Errorf("clause 0 style = %d", cs.Clauses[0].Style)
	}
	if cs.Clauses[1].Style != ClauseSelected {
		t.Errorf("clause 1 style = %d", cs.Clauses[1].Style)
	}
}

func TestCompositionClearClauses(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.HandleClause(0, 5, 1)
	cs.ClearClauses()
	if len(cs.Clauses) != 0 {
		t.Errorf("clauses not cleared: len=%d", len(cs.Clauses))
	}
	if cs.SelectedClause != -1 {
		t.Errorf("SelectedClause = %d, want -1", cs.SelectedClause)
	}
}

func TestCompositionHandleMarkedText(t *testing.T) {
	cs := NewCompositionState()
	if err := cs.HandleMarkedText("hello", 3, 10); err != nil {
		t.Fatal(err)
	}
	if !cs.IsComposing() {
		t.Error("should be composing after HandleMarkedText")
	}
	if cs.PreeditText != "hello" {
		t.Errorf("PreeditText = %q", cs.PreeditText)
	}
}

func TestCompositionHandleInsertText(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(5)
	cs.SetMarkedText("test", 4)

	result, err := cs.HandleInsertText("final")
	if err != nil {
		t.Fatal(err)
	}
	if result != "final" {
		t.Errorf("HandleInsertText = %q, want 'final'", result)
	}
	if cs.IsComposing() {
		t.Error("should not be composing after HandleInsertText")
	}
}

func TestCompositionHandleUnmarkText(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("abc", 3)
	cs.HandleUnmarkText()
	if cs.IsComposing() {
		t.Error("should not be composing after HandleUnmarkText")
	}
}

func TestCompositionHandleClauseInvalid(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.HandleClause(-1, 5, 0)
	cs.HandleClause(0, -1, 0)
	if len(cs.Clauses) != 0 {
		t.Error("invalid clauses should be rejected")
	}
}

func TestDeadKeyLifecycle(t *testing.T) {
	dks := DeadKeyState{}
	if dks.HasPending {
		t.Error("should not have pending initially")
	}

	dks.StartDeadKey('`', 5)
	if !dks.HasPending {
		t.Error("should have pending after StartDeadKey")
	}

	result, combined := dks.TryCombine('e')
	if !combined {
		t.Error("grave + e should combine")
	}
	if result != "\u00e8" { // è
		t.Errorf("result = %q, want è", result)
	}
	if dks.HasPending {
		t.Error("pending should be cleared after combine")
	}
}

func TestDeadKeyInvalidCombination(t *testing.T) {
	dks := DeadKeyState{}
	dks.StartDeadKey('`', 0)

	result, combined := dks.TryCombine('x')
	if combined {
		t.Error("grave + x should not combine")
	}
	if result != "`x" {
		t.Errorf("result = %q, want '`x'", result)
	}
}

func TestDeadKeyClear(t *testing.T) {
	dks := DeadKeyState{}
	dks.StartDeadKey('^', 0)
	dks.Clear()
	if dks.HasPending {
		t.Error("should not have pending after Clear")
	}
}

func TestDeadKeyNoPending(t *testing.T) {
	dks := DeadKeyState{}
	result, combined := dks.TryCombine('e')
	if combined || result != "" {
		t.Error("no pending should return empty")
	}
}

func TestIsDeadKey(t *testing.T) {
	deadKeys := []rune{'`', '\'', '^', '~', '"', ':', ','}
	for _, r := range deadKeys {
		if !IsDeadKey(r) {
			t.Errorf("IsDeadKey(%q) = false", r)
		}
	}
	if IsDeadKey('a') {
		t.Error("'a' should not be dead key")
	}
}

func TestCombineDeadKeyAllAccents(t *testing.T) {
	tests := []struct {
		dead, base rune
		want       rune
	}{
		{'`', 'a', 0x00E0},  // à
		{'\'', 'e', 0x00E9}, // é
		{'^', 'i', 0x00EE},  // î
		{'~', 'n', 0x00F1},  // ñ
		{'"', 'u', 0x00FC},  // ü
		{':', 'o', 0x00F6},  // ö
		{',', 'c', 0x00E7},  // ç
		{',', 'C', 0x00C7},  // Ç
		{'`', 'A', 0x00C0},  // À
		{'~', 'O', 0x00D5},  // Õ
	}
	for _, tc := range tests {
		got, ok := combineDeadKey(tc.dead, tc.base)
		if !ok {
			t.Errorf("combineDeadKey(%q, %q) failed", tc.dead, tc.base)
			continue
		}
		if got != tc.want {
			t.Errorf("combineDeadKey(%q, %q) = %U, want %U",
				tc.dead, tc.base, got, tc.want)
		}
	}
}

func TestGetClauseRectsNoClauses(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("hi", 2)

	l := testLayout()
	rects := cs.GetClauseRects(&l)
	// No explicit clauses, but preedit exists: should return
	// single raw clause rect.
	if len(rects) != 1 {
		t.Fatalf("clause rects = %d, want 1", len(rects))
	}
	if rects[0].Style != ClauseRaw {
		t.Errorf("style = %d, want ClauseRaw", rects[0].Style)
	}
}

// An IME sends an empty preedit when the user deletes all of it.
// Composition must end so the old preedit is not drawn.
func TestHandleMarkedTextEmptyEndsComposition(t *testing.T) {
	cs := NewCompositionState()
	if err := cs.HandleMarkedText("abc", 3, 0); err != nil {
		t.Fatal(err)
	}
	cs.HandleClause(0, 3, 0)
	if err := cs.HandleMarkedText("", 0, 0); err != nil {
		t.Errorf("empty marked text: err = %v, want nil", err)
	}
	if cs.IsComposing() {
		t.Error("empty marked text should end composition")
	}
	if cs.PreeditText != "" || len(cs.Clauses) != 0 {
		t.Errorf("state not cleared: text=%q clauses=%d",
			cs.PreeditText, len(cs.Clauses))
	}
}

// Empty marked text when not composing must not start composition.
func TestHandleMarkedTextEmptyNotComposing(t *testing.T) {
	cs := NewCompositionState()
	if err := cs.HandleMarkedText("", 0, 5); err != nil {
		t.Fatal(err)
	}
	if cs.IsComposing() {
		t.Error("empty marked text should not start composition")
	}
}

// SetClauses must copy. Before the fix, a later ClearClauses and
// HandleClause wrote into the caller's backing array.
func TestSetClausesCopiesInput(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("abcd", 0)
	in := []Clause{{Start: 0, Length: 2, Style: ClauseRaw}}
	cs.SetClauses(in, 0)
	cs.ClearClauses()
	cs.HandleClause(1, 3, 2)
	if in[0] != (Clause{Start: 0, Length: 2, Style: ClauseRaw}) {
		t.Errorf("caller slice changed: %+v", in[0])
	}
}

func TestSetClausesSelectedOutOfRange(t *testing.T) {
	cs := NewCompositionState()
	cs.SetClauses([]Clause{{Start: 0, Length: 1}}, 5)
	if cs.SelectedClause != -1 {
		t.Errorf("SelectedClause = %d, want -1", cs.SelectedClause)
	}
}

func TestSetMarkedTextClampsCursor(t *testing.T) {
	tests := []struct {
		text   string
		cursor int
		want   int
	}{
		{"abc", -4, 0},
		{"abc", 99, 3},
		{"abc", 2, 2},
		// "日" is 3 bytes. Offset 1 and 2 are inside the rune and
		// snap back to its start.
		{"日本", 1, 0},
		{"日本", 2, 0},
		{"日本", 4, 3},
		{"日本", 6, 6},
	}
	for _, tc := range tests {
		cs := NewCompositionState()
		cs.Start(10)
		cs.SetMarkedText(tc.text, tc.cursor)
		if cs.CursorOffset != tc.want {
			t.Errorf("SetMarkedText(%q, %d): CursorOffset = %d, want %d",
				tc.text, tc.cursor, cs.CursorOffset, tc.want)
		}
		if got := cs.DocumentCursorPos(); got != 10+tc.want {
			t.Errorf("DocumentCursorPos = %d, want %d", got, 10+tc.want)
		}
	}
}

// A cursor offset set directly on the field is clamped too.
func TestDocumentCursorPosClampsField(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(10)
	cs.SetMarkedText("abc", 0)
	cs.CursorOffset = -5
	if got := cs.DocumentCursorPos(); got != 10 {
		t.Errorf("DocumentCursorPos = %d, want 10", got)
	}
	cs.CursorOffset = 50
	if got := cs.DocumentCursorPos(); got != 13 {
		t.Errorf("DocumentCursorPos = %d, want 13", got)
	}
}

// A clause that goes past the preedit must not underline the
// committed text after it.
func TestGetClauseRectsClipsToPreedit(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("He", 2)
	cs.HandleClause(0, 5, 0)
	cs.HandleClause(4, 1, 0) // fully outside the preedit

	l := testLayout()
	rects := cs.GetClauseRects(&l)
	if len(rects) != 1 {
		t.Fatalf("clause rects = %d, want 1", len(rects))
	}
	if len(rects[0].Rects) != 1 || rects[0].Rects[0].Width != 20 {
		t.Errorf("rects = %+v, want one rect of width 20",
			rects[0].Rects)
	}
}

func TestHandleClauseCap(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	for i := range maxCompositionClauses + 10 {
		cs.HandleClause(i, 1, 0)
	}
	if len(cs.Clauses) != maxCompositionClauses {
		t.Errorf("clauses = %d, want %d", len(cs.Clauses),
			maxCompositionClauses)
	}
}

func TestSetClausesCap(t *testing.T) {
	cs := NewCompositionState()
	cs.SetClauses(make([]Clause, maxCompositionClauses+1), -1)
	if len(cs.Clauses) != maxCompositionClauses {
		t.Errorf("clauses = %d, want %d", len(cs.Clauses),
			maxCompositionClauses)
	}
}

func TestHandleClauseStyles(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.HandleClause(0, 1, 0)
	cs.HandleClause(1, 1, 1)
	cs.HandleClause(2, 1, 2)
	cs.HandleClause(3, 1, 7) // unknown style → raw
	cs.HandleClause(4, 1, -1)
	want := []ClauseStyle{ClauseRaw, ClauseConverted, ClauseSelected,
		ClauseRaw, ClauseRaw}
	for i, w := range want {
		if cs.Clauses[i].Style != w {
			t.Errorf("clause %d style = %d, want %d", i,
				cs.Clauses[i].Style, w)
		}
	}
	if cs.SelectedClause != 2 {
		t.Errorf("SelectedClause = %d, want 2", cs.SelectedClause)
	}
}

// Dead key followed by space gives the accent alone.
func TestDeadKeySpace(t *testing.T) {
	for _, dead := range []rune{'`', '\'', '^', '~', '"', ':', ','} {
		dks := DeadKeyState{}
		dks.StartDeadKey(dead, 0)
		got, ok := dks.TryCombine(' ')
		if !ok || got != string(dead) {
			t.Errorf("TryCombine(%q + space) = %q, %v; want %q, true",
				dead, got, ok, string(dead))
		}
	}
}

func TestCombineDeadKeyY(t *testing.T) {
	tests := []struct {
		dead, base rune
		want       rune
	}{
		{'\'', 'y', 0x00FD}, // ý
		{'\'', 'Y', 0x00DD}, // Ý
		{'"', 'Y', 0x0178},  // Ÿ
		{':', 'Y', 0x0178},  // Ÿ
	}
	for _, tc := range tests {
		got, ok := combineDeadKey(tc.dead, tc.base)
		if !ok || got != tc.want {
			t.Errorf("combineDeadKey(%q, %q) = %U, %v; want %U",
				tc.dead, tc.base, got, ok, tc.want)
		}
	}
}

// Invalid input is reported, not dropped silently, and state is kept.
func TestHandleTextInvalidReturnsError(t *testing.T) {
	cs := NewCompositionState()
	if err := cs.HandleMarkedText("ab", 2, 0); err != nil {
		t.Fatal(err)
	}
	if err := cs.HandleMarkedText("a\x00b", 0, 0); err == nil {
		t.Error("NUL in marked text: want error")
	}
	if cs.PreeditText != "ab" {
		t.Errorf("PreeditText = %q, want state kept", cs.PreeditText)
	}
	got, err := cs.HandleInsertText("\xff")
	if err == nil || got != "" {
		t.Errorf("bad UTF-8 insert = %q, %v; want \"\", error", got, err)
	}
	if !cs.IsComposing() {
		t.Error("failed insert should not end composition")
	}
	if _, err := cs.HandleInsertText(""); err == nil {
		t.Error("empty insert: want error")
	}
}

func TestCompositionNilLayout(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("ab", 1)
	if _, ok := cs.CompositionBounds(nil); ok {
		t.Error("CompositionBounds(nil): want ok=false")
	}
	if r := cs.GetClauseRects(nil); r != nil {
		t.Errorf("GetClauseRects(nil) = %v, want nil", r)
	}
}

// A negative clause start shrinks the length: [-3, 2) on a 5-byte
// preedit clips to [0, 2), two chars of width 20 in testLayout.
func TestClauseNegativeStartClips(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("Hello", 5)
	cs.SetClauses([]Clause{{Start: -3, Length: 5}}, -1)

	l := testLayout()
	rects := cs.GetClauseRects(&l)
	if len(rects) != 1 {
		t.Fatalf("clause rects = %d, want 1", len(rects))
	}
	if len(rects[0].Rects) != 1 || rects[0].Rects[0].Width != 20 {
		t.Errorf("rects = %+v, want one rect of width 20",
			rects[0].Rects)
	}
}

// A clause fully before the preedit ([-5, -2)) covers nothing.
func TestClauseFullyNegativeSkipped(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.SetMarkedText("Hello", 5)
	cs.SetClauses([]Clause{{Start: -5, Length: 3}}, -1)

	l := testLayout()
	if rects := cs.GetClauseRects(&l); len(rects) != 0 {
		t.Errorf("clause rects = %v, want none", rects)
	}
}

func TestStartNegativeClamps(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(-5)
	if cs.PreeditStart != 0 {
		t.Errorf("PreeditStart = %d, want 0", cs.PreeditStart)
	}
}

func TestHandleClauseEmptyDropped(t *testing.T) {
	cs := NewCompositionState()
	cs.Start(0)
	cs.HandleClause(0, 0, 0)
	if len(cs.Clauses) != 0 {
		t.Errorf("clauses = %d, want 0 for empty range", len(cs.Clauses))
	}
}
