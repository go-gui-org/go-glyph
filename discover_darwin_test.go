//go:build darwin

package glyph

import "testing"

func TestIsScriptFamily(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"arabic font", "sf arabic", true},
		{"hebrew font", "arial hebrew", true},
		{"thai font", "thonburi", true},
		{"devanagari font", "kohinoor devanagari", true},
		{"georgian font", "sf georgian", true},
		{"armenian font", "noto sans armenian", true},
		{"khmer font", "khmer sangam mn", true},
		{"lao font", "lao mn", true},
		{"myanmar font", "noto sans myanmar", true},
		{"tamil font", "noto sans tamil", true},
		{"bengali font", "bangla sangam mn", true},
		{"latin font", "helvetica", false},
		{"cjk font", "pingfang sc", false},
		{"emoji font", "apple color emoji", false},
		{"empty", "", false},
		{"naskh substring", "deconaskh", true},
		{"geeza substring", "geezapro", true},
		{"peninim substring", "newpeninimmt", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScriptFamily(tt.input)
			if got != tt.expected {
				t.Errorf("isScriptFamily(%q) = %v, want %v",
					tt.input, got, tt.expected)
			}
		})
	}
}

func TestDarwinGenericAlias(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"menlo monospace", "menlo", "monospace"},
		{"monaco monospace", "monaco", "monospace"},
		{"sf mono", "sf mono", "monospace"},
		{"helvetica sans-serif", "helvetica", "sans-serif"},
		{"helvetica neue sans-serif", "helvetica neue", "sans-serif"},
		{"times serif", "times", "serif"},
		{"times new roman serif", "times new roman", "serif"},
		{"generic unknown", "arial", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := darwinGenericAlias(tt.input)
			if got != tt.expected {
				t.Errorf("darwinGenericAlias(%q) = %q, want %q",
					tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveFontFamilyDarwin(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"sans shorthand", "Sans 12", "Helvetica"},
		{"sans-serif full", "sans-serif", "Helvetica"},
		{"system alias", "system", "Helvetica"},
		{"serif", "serif", "Times New Roman"},
		{"monospace", "monospace", "Menlo"},
		{"mono shorthand", "mono", "Menlo"},
		{"concrete family", "Helvetica Neue", "Helvetica Neue"},
		{"with size", "Helvetica 16", "Helvetica"},
		{"empty", "", "Helvetica"},
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
