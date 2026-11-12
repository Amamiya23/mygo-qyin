package ui

import (
	"bytes"
	"image"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/text"
)

// Retaining paint metadata must preserve the original drawing path and its plain-foreground fast path.
func TestTextLayoutRetainedPaintPreservesPainting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spans []Span
	}{
		{"plain text", nil},
		{"unpainted spans", []Span{{Text: "of"}, {Text: "fice 漢字 😀", Weight: 700}}},
		{"transparent spans", []Span{{Text: "hidden ", colorSet: true}, {Text: "visible", Color: RGB(20, 100, 180).Alpha(0.4)}}},
		{"decorated spans", []Span{
			{Text: "shadow ", Shadow: NewTextShadow(1, 2, 2, RGB(20, 30, 40).Alpha(0.5))},
			{Text: "outline ", Color: RGB(90, 100, 150).Alpha(0.6), strokeWidth: 1, strokeColor: RGB(80, 30, 10), strokeSet: true},
			{Text: "marked السّلام e\u0301", WavyUnderline: true, Strikethrough: true, Background: RGB(220, 210, 180), PaddingX: 2, Radius: 2},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			font := Font{Size: 23, Weight: 600, LineHeight: 1.3}
			var layout *TextLayout
			if tc.spans == nil {
				layout = ShapeText("office 漢字 😀", font, 140)
			} else {
				layout = ShapeRichTextWith(tc.spans, font, TextShape{Width: 140, Align: Center})
			}
			if (layout.paint == nil) != (newSpanPaint(layout.spans) == nil) {
				t.Fatal("retained paint changed the plain-foreground fast path")
			}
			tt := NewTester(func(c *Context) {
				Box(c).Fill().Background(RGB(245, 246, 247)).Draw(func(p *Painter, r Rect) {
					layout.mu.Lock()
					defer layout.mu.Unlock()
					// This is the original TextLayout path, independently rebuilding
					// all span metadata rather than using the retained paint.
					p.textLayout(layout.l, r.X+10, r.Y+10, RGB(30, 35, 40),
						textStyle{size: layout.l.Params.Style.FontSize()}, newSpanPaint(layout.spans))
				})
			}, 280, 200)
			want := tt.Image()
			got := renderTextLayoutColors(layout, nil)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatal("retained paint differs from the original TextLayout path")
			}
		})
	}
}

// Color changes on retained text paint exactly like reshaping the same spans, including joining, bidi, wrapping and transparent words.
func TestTextLayoutColorsPreservesShapedPainting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spans []Span
		shape TextShape
	}{
		{"ligatures", []Span{{Text: "of"}, {Text: "fice "}, {Text: "affinity", Underline: true}}, TextShape{}},
		{"bidi", []Span{{Text: "الس"}, {Text: "لام "}, {Text: "中文 "}, {Text: "123"}}, TextShape{Direction: RightToLeft}},
		{"wrapped", []Span{{Text: "春日影 "}, {Text: "a longer "}, {Text: "sentence 👩‍👩‍👧‍👦"}, {Text: " e\u0301"}}, TextShape{Width: 100, Align: Center}},
		{"decorated", []Span{
			{Text: "shadow ", Shadow: NewTextShadow(1, 2, 2, RGB(20, 30, 40).Alpha(0.5))},
			{Text: "outline ", strokeWidth: 1, strokeColor: RGB(80, 30, 10), strokeSet: true},
			{Text: "marked ", WavyUnderline: true, Strikethrough: true, Background: RGB(220, 210, 180), PaddingX: 2, Radius: 2},
			{Text: "up", Size: 16, BaselineShift: 5},
		}, TextShape{Width: 220}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			font := Font{Size: 23, Weight: 600, LineHeight: 1.3}
			spans := slices.Clone(tc.spans)
			for i := range spans {
				spans[i].Color = RGB(50, 80, 120).Alpha(0.5)
			}
			retained := ShapeRichTextWith(spans, font, tc.shape)
			original := renderTextLayoutColors(retained, nil)
			geometry := retained.l
			for _, colors := range [][]Color{
				{RGB(220, 30, 70), RGB(30, 160, 60).Alpha(0.3), RGB(20, 90, 200).Alpha(0.8), RGB(40, 20, 180)},
				{Transparent, RGB(190, 20, 100).Alpha(0.6)},
				{RGB(30, 170, 90), Transparent, RGB(110, 40, 180), Transparent, RGB(255, 0, 0)},
			} {
				recolored := slices.Clone(spans)
				for i, color := range colors[:min(len(colors), len(recolored))] {
					recolored[i].Color, recolored[i].colorSet = color, true
				}
				fresh := ShapeRichTextWith(recolored, font, tc.shape)
				want := renderTextLayoutColors(fresh, nil)
				before := text.Shared().LayoutsMade()
				got := renderTextLayoutColors(retained, colors)
				if n := text.Shared().LayoutsMade() - before; n != 0 {
					t.Fatalf("color-only drawing shaped %d new layouts", n)
				}
				if !bytes.Equal(got.Pix, want.Pix) {
					t.Fatal("retained colors differ from freshly shaped colors")
				}
				if retained.l != geometry || !slices.Equal(retained.spans, spans) {
					t.Fatal("color-only drawing changed the retained layout")
				}
			}
			if got := renderTextLayoutColors(retained, nil); !bytes.Equal(got.Pix, original.Pix) {
				t.Fatal("drawing recolored text changed its original painting")
			}
		})
	}
}

func renderTextLayoutColors(layout *TextLayout, colors []Color) *image.RGBA {
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Background(RGB(245, 246, 247)).Draw(func(p *Painter, r Rect) {
			if colors == nil {
				p.TextLayout(layout, r.X+10, r.Y+10, RGB(30, 35, 40))
			} else {
				p.TextLayoutColors(layout, r.X+10, r.Y+10, RGB(30, 35, 40), colors)
			}
		})
	}, 280, 200)
	return tt.Image()
}
