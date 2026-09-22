package glyph

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxTextLength is the maximum text input length (10KB) for
	// DoS prevention.
	MaxTextLength = 10240
	// MaxRichTextLength is the maximum total length (1 MiB) of all runs of
	// a RichText, for DoS prevention. Each run is also limited to
	// MaxTextLength.
	MaxRichTextLength = 1 << 20
	// MaxTextureDimension is the maximum texture size in pixels.
	MaxTextureDimension = 16384
	// MinFontSize is the minimum font size in points.
	MinFontSize = float32(0.1)
	// MaxFontSize is the maximum font size in points.
	MaxFontSize = float32(500.0)
)

// ValidateTextInput validates text for UTF-8, non-empty, and length.
func ValidateTextInput(text string, maxLen int, location string) error {
	if len(text) == 0 {
		return fmt.Errorf("empty string not allowed at %s", location)
	}
	if len(text) > maxLen {
		return fmt.Errorf("text exceeds max length %d bytes at %s",
			maxLen, location)
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("invalid UTF-8 encoding at %s", location)
	}
	if strings.ContainsRune(text, '\x00') {
		return fmt.Errorf("null byte in text at %s", location)
	}
	return nil
}

// ValidateSize validates a numeric size against min/max bounds.
func ValidateSize(size, minVal, maxVal float32,
	name, location string) error {
	if size < minVal || size > maxVal {
		return fmt.Errorf("%s %g out of range [%g, %g] at %s",
			name, size, minVal, maxVal, location)
	}
	return nil
}

// ValidateDimension validates an integer dimension (width/height).
func ValidateDimension(dim int, name, location string) error {
	if dim <= 0 {
		return fmt.Errorf("%s must be positive, got %d at %s",
			name, dim, location)
	}
	if dim > MaxTextureDimension {
		return fmt.Errorf("%s %d exceeds max %d at %s",
			name, dim, MaxTextureDimension, location)
	}
	return nil
}

// validateRichRuns checks every non-empty run of rt with ValidateTextInput
// and bounds the total length by MaxRichTextLength. It returns the total
// length. Empty runs are allowed: they contribute no text.
func validateRichRuns(rt RichText) (int, error) {
	total := 0
	for _, run := range rt.Runs {
		if run.Text == "" {
			continue
		}
		if err := ValidateTextInput(run.Text, MaxTextLength,
			"LayoutRichText"); err != nil {
			return 0, err
		}
		total += len(run.Text)
		if total > MaxRichTextLength {
			return 0, fmt.Errorf("rich text exceeds max length %d bytes at %s",
				MaxRichTextLength, "LayoutRichText")
		}
	}
	return total, nil
}
