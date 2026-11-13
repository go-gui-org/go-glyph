package glyph

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTextValid(t *testing.T) {
	err := ValidateTextInput("Hello, World!", 1024, "test")
	if err != nil {
		t.Fatalf("valid text: %v", err)
	}
}

func TestValidateTextValidUnicode(t *testing.T) {
	err := ValidateTextInput("Hello", 1024, "test")
	if err != nil {
		t.Fatalf("valid unicode: %v", err)
	}
}

func TestValidateTextInvalidUTF8(t *testing.T) {
	invalid := string([]byte{0xff, 0xfe})
	err := ValidateTextInput(invalid, 1024, "test")
	if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("expected UTF-8 error, got: %v", err)
	}
}

func TestValidateTextEmpty(t *testing.T) {
	err := ValidateTextInput("", 1024, "test")
	if err == nil || !strings.Contains(err.Error(), "empty string") {
		t.Fatalf("expected empty error, got: %v", err)
	}
}

func TestValidateTextTooLong(t *testing.T) {
	long := strings.Repeat("x", 2000)
	err := ValidateTextInput(long, 1000, "test")
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected length error, got: %v", err)
	}
}

func TestValidatePathValid(t *testing.T) {
	tmp := filepath.Join(os.TempDir(), "glyph_test_font.ttf")
	if err := os.WriteFile(tmp, []byte("dummy"), 0644); err != nil {
		t.Skip("cannot create temp file")
	}
	defer func() { _ = os.Remove(tmp) }()

	err := ValidateFontPath(tmp, "test")
	if err != nil {
		t.Fatalf("valid path: %v", err)
	}
}

func TestValidatePathTraversal(t *testing.T) {
	err := ValidateFontPath("/fonts/../etc/passwd", "test")
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("expected traversal error, got: %v", err)
	}
}

func TestValidatePathNonexistent(t *testing.T) {
	err := ValidateFontPath("/nonexistent/path/to/font.ttf", "test")
	if err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected not-accessible error, got: %v", err)
	}
}

func TestValidatePathEmpty(t *testing.T) {
	err := ValidateFontPath("", "test")
	if err == nil || !strings.Contains(err.Error(), "empty font path") {
		t.Fatalf("expected empty error, got: %v", err)
	}
}

func TestValidateSizeValid(t *testing.T) {
	err := ValidateSize(12.0, 0.1, 500.0, "font size", "test")
	if err != nil {
		t.Fatalf("valid size: %v", err)
	}
}

func TestValidateSizeBelowMin(t *testing.T) {
	err := ValidateSize(0.05, 0.1, 500.0, "font size", "test")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected range error, got: %v", err)
	}
}

func TestValidateSizeAboveMax(t *testing.T) {
	err := ValidateSize(600.0, 0.1, 500.0, "font size", "test")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected range error, got: %v", err)
	}
}

func TestValidateSizeRejectsNaN(t *testing.T) {
	err := ValidateSize(float32(math.NaN()), MinFontSize, MaxFontSize,
		"font size", "test")
	if err == nil {
		t.Fatal("expected error for NaN size")
	}
}

func TestValidateDimensionValid(t *testing.T) {
	err := ValidateDimension(1024, "width", "test")
	if err != nil {
		t.Fatalf("valid dim: %v", err)
	}
}

func TestValidateDimensionZero(t *testing.T) {
	err := ValidateDimension(0, "width", "test")
	if err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("expected positive error, got: %v", err)
	}
}

func TestValidateDimensionNegative(t *testing.T) {
	err := ValidateDimension(-100, "height", "test")
	if err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("expected positive error, got: %v", err)
	}
}

func TestValidateDimensionExceedsMax(t *testing.T) {
	err := ValidateDimension(20000, "atlas size", "test")
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected max error, got: %v", err)
	}
}
