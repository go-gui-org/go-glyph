package accessibility

import (
	"testing"
	"time"
)

func TestEmojiNames(t *testing.T) {
	tests := []struct {
		ch   rune
		want string
	}{
		{0x1F600, "grinning face"},
		{0x1F44D, "thumbs up"},
		{0x2764, "red heart"},
		{0x1F525, "fire"},
		{0x1F680, "rocket"},
		{'a', ""},    // Not emoji.
		{0x0041, ""}, // 'A'.
	}
	for _, tc := range tests {
		got := GetEmojiName(tc.ch)
		if got != tc.want {
			t.Errorf("GetEmojiName(%U) = %q, want %q", tc.ch, got, tc.want)
		}
	}
}

func TestAnnouncerCharacter(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceCharacter(' '); got != "space" {
		t.Errorf("space = %q", got)
	}
	// Reset debounce for next test.
	a.lastAnnouncementTime = 0
	if got := a.AnnounceCharacter('\n'); got != "newline" {
		t.Errorf("newline = %q", got)
	}
	a.lastAnnouncementTime = 0
	if got := a.AnnounceCharacter('\r'); got != "newline" {
		t.Errorf("carriage return = %q", got)
	}
	a.lastAnnouncementTime = 0
	if got := a.AnnounceCharacter('a'); got != "a" {
		t.Errorf("'a' = %q", got)
	}
	a.lastAnnouncementTime = 0
	if got := a.AnnounceCharacter(0x1F525); got != "fire" {
		t.Errorf("fire emoji = %q", got)
	}
}

func TestAnnouncerPunctuation(t *testing.T) {
	a := NewAnnouncer()
	cases := map[rune]string{
		'.': "period", ',': "comma", '!': "exclamation",
		'?': "question", '(': "open paren", ')': "close paren",
	}
	for ch, want := range cases {
		a.lastAnnouncementTime = 0
		if got := a.AnnounceCharacter(ch); got != want {
			t.Errorf("char %q = %q, want %q", ch, got, want)
		}
	}
}

func TestAnnouncerWordJump(t *testing.T) {
	a := NewAnnouncer()
	got := a.AnnounceWordJump("hello")
	if got != "moved to: hello" {
		t.Errorf("word jump = %q", got)
	}
}

func TestAnnouncerLineBoundary(t *testing.T) {
	a := NewAnnouncer()
	got := a.AnnounceLineBoundary(LineBoundaryBeginning)
	if got != "beginning of line" {
		t.Errorf("line beginning = %q", got)
	}
	a.lastAnnouncementTime = 0
	got = a.AnnounceLineBoundary(LineBoundaryEnd)
	if got != "end of line" {
		t.Errorf("line end = %q", got)
	}
}

func TestAnnouncerLineNumber(t *testing.T) {
	a := NewAnnouncer()
	got := a.AnnounceLineNumber(5)
	if got != "line 5" {
		t.Errorf("line number = %q", got)
	}
	// Same line should not re-announce.
	a.lastAnnouncementTime = 0
	got = a.AnnounceLineNumber(5)
	if got != "" {
		t.Errorf("same line = %q, want empty", got)
	}
	// Different line should announce.
	a.lastAnnouncementTime = 0
	got = a.AnnounceLineNumber(6)
	if got != "line 6" {
		t.Errorf("new line = %q", got)
	}
}

// A line change that arrives during the debounce is dropped, but it must not be
// marked as announced. The next call for the same line, after the debounce,
// announces it.
func TestAnnouncerLineNumberAfterDebounce(t *testing.T) {
	a := NewAnnouncer()
	a.lastAnnouncementTime = time.Now().UnixMilli() // inside the debounce
	if got := a.AnnounceLineNumber(3); got != "" {
		t.Fatalf("debounced call = %q, want empty", got)
	}
	a.lastAnnouncementTime = 0 // debounce over
	if got := a.AnnounceLineNumber(3); got != "line 3" {
		t.Errorf("after debounce = %q, want %q", got, "line 3")
	}
	a.lastAnnouncementTime = 0
	if got := a.AnnounceLineNumber(3); got != "" {
		t.Errorf("same line again = %q, want empty", got)
	}
}

func TestAnnouncerDocBoundary(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceDocumentBoundary(DocBoundaryBeginning); got != "beginning of document" {
		t.Errorf("doc begin = %q", got)
	}
	a.lastAnnouncementTime = 0
	if got := a.AnnounceDocumentBoundary(DocBoundaryEnd); got != "end of document" {
		t.Errorf("doc end = %q", got)
	}
}

func TestAnnouncerSelection(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceSelection("hello"); got != "hello" {
		t.Errorf("short selection = %q", got)
	}
	a.lastAnnouncementTime = 0
	long := "This is a much longer selection text that exceeds twenty characters"
	got := a.AnnounceSelection(long)
	if got == long {
		t.Error("long selection should be counted, not read")
	}
}

func TestAnnouncerSelectionExtended(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceSelectionExtended("world"); got != "added: world" {
		t.Errorf("extended = %q", got)
	}
}

func TestAnnouncerSelectionCleared(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceSelectionCleared(); got != "deselected" {
		t.Errorf("cleared = %q", got)
	}
}

func TestAnnouncerDeadKey(t *testing.T) {
	a := NewAnnouncer()
	cases := map[rune]string{
		'`':  "grave accent",
		'\'': "acute accent",
		'^':  "circumflex",
		'~':  "tilde",
		'"':  "diaeresis",
		',':  "cedilla",
	}
	for ch, want := range cases {
		a.lastAnnouncementTime = 0
		if got := a.AnnounceDeadKey(ch); got != want {
			t.Errorf("dead key %q = %q, want %q", ch, got, want)
		}
	}
}

func TestAnnouncerDeadKeyResult(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceDeadKeyResult(0x00E8); got != "\u00e8" {
		t.Errorf("dead key result = %q", got)
	}
}

func TestAnnouncerCompositionCancelled(t *testing.T) {
	a := NewAnnouncer()
	if got := a.AnnounceCompositionCancelled(); got != "composition cancelled" {
		t.Errorf("cancelled = %q", got)
	}
}

func TestAnnouncerDebounce(t *testing.T) {
	a := NewAnnouncer()
	a.AnnounceCharacter('a')
	// Immediate second call should be debounced.
	if got := a.AnnounceCharacter('b'); got != "" {
		t.Errorf("debounced call = %q, want empty", got)
	}
}

func TestManagerLifecycle(t *testing.T) {
	m := NewManager()
	m.AddTextNode("Hello", Rect{X: 0, Y: 0, Width: 100, Height: 20})
	m.Commit()
	// Should not panic; stub backend does nothing.
}

func TestManagerTextFieldNode(t *testing.T) {
	m := NewManager()
	id := m.CreateTextFieldNode(Rect{X: 10, Y: 10, Width: 200, Height: 30})
	if id <= 0 {
		t.Errorf("invalid node ID: %d", id)
	}
	m.UpdateTextField(id, "test", Range{Location: 4, Length: 0}, 1)
	m.SetFocus(id)
	m.PostNotification(id, NotifyValueChanged)
	m.Flush()
	m.Commit()
}

func TestManagerMultipleNodes(t *testing.T) {
	m := NewManager()
	m.AddTextNode("First", Rect{})
	m.AddTextNode("Second", Rect{})
	id := m.CreateTextFieldNode(Rect{})
	if id <= 0 {
		t.Error("invalid ID")
	}
	m.Commit()
}
