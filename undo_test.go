package glyph

import (
	"testing"
	"time"
)

func TestUndoManagerInsertUndo(t *testing.T) {
	um := NewUndoManager(100)
	r := InsertText("Hello", 5, " World")
	um.RecordMutation(r, " World", 5, 5)
	um.FlushPending()

	result := um.Undo("Hello World")
	if result == nil {
		t.Fatal("Undo returned nil")
	}
	if result.Text != "Hello" {
		t.Errorf("Undo text = %q, want 'Hello'", result.Text)
	}
	if result.Cursor != 5 {
		t.Errorf("Undo cursor = %d, want 5", result.Cursor)
	}
}

func TestUndoManagerRedo(t *testing.T) {
	um := NewUndoManager(100)
	r := InsertText("Hello", 5, " World")
	um.RecordMutation(r, " World", 5, 5)
	um.FlushPending()

	undone := um.Undo("Hello World")
	if undone == nil {
		t.Fatal("Undo nil")
	}

	redone := um.Redo(undone.Text)
	if redone == nil {
		t.Fatal("Redo nil")
	}
	if redone.Text != "Hello World" {
		t.Errorf("Redo text = %q", redone.Text)
	}
}

func TestUndoManagerDeleteUndo(t *testing.T) {
	um := NewUndoManager(100)
	r := DeleteSelection("Hello World", 5, 11)
	um.RecordMutation(r, "", 5, 11)
	um.FlushPending()

	result := um.Undo("Hello")
	if result == nil {
		t.Fatal("Undo nil")
	}
	if result.Text != "Hello World" {
		t.Errorf("Undo delete text = %q", result.Text)
	}
}

func TestUndoManagerCanUndo(t *testing.T) {
	um := NewUndoManager(100)
	if um.CanUndo() {
		t.Error("fresh manager should not CanUndo")
	}
	r := InsertText("", 0, "x")
	um.RecordMutation(r, "x", 0, 0)
	if !um.CanUndo() {
		t.Error("should CanUndo after mutation")
	}
}

func TestUndoManagerCanRedo(t *testing.T) {
	um := NewUndoManager(100)
	if um.CanRedo() {
		t.Error("fresh manager should not CanRedo")
	}
	r := InsertText("", 0, "x")
	um.RecordMutation(r, "x", 0, 0)
	um.FlushPending()
	um.Undo("x")
	if !um.CanRedo() {
		t.Error("should CanRedo after undo")
	}
}

func TestUndoManagerClear(t *testing.T) {
	um := NewUndoManager(100)
	r := InsertText("", 0, "x")
	um.RecordMutation(r, "x", 0, 0)
	um.FlushPending()
	um.Clear()
	if um.CanUndo() || um.CanRedo() {
		t.Error("Clear should reset stacks")
	}
}

func TestUndoManagerUndoDepth(t *testing.T) {
	um := NewUndoManager(100)
	if um.UndoDepth() != 0 {
		t.Error("initial depth should be 0")
	}
	r := InsertText("", 0, "x")
	um.RecordMutation(r, "x", 0, 0)
	if um.UndoDepth() != 1 { // pending
		t.Errorf("depth = %d, want 1", um.UndoDepth())
	}
	um.FlushPending()
	if um.UndoDepth() != 1 {
		t.Errorf("depth after flush = %d, want 1", um.UndoDepth())
	}
}

func TestUndoManagerHistoryLimit(t *testing.T) {
	um := NewUndoManager(3)
	for range 5 {
		r := InsertText("", 0, "x")
		um.RecordMutation(r, "x", 0, 0)
		um.BreakCoalescing()
		// Force timeout to prevent coalescing.
		um.lastMutationTime = 0
	}
	um.FlushPending()
	if len(um.undoStack) > 3 {
		t.Errorf("stack size = %d, want <= 3", len(um.undoStack))
	}
}

// Typing that never flushes must still respect maxHistory.
func TestUndoManagerHistoryLimitWithoutFlush(t *testing.T) {
	um := NewUndoManager(3)
	s := ""
	for range 10 {
		r := InsertText(s, len(s), "x")
		um.lastMutationTime = 0 // Force a non-coalescing push.
		um.RecordMutation(r, "x", len(s), len(s))
		s = r.NewText
	}
	if len(um.undoStack) > 3 {
		t.Errorf("stack size = %d, want <= 3", len(um.undoStack))
	}
}

// A mutation that changed nothing must not clear the redo stack.
func TestUndoManagerNoOpMutationKeepsRedo(t *testing.T) {
	um := NewUndoManager(10)
	r := InsertText("", 0, "x")
	um.RecordMutation(r, "x", 0, 0)
	um.FlushPending()
	if um.Undo("x") == nil {
		t.Fatal("Undo returned nil")
	}
	if !um.CanRedo() {
		t.Fatal("redo stack empty after Undo")
	}

	noop := DeleteBackward("", Layout{}, 0)
	um.RecordMutation(noop, "", 0, 0)

	if !um.CanRedo() {
		t.Error("no-op mutation cleared the redo stack")
	}
	if um.UndoDepth() != 0 {
		t.Errorf("UndoDepth = %d, want 0", um.UndoDepth())
	}
}

