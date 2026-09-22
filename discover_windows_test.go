//go:build windows

package glyph

import (
	"path/filepath"
	"testing"
)

func TestWindowsGenericAlias(t *testing.T) {
	tests := []struct {
		family string
		want   string
	}{
		{"segoe ui", "sans-serif"},
		{"arial", "sans-serif"},
		{"consolas", "monospace"},
		{"courier new", "monospace"},
		{"lucida console", "monospace"},
		{"times new roman", "serif"},
		{"georgia", "serif"},
		{"verdana", ""},
		{"", ""},
		{"segoe ui emoji", ""},
		{"segoe ui symbol", ""},
		{"arial black", ""},
	}
	for _, tt := range tests {
		got, _, _ := platformAliases.lookup(tt.family)
		if got != tt.want {
			t.Errorf("platformAliases.lookup(%q) = %q, want %q",
				tt.family, got, tt.want)
		}
	}
}

func TestResolveFontFamilyWindows(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"sans shorthand", "Sans 12", "Segoe UI"},
		{"sans-serif full", "sans-serif", "Segoe UI"},
		{"system alias", "system", "Segoe UI"},
		{"serif", "serif", "Times New Roman"},
		{"monospace", "monospace", "Consolas"},
		{"mono shorthand", "mono", "Consolas"},
		{"concrete family", "Arial", "Arial"},
		{"with size", "Segoe UI 16", "Segoe UI"},
		{"empty", "", "Segoe UI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFontFamily(tt.input)
			if got != tt.expected {
				t.Errorf("resolveFontFamily(%q) = %q, want %q",
					tt.input, got, tt.expected)
			}
		})
	}
}

// TestWindowsDirFallback is the regression test for an unset %WINDIR%
// turning the font dir into the relative path "Fonts".
func TestWindowsDirFallback(t *testing.T) {
	t.Setenv("WINDIR", "")
	t.Setenv("SystemRoot", "")
	if got := windowsDir(); got != `C:\Windows` {
		t.Errorf("windowsDir() = %q, want C:\\Windows", got)
	}
	t.Setenv("SystemRoot", `D:\Win`)
	if got := windowsDir(); got != `D:\Win` {
		t.Errorf("windowsDir() = %q, want D:\\Win", got)
	}
	m := map[string]string{}
	ensurePlatformDefaults(m)
	if p := m["sans-serif"]; !filepath.IsAbs(p) {
		t.Errorf("sans-serif default = %q, want an absolute path", p)
	}
}
