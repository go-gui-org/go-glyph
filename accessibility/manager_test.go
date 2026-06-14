package accessibility

import (
	"testing"
)

// recordingAccBackend records all accessibility backend calls for
// verification in tests.
type recordingAccBackend struct {
	lastTree         map[int]Node
	lastRootID       int
	focusIDs         []int
	notifs           []notifRecord
	textFieldUpdates []textFieldRecord
	flushCount       int
}

type notifRecord struct {
	nodeID int
	n      Notification
}

type textFieldRecord struct {
	nodeID     int
	value      string
	sel        Range
	cursorLine int
}

func (b *recordingAccBackend) UpdateTree(nodes map[int]Node, rootID int) {
	b.lastTree = make(map[int]Node, len(nodes))
	for k, v := range nodes {
		b.lastTree[k] = v
	}
	b.lastRootID = rootID
}

func (b *recordingAccBackend) SetFocus(nodeID int) {
	b.focusIDs = append(b.focusIDs, nodeID)
}

func (b *recordingAccBackend) PostNotification(nodeID int, n Notification) {
	b.notifs = append(b.notifs, notifRecord{nodeID, n})
}

func (b *recordingAccBackend) UpdateTextField(nodeID int, value string, sel Range, cursorLine int) {
	b.textFieldUpdates = append(b.textFieldUpdates, textFieldRecord{nodeID, value, sel, cursorLine})
}

func (b *recordingAccBackend) Flush() { b.flushCount++ }

// Announce implements AnnouncerBackend so recordingAccBackend can be
// used as both a Manager Backend and an Announcer Backend.
func (b *recordingAccBackend) Announce(message string) {
	b.notifs = append(b.notifs, notifRecord{n: Notification(len(message))})
}

func newManagerWithBackend(b Backend) *Manager {
	return &Manager{
		backend: b,
		nodes:   make(map[int]Node),
		nextID:  1,
	}
}

func TestManagerRecording_CommitEmptyNoTree(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	// Commit with no nodes added — should not call UpdateTree.
	m.Commit()
	if rec.lastTree != nil {
		t.Error("UpdateTree should not be called when nodes is empty")
	}
}

func TestManagerRecording_CommitTreeStructure(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.AddTextNode("Hello", Rect{X: 10, Y: 20, Width: 100, Height: 30})
	m.Commit()

	if rec.lastTree == nil {
		t.Fatal("UpdateTree should have been called")
	}
	if rec.lastRootID == 0 {
		t.Error("lastRootID should not be 0")
	}
	root, ok := rec.lastTree[rec.lastRootID]
	if !ok {
		t.Fatal("root node not found in tree")
	}
	if root.Role != RoleContainer {
		t.Errorf("root role = %v, want %v", root.Role, RoleContainer)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(root.Children))
	}
	childID := root.Children[0]
	child, ok := rec.lastTree[childID]
	if !ok {
		t.Fatal("child node not found")
	}
	if child.Role != RoleText {
		t.Errorf("child role = %v, want %v", child.Role, RoleText)
	}
	if child.Text != "Hello" {
		t.Errorf("child text = %q, want %q", child.Text, "Hello")
	}
	if child.Parent != rec.lastRootID {
		t.Errorf("child parent = %d, want %d", child.Parent, rec.lastRootID)
	}
}

func TestManagerRecording_CommitMultipleNodes(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.AddTextNode("A", Rect{})
	m.AddTextNode("B", Rect{})
	m.AddTextNode("C", Rect{})
	m.Commit()

	if rec.lastTree == nil {
		t.Fatal("UpdateTree should have been called")
	}
	root, ok := rec.lastTree[rec.lastRootID]
	if !ok {
		t.Fatal("root node not found")
	}
	if len(root.Children) != 3 {
		t.Fatalf("root children = %d, want 3", len(root.Children))
	}
	for _, childID := range root.Children {
		child, ok := rec.lastTree[childID]
		if !ok {
			t.Errorf("child %d not found", childID)
			continue
		}
		if child.Parent != rec.lastRootID {
			t.Errorf("child %d parent = %d, want %d", childID, child.Parent, rec.lastRootID)
		}
	}
}

