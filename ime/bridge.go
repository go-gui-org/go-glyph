// Package ime provides platform-specific IME (Input Method Editor)
// bridges. The Bridge interface abstracts IME interaction across
// macOS (NSTextInputClient), Linux (IBus), and stub platforms.
package ime

// Bridge is the platform interface for IME input.
type Bridge interface {
	// Enable activates IME handling for a text field at the
	// given screen rect.
	Enable(x, y, width, height float32)

	// Disable deactivates IME handling.
	Disable()

	// SetCursorRect updates the candidate window position.
	SetCursorRect(x, y, width, height float32)

	// IsActive returns true if IME is currently active.
	IsActive() bool
}

// Callbacks receives IME events from the platform bridge.
type Callbacks struct {
	// OnMarkedText is called when preedit text changes.
	// cursorInPreedit is a UTF-8 byte offset into text. A bridge
	// that gets UTF-16 offsets (macOS NSRange) must convert them.
	OnMarkedText func(text string, cursorInPreedit int)

	// OnInsertText is called when text is committed.
	OnInsertText func(text string)

	// OnUnmarkText is called when composition is cancelled.
	OnUnmarkText func()
}

// NewBridge creates a platform-specific IME bridge.
// The callbacks receive IME events from the platform.
func NewBridge(cb Callbacks) Bridge {
	return newPlatformBridge(cb)
}
