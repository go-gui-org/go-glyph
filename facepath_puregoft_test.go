//go:build linux || darwin || windows

package glyph

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
)

func TestFacePathRoundTrip(t *testing.T) {
	cases := []struct {
		file  string
		index int
	}{
		{"/a/b.ttf", 0},
		{"/a/b.ttc", 1},
		{"/a/b#3.ttc", 12},
		{`C:\Fonts\msgothic.ttc`, 2},
	}
	for _, c := range cases {
		p := facePath(c.file, c.index)
		if c.index == 0 && p != c.file {
			t.Errorf("facePath(%q, 0) = %q, want the plain path", c.file, p)
		}
		f, i := splitFacePath(p)
		if f != c.file || i != c.index {
			t.Errorf("splitFacePath(%q) = (%q, %d), want (%q, %d)",
				p, f, i, c.file, c.index)
		}
	}
	// A malformed index suffix falls back to face 0 of the file part.
	if f, i := splitFacePath("/x.ttc" + faceIndexSep + "zz"); f != "/x.ttc" || i != 0 {
		t.Errorf("malformed suffix = (%q, %d), want (/x.ttc, 0)", f, i)
	}
}

// buildTTC writes a font collection holding the given single-face font
// files, in order. Each member's table directory is rewritten so its
// table offsets point at the copies inside the collection.
func buildTTC(t *testing.T, members ...string) string {
	t.Helper()
	type table struct {
		rec  []byte // 16-byte table record
		data []byte
	}
	type member struct {
		header []byte // 12-byte offset table
		tables []table
	}
	var ms []member
	for _, path := range members {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		n := int(binary.BigEndian.Uint16(raw[4:6]))
		m := member{header: raw[:12]}
		for i := range n {
			rec := raw[12+16*i : 12+16*i+16]
			off := binary.BigEndian.Uint32(rec[8:12])
			ln := binary.BigEndian.Uint32(rec[12:16])
			m.tables = append(m.tables, table{
				rec:  append([]byte(nil), rec...),
				data: raw[off : off+ln],
			})
		}
		ms = append(ms, m)
	}
	pad4 := func(n int) int { return (n + 3) &^ 3 }

	// Layout: TTC header, then each member's directory followed by its
	// tables, each 4-byte aligned.
	out := make([]byte, 12+4*len(ms))
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], uint32(len(ms)))
	for mi, m := range ms {
		dirOff := len(out)
		binary.BigEndian.PutUint32(out[12+4*mi:], uint32(dirOff))
		dirLen := 12 + 16*len(m.tables)
		pos := pad4(dirOff + dirLen)
		dir := append([]byte(nil), m.header...)
		var body []byte
		for _, tb := range m.tables {
			binary.BigEndian.PutUint32(tb.rec[8:], uint32(pos+len(body)))
			dir = append(dir, tb.rec...)
			body = append(body, tb.data...)
			body = append(body, make([]byte, pad4(len(tb.data))-len(tb.data))...)
		}
		out = append(out, dir...)
		out = append(out, make([]byte, pos-(dirOff+dirLen))...)
		out = append(out, body...)
	}
	dst := filepath.Join(t.TempDir(), "synthetic.ttc")
	if err := os.WriteFile(dst, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// twoSystemTTFs returns two installed single-face .ttf files with
// different family names, or skips the test.
func twoSystemTTFs(t *testing.T) (a, b string, famA, famB string) {
	t.Helper()
	ctx, err := NewContext(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Free()
	for _, p := range ctx.fontPaths {
		if !strings.HasSuffix(strings.ToLower(p), ".ttf") {
			continue
		}
		faces := describeFontFaces(p)
		if len(faces) != 1 || faces[0].desc.Family == "" {
			continue
		}
		fam := faces[0].desc.Family
		switch {
		case a == "":
			a, famA = p, fam
		case fam != famA:
			return a, p, famA, fam
		}
	}
	t.Skip("need two installed .ttf fonts with different families")
	return
}

// TestCollectionFacesRegistered is the regression test for .ttc handling:
// only face 0 of a collection was registered, so any other member (the
// Bold face of Helvetica.ttc, the SC face of a CJK collection) could not
// be found by name, and parseFace/parseCoverage always read face 0.
func TestCollectionFacesRegistered(t *testing.T) {
	a, b, famA, famB := twoSystemTTFs(t)
	ttc := buildTTC(t, a, b)

	faces := describeFontFaces(ttc)
	if len(faces) != 2 {
		t.Fatalf("describeFontFaces found %d faces, want 2", len(faces))
	}
	if faces[0].path != ttc || faces[1].path != facePath(ttc, 1) {
		t.Fatalf("face paths = %q, %q", faces[0].path, faces[1].path)
	}

	ctx := &Context{
		fontPaths:   map[string]string{},
		fontWeights: map[string]font.Weight{},
		fontItalics: map[string]bool{},
		families:    map[string]string{},
	}
	if err := ctx.AddFontFile(ttc); err != nil {
		t.Fatal(err)
	}
	if got := ctx.fontPaths[famA]; got != ttc {
		t.Errorf("fontPaths[%q] = %q, want face 0 %q", famA, got, ttc)
	}
	want1 := facePath(ttc, 1)
	if got := ctx.fontPaths[famB]; got != want1 {
		t.Errorf("fontPaths[%q] = %q, want face 1 %q", famB, got, want1)
	}

	// Face 1 must load its own tables, not face 0's.
	cf := loadCachedFace(want1)
	if cf == nil {
		t.Fatal("parseFace failed for collection face 1")
	}
	if got := cf.face.Describe().Family; got != famB {
		t.Errorf("face 1 family = %q, want %q", got, famB)
	}
	if cov := loadCoverage(want1); cov == nil {
		t.Error("parseCoverage failed for collection face 1")
	}

	// An out-of-range index fails cleanly.
	if _, _, ok := openFaceLoader(facePath(ttc, 5)); ok {
		t.Error("openFaceLoader accepted face index 5 of a 2-face file")
	}
}
