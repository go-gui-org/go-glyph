//go:build linux || darwin || windows

package glyph

import (
	"math"
	"sort"
)

// This file holds the two corrections that make a fallback glyph look like it
// belongs next to the primary font's text:
//
//   - Ranking (rankIconFallbacks): for Private Use Area codepoints, coverage
//     alone is a terrible signal. Dozens of ordinary text faces map a handful
//     of PUA slots to unrelated shapes — Athelas answers U+F608 with a serif
//     "D", Webdings answers U+F0C5 with a pictogram one third of a cell wide.
//     Taking the first cmap hit in tier order therefore picks a wrong glyph
//     whenever a real icon font sits later in the list. Candidates are scored
//     by how much of the Nerd Font layout they cover and the best-covered font
//     leads.
//
//   - Fitting (fallbackFitScale): a fallback face is opened at the primary
//     font's pixel size, which equalizes the em, not the visible letter size.
//     Design sizes differ widely — a CJK face reserves its em for ideographs,
//     so its Latin and symbol glyphs sit far below the primary's cap height
//     and read as "small". The face is opened at a size scaled by the cap
//     height ratio instead, which is what kitty and WezTerm do.
//
//     A grid caller (a terminal, TextStyle.CellWidth > 0) is different. There
//     an ideograph owns exactly two cells, and a Latin monospace cell is about
//     0.6 em, so two cells are 1.2 em while the ideograph is 1 em. At the cap
//     height size the ideograph fills only 80-85% of its box, and a gap shows
//     after every one (#207). So for a grid caller a face that has ideographs
//     is sized so one ideograph is two cells wide. Ghostty does the same
//     (its ic_width normalization).
//
// Both are keyed off data already cached (the coverage cmap, the parsed face),
// so neither adds a font parse to the layout path.

// iconProbeRunes samples the Nerd Font v3 codepoint layout: Pomicons,
// Powerline, Font Awesome Extension, Weather, Seti-UI, Devicons, Codicons,
// Font Awesome and Octicons. A real icon font covers nearly all of them; a
// text face that merely squats a few PUA slots covers one or two. Sampling
// rather than counting whole ranges keeps scoring to a fixed handful of cmap
// lookups per font.
var iconProbeRunes = [...]rune{
	0xE00A,  // Pomicons
	0xE0A0,  // Powerline branch
	0xE0B0,  // Powerline separator
	0xE216,  // Font Awesome Extension
	0xE300,  // Weather
	0xE5FB,  // Seti-UI
	0xE62B,  // Seti-UI / custom
	0xE706,  // Devicons
	0xE7A8,  // Devicons
	0xEA61,  // Codicons
	0xEB05,  // Codicons
	0xF00C,  // Font Awesome check
	0xF0F3,  // Font Awesome bell
	0xF17C,  // Font Awesome brands
	0xF400,  // Octicons
	0xF4A8,  // Octicons
	0xF0004, // Material Design (plane 15)
	0xF1000, // Material Design (plane 15)
}

// scoreIconCoverage counts how many iconProbeRunes the cmap maps. Called once
// per font, from parseCoverage, so the layout path only ever reads the result.
func scoreIconCoverage(c *coverage) int8 {
	n := int8(0)
	for _, r := range iconProbeRunes {
		if _, ok := c.cmap.Lookup(r); ok {
			n++
		}
	}
	return n
}

// isPUA reports whether r lies in a Unicode Private Use Area. Nerd Font icons
// live in the BMP area and in plane 15 (Material Design icons); plane 16 is
// included for completeness.
func isPUA(r rune) bool {
	return (r >= 0xE000 && r <= 0xF8FF) ||
		(r >= 0xF0000 && r <= 0xFFFFD) ||
		(r >= 0x100000 && r <= 0x10FFFD)
}

// textIsPUA reports whether a cluster's base rune is a private-use codepoint.
// Only the first rune matters: an icon carries no combining marks, and a
// cluster whose base is ordinary text must not be reordered.
func textIsPUA(text string) bool {
	for _, r := range text {
		return isPUA(r)
	}
	return false
}