// Trimming must drop the oldest op and keep order.
func TestUndoManagerTrimReleasesOldest(t *testing.T) {
	um := NewUndoManager(2)
	s := ""
	for _, ins := range []string{"a", "b", "c"} {
		r := InsertText(s, len(s), ins)
		um.lastMutationTime = 0
		um.RecordMutation(r, ins, len(s), len(s))
		um.FlushPending()
		s = r.NewText
	}
	if len(um.undoStack) != 2 {
		t.Fatalf("stack size = %d, want 2", len(um.undoStack))
	}
	if um.undoStack[0].InsertedText != "b" || um.undoStack[1].InsertedText != "c" {
		t.Errorf("stack = %q, %q; want b, c",
			um.undoStack[0].InsertedText, um.undoStack[1].InsertedText)
	}
}

func TestUndoManagerCoalescing(t *testing.T) {
	um := NewUndoManager(100)

	// Type "abc" character by character within timeout.
	r1 := InsertText("", 0, "a")
	um.RecordMutation(r1, "a", 0, 0)

	r2 := InsertText("a", 1, "b")
	um.RecordMutation(r2, "b", 1, 1)

	r3 := InsertText("ab", 2, "c")
	um.RecordMutation(r3, "c", 2, 2)

	um.FlushPending()

	// Should be coalesced into single operation.
	if um.UndoDepth() != 1 {
		t.Errorf("coalesced depth = %d, want 1", um.UndoDepth())
	}

	result := um.Undo("abc")
	if result == nil {
		t.Fatal("Undo nil")
	}
	if result.Text != "" {
		t.Errorf("undo coalesced = %q, want empty", result.Text)
	}
}

func TestUndoManagerCoalesceTimeout(t *testing.T) {
	um := NewUndoManager(100)

	r1 := InsertText("", 0, "a")
	um.RecordMutation(r1, "a", 0, 0)

	// Simulate timeout.
	um.lastMutationTime = time.Now().UnixMilli() - 2000

	r2 := InsertText("a", 1, "b")
	um.RecordMutation(r2, "b", 1, 1)
	um.FlushPending()

	// Should NOT coalesce due to timeout.
	if um.UndoDepth() != 2 {
		t.Errorf("timeout depth = %d, want 2", um.UndoDepth())
	}
}

func TestUndoManagerBreakCoalescing(t *testing.T) {
	um := NewUndoManager(100)
	r1 := InsertText("", 0, "a")
	um.RecordMutation(r1, "a", 0, 0)
	um.BreakCoalescing()

	r2 := InsertText("a", 1, "b")
	um.RecordMutation(r2, "b", 1, 1)
	um.FlushPending()

	if um.UndoDepth() != 2 {
		t.Errorf("break coalescing depth = %d, want 2", um.UndoDepth())
	}
}

func TestUndoManagerRedoClearedOnNew(t *testing.T) {
	um := NewUndoManager(100)
	r := InsertText("", 0, "a")
	um.RecordMutation(r, "a", 0, 0)
	um.FlushPending()
	um.Undo("a")

	if !um.CanRedo() {
		t.Fatal("should CanRedo after undo")
	}

	// New mutation clears redo.
	r2 := InsertText("", 0, "b")
	um.RecordMutation(r2, "b", 0, 0)
	if um.CanRedo() {
		t.Error("redo should be cleared after new mutation")
	}
}

func TestUndoManagerReplaceUndo(t *testing.T) {
	um := NewUndoManager(100)
	r := InsertReplacingSelection("Hello World", 6, 11, "Go")
	um.RecordMutation(r, "Go", 6, 11)
	um.FlushPending()

	result := um.Undo("Hello Go")
	if result == nil {
		t.Fatal("Undo nil")
	}
	if result.Text != "Hello World" {
		t.Errorf("undo replace = %q", result.Text)
	}
}

func TestUndoManagerNilOnEmpty(t *testing.T) {
	um := NewUndoManager(100)
	if um.Undo("text") != nil {
		t.Error("Undo should return nil on empty stack")
	}
	if um.Redo("text") != nil {
		t.Error("Redo should return nil on empty stack")
	}
}

func TestMutationToUndoOp(t *testing.T) {
	r := MutationResult{
		NewText: "ab", CursorPos: 2,
		RangeStart: 0, RangeEnd: 2,
	}
	op := MutationToUndoOp(r, "ab", 0, 0)
	if op.OpType != OpInsert {
		t.Errorf("OpType = %d, want OpInsert", op.OpType)
	}

	r2 := MutationResult{
		NewText: "", CursorPos: 0,
		DeletedText: "ab",
		RangeStart:  0, RangeEnd: 0,
	}
	op2 := MutationToUndoOp(r2, "", 2, 2)
	if op2.OpType != OpDelete {
		t.Errorf("OpType = %d, want OpDelete", op2.OpType)
	}

	r3 := MutationResult{
		NewText: "xy", CursorPos: 2,
		DeletedText: "ab",
		RangeStart:  0, RangeEnd: 2,
	}
	op3 := MutationToUndoOp(r3, "xy", 0, 2)
	if op3.OpType != OpReplace {
		t.Errorf("OpType = %d, want OpReplace", op3.OpType)
	}
}
