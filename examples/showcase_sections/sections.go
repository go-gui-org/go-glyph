package showcase_sections

import (
	"fmt"
	"math"

	"github.com/go-gui-org/go-glyph"
)

func DrawIntro(a *App, x, y, w float32) {
	grad := &glyph.GradientConfig{
		Direction: glyph.GradientHorizontal,
		Stops: []glyph.GradientStop{
			{Color: GC(100, 160, 255, 255), Position: 0.0},
			{Color: GC(200, 120, 255, 255), Position: 0.5},
			{Color: GC(255, 100, 160, 255), Position: 1.0},
		},
	}
	_ = a.TS.DrawText(x, y, "Go-Glyph", glyph.TextConfig{
		Style:    glyph.TextStyle{FontName: "Sans Bold 48", Color: TextColor},
		Gradient: grad,
	})
	_ = a.TS.DrawText(x, y+58,
		"GPU-accelerated text rendering for Go", glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 16", Color: DimColor},
		})
}

func DrawTypography(a *App, x, y, w float32) {
	dy := float32(0)
	families := [][2]string{
		{"Sans (default)", "Sans 18"},
		{"Serif", "Serif 18"},
		{"Monospace", "Monospace 18"},
	}
	for _, f := range families {
		_ = a.TS.DrawText(x, y+dy, f[0], glyph.TextConfig{
			Style: glyph.TextStyle{FontName: f[1], Color: TextColor},
		})
		dy += 28
	}
	dy += 8
	faces := []struct {
		label string
		tf    glyph.Typeface
	}{
		{"Bold", glyph.TypefaceBold},
		{"Italic", glyph.TypefaceItalic},
		{"Bold Italic", glyph.TypefaceBoldItalic},
	}
	for _, f := range faces {
		_ = a.TS.DrawText(x, y+dy, f.label, glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName: "Sans 18", Color: TextColor, Typeface: f.tf,
			},
		})
		dy += 28
	}
}

func DrawDecorations(a *App, x, y, w float32) {
	_ = a.TS.DrawText(x, y, "Underlined text", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: "Sans 18", Color: TextColor, Underline: true,
		},
	})
	_ = a.TS.DrawText(x, y+30, "Strikethrough text", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: "Sans 18", Color: TextColor, Strikethrough: true,
		},
	})
	_ = a.TS.DrawText(x, y+60, "Highlighted text", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName: "Sans 18",
			Color:    GC(30, 30, 30, 255),
			BgColor:  Highlight,
		},
	})
}

func DrawStroke(a *App, x, y, w float32) {
	_ = a.TS.DrawText(x, y, "Hollow Text", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:    "Sans Bold 32",
			Color:       GC(0, 0, 0, 0),
			StrokeWidth: 2.0,
			StrokeColor: TextColor,
		},
	})
	_ = a.TS.DrawText(x, y+45, "Outlined Text", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:    "Sans Bold 32",
			Color:       Accent,
			StrokeWidth: 1.5,
			StrokeColor: GC(255, 255, 255, 255),
		},
	})
	_ = a.TS.DrawText(x, y+90, "Neon Stroke", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:    "Sans Bold 32",
			Color:       GC(0, 0, 0, 0),
			StrokeWidth: 2.5,
			StrokeColor: GC(0, 255, 180, 255),
		},
	})
}

func DrawLayout(a *App, x, y, w float32) {
	wrapW := float32(400)
	if w < 450 {
		wrapW = w - 20
	}

	a.Backend.DrawFilledRect(glyph.Rect{
		X: x, Y: y, Width: wrapW, Height: 72,
	}, GC(30, 30, 38, 255))
	_ = a.TS.DrawText(x+4, y+4,
		"This paragraph demonstrates word wrapping within a "+
			"constrained width. The layout engine automatically "+
			"breaks lines at word boundaries.",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 14", Color: TextColor},
			Block: glyph.BlockStyle{Wrap: glyph.WrapWord, Width: wrapW - 8},
		})

	dy := float32(88)
	aligns := []struct {
		label string
		a     glyph.Alignment
	}{
		{"Left aligned", glyph.AlignLeft},
		{"Center aligned", glyph.AlignCenter},
		{"Right aligned", glyph.AlignRight},
	}
	for _, al := range aligns {
		_ = a.TS.DrawText(x, y+dy, al.label, glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 14", Color: TextColor},
			Block: glyph.BlockStyle{Width: wrapW, Align: al.a},
		})
		dy += 22
	}

	dy += 10
	_ = a.TS.DrawText(x+20, y+dy,
		"1. This is a numbered item with a hanging indent. "+
			"Continuation lines align to the indent, not the number.",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 14", Color: TextColor},
			Block: glyph.BlockStyle{
				Wrap: glyph.WrapWord, Width: wrapW - 20,
				Indent: -20,
			},
		})
}

