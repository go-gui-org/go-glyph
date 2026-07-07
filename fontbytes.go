//go:build !js

package glyph

import "os"

// writeTempFontFile writes font bytes to a private temp file and
// returns its path. The caller owns the file and must remove it (the
// TextSystem does so in Free). On failure the partial file is removed.
func writeTempFontFile(data []byte) (string, error) {
	f, err := os.CreateTemp("", "goglyph-font-*.ttf")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
