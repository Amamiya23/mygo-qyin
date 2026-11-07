package gpu

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// Transformed content inside a stationary rounded card used to allocate a whole-card mask on every animation frame.
func TestStationaryRoundedClipUsesFrameCoordinates(t *testing.T) {
	transforms := []struct {
		name string
		m    scene.Affine
	}{
		{"scale", scene.Scaling(0.85, 0.9)},
		{"rotate", scene.Translation(30, 25).Mul(scene.Rotation(12))},
		{"perspective", scene.Projective(1, 0, 0, 1, 20, 15, 0.001, 0.0004)},
	}
	for _, transform := range transforms {
		for _, inner := range []bool{false, true} {
			name := transform.name
			if inner {
				name += "/integer-child"
			}
			t.Run(name, func(t *testing.T) {
				outer := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 256, H: 256}}
				card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 10, Y: 10, W: 200, H: 150}, Radii: [4]float32{17, 17, 17, 17}, Continuous: true}
				ops := []scene.Op{outer, card}
				wantScissor := Scissor{10, 10, 210, 160}
				if inner {
					ops = append(ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 120, H: 100}})
					wantScissor = Scissor{20, 20, 140, 120}
				}
				ops = append(ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 20, Y: 20, W: 100, H: 80}, Color: scene.Color{R: 255, A: 255}, Transform: transform.m})
				if inner {
					ops = append(ops, scene.Op{Kind: scene.OpPopClip})
				}
				ops = append(ops, scene.Op{Kind: scene.OpPopClip}, scene.Op{Kind: scene.OpPopClip})
				b := Builder{NoCull: true, NoFold: true}
				defer b.Release()
				uploads := 0
				b.Build(&scene.Scene{Width: 256, Height: 256, Ops: ops}, func(img *scene.Image) uintptr { uploads++; return uintptr(img.ID()) })
				if uploads != 0 || len(b.maskSlots) != 0 || len(b.Instances) == 0 {
					t.Fatalf("stationary card uploaded %d masks, retained %d, drew %d instances", uploads, len(b.maskSlots), len(b.Instances))
				}
				wantClip := [4]float32{10, 10, 200, 150}
				wantRadii := scene.Corners(card.Rect, card.Radii, card.Continuous)
				for _, in := range b.Instances {
					if in.Transform1[3] != 0 || in.Clip != wantClip || in.ClipRadii != wantRadii {
						t.Fatalf("analytic clip mode %v rect %v radii %v, want 0 %v %v", in.Transform1[3], in.Clip, in.ClipRadii, wantClip, wantRadii)
					}
				}
				for _, batch := range b.Batches {
					if batch.ClipMask != 0 || batch.Scissor != wantScissor {
						t.Fatalf("analytic card lost exact hard scissor: mask %v scissor %v, want %v", batch.ClipMask, batch.Scissor, wantScissor)
					}
				}
			})
		}
	}
}

// Analytic single-card clipping must retain pixel masks when clips need combined coverage or separate coordinate systems.
func TestComplexClipRetainsCoverageMask(t *testing.T) {
	for _, name := range []string{"fractional-child", "fractional-ancestor", "multiple-rounded", "transformed-clip", "gradient-mask", "image-mask"} {
		t.Run(name, func(t *testing.T) {
			outer := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 256, H: 256}}
			card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 10, Y: 10, W: 200, H: 150}, Radii: [4]float32{17, 17, 17, 17}}
			child := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 120, H: 100}}
			switch name {
			case "fractional-child":
				child.Rect.X += 0.25
			case "fractional-ancestor":
				outer.Rect.X = 0.25
			case "multiple-rounded":
				child.Radii = [4]float32{8, 8, 8, 8}
			case "transformed-clip":
				child.Transform = scene.Rotation(5)
			case "gradient-mask":
				card.Paint, card.Color, card.Color2, card.Gradient = scene.PaintLinear, scene.Color{A: 0}, scene.Color{A: 255}, [4]float32{10, 10, 210, 10}
			case "image-mask":
				card.Image = scene.NewImageRGBA(2, 1, []byte{0, 0, 0, 128, 0, 0, 0, 255})
				card.Src = scene.Rect{W: 2, H: 1}
			}
			fill := scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 20, Y: 20, W: 100, H: 80}, Color: scene.Color{R: 255, A: 255}, Transform: scene.Scaling(0.85, 0.9)}
			ops := []scene.Op{outer, card, child, fill, {Kind: scene.OpPopClip}, {Kind: scene.OpPopClip}, {Kind: scene.OpPopClip}}
			b := Builder{NoCull: true, NoFold: true}
			defer b.Release()
			uploads := 0
			b.Build(&scene.Scene{Width: 256, Height: 256, Ops: ops}, func(img *scene.Image) uintptr { uploads++; return uintptr(img.ID()) })
			if uploads == 0 || len(b.maskSlots) == 0 || len(b.Instances) == 0 {
				t.Fatalf("complex clip lost its mask: uploads %d retained %d instances %d", uploads, len(b.maskSlots), len(b.Instances))
			}
			for _, in := range b.Instances {
				if in.Transform1[3] != 1 && in.Transform1[3] != 2 {
					t.Fatalf("complex clip uses analytic mode %v", in.Transform1[3])
				}
			}
		})
	}
}