func DrawRichText(a *App, x, y, w float32) {
	rt := glyph.RichText{
		Runs: []glyph.StyleRun{
			{Text: "Rich text: ", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: TextColor,
			}},
			{Text: "bold", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: Warm, Typeface: glyph.TypefaceBold,
			}},
			{Text: ", ", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: TextColor,
			}},
			{Text: "italic", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: Cool, Typeface: glyph.TypefaceItalic,
			}},
			{Text: ", ", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: TextColor,
			}},
			{Text: "underlined", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: GC(140, 255, 140, 255),
				Underline: true,
			}},
			{Text: ", and ", Style: glyph.TextStyle{
				FontName: "Sans 18", Color: TextColor,
			}},
			{Text: "monospace", Style: glyph.TextStyle{
				FontName: "Monospace 16", Color: CodeGreen,
				BgColor: GC(40, 40, 50, 255),
			}},
		},
	}
	layout, err := a.TS.LayoutRichText(rt, glyph.TextConfig{})
	if err == nil {
		a.TS.DrawLayout(layout, x, y)
	}
}

func DrawMarkup(a *App, x, y, w float32) {
	markup := `<b>Bold</b>, <i>italic</i>, ` +
		`<span foreground="#ff9966">orange</span>, ` +
		`<span size="x-large">large</span>, ` +
		`<u>underline</u>, ` +
		`<span font_family="monospace">mono</span>`
	_ = a.TS.DrawText(x, y, markup, glyph.TextConfig{
		Style:     glyph.TextStyle{FontName: "Sans 16", Color: TextColor},
		UseMarkup: true,
	})
}

func DrawGradients(a *App, x, y, w float32) {
	rainbow := &glyph.GradientConfig{
		Direction: glyph.GradientHorizontal,
		Stops: []glyph.GradientStop{
			{Color: GC(255, 0, 0, 255), Position: 0.0},
			{Color: GC(255, 165, 0, 255), Position: 0.2},
			{Color: GC(255, 255, 0, 255), Position: 0.4},
			{Color: GC(0, 200, 0, 255), Position: 0.6},
			{Color: GC(0, 100, 255, 255), Position: 0.8},
			{Color: GC(160, 0, 255, 255), Position: 1.0},
		},
	}
	_ = a.TS.DrawText(x, y, "Horizontal Rainbow Gradient", glyph.TextConfig{
		Style:    glyph.TextStyle{FontName: "Sans Bold 28", Color: TextColor},
		Gradient: rainbow,
	})

	vertical := &glyph.GradientConfig{
		Direction: glyph.GradientVertical,
		Stops: []glyph.GradientStop{
			{Color: GC(255, 100, 50, 255), Position: 0.0},
			{Color: GC(50, 150, 255, 255), Position: 1.0},
		},
	}
	_ = a.TS.DrawText(x, y+45, "Vertical Warm to Cool", glyph.TextConfig{
		Style:    glyph.TextStyle{FontName: "Sans Bold 28", Color: TextColor},
		Gradient: vertical,
	})

	_ = a.TS.DrawText(x, y+90, "Gradient + Stroke", glyph.TextConfig{
		Style: glyph.TextStyle{
			FontName:    "Sans Bold 28",
			Color:       TextColor,
			StrokeWidth: 1.5,
			StrokeColor: GC(255, 255, 255, 180),
		},
		Gradient: rainbow,
	})
}

