package glyph

import "testing"

func TestBuildCSSFont(t *testing.T) {
	cases := []struct {
		style TextStyle
		want  string
	}{
		{TextStyle{FontName: "Sans 12"}, "12px sans-serif"},
		{TextStyle{FontName: "Sans Bold 12"}, "bold 12px sans-serif"},
		{TextStyle{FontName: "Sans Bold Italic 12"}, "italic bold 12px sans-serif"},
		{TextStyle{FontName: "Sans", Typeface: TypefaceBoldItalic, Size: 10.5},
			"italic bold 10.5px sans-serif"},
		// Regression: Light/Medium/Semibold/Heavy were stripped from the
		// family and then dropped, so the canvas measured Regular.
		{TextStyle{FontName: "Fira Code Light 11"}, "300 11px 'Fira Code', sans-serif"},
		{TextStyle{FontName: "Noto Sans Semibold 9"}, "600 9px 'Noto Sans', sans-serif"},
		{TextStyle{FontName: "Inter Heavy Oblique 14"}, "italic 900 14px 'Inter', sans-serif"},
		// A lone keyword is a family name, not a style.
		{TextStyle{FontName: "Bold 12"}, "12px 'Bold', sans-serif"},
		// Typeface wins over name keywords.
		{TextStyle{FontName: "Sans Light 12", Typeface: TypefaceBold},
			"bold 12px sans-serif"},
	}
	for _, c := range cases {
		if got := buildCSSFont(c.style); got != c.want {
			t.Errorf("buildCSSFont(%+v) = %q, want %q", c.style, got, c.want)
		}
	}
}

// TestCSSQuote is the regression test for family names that broke the
// CSS string. Only the quote was removed, so a trailing backslash escaped
// the closing quote and the canvas silently kept the previous font.
func TestCSSQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Fira Code", `'Fira Code'`},
		{`Evil\`, `'Evil\\'`},
		{"O'Neil", `'O\'Neil'`},
		{"a\nb\x7f", `'ab'`},
	}
	for _, c := range cases {
		if got := cssQuote(c.in); got != c.want {
			t.Errorf("cssQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
