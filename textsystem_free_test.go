package glyph

import (
	"errors"
	"testing"
)

// TestTextSystemAfterFree checks that no TextSystem method panics after Free.
// Methods that return an error must return ErrTextSystemFreed. The rest do
// nothing.
func TestTextSystemAfterFree(t *testing.T) {
	ts, err := NewTextSystem(newRecordingBackend())
	if err != nil {
		t.Fatal(err)
	}
	ts.Free()

	cfg := TextConfig{}
	rt := RichText{Runs: []StyleRun{{Text: "a"}}}

	errCalls := []struct {
		name string
		fn   func() error
	}{
		{"DrawText", func() error { return ts.DrawText(0, 0, "a", cfg) }},
		{"TextWidth", func() error { _, e := ts.TextWidth("a", cfg); return e }},
		{"TextHeight", func() error { _, e := ts.TextHeight("a", cfg); return e }},
		{"LayoutText", func() error { _, e := ts.LayoutText("a", cfg); return e }},
		{"LayoutTextCached", func() error { _, e := ts.LayoutTextCached("a", cfg); return e }},
		{"LayoutRichText", func() error { _, e := ts.LayoutRichText(rt, cfg); return e }},
		{"FontHeight", func() error { _, e := ts.FontHeight(cfg); return e }},
		{"FontMetrics", func() error { _, e := ts.FontMetrics(cfg); return e }},
		{"AddFontFile", func() error { return ts.AddFontFile("/nonexistent.ttf") }},
		{"AddFontBytes", func() error { return ts.AddFontBytes([]byte{1}) }},
		{"ResolveFontName", func() error { _, e := ts.ResolveFontName("Sans"); return e }},
	}
	for _, c := range errCalls {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked after Free: %v", c.name, r)
				}
			}()
			if err := c.fn(); !errors.Is(err, ErrTextSystemFreed) {
				t.Errorf("%s: err = %v, want ErrTextSystemFreed", c.name, err)
			}
		})
	}

	// No temp file may be written for a freed system.
	if len(ts.tempFontFiles) != 0 {
		t.Errorf("AddFontBytes recorded temp files after Free: %v", ts.tempFontFiles)
	}

	quietCalls := []struct {
		name string
		fn   func()
	}{
		{"ListFontFamilies", func() {
			if got := ts.ListFontFamilies(); got != nil {
				t.Errorf("ListFontFamilies = %v, want nil", got)
			}
		}},
		{"Commit", func() { ts.Commit() }},
		{"Purge", func() { ts.Purge() }},
		{"DrawLayout", func() { ts.DrawLayout(Layout{}, 0, 0) }},
		{"Free", func() { ts.Free() }},
	}
	for _, c := range quietCalls {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked after Free: %v", c.name, r)
				}
			}()
			c.fn()
		})
	}
}
