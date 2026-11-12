package scene

import (
	"image"
	"slices"
	"testing"
)

func lyricDamageScene() *Scene {
	s := &Scene{Width: 300, Height: 100}
	for _, x := range []float32{20, 100, 180} {
		s.Glyphs = append(s.Glyphs, Glyph{X: x, Y: 20, W: 10, H: 20, UW: 10, VH: 20, Color: Color{R: 255, A: 120}})
	}
	s.Ops = []Op{{Kind: OpGlyphs, End: int32(len(s.Glyphs))}}
	return s
}

func rememberLyricDamage(t *testing.T, d *Differ, s *Scene) {
	t.Helper()
	if _, known := d.Diff(s); known {
		t.Fatal("the first frame unexpectedly has a preceding frame")
	}
	d.Remember(s)
}

func requireLyricDamage(t *testing.T, d *Differ, s *Scene, want ...image.Rectangle) {
	t.Helper()
	got, known := d.Diff(s)
	if !known || !slices.Equal(got, want) {
		t.Fatalf("damage = %v, known = %t; want %v", got, known, want)
	}
}

// A changing word used to damage the entire sentence, redrawing every static outline behind it.
func TestGlyphColorDamage(t *testing.T) {
	for _, alpha := range []uint8{0, 180, 255} {
		s := lyricDamageScene()
		d := Differ{Reach: 1}
		rememberLyricDamage(t, &d, s)
		s.Glyphs[1].Color.A = alpha
		requireLyricDamage(t, &d, s, image.Rect(99, 19, 111, 41))
		d.Remember(s)
		requireLyricDamage(t, &d, s)
	}
}

// Every changed glyph remains damaged when a seek changes several separated words at once.
func TestGlyphColorDamageMultipleWords(t *testing.T) {
	s := lyricDamageScene()
	d := Differ{Reach: 1}
	rememberLyricDamage(t, &d, s)
	s.Glyphs[0].Color.A, s.Glyphs[2].Color.A = 255, 0
	requireLyricDamage(t, &d, s, image.Rect(19, 19, 31, 41), image.Rect(179, 19, 191, 41))
}

// Refined damage includes the full atlas bitmap and respects enclosing clipping and both glyph transforms.
func TestGlyphColorDamageBounds(t *testing.T) {
	t.Run("bitmap size", func(t *testing.T) {
		s := lyricDamageScene()
		s.Glyphs[1].W, s.Glyphs[1].H = 5, 10
		d := Differ{Reach: 1}
		rememberLyricDamage(t, &d, s)
		s.Glyphs[1].Color.A = 255
		requireLyricDamage(t, &d, s, image.Rect(99, 19, 111, 41))
	})
	t.Run("clip", func(t *testing.T) {
		s := lyricDamageScene()
		s.Ops = []Op{{Kind: OpPushClip, Rect: Rect{X: 50, Y: 22, W: 120, H: 15}}, s.Ops[0], {Kind: OpPopClip}}
		d := Differ{Reach: 1}
		rememberLyricDamage(t, &d, s)
		s.Glyphs[1].Color.A = 255
		// opBounds conservatively grows the clip by Reach too.
		requireLyricDamage(t, &d, s, image.Rect(99, 21, 111, 38))
	})
	t.Run("transforms", func(t *testing.T) {
		s := lyricDamageScene()
		s.Ops[0].Transform = Translation(10, 5)
		s.Glyphs[1].Transform = Scaling(1.5, 1.5)
		d := Differ{Reach: 1}
		rememberLyricDamage(t, &d, s)
		s.Glyphs[1].Color.A = 255
		requireLyricDamage(t, &d, s, image.Rect(156, 31, 179, 69))
	})
}

// Any structural change keeps full operation damage, even after an earlier glyph changed only color.
func TestGlyphColorDamageFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Scene)
	}{
		{"position", func(s *Scene) { s.Glyphs[2].X++ }},
		{"atlas coordinates", func(s *Scene) { s.Glyphs[2].U++ }},
		{"glyph transform", func(s *Scene) { s.Glyphs[2].Transform = Translation(1, 0) }},
		{"glyph flags", func(s *Scene) { s.Glyphs[2].Subpixel = true }},
		{"operation transform", func(s *Scene) { s.Ops[0].Transform = Translation(1, 0) }},
		{"operation fields", func(s *Scene) { s.Ops[0].Color.A = 200 }},
		{"glyph count", func(s *Scene) { s.Ops[0].End-- }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := lyricDamageScene()
			d := Differ{Reach: 1}
			rememberLyricDamage(t, &d, s)
			old := d.bounds[0]
			s.Glyphs[0].Color.A = 255
			tc.change(s)
			got, known := d.Diff(s)
			if !known || len(got) != 1 || got[0] != old.Union(d.next[0]) {
				t.Fatalf("structural change refined damage: %v", got)
			}
		})
	}
}

// Atlas updates retain the full operation path rather than trusting unchanged texture coordinates.
func TestGlyphColorDamageAtlasFallback(t *testing.T) {
	for _, colored := range []bool{false, true} {
		s := lyricDamageScene()
		atlas := NewAtlas(1, 32, 32)
		if colored {
			s.ColorAtlas = atlas
		} else {
			s.MaskAtlas = atlas
		}
		d := Differ{Reach: 1}
		rememberLyricDamage(t, &d, s)
		atlas.Put(0, 0, 1, 1, []byte{255}, 1)
		s.Glyphs[1].Color.A = 255
		requireLyricDamage(t, &d, s, image.Rect(19, 19, 191, 41))
	}
}

// Wide renderers damage changed wide colors even when their nearest sRGB values stay identical.
func TestGlyphColorDamageWideColors(t *testing.T) {
	s := lyricDamageScene()
	s.Glyphs[1].Wide = 1
	s.Wide = []WideColors{{Color: [4]float32{1.2, 0.3, 0.4, 1}, Set: WideColor}}
	d := Differ{Reach: 1, Wide: true}
	rememberLyricDamage(t, &d, s)
	s.Wide[0].Color[0] = 1.5
	requireLyricDamage(t, &d, s, image.Rect(99, 19, 111, 41))
	d.Remember(s)
	s.Wide = append(s.Wide, s.Wide[0])
	s.Glyphs[1].Wide = 2
	requireLyricDamage(t, &d, s)
}
