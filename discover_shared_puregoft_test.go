//go:build linux || darwin || windows

package glyph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
)

// TestAliasRankBeatsWalkOrder is the regression test for the generic alias
// taking the first matching family in walk order. On Android
// "NotoSansAdlam-VF.ttf" sorts before "Roboto-Regular.ttf" and the old
// substring rule ("noto sans") matched it, so an Adlam font became the
// sans-serif fallback for every unresolved family.
func TestAliasRankBeatsWalkOrder(t *testing.T) {
	table := aliasTable{"sans-serif": {"roboto", "noto sans"}}
	reg := font.Aspect{Weight: font.WeightNormal}
	bold := font.Aspect{Weight: font.WeightBold}
	faces := []faceInfo{
		// Not in the table at all: a script font must never fill the alias.
		{path: "/f/NotoSansAdlam-VF.ttf", desc: font.Description{
			Family: "Noto Sans Adlam", Aspect: reg}},
		// Listed, but ranked after Roboto.
		{path: "/f/NotoSans-Regular.ttf", desc: font.Description{
			Family: "Noto Sans", Aspect: reg}},
		// Most preferred family: wins even though it is Bold and comes later.
		{path: "/f/Roboto-Bold.ttf", desc: font.Description{
			Family: "Roboto", Aspect: bold}},
		// Same family, closer to Regular: replaces the Bold face.
		{path: "/f/Roboto-Regular.ttf", desc: font.Description{
			Family: "Roboto", Aspect: reg}},
		// Lower-ranked family after the key is taken: ignored.
		{path: "/f/NotoSans-Other.ttf", desc: font.Description{
			Family: "Noto Sans", Aspect: reg}},
	}
	ctx := newScanCtx()
	s := newFontScan(ctx)
	for i, fc := range faces {
		s.considerFace(fc, table)
		if i == 0 {
			if got, ok := ctx.fontPaths["sans-serif"]; ok {
				t.Fatalf("unlisted family filled sans-serif: %q", got)
			}
		}
		if i == 1 {
			if got := ctx.fontPaths["sans-serif"]; got != "/f/NotoSans-Regular.ttf" {
				t.Fatalf("after Noto Sans: sans-serif = %q", got)
			}
		}
	}
	if got := ctx.fontPaths["sans-serif"]; got != "/f/Roboto-Regular.ttf" {
		t.Errorf("sans-serif = %q, want /f/Roboto-Regular.ttf", got)
	}
	if got := ctx.fontWeights["sans-serif"]; got != font.WeightNormal {
		t.Errorf("sans-serif weight = %v, want Normal", got)
	}
}