func DrawI18n(a *App, x, y, w float32) {
	lines := []struct {
		label, text string
		c           glyph.Color
	}{
		{"Chinese", "\u4f60\u597d\u4e16\u754c\uff01", Accent},
		{"Japanese", "\u3053\u3093\u306b\u3061\u306f\u4e16\u754c", Cool},
		{"Korean", "\uc548\ub155\ud558\uc138\uc694", Warm},
		{"Arabic (RTL)", "\u0645\u0631\u062d\u0628\u0627 \u0628\u0627\u0644\u0639\u0627\u0644\u0645",
			GC(200, 160, 80, 255)},
		{"Hebrew (RTL)", "\u05e9\u05dc\u05d5\u05dd \u05e2\u05d5\u05dc\u05dd",
			GC(160, 200, 80, 255)},
		{"Cyrillic", "\u041f\u0440\u0438\u0432\u0435\u0442, \u043c\u0438\u0440!",
			GC(180, 140, 200, 255)},
		{"Greek", "\u0393\u03b5\u03b9\u03b1 \u03c3\u03bf\u03c5 \u039a\u03cc\u03c3\u03bc\u03b5!",
			GC(140, 200, 180, 255)},
		{"Emoji",
			"\U0001F680 \U0001F30D \U0001F525 \U0001F44D " +
				"\U0001F600 \u2764\ufe0f \U0001F308 \U0001F389",
			TextColor},
	}
	dy := float32(0)
	for _, l := range lines {
		_ = a.TS.DrawText(x, y+dy, l.label, glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
		_ = a.TS.DrawText(x+130, y+dy, l.text, glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 20", Color: l.c},
		})
		dy += 30
	}
}

func DrawBidi(a *App, x, y, w float32) {
	_ = a.TS.DrawText(x, y,
		"The word \"\u0633\u0644\u0627\u0645\" means peace in Arabic.",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 18", Color: TextColor},
		})
	_ = a.TS.DrawText(x, y+35,
		"Mixed scripts: Latin, Greek (\u0393\u03b5\u03b9\u03ac \u03c3\u03bf\u03c5), "+
			"Cyrillic (\u041f\u0440\u0438\u0432\u0435\u0442)",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 18", Color: TextColor},
		})
	_ = a.TS.DrawText(x, y+80,
		"Pango handles bidi reordering automatically",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
}

func DrawOpenType(a *App, x, y, w float32) {
	feats := []struct {
		label, text string
		features    []glyph.FontFeature
	}{
		{"Ligatures (dlig)", "ff fi fl ffi ffl ct st",
			[]glyph.FontFeature{{Tag: "dlig", Value: 1}}},
		{"Small Caps (smcp)", "Small Capitals Text",
			[]glyph.FontFeature{{Tag: "smcp", Value: 1}}},
		{"Old-style Figures (onum)", "0123456789",
			[]glyph.FontFeature{{Tag: "onum", Value: 1}}},
		{"Tabular Figures (tnum)", "0123456789",
			[]glyph.FontFeature{{Tag: "tnum", Value: 1}}},
	}
	dy := float32(0)
	for _, f := range feats {
		_ = a.TS.DrawText(x, y+dy, f.label, glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
		dy += 18
		_ = a.TS.DrawText(x, y+dy, f.text, glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName: "Serif 20", Color: TextColor,
				Features: &glyph.FontFeatures{
					OpenTypeFeatures: f.features,
				},
			},
		})
		dy += 34
	}
}

