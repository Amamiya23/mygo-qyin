package gpu

import (
	"bytes"
	"image"
	"testing"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// Growing a rounded clip used to replace its whole pixel buffer and GPU texture every frame.
func TestAnimatedClipMaskReusesTextureSize(t *testing.T) {
	b := clipMemoryBuilder()
	var lastW, lastH, resizes int
	var id uint64
	for frame := range 128 {
		b.frame++
		c := clipMemoryShape(Scissor{-13, -9, int32(100 + frame), int32(80 + frame/2)}, 0)
		b.clipShapes = []scene.ClipShape{c.shape}
		in := Instance{Rect: [4]float32{-20, -20, 300, 200}}
		if !b.prepare(&in, scene.Affine{}, &c) {
			t.Fatal("rounded clip discarded its content")
		}
		if len(b.maskSlots) != 1 {
			t.Fatalf("frame %d: retained %d masks for one animated clip", frame, len(b.maskSlots))
		}
		mask := &b.maskSlots[0]
		if frame == 0 {
			id = mask.image.ID()
		}
		if mask.image.ID() != id {
			t.Fatalf("frame %d: changed the animated mask's texture identity", frame)
		}
		if mask.image.W != lastW || mask.image.H != lastH {
			resizes++
			lastW, lastH = mask.image.W, mask.image.H
		}
		wantClip := clipMemoryRect(mask.area)
		if in.Clip != wantClip || in.Transform1[3] != 1 {
			t.Fatalf("frame %d: mask sampling area %v mode %v, want %v mode 1", frame, in.Clip, in.Transform1[3], wantClip)
		}
		clipMemoryCoverage(t, mask, b.clipShapes, c.scissor)
	}
	if resizes > 6 {
		t.Fatalf("128 one-pixel clip changes resized the texture %d times", resizes)
	}
}

// Mask growth used to allocate exact-size buffers, retaining hundreds of megabytes of discarded pixels during an opening.
func TestClipMaskBufferGrowthIsAmortized(t *testing.T) {
	var mask *scene.Image
	var previous, allocations int
	for width := 32; width < 288; width++ {
		const height = 20
		shape := scene.ClipShape{Rect: scene.Rect{W: float32(width / 2), H: height}}
		mask = raster.ClipMask(mask, []scene.ClipShape{shape}, image.Rect(0, 0, width, height))
		if cap(mask.Pix) != previous {
			allocations++
			previous = cap(mask.Pix)
		}
		if len(mask.Pix) != 4*width*height {
			t.Fatalf("width %d: pixel buffer length %d", width, len(mask.Pix))
		}
		for y := range height {
			for x := range width {
				want := byte(0)
				if x < width/2 {
					want = 255
				}
				at := 4 * (y*width + x)
				if mask.Pix[at] != 0 || mask.Pix[at+1] != 0 || mask.Pix[at+2] != 0 || mask.Pix[at+3] != want {
					t.Fatalf("width %d: pixel (%d,%d) has RGBA %v, want [0 0 0 %d]", width, x, y, mask.Pix[at:at+4], want)
				}
			}
		}
	}
	if allocations > 10 {
		t.Fatalf("256 one-pixel size changes allocated %d backing buffers", allocations)
	}
}

// An inner hard clip must sample its rounded parent's padded mask without stretching it.
func TestNestedClipSamplesRetainedMaskArea(t *testing.T) {
	b := clipMemoryBuilder()
	b.frame = 1
	parent := clipMemoryShape(Scissor{-5, -7, 101, 99}, 0)
	child := clip{
		rect:    scene.Rect{X: 2, Y: 3, W: 80, H: 70},
		scissor: Scissor{13, 10, 93, 80}, masked: true, moved: true, maskSlot: 1,
		shape: scene.ClipShape{Rect: scene.Rect{X: 2, Y: 3, W: 80, H: 70}, Transform: scene.Translation(11, 7)},
	}
	b.stack = []clip{parent, child}
	b.clipShapes = []scene.ClipShape{parent.shape, child.shape}
	in := Instance{Rect: [4]float32{0, 0, 100, 100}}
	if !b.prepare(&in, scene.Affine{}, &b.stack[1]) {
		t.Fatal("nested clip discarded its content")
	}
	if len(b.maskSlots) != 1 {
		t.Fatalf("inner hard clip made %d masks instead of sharing its parent", len(b.maskSlots))
	}
	mask := &b.maskSlots[0]
	if in.ClipRadii != clipMemoryRect(mask.area) || in.Transform1[3] != 2 {
		t.Fatalf("parent mask sampling area %v mode %v, want %v mode 2", in.ClipRadii, in.Transform1[3], clipMemoryRect(mask.area))
	}
	if in.Clip != [4]float32{13, 10, 80, 70} || b.stack[1].scissor != child.scissor {
		t.Fatalf("padding changed the inner hard clip: shader %v scissor %v", in.Clip, b.stack[1].scissor)
	}
	clipMemoryCoverage(t, mask, b.clipShapes[:1], parent.scissor)
}

// Shrinking a mask used to hide its retained backing buffer from the idle-cache byte budget.
func TestIdleClipMaskBudgetCountsCapacity(t *testing.T) {
	img := scene.NewImageRGBA(128, 128, make([]byte, 4*128*128))
	img.Resize(4, 4)
	b := Builder{frame: 2, maskSlots: []clipMaskSlot{{frame: 1, image: img}}}
	b.dropIdleMasks(cap(img.Pix) - 1)
	if len(b.maskSlots) != 0 {
		t.Fatalf("kept a %d-byte backing buffer under a %d-byte budget after shrinking to %d bytes", cap(img.Pix), cap(img.Pix)-1, len(img.Pix))
	}
	current := scene.NewImageRGBA(2, 2, make([]byte, 16))
	b.maskSlots = []clipMaskSlot{{frame: 1, image: img}, {frame: 2, image: current}}
	b.dropIdleMasks(0)
	if len(b.maskSlots) != 1 || b.maskSlots[0].image != current {
		t.Fatal("idle eviction dropped the current frame's mask or retained an idle one")
	}
}

// Reusing every idle slot would repaint a stationary clip whenever another branch briefly appeared.
func TestReturningClipKeepsCachedMask(t *testing.T) {
	b := clipMemoryBuilder()
	b.frame = 1
	first := clipMemoryShape(Scissor{0, 0, 90, 90}, 0)
	b.clipTexture(&first, []scene.ClipShape{first.shape})
	if len(b.maskSlots) != 1 {
		t.Fatal("first clip has no retained mask")
	}
	mask := b.maskSlots[0].image
	id, version := mask.ID(), mask.Version()
	for range 5 {
		b.frame += 2 // Hidden for a frame before another branch needs a mask.
		b.dropIdleMasks(idleMaskBytes)
		other := clipMemoryShape(Scissor{100, 0, 180, 80}, 0)
		b.clipTexture(&other, []scene.ClipShape{other.shape})
		b.frame++
		returned := clipMemoryShape(first.scissor, 0)
		texture := b.clipTexture(&returned, []scene.ClipShape{returned.shape})
		if len(b.maskSlots) != 2 || texture != uintptr(id) || mask.Version() != version {
			t.Fatal("returning clip lost its cached mask when another branch appeared")
		}
	}

	kept := append([]byte(nil), b.maskSlots[0].image.Pix...)
	second := clipMemoryShape(Scissor{20, 20, 100, 100}, 1)
	b.clipTexture(&second, []scene.ClipShape{second.shape})
	if !bytes.Equal(kept, b.maskSlots[0].image.Pix) || mask.ID() != id || mask.Version() != version {
		t.Fatal("another clip overwrote a mask already used in the current frame")
	}
}

// Under budget pressure, opening another clip used to add another texture instead of reusing idle storage.
func TestClipMaskReusesIdleStorageUnderPressure(t *testing.T) {
	b := clipMemoryBuilder()
	b.frame = 5
	current := clipMemoryShape(Scissor{0, 0, 40, 40}, 0)
	b.clipTexture(&current, []scene.ClipShape{current.shape})
	used := b.maskSlots[0].image
	kept := append([]byte(nil), used.Pix...)
	// Simulate a tiny mask with a full-budget retained backing buffer; only
	// the small requested area is painted, keeping this a cheap CPU test.
	idle := scene.NewImageRGBA(1, 1, make([]byte, 4, idleMaskBytes))
	b.maskSlots = append(b.maskSlots, clipMaskSlot{frame: 1, image: idle, area: image.Rect(0, 0, 1, 1)})
	second := clipMemoryShape(Scissor{40, 0, 100, 60}, 1)
	texture := b.clipTexture(&second, []scene.ClipShape{second.shape})
	if len(b.maskSlots) != 2 || texture != uintptr(idle.ID()) {
		t.Fatal("budget pressure allocated another mask instead of reusing idle storage")
	}
	if b.maskSlots[0].image != used || !bytes.Equal(kept, used.Pix) {
		t.Fatal("budget pressure overwrote a mask already used by the current frame")
	}
	clipMemoryCoverage(t, &b.maskSlots[1], []scene.ClipShape{second.shape}, second.scissor)
}

// Shrinking then reopening an animated clip used to rebuild a sequence of differently sized GPU textures.
func TestAnimatedClipMaskKeepsSizeAfterShrink(t *testing.T) {
	b := clipMemoryBuilder()
	b.stack = []clip{{scissor: Scissor{0, 0, 256, 256}}}
	var fullW, fullH int
	for frame, size := range []int32{90, 150, 250, 20, 130, 250} {
		b.frame++
		c := clipMemoryShape(Scissor{0, 0, size, size}, 0)
		b.clipTexture(&c, []scene.ClipShape{c.shape})
		if len(b.maskSlots) != 1 {
			t.Fatal("clip size changes accumulated retained masks")
		}
		mask := &b.maskSlots[0]
		if mask.image.W > 256 || mask.image.H > 256 {
			t.Fatal("growth slack exceeded the viewport")
		}
		if frame == 2 {
			fullW, fullH = mask.image.W, mask.image.H
		}
		if frame > 2 && (mask.image.W != fullW || mask.image.H != fullH) {
			t.Fatal("shrinking and reopening changed the retained texture's size")
		}
		clipMemoryCoverage(t, mask, []scene.ClipShape{c.shape}, c.scissor)
	}
}

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
				b := Builder{NoCull: true, NoFold: true, NoCornerSplit: true}
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
			b := Builder{NoCull: true, NoFold: true, NoCornerSplit: true}
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

func clipMemoryBuilder() *Builder {
	return &Builder{imageTexture: func(img *scene.Image) uintptr { return uintptr(img.ID()) }}
}

func clipMemoryShape(sc Scissor, slot int) clip {
	r := scene.Rect{X: float32(sc.Left), Y: float32(sc.Top), W: float32(sc.Right - sc.Left), H: float32(sc.Bottom - sc.Top)}
	radii := [4]float32{17, 17, 17, 17}
	return clip{rect: r, radii: radii, scissor: sc, shape: scene.ClipShape{Rect: r, Radii: radii, Transform: scene.Rotation(0.25)}, moved: true, masked: true, maskSlot: slot}
}

func clipMemoryRect(area image.Rectangle) [4]float32 {
	return [4]float32{float32(area.Min.X), float32(area.Min.Y), float32(area.Dx()), float32(area.Dy())}
}

func clipMemoryCoverage(t *testing.T, mask *clipMaskSlot, shapes []scene.ClipShape, sc Scissor) {
	t.Helper()
	exact := image.Rect(int(sc.Left), int(sc.Top), int(sc.Right), int(sc.Bottom))
	if !exact.In(mask.area) || mask.image.W != mask.area.Dx() || mask.image.H != mask.area.Dy() {
		t.Fatalf("mask area %v (%dx%d) does not contain clip %v", mask.area, mask.image.W, mask.image.H, exact)
	}
	want := raster.ClipMask(nil, shapes, exact)
	for y := exact.Min.Y; y < exact.Max.Y; y++ {
		gotAt := 4 * ((y-mask.area.Min.Y)*mask.image.W + exact.Min.X - mask.area.Min.X)
		wantAt := 4 * (y - exact.Min.Y) * want.W
		if !bytes.Equal(mask.image.Pix[gotAt:gotAt+4*want.W], want.Pix[wantAt:wantAt+4*want.W]) {
			t.Fatalf("padded mask changes clip coverage on row %d", y)
		}
	}
}
