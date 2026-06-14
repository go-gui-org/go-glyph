package glyph

// GradientStop defines a color at a normalized position (0.0–1.0).
type GradientStop struct {
	Color    Color
	Position float32
}

// GradientConfig defines an N-stop gradient for text rendering.
// Stops must be sorted by position in ascending order.
type GradientConfig struct {
	Stops     []GradientStop
	Direction GradientDirection
}

// gradientColorForGlyph computes the gradient color at a glyph position
// within the gradient's bounding rectangle.
func gradientColorForGlyph(gradient *GradientConfig, cx, cy, ascent float32,
	gradXOff, gradYOff, gradW, gradH float32) Color {
	if gradient == nil || len(gradient.Stops) == 0 {
		return Color{0, 0, 0, 255}
	}
	var t float32
	switch gradient.Direction {
	case GradientHorizontal:
		if gradW > 0 {
			t = (cx - gradXOff) / gradW
		}
	case GradientVertical:
		if gradH > 0 {
			t = (cy - ascent - gradYOff) / gradH
		}
	case GradientDiagonal:
		if gradW > 0 || gradH > 0 {
			tx := float32(0.0)
			ty := float32(0.0)
			if gradW > 0 {
				tx = (cx - gradXOff) / gradW
			}
			if gradH > 0 {
				ty = (cy - ascent - gradYOff) / gradH
			}
			t = (tx + ty) * 0.5
		}
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return GradientColorAt(gradient.Stops, t)
}

// GradientColorAt samples the gradient at normalized position t.
func GradientColorAt(stops []GradientStop, t float32) Color {
	if len(stops) == 0 {
		return Color{0, 0, 0, 255}
	}
	if len(stops) == 1 || t <= stops[0].Position {
		return stops[0].Color
	}
	last := stops[len(stops)-1]
	if t >= last.Position {
		return last.Color
	}
	for i := range len(stops) - 1 {
		if t >= stops[i].Position && t <= stops[i+1].Position {
			span := stops[i+1].Position - stops[i].Position
			if span <= 0 {
				return stops[i].Color
			}
			localT := (t - stops[i].Position) / span
			return LerpColor(stops[i].Color, stops[i+1].Color, localT)
		}
	}
	return last.Color
}