// rankIconFallbacks reorders PUA candidates so the font with the most Nerd
// Font coverage leads, leaving fallback tier order intact among equals
// (sort.SliceStable). Non-PUA clusters return untouched — the overwhelmingly
// common case, so neither the filter nor the sort is on the normal path.
//
// Scores come from the resident coverage cache, which every candidate here has
// already been through (that is how it was found to cover the cluster), so
// ranking costs a map lookup per candidate and no font I/O.
//
// Candidates that map none of the probes are dropped outright rather than
// merely demoted. A private-use codepoint means nothing outside the font that
// defines it, so a text face mapping a few PUA slots is not a worse icon — it
// is an unrelated glyph wearing an icon's codepoint, and drawing Athelas's "D"
// for a folder icon is a silent lie. With no candidate left the base font's
// .notdef renders instead, which tells the reader the truth: this build has no
// font for that icon.
func rankIconFallbacks(paths []string, text string) []string {
	if len(paths) == 0 || !textIsPUA(text) {
		return paths
	}
	score := func(p string) int8 {
		if cov := loadCoverage(p); cov != nil {
			return cov.iconScore
		}
		return 0
	}
	kept := paths[:0]
	for _, p := range paths {
		if score(p) > 0 {
			kept = append(kept, p)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		return score(kept[i]) > score(kept[j])
	})
	return kept
}

// minFitScale/maxFitScale clamp the fit scale. A fallback whose cap height is
// wildly different from the primary's is more likely to be a face whose 'H'
// is not a cap (an icon or symbol font that maps the slot to a pictogram)
// than a face that genuinely needs a 2x correction, so the correction is
// bounded rather than trusted outright.
const (
	minFitScale = 0.8
	maxFitScale = 1.3
	// fitScaleDeadband leaves near-matching faces at exactly 1.0, so the
	// common case adds no distinct glyph-cache entries or atlas rasters.
	fitScaleDeadband = 0.02
)

// fallbackFitScale returns the multiplier to apply to the primary font's
// pixel size when opening fallback face fb.
//
// cellEm is the grid cell width in em units of the primary size (gridCellEm),
// or 0 for a caller that is not a grid. When it is set and fb has an
// ideograph, the scale makes one ideograph exactly two cells wide. Otherwise
// the scale makes the fallback's capital letters match the primary's.
//
// Returns 1 when the needed measurement is unknown (a face with no 'H'/'M' —
// pure emoji, pure ideographic — on the cap path) or when the result is
// already within the deadband.
func fallbackFitScale(base, fb *cachedFace, cellEm float64) float64 {
	if base == nil || fb == nil {
		return 1
	}
	var s float64
	// The NaN/Inf check repeats cellEmAt's guard for callers that pass a
	// computed cellEm directly: a NaN survives min/max and would reach the
	// rasterizer as the font size.
	if ic := fb.icEm(); cellEm > 0 && !math.IsInf(cellEm, 0) && ic > 0 {
		// Grid fit: two cells over the ideograph advance. Does not depend on
		// the base face, because the cell already describes it.
		s = 2 * cellEm / ic
	} else {
		baseCap, fbCap := base.capEm(), fb.capEm()
		if baseCap <= 0 || fbCap <= 0 {
			return 1
		}
		s = baseCap / fbCap
	}
	if s > 1-fitScaleDeadband && s < 1+fitScaleDeadband {
		return 1
	}
	return min(max(s, minFitScale), maxFitScale)
}

// gridCellEm returns style's cell width in em units of its font size, the
// cellEm argument of fallbackFitScale. 0 means "not a grid caller": no cell
// width, or a size or width that is not a finite positive number. The guard
// matters because a NaN passes through min/max unchanged and would reach the
// rasterizer as the font size.
func gridCellEm(style TextStyle) float64 {
	return cellEmAt(float64(style.CellWidth), float64(resolveFontSize(style)))
}

// cellEmAt is gridCellEm for a cell width and a font size that are already in
// the same unit (both logical px, or both device px).
func cellEmAt(cellW, size float64) float64 {
	if !(cellW > 0) || !(size > 0) || math.IsInf(cellW, 0) || math.IsInf(size, 0) {
		return 0
	}
	return cellW / size
}

