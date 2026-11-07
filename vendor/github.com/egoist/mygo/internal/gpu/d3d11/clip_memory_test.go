//go:build windows

package d3d11

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// An opening animation used to leave its clip-mask textures cached for hundreds of idle frames.
func TestClipMaskTextureRetiresAfterAnimation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd, err := newHiddenWindow(192, 160)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyWindow(hwnd)
	r, err := New(hwnd)
	if err != nil {
		t.Skip("Direct3D 11 unavailable:", err)
	}
	defer r.Release()
	cover := scene.NewImageRGBA(1, 1, []byte{255, 255, 255, 255})
	s := &scene.Scene{Width: 192, Height: 160, Clear: scene.Color{A: 255}}
	ordinary := scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 180, Y: 150, W: 4, H: 4}, Image: cover, Src: scene.Rect{W: 1, H: 1}}
	for frame := range 6 {
		clipped := frame%2 == 0
		s.Ops = []scene.Op{ordinary}
		if clipped {
			s.Ops = append(s.Ops,
				scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 13, Y: 11, W: 133, H: 117}, Radii: [4]float32{19, 19, 19, 19}, Transform: scene.Rotation(3)},
				scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 190, H: 150}, Color: scene.Color{R: 255, G: 255, B: 255, A: 255}, Transform: scene.Scaling(1.01, 1.01)},
				scene.Op{Kind: scene.OpPopClip})
		}
		if err := r.draw(s); err != nil {
			t.Fatal(err)
		}
		masks := 0
		for _, tex := range r.images {
			if tex.transient {
				masks++
			}
		}
		if clipped && masks != 1 {
			t.Fatalf("frame %d: %d mask textures after clipped=%t", frame, masks, clipped)
		}
		if r.images[cover.ID()] == nil {
			t.Fatal("ordinary cover texture was evicted with a temporary mask")
		}
		if clipped {
			pix, stride, err := r.read()
			if err != nil {
				t.Fatal(err)
			}
			ref := raster.NewImage(s.Width, s.Height)
			raster.Render(ref, s)
			for y := range s.Height {
				for x := range s.Width {
					// This grayscale fixture uses equal RGB channels in BGRA and RGBA.
					got, want := int(pix[y*stride+4*x]), int(ref.Pix[y*ref.Stride+4*x])
					if d := got - want; d < -2 || d > 2 {
						t.Fatalf("frame %d: mask coverage at %d,%d is %d, CPU %d", frame, x, y, got, want)
					}
				}
			}
		}
		if err := r.present(0); err != nil {
			t.Fatal(err)
		}
		if !clipped {
			for _, tex := range r.images {
				if tex.transient {
					t.Fatal("unused mask survived presentation")
				}
			}
		}
	}
}

// A stationary rounded card can clip transformed content directly, without a whole-window mask.
func TestRoundedCardClipsTransformedContentWithoutMask(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd, err := newHiddenWindow(192, 160)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyWindow(hwnd)
	r, err := New(hwnd)
	if err != nil {
		t.Skip("Direct3D 11 unavailable:", err)
	}
	defer r.Release()
	for _, inner := range []bool{false, true} {
		for _, transform := range []scene.Affine{scene.Scaling(1.01, 1.01), scene.Rotation(11)} {
			s := &scene.Scene{Width: 192, Height: 160, Clear: scene.Color{A: 255}}
			s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 13, Y: 11, W: 133, H: 117}, Radii: [4]float32{19, 19, 19, 19}})
			if inner {
				s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 18, W: 110, H: 100}})
			}
			s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: -200, Y: -200, W: 600, H: 600}, Color: scene.Color{R: 255, G: 255, B: 255, A: 255}, Transform: transform}, scene.Op{Kind: scene.OpPopClip})
			if inner {
				s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
			}
			if err := r.draw(s); err != nil {
				t.Fatal(err)
			}
			for _, tex := range r.images {
				if tex.transient {
					t.Fatal("stationary rounded clip still uploaded a pixel mask")
				}
			}
			pix, stride, err := r.read()
			if err != nil {
				t.Fatal(err)
			}
			ref := raster.NewImage(s.Width, s.Height)
			raster.Render(ref, s)
			maxDiff, totalDiff := 0, 0
			for y := range s.Height {
				for x := range s.Width {
					d := int(pix[y*stride+4*x]) - int(ref.Pix[y*ref.Stride+4*x])
					if d < 0 {
						d = -d
					}
					maxDiff = max(maxDiff, d)
					totalDiff += d
				}
			}
			if maxDiff > 20 || float64(totalDiff)/float64(s.Width*s.Height) > 0.2 {
				t.Fatalf("inner=%t: rounded coverage differs from CPU: max=%d mean=%f", inner, maxDiff, float64(totalDiff)/float64(s.Width*s.Height))
			}
		}
	}
}