func TestManagerRecording_CommitResetsState(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.AddTextNode("First", Rect{})
	m.Commit()
	firstRootID := rec.lastRootID
	if firstRootID == 0 {
		t.Fatal("first root ID should not be 0")
	}

	// Second commit: state is fully reset, new tree with fresh root.
	m.AddTextNode("Second", Rect{})
	m.Commit()
	secondRootID := rec.lastRootID
	// Root ID reused (reset always produces the same ID sequence).
	// Second commit should only have one child.
	root, ok := rec.lastTree[secondRootID]
	if !ok {
		t.Fatal("second root not found")
	}
	if len(root.Children) != 1 {
		t.Errorf("second commit children = %d, want 1", len(root.Children))
	}
	// First commit's old key (e.g. child 2) should not be in second tree.
	if node, ok := rec.lastTree[2]; ok && node.Text == "First" {
		t.Error("first commit's nodes should not leak into second tree")
	}
}

func TestManagerRecording_TextFieldNode(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	id := m.CreateTextFieldNode(Rect{X: 5, Y: 5, Width: 200, Height: 30})
	m.Commit()

	child, ok := rec.lastTree[id]
	if !ok {
		t.Fatal("text field node not found")
	}
	if child.Role != RoleTextField {
		t.Errorf("role = %v, want %v", child.Role, RoleTextField)
	}
	if child.Rect.X != 5 || child.Rect.Width != 200 {
		t.Errorf("rect mismatch: %+v", child.Rect)
	}
}

func TestManagerRecording_SetFocus(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.SetFocus(42)
	if len(rec.focusIDs) != 1 || rec.focusIDs[0] != 42 {
		t.Errorf("focusIDs = %v, want [42]", rec.focusIDs)
	}
}

func TestManagerRecording_PostNotification(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.PostNotification(7, NotifyValueChanged)
	if len(rec.notifs) != 1 {
		t.Fatalf("notifs = %d, want 1", len(rec.notifs))
	}
	if rec.notifs[0].nodeID != 7 || rec.notifs[0].n != NotifyValueChanged {
		t.Errorf("notif = %+v, want {7, NotifyValueChanged}", rec.notifs[0])
	}
}

func TestManagerRecording_UpdateTextField(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.UpdateTextField(3, "value", Range{1, 5}, 2)
	if len(rec.textFieldUpdates) != 1 {
		t.Fatalf("textFieldUpdates = %d, want 1", len(rec.textFieldUpdates))
	}
	u := rec.textFieldUpdates[0]
	if u.nodeID != 3 || u.value != "value" ||
		u.sel != (Range{1, 5}) || u.cursorLine != 2 {
		t.Errorf("update = %+v, want {3, value, Range{1,5}, 2}", u)
	}
}

func TestManagerRecording_Flush(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.Flush()
	if rec.flushCount != 1 {
		t.Errorf("flushCount = %d, want 1", rec.flushCount)
	}
}

func TestManagerRecording_AddAfterCommit(t *testing.T) {
	rec := &recordingAccBackend{}
	m := newManagerWithBackend(rec)
	m.AddTextNode("A", Rect{})
	m.Commit()

	// Add another node and commit — fresh tree.
	m.AddTextNode("B", Rect{})
	m.Commit()

	root, ok := rec.lastTree[rec.lastRootID]
	if !ok {
		t.Fatal("root not found in second tree")
	}
	if len(root.Children) != 1 {
		t.Errorf("second tree should have 1 child, got %d", len(root.Children))
	}
}

func TestManagerAndAnnouncer_IndependentBackends(t *testing.T) {
	mgrRec := &recordingAccBackend{}
	annRec := &recordingAccBackend{}

	mgr := newManagerWithBackend(mgrRec)
	ann := newAnnouncerWithBackend(annRec)

	mgr.AddTextNode("Hello", Rect{})
	mgr.Commit()
	ann.AnnounceCharacter('A')

	if len(mgrRec.lastTree) == 0 {
		t.Error("manager backend should have recorded tree")
	}
	if len(annRec.notifs) == 0 {
		t.Error("announcer backend should have recorded notification")
	}
	// Verify backends don't interfere.
	if mgrRec.flushCount != 0 {
		t.Error("manager flush should not be affected by announcer")
	}
}

// newAnnouncerWithBackend creates an Announcer with a specific
// recording backend for testing.
func newAnnouncerWithBackend(b AnnouncerBackend) *Announcer {
	return &Announcer{
		backend:    b,
		debounceMs: 0, // No debounce for testing.
		lastLine:   -1,
	}
}
