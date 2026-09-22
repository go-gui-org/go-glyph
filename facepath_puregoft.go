//go:build linux || darwin || windows

package glyph

import (
	"os"
	"strconv"
	"strings"

	ot "github.com/go-text/typesetting/font/opentype"
)

// A font collection (.ttc) holds several faces in one file. For example,
// Helvetica.ttc holds Regular, Bold, and Oblique faces, and Noto Sans
// CJK holds the JP, KR, SC, and TC faces. A face is addressed by a "face
// path": the file path, optionally followed by faceIndexSep and the face
// index. Face 0 uses the plain file path, so single-face fonts and the
// first face of a collection keep the keys they always had in fontPaths
// and in the face and coverage caches.
//
// NUL is the separator because no file system allows it in a path, so a
// real file name cannot be misread as a face path.
const faceIndexSep = "\x00"

// facePath returns the face path for face index of file.
func facePath(file string, index int) string {
	if index <= 0 {
		return file
	}
	return file + faceIndexSep + strconv.Itoa(index)
}

// splitFacePath splits a face path into its file path and face index. A
// plain path, or a suffix that is not a valid index, gives index 0.
func splitFacePath(p string) (file string, index int) {
	i := strings.LastIndex(p, faceIndexSep)
	if i < 0 {
		return p, 0
	}
	n, err := strconv.Atoi(p[i+len(faceIndexSep):])
	if err != nil || n < 0 {
		return p[:i], 0
	}
	return p[:i], n
}

// openFaceLoader opens the file of face path p and returns the loader
// for its face. The caller must close f. The loader reads tables with
// ReadAt on demand, so f must stay open while the loader is in use.
// ok is false when the file cannot be opened or parsed, or when the face
// index is out of range; f is nil in that case.
func openFaceLoader(p string) (f *os.File, ld *ot.Loader, ok bool) {
	file, index := splitFacePath(p)
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, false
	}
	loaders, err := ot.NewLoaders(f)
	if err != nil || index >= len(loaders) {
		_ = f.Close()
		return nil, nil, false
	}
	return f, loaders[index], true
}
