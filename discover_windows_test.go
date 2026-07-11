//go:build windows

package glyph

import "testing"

func TestWindowsGenericAlias(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"segoe ui sans-serif", "segoe ui", "sans-serif"},
		{"arial sans-serif", "arial", "sans-serif"},
		{"consolas monospace", "consolas", "monospace"},
		{"courier new monospace", "courier new", "monospace"},
		{"lucida console monospace", "lucida console", "monospace"},
		{"times new roman serif", "times new roman", "serif"},
		{"georgia serif", "georgia", "serif"},
		{"unknown", "verdana", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := windowsGenericAlias(tt.input)
			if got != tt.expected {
				t.Errorf("windowsGenericAlias(%q) = %q, want %q",
					tt.input, got, tt.expected)
			}
		})
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