func DrawSubSup(a *App, x, y, w float32) {
	subsFeature := &glyph.FontFeatures{
		OpenTypeFeatures: []glyph.FontFeature{{Tag: "subs", Value: 1}},
	}
	supsFeature := &glyph.FontFeatures{
		OpenTypeFeatures: []glyph.FontFeature{{Tag: "sups", Value: 1}},
	}

	normal := glyph.TextStyle{FontName: "Sans 24", Color: TextColor}
	sub := glyph.TextStyle{FontName: "Sans 24", Color: Warm, Features: subsFeature}
	sup := glyph.TextStyle{FontName: "Serif Italic 24", Color: Accent, Features: supsFeature}
	serif := glyph.TextStyle{FontName: "Serif Italic 24", Color: TextColor}

	h2o, err := a.TS.LayoutRichText(glyph.RichText{Runs: []glyph.StyleRun{
		{Text: "Chemical: H", Style: normal},
		{Text: "2", Style: sub},
		{Text: "O", Style: normal},
	}}, glyph.TextConfig{})
	if err == nil {
		a.TS.DrawLayout(h2o, x, y)
	}

	emc2, err := a.TS.LayoutRichText(glyph.RichText{Runs: []glyph.StyleRun{
		{Text: "Physics: E=mc", Style: serif},
		{Text: "2", Style: sup},
	}}, glyph.TextConfig{})
	if err == nil {
		a.TS.DrawLayout(emc2, x+280, y)
	}

	pyth, err := a.TS.LayoutRichText(glyph.RichText{Runs: []glyph.StyleRun{
		{Text: "x", Style: serif},
		{Text: "2", Style: sup},
		{Text: " + y", Style: serif},
		{Text: "2", Style: sup},
		{Text: " = z", Style: serif},
		{Text: "2", Style: sup},
	}}, glyph.TextConfig{})
	if err == nil {
		a.TS.DrawLayout(pyth, x, y+40)
	}

	_ = a.TS.DrawText(x, y+80,
		"Uses OpenType subs/sups features (font support required)",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
}

func DrawSpacing(a *App, x, y, w float32) {
	spacings := []struct {
		label   string
		spacing float32
	}{
		{"Tight (-1.5pt)", -1.5},
		{"Normal (0pt)", 0},
		{"Wide (3pt)", 3.0},
		{"Extra wide (8pt)", 8.0},
	}
	dy := float32(0)
	for _, s := range spacings {
		_ = a.TS.DrawText(x, y+dy, s.label, glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName:      "Sans 18",
				Color:         TextColor,
				LetterSpacing: s.spacing,
			},
		})
		dy += 30
	}
}

func DrawSizes(a *App, x, y, w float32) {
	sizes := []int{10, 12, 14, 18, 24, 32, 48}
	dy := float32(0)
	for _, sz := range sizes {
		_ = a.TS.DrawText(x, y+dy,
			fmt.Sprintf("%dpt The quick brown fox", sz),
			glyph.TextConfig{
				Style: glyph.TextStyle{
					FontName: fmt.Sprintf("Sans %d", sz),
					Color:    TextColor,
				},
			})
		dy += float32(sz) + 10
	}
}

func DrawRotated(a *App, x, y, w float32) {
	angle := float32(a.Frame%360) * math.Pi / 180.0
	layout, err := a.TS.LayoutText("Spinning!", glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans Bold 24", Color: Warm},
	})
	if err == nil {
		a.TS.DrawLayoutRotated(layout, x+80, y+60, angle)
	}

	rx := x + 250
	for _, deg := range []float32{15, 30, 45, -15} {
		rad := deg * math.Pi / 180.0
		l2, err := a.TS.LayoutText(
			fmt.Sprintf("%.0f\u00b0", deg),
			glyph.TextConfig{
				Style: glyph.TextStyle{FontName: "Sans 16", Color: Accent},
			})
		if err == nil {
			a.TS.DrawLayoutRotated(l2, rx, y+60, rad)
		}
		rx += 80
	}
}

func DrawVertical(a *App, x, y, w float32) {
	texts := []struct {
		text string
		c    glyph.Color
	}{
		{"\u7e26\u66f8\u304d\u30c6\u30b9\u30c8", Cool},
		{"\u5782\u76f4\u6587\u5b57\u6d4b\u8bd5", Warm},
		{"\ud55c\uad6d\uc5b4\ud14c\uc2a4\ud2b8", GC(180, 140, 200, 255)},
	}
	dx := float32(0)
	for _, t := range texts {
		_ = a.TS.DrawText(x+dx, y, t.text, glyph.TextConfig{
			Style:       glyph.TextStyle{FontName: "Sans 22", Color: t.c},
			Orientation: glyph.OrientationVertical,
		})
		dx += 40
	}
}