// TestAliasTableLookup checks whole-name matching and rank.
func TestAliasTableLookup(t *testing.T) {
	table := aliasTable{
		"sans-serif": {"dejavu sans", "noto sans"},
		"monospace":  {"dejavu sans mono"},
	}
	tests := []struct {
		fam   string
		alias string
		rank  int
		ok    bool
	}{
		{"dejavu sans", "sans-serif", 0, true},
		{"noto sans", "sans-serif", 1, true},
		{"dejavu sans mono", "monospace", 0, true},
		{"noto sans arabic", "", 0, false},
		{"dejavu", "", 0, false},
		{"", "", 0, false},
	}
	for _, tt := range tests {
		alias, rank, ok := table.lookup(tt.fam)
		if alias != tt.alias || rank != tt.rank || ok != tt.ok {
			t.Errorf("lookup(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tt.fam, alias, rank, ok, tt.alias, tt.rank, tt.ok)
		}
	}
	if _, _, ok := aliasTable(nil).lookup("dejavu sans"); ok {
		t.Error("nil table matched")
	}
}

// TestPlatformAliasesConsistent checks the platform tables: every family is
// lower-cased and listed under one key only, and each key's rank-0 family
// is the one resolveFontFamily picks, so the alias fallback and the generic
// name agree when both fonts exist.
func TestPlatformAliasesConsistent(t *testing.T) {
	owner := map[string]string{}
	for key, fams := range platformAliases {
		if len(fams) == 0 {
			t.Errorf("alias %q has no families", key)
			continue
		}
		for _, f := range fams {
			if f != strings.ToLower(f) {
				t.Errorf("alias %q family %q is not lower-case", key, f)
			}
			if prev, dup := owner[f]; dup {
				t.Errorf("family %q listed under %q and %q", f, prev, key)
			}
			owner[f] = key
		}
	}
	want := map[string]string{
		"sans-serif": platformGenerics.sans,
		"serif":      platformGenerics.serif,
		"monospace":  platformGenerics.mono,
	}
	for key, fam := range want {
		fams := platformAliases[key]
		if len(fams) == 0 || fams[0] != strings.ToLower(fam) {
			t.Errorf("platformAliases[%q][0] = %v, want %q", key, fams,
				strings.ToLower(fam))
		}
	}
}

// TestGenericNamesResolve checks the shared resolveFontFamily logic
// independently of the platform tables.
func TestGenericNamesResolve(t *testing.T) {
	g := genericNames{sans: "S", serif: "R", mono: "M"}
	tests := []struct{ in, want string }{
		{"", "S"},
		{"Sans 12", "S"},
		{"sans-serif Bold", "S"},
		{"system", "S"},
		{"Serif 11", "R"},
		{"monospace", "M"},
		{"Mono Bold 12", "M"},
		{"Fira Code 12", "Fira Code"},
	}
	for _, tt := range tests {
		if got := g.resolve(tt.in); got != tt.want {
			t.Errorf("resolve(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// copyTestFont copies one discovered system font into dir and returns the
// copy's path. The test is skipped when the machine has no system font.
func copyTestFont(t *testing.T, dir string) string {
	t.Helper()
	src, _ := splitFacePath(cachedSystemFonts().fontPaths["sans-serif"])
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("no readable system font: %v", err)
	}
	dst := filepath.Join(dir, "test"+filepath.Ext(src))
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// TestWalkDirsSkipsRelative is the regression test for discovery walking a
// relative dir: with $HOME or %WINDIR% unset, "$HOME/.fonts" became
// ".fonts", resolved against the working directory, and any font file
// placed there was parsed.
func TestWalkDirsSkipsRelative(t *testing.T) {
	tmp := t.TempDir()
	copyTestFont(t, tmp)
	t.Chdir(tmp)

	ctx := newScanCtx()
	newFontScan(ctx).walkDirs([]string{".", ""}, nil)
	if len(ctx.fontPaths) != 0 {
		t.Errorf("relative dir was walked: fontPaths = %v", ctx.fontPaths)
	}

	// Control: the same dir given as an absolute path is walked.
	ctx = newScanCtx()
	newFontScan(ctx).walkDirs([]string{tmp}, nil)
	if len(ctx.fontPaths) == 0 {
		t.Error("absolute dir was not walked")
	}
}

// TestDiscoverSystemFontsCopiesMaps checks that the once-per-process scan
// is shared safely: each Context gets its own maps, so AddFontFile on one
// Context does not leak into another Context or the cache.
func TestDiscoverSystemFontsCopiesMaps(t *testing.T) {
	a := &Context{}
	a.discoverSystemFonts()
	b := &Context{}
	b.discoverSystemFonts()

	a.fontPaths["__probe__"] = "/x"
	a.fontWeights["__probe__"] = font.WeightBold
	a.fontItalics["__probe__"] = true
	a.families["__probe__"] = "Probe"
	if len(a.colorPaths) > 0 {
		a.colorPaths[0] = "/changed"
	}
	if len(a.fallbackPaths) > 0 {
		a.fallbackPaths[0] = "/changed"
	}

	sf := cachedSystemFonts()
	for name, m := range map[string]map[string]string{
		"b.fontPaths": b.fontPaths, "cache.fontPaths": sf.fontPaths,
		"b.families": b.families, "cache.families": sf.families,
	} {
		if _, ok := m["__probe__"]; ok {
			t.Errorf("%s shares storage with another Context", name)
		}
	}
	if _, ok := sf.fontWeights["__probe__"]; ok {
		t.Error("cache.fontWeights shares storage with a Context")
	}
	if _, ok := sf.fontItalics["__probe__"]; ok {
		t.Error("cache.fontItalics shares storage with a Context")
	}
	for _, p := range append(append([]string{}, b.colorPaths...), b.fallbackPaths...) {
		if p == "/changed" {
			t.Error("fallback slices share storage between Contexts")
		}
	}
	for _, p := range append(append([]string{}, sf.color...), sf.general...) {
		if p == "/changed" {
			t.Error("fallback slices share storage with the cache")
		}
	}
}

// TestFillDefaultKeys checks that defaults fill only unset keys.
func TestFillDefaultKeys(t *testing.T) {
	m := map[string]string{"a": "/found"}
	fillDefaultKeys(m, "/default", "a", "b")
	if m["a"] != "/found" || m["b"] != "/default" {
		t.Errorf("fillDefaultKeys = %v", m)
	}
}
