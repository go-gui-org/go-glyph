package glyph

import "time"

// UndoOperation stores inverse operation data for undo/redo.
type UndoOperation struct {
	DeletedText  string
	InsertedText string
	OpType       OperationType
	CursorBefore int
	AnchorBefore int
	RangeStart   int
	RangeEnd     int
	CursorAfter  int
	AnchorAfter  int
}

// UndoManager tracks undo/redo stacks with coalescing.
type UndoManager struct {
	coalescableOp     *UndoOperation
	undoStack         []UndoOperation
	redoStack         []UndoOperation
	maxHistory        int
	lastMutationTime  int64 // Unix millis.
	coalesceTimeoutMs int64
}

// UndoResult holds the result of an undo/redo operation.
type UndoResult struct {
	Text   string
	Cursor int
	Anchor int
}

// NewUndoManager creates an UndoManager with specified history limit.
func NewUndoManager(maxHistory int) *UndoManager {
	if maxHistory <= 0 {
		maxHistory = 100
	}
	return &UndoManager{
		maxHistory:        maxHistory,
		coalesceTimeoutMs: 1000,
	}
}

// MutationToUndoOp converts a MutationResult to an UndoOperation.
func MutationToUndoOp(result MutationResult, inserted string,
	cursorBefore, anchorBefore int) UndoOperation {

	var opType OperationType
	switch {
	case len(result.DeletedText) > 0 && len(inserted) > 0:
		opType = OpReplace
	case len(inserted) > 0:
		opType = OpInsert
	default:
		opType = OpDelete
	}
	return UndoOperation{
		OpType:       opType,
		RangeStart:   result.RangeStart,
		RangeEnd:     result.RangeEnd,
		DeletedText:  result.DeletedText,
		InsertedText: inserted,
		CursorBefore: cursorBefore,
		CursorAfter:  result.CursorPos,
		AnchorBefore: anchorBefore,
		AnchorAfter:  result.CursorPos,
	}
}

func (um *UndoManager) shouldCoalesce(op UndoOperation, now int64) bool {
	if um.coalescableOp == nil {
		return false
	}
	if now-um.lastMutationTime > um.coalesceTimeoutMs {
		return false
	}
	if um.coalescableOp.OpType != op.OpType {
		return false
	}
	if op.OpType == OpReplace {
		return false
	}
	if op.OpType == OpInsert {
		if op.RangeStart != um.coalescableOp.RangeEnd {
			return false
		}
	}
	if op.OpType == OpDelete {
		if op.RangeEnd != um.coalescableOp.RangeStart {
			return false
		}
	}
	return true
}

func (um *UndoManager) coalesceOperation(op UndoOperation) {
	if um.coalescableOp == nil {
		return
	}
	switch op.OpType {
	case OpInsert:
		um.coalescableOp.InsertedText += op.InsertedText
		um.coalescableOp.RangeEnd = op.RangeEnd
		um.coalescableOp.CursorAfter = op.CursorAfter
		um.coalescableOp.AnchorAfter = op.AnchorAfter
	case OpDelete:
		um.coalescableOp.DeletedText = op.DeletedText + um.coalescableOp.DeletedText
		um.coalescableOp.RangeStart = op.RangeStart
		um.coalescableOp.CursorAfter = op.CursorAfter
		um.coalescableOp.AnchorAfter = op.AnchorAfter
	}
}

// RecordMutation tracks a mutation for undo support.
func (um *UndoManager) RecordMutation(result MutationResult,
	inserted string, cursorBefore, anchorBefore int) {

	now := time.Now().UnixMilli()
	op := MutationToUndoOp(result, inserted, cursorBefore, anchorBefore)

	if um.shouldCoalesce(op, now) {
		um.coalesceOperation(op)
		um.lastMutationTime = now
	} else {
		if um.coalescableOp != nil {
			um.undoStack = append(um.undoStack, *um.coalescableOp)
			um.coalescableOp = nil
		}
		cp := op
		um.coalescableOp = &cp
		um.lastMutationTime = now
		um.redoStack = nil
	}
}

// FlushPending pushes pending coalescable op to undo stack.
func (um *UndoManager) FlushPending() {
	if um.coalescableOp != nil {
		if len(um.undoStack) >= um.maxHistory {
			um.undoStack = um.undoStack[1:]
		}
		um.undoStack = append(um.undoStack, *um.coalescableOp)
		um.coalescableOp = nil
	}
}

// Undo reverses the last operation. Returns nil if nothing to undo.
func (um *UndoManager) Undo(text string) *UndoResult {
	um.FlushPending()
	if len(um.undoStack) == 0 {
		return nil
	}

	op := um.undoStack[len(um.undoStack)-1]
	um.undoStack = um.undoStack[:len(um.undoStack)-1]

	// Bounds guard.
	if op.RangeStart > len(text) || op.RangeEnd > len(text) ||
		op.RangeStart > op.RangeEnd {
		return nil
	}

	var newText string
	switch op.OpType {
	case OpInsert:
		newText = text[:op.RangeStart] + text[op.RangeEnd:]
	case OpDelete:
		newText = text[:op.RangeStart] + op.DeletedText + text[op.RangeStart:]
	case OpReplace:
		newText = text[:op.RangeStart] + op.DeletedText + text[op.RangeEnd:]
	}

	um.redoStack = append(um.redoStack, op)
	return &UndoResult{
		Text:   newText,
		Cursor: op.CursorBefore,
		Anchor: op.AnchorBefore,
	}
}

// Redo reapplies an undone operation. Returns nil if nothing to redo.
func (um *UndoManager) Redo(text string) *UndoResult {
	if len(um.redoStack) == 0 {
		return nil
	}

	op := um.redoStack[len(um.redoStack)-1]
	um.redoStack = um.redoStack[:len(um.redoStack)-1]

	if op.RangeStart > len(text) {
		return nil
	}

	var newText string
	switch op.OpType {
	case OpInsert:
		newText = text[:op.RangeStart] + op.InsertedText + text[op.RangeStart:]
	case OpDelete:
		if op.RangeEnd > len(text) || op.RangeStart > op.RangeEnd {
			return nil
		}
		newText = text[:op.RangeStart] + text[op.RangeEnd:]
	case OpReplace:
		oldEnd := op.RangeStart + len(op.DeletedText)
		if oldEnd > len(text) {
			return nil
		}
		newText = text[:op.RangeStart] + op.InsertedText + text[oldEnd:]
	}

	if len(um.undoStack) >= um.maxHistory {
		um.undoStack = um.undoStack[1:]
	}
	um.undoStack = append(um.undoStack, op)
	return &UndoResult{
		Text:   newText,
		Cursor: op.CursorAfter,
		Anchor: op.AnchorAfter,
	}
}

// BreakCoalescing flushes pending operation on cursor navigation.
func (um *UndoManager) BreakCoalescing() {
	um.FlushPending()
}

// CanUndo returns true if undo is possible.
func (um *UndoManager) CanUndo() bool {
	return um.coalescableOp != nil || len(um.undoStack) > 0
}

// CanRedo returns true if redo is possible.
func (um *UndoManager) CanRedo() bool {
	return len(um.redoStack) > 0
}

// Clear resets all undo/redo state.
func (um *UndoManager) Clear() {
	um.undoStack = nil
	um.redoStack = nil
	um.coalescableOp = nil
}

// UndoDepth returns count of operations available for undo.
func (um *UndoManager) UndoDepth() int {
	pending := 0
	if um.coalescableOp != nil {
		pending = 1
	}
	return len(um.undoStack) + pending
}