func DrawPathText(a *App, x, y, w float32) {
	text := "Text flowing along a circular path!"
	cfg := glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans Bold 18", Color: Accent},
	}
	layout, err := a.TS.LayoutText(text, cfg)
	if err != nil {
		return
	}
	positions := layout.GlyphPositions()
	if len(positions) == 0 {
		return
	}

	cx := x + min(w*0.35, 300)
	cy := y + 120
	radius := float32(150)

	var totalAdv float32
	for _, p := range positions {
		totalAdv += p.Advance
	}

	arcSpan := totalAdv / radius
	startAngle := -arcSpan / 2

	placements := make([]glyph.GlyphPlacement, len(layout.Glyphs))
	for i := range placements {
		placements[i] = glyph.GlyphPlacement{X: -9999, Y: -9999}
	}

	cumAdv := float32(0)
	for _, p := range positions {
		mid := cumAdv + p.Advance*0.5
		theta := startAngle + mid/radius
		tangent := theta + math.Pi/2

		arcX := cx + radius*float32(math.Cos(float64(theta)))
		arcY := cy + radius*float32(math.Sin(float64(theta)))

		halfAdv := p.Advance * 0.5
		gx := arcX - halfAdv*float32(math.Cos(float64(tangent)))
		gy := arcY - halfAdv*float32(math.Sin(float64(tangent)))

		placements[p.Index] = glyph.GlyphPlacement{
			X: gx, Y: gy, Angle: tangent,
		}
		cumAdv += p.Advance
	}

	a.TS.DrawLayoutPlaced(layout, placements)
}

func DrawSkewed(a *App, x, y, w float32) {
	l1, err := a.TS.LayoutText("Skewed Text", glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans Bold 28", Color: Warm},
	})
	if err == nil {
		a.TS.DrawLayoutTransformed(l1, x, y, glyph.AffineSkew(-0.3, 0))
	}

	l2, err := a.TS.LayoutText("Reverse Skew", glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans Bold 28", Color: Cool},
	})
	if err == nil {
		a.TS.DrawLayoutTransformed(l2, x, y+50, glyph.AffineSkew(0.3, 0))
	}

	l3, err := a.TS.LayoutText("Skew + Gradient", glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans Bold 28", Color: TextColor},
	})
	if err == nil {
		grad := &glyph.GradientConfig{
			Direction: glyph.GradientHorizontal,
			Stops: []glyph.GradientStop{
				{Color: GC(255, 100, 100, 255), Position: 0},
				{Color: GC(100, 100, 255, 255), Position: 1},
			},
		}
		a.TS.Renderer().DrawLayoutTransformedWithGradient(
			l3, x, y+100, glyph.AffineSkew(-0.2, 0), grad)
	}
}

func DrawSubpixel(a *App, x, y, w float32) {
	a.SubpixelX += 0.05
	if a.SubpixelX > 50 {
		a.SubpixelX = 0
	}

	_ = a.TS.DrawText(x+a.SubpixelX, y,
		"Smooth Subpixel Motion", glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName: "Sans 18", Color: GC(140, 220, 140, 255),
			},
		})

	snapped := float32(math.Round(float64(a.SubpixelX)))
	_ = a.TS.DrawText(x+snapped, y+35,
		"Integer Snapped Motion", glyph.TextConfig{
			Style: glyph.TextStyle{
				FontName: "Sans 18", Color: GC(220, 100, 100, 255),
			},
		})

	_ = a.TS.DrawText(x, y+75,
		"Watch the green text glide vs red text jitter",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
}