// itemFontScale reads an item's fallback fit factor, treating the zero value
// as "no scaling" so every item built before FontScale existed (and every
// non-fallback item) rasterizes unchanged.
func itemFontScale(item Item) float64 {
	if item.FontScale <= 0 {
		return 1
	}
	return item.FontScale
}

// textFallbackFitScale is the render-side equivalent of fallbackFitScale for
// the text (re-shaping) path, where the primary font is known only as the
// resolved candidate list. primaryPaths[0] is the requested face; anything
// after it is already a style/family degradation, so the first entry is the
// right cap-height reference.
//
// cellEm is the item style's gridCellEm, so the text path picks the same grid
// fit as the layout did.
func textFallbackFitScale(primaryPaths []string, fbPath string, cellEm float64) float64 {
	if len(primaryPaths) == 0 {
		return 1
	}
	return fallbackFitScale(loadCachedFace(primaryPaths[0]),
		loadCachedFace(fbPath), cellEm)
}

// icProbes are tried in order for the ideograph advance. U+6C34 (水) is the
// probe Ghostty and CSS (the "ic" unit) use; U+4E2D (中) backs it up. Every
// CJK face sets the same advance on all ideographs, so one hit is enough.
var icProbes = [...]rune{0x6C34, 0x4E2D}

// icEm returns the face's ideograph advance in em units, measured once and
// memoized on the cached face (0 = measured and the face has no ideograph).
// The locking and panic rules are the same as capEm.
func (cf *cachedFace) icEm() float64 {
	if cf.base != nil {
		return cf.base.icEm()
	}
	cf.mu.Lock()
	defer cf.mu.Unlock()
	if cf.icMeasured {
		return cf.ic
	}
	cf.icMeasured = true
	defer func() { _ = recover() }()
	upem := float64(nonZeroUpem(cf.upem))
	for _, r := range icProbes {
		gid, ok := cf.face.NominalGlyph(r)
		if !ok {
			continue
		}
		if a := float64(cf.face.HorizontalAdvance(gid)) / upem; a > 0 {
			cf.ic = a
			return cf.ic
		}
	}
	return 0
}

// capHeightProbes are tried in order for the cap-height measurement. 'H' and
// 'M' are flat-topped in essentially every design, so their ink height is the
// cap height without an overshoot correction; 'X' backs them up for faces
// carrying only a partial Latin set.
var capHeightProbes = [...]rune{'H', 'M', 'X'}

// capEm returns the face's cap height in em units, measured once and memoized
// on the cached face (0 = measured and unavailable, so the miss is not
// re-measured per glyph). Guarded by cf.mu: the measurement mutates the
// shared face (GlyphExtents fills the face's extent cache), which must not
// race the color path's SetPpem+GlyphData pair on the same cachedFace, nor
// another Context's measurement of the same face.
func (cf *cachedFace) capEm() float64 {
	// An instance measures on its base face. The raster side computes the fit
	// from base faces (textFallbackFitScale), so layout must use the same cap
	// height, or a fallback would shape and draw at two sizes.
	if cf.base != nil {
		return cf.base.capEm()
	}
	cf.mu.Lock()
	defer cf.mu.Unlock()
	if cf.capMeasured {
		return cf.cap
	}
	// capMeasured is set before probing so a panic below (recovered by the
	// deferred call) leaves the face marked measured with cap 0: no re-probe
	// storm per glyph, and 0 reads as "no cap height" (fit scale 1) everywhere.
	cf.capMeasured = true
	// GlyphExtents can panic on defective font tables, as the shaper panics
	// safeShape recovers (see safeShape); a bad face degrades to "no cap
	// height" instead of crashing the render thread.
	defer func() { _ = recover() }()
	upem := float64(nonZeroUpem(cf.upem))
	for _, r := range capHeightProbes {
		gid, ok := cf.face.NominalGlyph(r)
		if !ok {
			continue
		}
		ext, ok := cf.face.GlyphExtents(gid)
		if !ok {
			continue
		}
		// GlyphExtents reports height downward from the top of the ink box,
		// so a normal glyph's Height is negative.
		if h := float64(-ext.Height) / upem; h > 0 {
			cf.cap = h
			return cf.cap
		}
	}
	return 0
}
