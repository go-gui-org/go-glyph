package accessibility

import (
	"fmt"
	"time"
	"unicode/utf8"
)

// Announcer provides screen reader announcements with debounce.
type Announcer struct {
	backend              AnnouncerBackend
	lastAnnouncementTime int64 // Unix millis.
	debounceMs           int64
	lastLine             int
}

// AnnouncerBackend is the platform interface for posting
// announcements. Implementations live in build-tagged files.
type AnnouncerBackend interface {
	Announce(message string)
}

// NewAnnouncer creates an announcer with default 150ms debounce.
func NewAnnouncer() *Announcer {
	return &Announcer{
		debounceMs: 150,
		lastLine:   -1,
		backend:    newAnnouncerBackend(),
	}
}

// AnnounceCharacter returns announcement for a character.
// Punctuation/whitespace get symbolic names; emoji get short names.
func (a *Announcer) AnnounceCharacter(ch rune) string {
	if !a.shouldAnnounce() {
		return ""
	}
	var msg string
	switch ch {
	case ' ':
		msg = "space"
	case '\t':
		msg = "tab"
	case '\n':
		msg = "newline"
	case '.':
		msg = "period"
	case ',':
		msg = "comma"
	case ';':
		msg = "semicolon"
	case ':':
		msg = "colon"
	case '!':
		msg = "exclamation"
	case '?':
		msg = "question"
	case '\'':
		msg = "apostrophe"
	case '"':
		msg = "quote"
	case '(':
		msg = "open paren"
	case ')':
		msg = "close paren"
	case '[':
		msg = "open bracket"
	case ']':
		msg = "close bracket"
	case '{':
		msg = "open brace"
	case '}':
		msg = "close brace"
	default:
		if name := GetEmojiName(ch); name != "" {
			msg = name
		} else {
			msg = string(ch)
		}
	}
	a.post(msg)
	return msg
}

// AnnounceWordJump announces "moved to: <word>".
func (a *Announcer) AnnounceWordJump(word string) string {
	if !a.shouldAnnounce() {
		return ""
	}
	msg := "moved to: " + word
	a.post(msg)
	return msg
}

// AnnounceLineBoundary announces "beginning/end of line".
func (a *Announcer) AnnounceLineBoundary(b LineBoundary) string {
	if !a.shouldAnnounce() {
		return ""
	}
	msg := "beginning of line"
	if b == LineBoundaryEnd {
		msg = "end of line"
	}
	a.post(msg)
	return msg
}

// AnnounceLineNumber announces "line N" on line change.
func (a *Announcer) AnnounceLineNumber(line int) string {
	if line == a.lastLine {
		return ""
	}
	a.lastLine = line
	if !a.shouldAnnounce() {
		return ""
	}
	msg := fmt.Sprintf("line %d", line)
	a.post(msg)
	return msg
}

// AnnounceDocumentBoundary announces beginning/end of document.
func (a *Announcer) AnnounceDocumentBoundary(b DocBoundary) string {
	if !a.shouldAnnounce() {
		return ""
	}
	msg := "beginning of document"
	if b == DocBoundaryEnd {
		msg = "end of document"
	}
	a.post(msg)
	return msg
}

// AnnounceSelection reads short text or counts long text.
func (a *Announcer) AnnounceSelection(selectedText string) string {
	if !a.shouldAnnounce() {
		return ""
	}
	runeCount := utf8.RuneCountInString(selectedText)
	var msg string
	if runeCount <= 20 {
		msg = selectedText
	} else {
		msg = fmt.Sprintf("%d characters selected", runeCount)
	}
	a.post(msg)
	return msg
}

// AnnounceSelectionExtended announces "added: <text>".
func (a *Announcer) AnnounceSelectionExtended(addedText string) string {
	if !a.shouldAnnounce() {
		return ""
	}
	msg := "added: " + addedText
	a.post(msg)
	return msg
}

// AnnounceSelectionCleared announces "deselected".
func (a *Announcer) AnnounceSelectionCleared() string {
	if !a.shouldAnnounce() {
		return ""
	}
	msg := "deselected"
	a.post(msg)
	return msg
}

// AnnounceDeadKey announces the dead key name.
func (a *Announcer) AnnounceDeadKey(deadKey rune) string {
	if !a.shouldAnnounce() {
		return ""
	}
	var msg string
	switch deadKey {
	case '`':
		msg = "grave accent"
	case '\'':
		msg = "acute accent"
	case '^':
		msg = "circumflex"
	case '~':
		msg = "tilde"
	case '"', ':':
		msg = "diaeresis"
	case ',':
		msg = "cedilla"
	default:
		msg = "dead key"
	}
	a.post(msg)
	return msg
}

// AnnounceDeadKeyResult announces the composed character.
func (a *Announcer) AnnounceDeadKeyResult(ch rune) string {
	msg := string(ch)
	a.post(msg)
	return msg
}

// AnnounceCompositionCancelled announces "composition cancelled".
func (a *Announcer) AnnounceCompositionCancelled() string {
	if !a.shouldAnnounce() {
		return ""
	}
	msg := "composition cancelled"
	a.post(msg)
	return msg
}

func (a *Announcer) shouldAnnounce() bool {
	now := time.Now().UnixMilli()
	if now-a.lastAnnouncementTime < a.debounceMs {
		return false
	}
	a.lastAnnouncementTime = now
	return true
}

func (a *Announcer) post(message string) {
	if a.backend != nil {
		a.backend.Announce(message)
	}
}
