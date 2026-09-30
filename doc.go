// Package glyph provides high-quality text shaping, layout, and rendering
// for GPU-accelerated applications. Shaping (HarfBuzz via go-text/typesetting)
// and rasterization (x/image/vector) are pure Go on all native platforms —
// no C libraries or system text APIs are required for the core library.
// The WASM path uses the browser's Canvas2D for measuring and drawing.
//
// # Platform matrix
//
//	OS          Shaper              Rasterizer
//	Linux       HarfBuzz (go-text)  x/image/vector
//	macOS       HarfBuzz (go-text)  x/image/vector
//	iOS         HarfBuzz (go-text)  x/image/vector
//	Windows     HarfBuzz (go-text)  x/image/vector
//	Android     HarfBuzz (go-text)  x/image/vector
//	WASM        Canvas2D            Canvas2D
//
// The glyph package builds with CGO_ENABLED=0 everywhere. CGo is only used
// by the optional GPU backends (backend/gpu, backend/ios, backend/android)
// to reach native graphics APIs (Metal, OpenGL, GLES).
//
// # Quick start
//
//	backend := ebitengine.NewBackend() // or gpu, web, etc.
//	ts, err := glyph.NewTextSystem(backend)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer ts.Free()
//
//	layout, err := ts.LayoutText("Hello, world!", glyph.TextConfig{
//	    Style: glyph.TextStyle{FontName: "Sans 18"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ts.DrawLayout(layout, 10, 10)
//	ts.Commit() // once per frame, after all draw calls
//
// # Core concepts
//
// [TextSystem] is the main entry point. It owns a [Context] (shaping,
// font management), a [Renderer] (glyph atlas + draw calls), and a layout
// cache (FNV-1a hash of text + config, LRU eviction, 5s idle prune on
// [TextSystem.Commit]).
//
// Pre-computed [Layout] values from [TextSystem.LayoutText] or
// [TextSystem.LayoutRichText] can be drawn repeatedly. For one-shot
// rendering use [TextSystem.DrawText], which looks up or creates a cached
// layout internally (gradient excluded from the key — it affects color
// only). Use [TextSystem.LayoutTextCached] when the [Layout] itself is
// needed repeatedly. Call [TextSystem.Purge] after a full clear (e.g.
// terminal CSI 3 J) to drop the layout cache, glyph cache, and atlas pages
// without tearing down the [TextSystem].
//
// # Styling
//
// [TextConfig] controls rendering: [TextStyle] sets font, color, background
// highlight ([TextStyle.BgColor]), decorations (underline, strikethrough),
// stroke ([TextStyle.StrokeWidth], [TextStyle.StrokeColor]), and
// [TextStyle.LetterSpacing]. [TextStyle.Typeface] overrides weight/style
// programmatically. [BlockStyle] sets wrapping ([WrapNone], [WrapWord],
// [WrapChar], [WrapWordChar]), alignment, first-line indent (negative =
// hanging), line spacing, width, and tab stops. [TextMetrics.LineHeight]
// (ascent+descent+leading, floored to 1.15×em) is the recommended
// baseline-to-baseline advance for stacking lines.
//
//	cfg := glyph.TextConfig{
//	    Style: glyph.TextStyle{
//	        FontName:      "Sans 16",
//	        Typeface:      glyph.TypefaceBold,
//	        Color:         glyph.Color{R: 255, A: 255},
//	        BgColor:       glyph.Color{A: 255},
//	        Underline:     true,
//	        LetterSpacing: 2.0,
//	        StrokeWidth:   1.5,
//	        StrokeColor:   glyph.Color{A: 255},
//	    },
//	    Block: glyph.BlockStyle{
//	        Wrap:        glyph.WrapWord,
//	        Width:       400,
//	        Align:       glyph.AlignCenter,
//	        Indent:      20,
//	        LineSpacing: 4,
//	    },
//	    UseMarkup: false,
//	}
//
// OpenType features and variable-font axes ride on [FontFeatures]
// ([TextStyle.Features]); inline non-text elements use [InlineObject]
// ([TextStyle.Object]). [TextConfig.Orientation] selects horizontal or
// vertical (upright CJK) flow.
//
// # Gradients
//
//	cfg := glyph.TextConfig{
//	    Style: glyph.TextStyle{FontName: "Sans 28"},
//	    Gradient: &glyph.GradientConfig{
//	        Direction: glyph.GradientHorizontal, // or Vertical, Diagonal
//	        Stops: []glyph.GradientStop{
//	            {Color: glyph.Color{R: 255, A: 255}, Position: 0},
//	            {Color: glyph.Color{B: 255, A: 255}, Position: 1},
//	        },
//	    },
//	}
//	ts.DrawText(x, y, "Gradient text", cfg)
//
// # Rich text, markup, inline objects
//
// Render multiple styles in one layout:
//
//	rt := glyph.RichText{
//	    Runs: []glyph.StyleRun{
//	        {Text: "Bold ", Style: glyph.TextStyle{
//	            FontName: "Sans 16", Typeface: glyph.TypefaceBold,
//	            Color: glyph.Color{A: 255},
//	        }},
//	        {Text: "and italic", Style: glyph.TextStyle{
//	            FontName: "Sans 16", Typeface: glyph.TypefaceItalic,
//	            Color: glyph.Color{R: 200, A: 255},
//	        }},
//	    },
//	}
//	layout, _ := ts.LayoutRichText(rt, cfg)
//	ts.DrawLayout(layout, x, y)
//
// Pango markup ([TextConfig.UseMarkup]) covers <b>, <i>, <span> and friends.
// An [InlineObject] run reserves a box (width/height/baseline offset) the
// caller draws into; [Layout] carries its IDs for post-shape lookup.
//
// # Fonts
//
// System fonts are discovered per OS (macOS, Linux incl. XDG dirs, Windows
// incl. %SystemRoot% fallback, Android /system/fonts); results are cached
// per process. [TextSystem.AddFontFile] and [TextSystem.AddFontBytes]
// (go:embed-friendly; no-op on WASM, use the FontFace API there) register
// app fonts, including every face in a .ttc. [TextSystem.ResolveFontName]
// reports the family Pango resolution picks;
// [TextSystem.ListFontFamilies] enumerates registered families. Fallback
// faces open scaled by cap-height ratio (clamped, memoized) so CJK/icon
// glyphs match the primary size; locale-ordered CJK tiers follow
// LC_ALL/LC_CTYPE/LANG.
//
// # Emoji and box drawing
//
// Emoji take the color path only when Unicode marks default emoji
// presentation (or VS16 requests it); text-presentation symbols and VS15
// stay monochrome, preferring a text font over a color-emoji font that
// merely covers the codepoint. CBDT/sbix bitmaps and COLR v0 are decoded;
// [TextStyle.EmojiBoxWidth] scales emoji into a caller-reserved cell box
// (terminals). Box-drawing / block elements (U+2500–257F, U+2580–259F) and
// Powerline separators (U+E0B0–E0B3, when the font lacks them) bypass the
// font and rasterize procedurally at whole-pixel cell geometry so TUI
// frames abut without gaps; grid callers set [TextStyle.CellWidth]/
// [TextStyle.CellHeight]. Opt out with [TextStyle.NoBuiltinBoxGlyphs];
// stroked runs always use the font.
//
// # Layout queries
//
// All query methods operate on a pre-computed [Layout]:
//
//	layout, _ := ts.LayoutText(text, cfg)
//
//	idx := layout.HitTest(mouseX, mouseY)
//	rect, ok := layout.GetCharRect(idx)
//	cursor, ok := layout.GetCursorPos(idx)
//	rects := layout.GetSelectionRects(start, end)
//
//	next := layout.MoveCursorRight(idx)
//	prev := layout.MoveCursorLeft(idx)
//	up := layout.MoveCursorUp(idx, preferredX)
//	down := layout.MoveCursorDown(idx, preferredX)
//	wordL := layout.MoveCursorWordLeft(idx)
//	wordR := layout.MoveCursorWordRight(idx)
//
//	start, end := layout.GetWordAtIndex(idx)
//	pStart, pEnd := layout.GetParagraphAtIndex(idx, text)
//	name := layout.GetFontNameAtIndex(idx)
//
// Words are maximal runs of one rune class (whitespace, punctuation, word,
// Han, Hiragana, Katakana) with bidi L2 reordering and per-paragraph
// direction. [WordBoundsInString], [WordStartLeft], and [WordStartRight]
// apply the same rules to a plain string with no [Layout].
//
// # Text mutation and undo
//
// Package-level grapheme-aware editing helpers:
//
//	result := glyph.InsertText(text, cursor, "hello")
//	result = glyph.DeleteBackward(text, layout, cursor)
//	result = glyph.DeleteForward(text, layout, cursor)
//	result = glyph.DeleteSelection(text, cursor, anchor)
//	result = glyph.InsertReplacingSelection(text, cursor, anchor, "new")
//	selected := glyph.GetSelectedText(text, cursor, anchor)
//
// Undo/redo with time-based coalescing:
//
//	um := glyph.NewUndoManager(100)
//	um.RecordMutation(result, insertedText, cursorBefore, anchorBefore)
//	if undo := um.Undo(currentText); undo != nil { ... }
//	if redo := um.Redo(currentText); redo != nil { ... }
//
// # IME composition
//
// [CompositionState] tracks the preedit (byte offsets; UTF-16 bridges must
// convert first). Feed platform events into it, then render feedback after
// the layout:
//
//	ts.DrawLayout(layout, x, y)
//	ts.Renderer().DrawComposition(layout, x, y, &cs, cursorColor)
//
// [Renderer.DrawCompositionTransformed] pairs with transformed layouts so
// clause underlines and the preedit cursor stay on rotated glyphs. The
// [ime] sub-package provides the platform bridge (macOS NSTextInputClient,
// Linux IBus, stub elsewhere).
//
// # Transforms and placed glyphs
//
//	transform := glyph.AffineRotation(0.3).
//	    Multiply(glyph.AffineSkew(0.2, 0))
//	ts.DrawLayoutTransformed(layout, x, y, transform)
//
// For simple rotation:
//
//	ts.DrawLayoutRotated(layout, x, y, angleRadians)
//
// Helpers: [AffineIdentity], [AffineTranslation], [AffineScale],
// [AffineSkew], [AffineRotationAround], Inverse, IsIdentity, IsFinite.
// Non-finite transforms draw nothing. Backgrounds, decorations, and IME
// feedback rotate with glyphs when the backend implements
// [TransformedFillBackend] (all bundled backends do).
//
// Position each glyph independently (e.g. text on a path).
// DrawLayoutPlaced needs one placement per entry in layout.Glyphs, so size
// the slice to len(layout.Glyphs) and index it by GlyphInfo.Index — the
// glyph count is not the grapheme count (ligatures collapse clusters,
// marks add glyphs), and GlyphPositions omits unknown glyphs:
//
//	positions := layout.GlyphPositions()
//	placements := make([]glyph.GlyphPlacement, len(layout.Glyphs))
//	for i := range placements {
//	    placements[i] = glyph.GlyphPlacement{X: offX, Y: offY} // off-screen default
//	}
//	for _, g := range positions {
//	    placements[g.Index] = glyph.GlyphPlacement{
//	        X: pathX(g.X), Y: pathY(g.Y), Angle: pathAngle(g.X),
//	    }
//	}
//	ts.DrawLayoutPlaced(layout, placements)
//
// # Ink bounds
//
// [TextSystem.InkBounds] reports the box a run actually paints into, for
// centering a single glyph (icon, check mark) where the advance box
// (ascent+descent+bearings) would sit visibly off-center. It returns
// ok=false for vertical layouts or unmeasurable faces — fall back to the
// advance box.
//
// # DPI scale
//
// [TextSystem.SetDPIScale] re-points shaping and rasterization at a new
// density and purges scale-keyed caches; pair it with SetDPIScale on the
// backend (all bundled backends have it), which moves the quads. No-op on
// unchanged, non-positive, non-finite, or >10× values, so a resize handler
// may call it every frame. The web backend works at devicePixelRatio: pass
// it to web.New and size the canvas buffer accordingly.
//
// # Backends
//
// [DrawBackend] is the interface for plugging in a rendering framework
// (texture management + textured quads + filled rects). Optional
// extensions: [RectTextureUpdater] for mid-frame sub-rectangle uploads
// (required for OpenGL correctness — glDrawArrays samples immediately —
// and cheaper everywhere), [TransformedFillBackend] for rotated fills.
// Five backends are provided:
//   - [github.com/go-gui-org/go-glyph/backend/ebitengine]: Ebitengine integration
//     (separate Go module; import path unchanged).
//   - [github.com/go-gui-org/go-glyph/backend/gpu]: raw OpenGL 3.3 / Metal.
//   - [github.com/go-gui-org/go-glyph/backend/web]: HTML Canvas (WASM).
//   - [github.com/go-gui-org/go-glyph/backend/android]: Android GPU.
//   - [github.com/go-gui-org/go-glyph/backend/ios]: iOS Metal.
//
// See the sub-package documentation for usage details.
//
// # Thread Safety
//
// [Context], [Renderer], [TextSystem], [GlyphAtlas], and [UndoManager] are
// not safe for concurrent use. Call all glyph methods from the main/render
// goroutine. No locking is performed internally — this is a deliberate
// design choice for performance.
//
// # Sub-packages
//
//   - [github.com/go-gui-org/go-glyph/accessibility]: screen-reader tree management.
//   - [github.com/go-gui-org/go-glyph/ime]: IME bridge (macOS/Linux, stub elsewhere).
package glyph
