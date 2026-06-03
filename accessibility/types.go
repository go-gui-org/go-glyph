// Package accessibility provides screen reader support for the
// glyph text rendering library. Platform backends announce text
// changes via VoiceOver (macOS) or AT-SPI (Linux).
package accessibility

// Role defines the semantic role of an accessibility node.
type Role int

const (
	RoleText       Role = iota // Leaf text node.
	RoleStaticText             // Label or static text.
	RoleContainer              // Generic container.
	RoleGroup                  // Logical grouping.
	RoleWindow                 // Root window.
	RoleProse                  // Large text block.
	RoleList                   // List container.
	RoleListItem               // Item in a list.
	RoleTextField              // Editable text field.
)

// Notification identifies accessibility state changes.
type Notification int

const (
	NotifyValueChanged Notification = iota
	NotifySelectedTextChanged
)

// LineBoundary indicates cursor at line start or end.
type LineBoundary int

const (
	LineBoundaryBeginning LineBoundary = iota
	LineBoundaryEnd
)

// DocBoundary indicates cursor at document start or end.
type DocBoundary int

const (
	DocBoundaryBeginning DocBoundary = iota
	DocBoundaryEnd
)

// Rect is a bounding rectangle in window coordinates.
type Rect struct {
	X, Y, Width, Height float32
}

// Range represents a text range (location + length).
type Range struct {
	Location int
	Length   int
}

// Node represents a single node in the accessibility tree.
type Node struct {
	Text     string
	Children []int
	ID       int
	Role     Role
	Parent   int // -1 if root.

	Rect Rect

	IsFocused  bool
	IsSelected bool
}

// TextFieldNode extends Node for editable text fields.
type TextFieldNode struct {
	Value         string
	Node          Node
	SelectedRange Range
	CursorLine    int
	NumCharacters int
}
