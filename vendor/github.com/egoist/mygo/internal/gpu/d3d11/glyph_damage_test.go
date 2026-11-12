//go:build windows

package d3d11

import (
	"bytes"
	"math"
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// Word-color updates used to dirty the whole sentence; their smaller redraw must preserve all outlined, clipped and transformed pixels.
func TestGlyphColorPartialRedrawMatchesFull(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, tc := range []struct {
		name      string
		transform scene.Affine
		clip      bool
	}{
		{name: "fractional"},
		{name: "scaled-clipped", transform: scene.Scaling(1.05, 1.1), clip: true},
		{name: "rotated-clipped", transform: scene.Rotation(3), clip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			partial, full := glyphDamageRenderer(t), glyphDamageRenderer(t)
			defer partial.Release()
			defer full.Release()
			if partial.ctx1 == 0 {
				t.Skip("no Direct3D 11.1 partial redraw")
			}
			atlas := glyphDamageAtlas()
			for frame := 0; frame < 6; frame++ {
				s := glyphDamageScene(atlas, tc.transform, tc.clip, frame)
				if frame > 0 {
					partial.held = true
				}
				full.held = false
				if drawn, err := partial.drawFrame(s); err != nil || !drawn {
					t.Fatalf("partial frame %d: drawn=%v error=%v", frame, drawn, err)
				}
				if drawn, err := full.drawFrame(s); err != nil || !drawn {
					t.Fatalf("full frame %d: drawn=%v error=%v", frame, drawn, err)
				}
				// The first two updates change one word's foreground only. The
				// original whole-op invalidation also matched full pixels, so
				// require that this regression exercises the smaller redraw.
				if frame == 1 || frame == 2 {
					if partial.redrew != redrewPart {
						t.Fatalf("frame %d did not partially redraw", frame)
					}
					area := 0
					for _, rect := range partial.part.Rects {
						area += int((rect.Right - rect.Left) * (rect.Bottom - rect.Top))
					}
					if area >= 2000 {
						t.Fatalf("frame %d redrew %d pixels for one word", frame, area)
					}
				}
				got, gs, err := partial.read()
				if err != nil {
					t.Fatal(err)
				}
				want, ws, err := full.read()
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < s.Height; y++ {
					if !bytes.Equal(got[y*gs:y*gs+s.Width*4], want[y*ws:y*ws+s.Width*4]) {
						t.Fatalf("frame %d row %d: partial redraw changed full-render pixels", frame, y)
					}
				}
			}
		})
	}
}

func glyphDamageRenderer(t *testing.T) *Renderer {
	t.Helper()
	hwnd, err := newHiddenWindow(320, 128)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { destroyWindow(hwnd) })
	r, err := New(hwnd)
	if err != nil {
		t.Skip("Direct3D 11 unavailable:", err)
	}
	return r
}

func glyphDamageAtlas() *scene.Atlas {
	a := scene.NewAtlas(1, 32, 32)
	pix := make([]byte, 12*18)
	for y := 0; y < 18; y++ {
		for x := 0; x < 12; x++ {
			if x >= 1 && x <= 10 && y >= 1 && y <= 16 {
				pix[y*12+x] = 230
				if x == 1 || x == 10 || y == 1 || y == 16 {
					pix[y*12+x] = 105
				}
			}
		}
	}
	a.Put(0, 0, 12, 18, pix, 12)
	return a
}

func glyphDamageScene(atlas *scene.Atlas, transform scene.Affine, clipped bool, frame int) *scene.Scene {
	s := &scene.Scene{Width: 320, Height: 128, MaskAtlas: atlas}
	// A translucent, colored backing catches incorrect clears beneath text.
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 5, Y: 5, W: 300, H: 110}, Color: scene.Color{R: 30, G: 70, B: 95, A: 80}})
	if clipped {
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 27.25, Y: 24.5, W: 222.5, H: 47.25}})
	}
	addLine := func(dx, dy float32, color scene.Color, foreground bool) {
		start := len(s.Glyphs)
		for i := 0; i < 8; i++ {
			g := scene.Glyph{X: 18.25 + float32(i)*30 + dx, Y: 32.5 + dy, W: 12, H: 18, UW: 12, VH: 18, Color: color}
			if i == 4 {
				g.Transform = scene.Translation(0.25, -0.5)
			}
			if foreground {
				if frame >= 1 && i == 3 {
					g.Color = scene.Color{R: 230, G: 180, B: 20, A: 180}
				}
				if frame >= 2 && i == 3 {
					g.Color.A = 230
				}
				if frame >= 3 && i == 3 {
					g.Color.A = 0
				}
				// Whole-line fades and a real geometry change retain their
				// conservative fallback while matching a complete redraw.
				if frame >= 4 {
					g.Color.A /= 2
				}
				if frame >= 5 && i == 5 {
					g.X += 3.75
				}
			}
			s.Glyphs = append(s.Glyphs, g)
		}
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpGlyphs, Start: int32(start), End: int32(len(s.Glyphs)), Transform: transform})
	}
	addLine(0, 2, scene.Color{A: 100}, false)
	for i := 0; i < 8; i++ {
		angle := float64(i) * math.Pi / 4
		addLine(float32(math.Cos(angle))*0.8, float32(math.Sin(angle))*0.8, scene.Color{A: 240}, false)
	}
	addLine(0, 0, scene.Color{R: 100, G: 220, B: 200, A: 122}, true)
	if clipped {
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
	}
	return s
}
