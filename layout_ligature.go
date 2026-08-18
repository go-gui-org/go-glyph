package glyph

// Canvas2D exposes no shaping information: measureText returns one width
// and fillText draws whatever the browser shapes internally. The helpers
// here recover the one shaping fact the layout needs — which grapheme
// clusters a mandatory ligature swallowed — from prefix widths alone, so
// the wasm backend can suppress caret stops inside a ligature the way the
// FreeType backend does from HarfBuzz cluster data (layout_ft.go:787).
//
// They are deliberately free of any js/wasm dependency (no build tag, no
// syscall/js): CI only builds and vets GOOS=js, it never runs it, so the
// logic is unit tested on the host with a stub measure function.

// ligatureEpsilon is the advance, in logical pixels, at or below which a
// cluster is treated as contributing nothing to the run — i.e. absorbed
// into its predecessor. Kept tight on purpose: an ordinary discretionary
// ligature ("fi") still advances by most of an "i", so only a true
// zero/negative delta (the alef of a lam-alef) qualifies. Browsers let the
// caret stop inside an "fi", and so does this backend.
const ligatureEpsilon = 0.05

// hasComplexShaping reports whether s contains a rune from a script with
// joining behaviour or mandatory ligatures, where a cluster can be
// swallowed by its neighbour. Used as a cheap gate so pure Latin text
// pays no extra measureText calls at all. The detection itself is
// measurement-driven (a swallowed cluster measures at or below
// ligatureEpsilon), so including a script here can only widen the
// detection, never misfire on real letters.
func hasComplexShaping(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0600 && r <= 0x06FF, // Arabic
			r >= 0x0700 && r <= 0x074F,   // Syriac
			r >= 0x0750 && r <= 0x077F,   // Arabic Supplement
			r >= 0x07C0 && r <= 0x07FF,   // N'Ko (mandatory ligatures)
			r >= 0x0870 && r <= 0x08FF,   // Arabic Extended-A/B
			r >= 0x0900 && r <= 0x0DFF,   // Indic (Devanagari..Sinhala)
			r >= 0xFB50 && r <= 0xFDFF,   // Arabic Presentation Forms-A
			r >= 0xFE70 && r <= 0xFEFF,   // Arabic Presentation Forms-B
			r >= 0x1E900 && r <= 0x1E95F: // Adlam (mandatory ligatures)
			return true
		}
	}
	return false
}

// detectAbsorbed reports, per cluster of a shaping run, whether the
// browser folded that cluster into a preceding one.
//
// measure(n) must return the width of the run's first n clusters shaped
// together (n in 1..count); measure(0) is never called and is defined as
// zero. A cluster whose incremental width is at or below eps added
// nothing to the run: the glyph it would have drawn is already part of the
// ligature its predecessor now carries, so it is not a caret stop.
//
// The first cluster of a run is never absorbed — there is no predecessor
// to absorb it, and a zero-width leading cluster (a combining mark opening
// the run) still owns its own position.
//
// Returns nil when nothing was absorbed, so the caller can skip the whole
// merge pass on the common path without allocating.
func detectAbsorbed(count int, measure func(prefixLen int) float64,
	eps float64) []bool {

	if count < 2 {
		return nil
	}
	var absorbed []bool
	prev := measure(1)
	for k := 1; k < count; k++ {
		w := measure(k + 1)
		if w-prev <= eps {
			if absorbed == nil {
				absorbed = make([]bool, count)
			}
			absorbed[k] = true
		}
		// Advance the baseline even for an absorbed cluster: widths are
		// cumulative over the run, so the next delta must be taken from
		// the widest prefix seen, not from the last non-absorbed one.
		prev = w
	}
	return absorbed
}