func DrawHitTest(a *App, x, y, w float32) {
	text := "Move the mouse over this text to see hit testing. " +
		"The character under the cursor is highlighted and a " +
		"cursor line is drawn at the nearest position."

	wrapW := float32(500)
	if w < 540 {
		wrapW = w - 20
	}

	cfg := glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans 16", Color: TextColor},
		Block: glyph.BlockStyle{Wrap: glyph.WrapWord, Width: wrapW},
	}

	layout, err := a.TS.LayoutTextCached(text, cfg)
	if err != nil {
		return
	}

	a.TS.DrawLayout(layout, x, y)

	localX := float32(a.MouseX) - x
	localY := float32(a.MouseY) - y

	if localX < -20 || localX > wrapW+20 ||
		localY < -20 || localY > layout.VisualHeight+20 {
		return
	}

	idx := layout.GetClosestOffset(localX, localY)

	if cr, ok := layout.GetCharRect(idx); ok {
		a.Backend.DrawFilledRect(glyph.Rect{
			X: x + cr.X, Y: y + cr.Y,
			Width: cr.Width, Height: cr.Height,
		}, GC(255, 255, 100, 60))
	}

	if cp, ok := layout.GetCursorPos(idx); ok {
		a.Backend.DrawFilledRect(glyph.Rect{
			X: x + cp.X, Y: y + cp.Y,
			Width: 2, Height: cp.Height,
		}, GC(255, 100, 100, 200))
	}

	_ = a.TS.DrawText(x, y+layout.VisualHeight+15,
		fmt.Sprintf("Byte index: %d", idx),
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
}

func DrawDirectText(a *App, x, y, w float32) {
	code := glyph.RichText{Runs: []glyph.StyleRun{
		{Text: "ts", Style: glyph.TextStyle{
			FontName: "Monospace 14", Color: TextColor,
		}},
		{Text: ".", Style: glyph.TextStyle{
			FontName: "Monospace 14", Color: DimColor,
		}},
		{Text: "DrawText", Style: glyph.TextStyle{
			FontName: "Monospace 14", Color: Accent,
		}},
		{Text: "(x, y, ", Style: glyph.TextStyle{
			FontName: "Monospace 14", Color: DimColor,
		}},
		{Text: `"Hello Go!"`, Style: glyph.TextStyle{
			FontName: "Monospace 14", Color: CodeGreen,
		}},
		{Text: ", cfg)", Style: glyph.TextStyle{
			FontName: "Monospace 14", Color: DimColor,
		}},
	}}

	a.Backend.DrawFilledRect(glyph.Rect{
		X: x, Y: y, Width: 420, Height: 26,
	}, GC(30, 30, 40, 255))

	cl, err := a.TS.LayoutRichText(code, glyph.TextConfig{})
	if err == nil {
		a.TS.DrawLayout(cl, x+8, y+4)
	}

	_ = a.TS.DrawText(x, y+40, "Result:", glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
	})

	_ = a.TS.DrawText(x, y+60, "Hello Go!", glyph.TextConfig{
		Style: glyph.TextStyle{FontName: "Sans Bold 32", Color: Warm},
	})

	_ = a.TS.DrawText(x, y+110,
		"DrawText is the simplest API \u2014 one call, no layout management",
		glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 12", Color: DimColor},
		})
}

func DrawTransforms(a *App, x, y, w float32) {
	type sample struct {
		label     string
		text      string
		color     glyph.Color
		transform glyph.AffineTransform
		dx, dy    float32
	}

	rot15 := glyph.AffineRotation(15 * math.Pi / 180)
	rot30 := glyph.AffineRotation(30 * math.Pi / 180)
	scale := glyph.AffineTransform{XX: 1.4, YY: 0.7}
	combined := glyph.AffineRotation(20 * math.Pi / 180).
		Multiply(glyph.AffineSkew(-0.25, 0))

	samples := []sample{
		{"Rotate 15\u00b0", "Rotated", Warm, rot15, 0, 0},
		{"Rotate 30\u00b0", "Rotated", Cool, rot30, 220, 0},
		{"Skew", "Skewed", Accent, glyph.AffineSkew(-0.3, 0), 440, 0},
		{"Scale 1.4\u00d70.7", "Scaled", Highlight, scale, 0, 90},
		{"Rotate+Skew", "Combined", GC(180, 140, 255, 255), combined, 220, 90},
	}

	for _, s := range samples {
		_ = a.TS.DrawText(x+s.dx, y+s.dy, s.label, glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans 10", Color: DimColor},
		})
		l, err := a.TS.LayoutText(s.text, glyph.TextConfig{
			Style: glyph.TextStyle{FontName: "Sans Bold 24", Color: s.color},
		})
		if err == nil {
			a.TS.DrawLayoutTransformed(l, x+s.dx, y+s.dy+18, s.transform)
		}
	}
}
