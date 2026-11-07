package gpu

import (
	"bytes"
	"image"
	"testing"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

func reusableMaskScene(width, height int, radius float32) *scene.Scene {
	return &scene.Scene{Width: width + 20, Height: height + 20, Ops: []scene.Op{
		{Kind: scene.OpPushClip, Rect: scene.Rect{X: 4, Y: 4, W: float32(width), H: float32(height)}, Radii: [4]float32{radius, radius, radius, radius}, Transform: scene.Rotation(0.01)},
		{Kind: scene.OpFill, Rect: scene.Rect{X: 10, Y: 10, W: 20, H: 20}, Color: scene.Color{A: 255}},
		{Kind: scene.OpPopClip},
	}}
}

func fixedMaskScene(width, height int, radius float32) *scene.Scene {
	s := reusableMaskScene(width, height, radius)
	s.Ops[0].Transform = scene.Affine{}
	s.Ops[1].Transform = scene.Scaling(1.05, 1.05)
	// Distinct overlapping rounded corners need their combined coverage;
	// neither clip is redundant within the other's scissor.
	s.Ops = append([]scene.Op{{Kind: scene.OpPushClip, Rect: scene.Rect{X: 0.25, Y: 0.25, W: float32(s.Width), H: float32(s.Height)}, Radii: [4]float32{28, 28, 28, 28}}}, s.Ops...)
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
	return s
}

// Reopening an animated clip after idle frames used to allocate another large mask instead of reusing the idle buffer.
func TestReopenedClipReusesIdleMaskUnderPressure(t *testing.T) {
	var b Builder
	defer b.Release()
	var mask *scene.Image
	upload := func(m *scene.Image) uintptr { mask = m; return uintptr(m.ID()) }
	b.Build(reusableMaskScene(3000, 2100, 12), upload)
	if mask == nil {
		t.Fatal("transformed clip did not upload a mask")
	}
	id, pixels, version := mask.ID(), &mask.Pix[0], mask.Version()
	for i := range 4 {
		for range 3 {
			b.Build(&scene.Scene{Width: 3020, Height: 2120}, upload)
		}
		b.Build(reusableMaskScene(3000, 2100, float32(14+i)), upload)
		if mask.ID() != id || &mask.Pix[0] != pixels || len(b.maskSlots) != 1 {
			t.Fatalf("reopening %d allocated another mask: id %d, slots %d", i, mask.ID(), len(b.maskSlots))
		}
		if mask.Version() == version {
			t.Fatal("reused mask did not update its coverage")
		}
		version = mask.Version()
	}
}

// Reusing an idle mask must never overwrite pixels or textures already referenced by an earlier batch of the same frame.
func TestIdleMaskReusePreservesCurrentFrame(t *testing.T) {
	var b Builder
	defer b.Release()
	upload := func(m *scene.Image) uintptr { return uintptr(m.ID()) }
	b.Build(reusableMaskScene(3000, 2100, 12), upload)
	b.Build(&scene.Scene{Width: 3020, Height: 2120}, upload)
	s := reusableMaskScene(3000, 2100, 18)
	s.Ops = append(s.Ops, reusableMaskScene(3000, 2100, 40).Ops...)
	var first *scene.Image
	var before []byte
	var ids []uint64
	b.Build(s, func(m *scene.Image) uintptr {
		ids = append(ids, m.ID())
		if first == nil {
			first, before = m, bytes.Clone(m.Pix)
		}
		return uintptr(m.ID())
	})
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("independent clips shared a current-frame mask: %v", ids)
	}
	if !bytes.Equal(first.Pix, before) {
		t.Fatal("a later clip overwrote the earlier batch's pixels")
	}
	if b.Batches[0].ClipMask != uintptr(ids[0]) || b.Batches[len(b.Batches)-1].ClipMask != uintptr(ids[1]) {
		t.Fatal("batches lost their independent clip textures")
	}
}

// Small clips that alternate between branches keep their cached coverage instead of rasterizing it again on every return.
func TestIdleMaskReuseRetainsCachedBranches(t *testing.T) {
	var b Builder
	defer b.Release()
	a, other := reusableMaskScene(120, 80, 12), reusableMaskScene(120, 80, 18)
	versions := map[uint64]uint64{}
	rasterized := 0
	upload := func(m *scene.Image) uintptr {
		if version, ok := versions[m.ID()]; !ok || version != m.Version() {
			rasterized++
			versions[m.ID()] = m.Version()
		}
		return uintptr(m.ID())
	}
	for range 5 {
		b.Build(a, upload)
		b.Build(&scene.Scene{Width: 140, Height: 100}, upload)
		b.Build(other, upload)
	}
	if rasterized != 2 {
		t.Fatalf("returning branches rasterized %d masks, want 2", rasterized)
	}
}

// Clip-mask textures must be marked temporary so their GPU storage can be freed as soon as their clips leave the frame.
func TestClipMaskTextureIsTransient(t *testing.T) {
	var b Builder
	defer b.Release()
	var mask *scene.Image
	b.Build(reusableMaskScene(120, 80, 12), func(m *scene.Image) uintptr { mask = m; return uintptr(m.ID()) })
	if mask == nil || !mask.Transient {
		t.Fatal("clip-mask texture does not have a temporary lifetime")
	}
}

// A closing card used to shrink its texture repeatedly and allocate it again on the next opening in the same viewport.
func TestClipMaskBufferStaysSizedWithinViewport(t *testing.T) {
	var b Builder
	defer b.Release()
	var mask *scene.Image
	upload := func(m *scene.Image) uintptr { mask = m; return uintptr(m.ID()) }
	var size [2]int
	for i, dimensions := range [][2]int{{600, 400}, {160, 100}, {560, 360}, {64, 48}, {600, 400}} {
		s := fixedMaskScene(dimensions[0], dimensions[1], 12)
		s.Width, s.Height = 640, 480
		b.Build(s, upload)
		if mask == nil {
			t.Fatal("transformed card did not upload a mask")
		}
		if i == 0 {
			size = [2]int{mask.W, mask.H}
		} else if [2]int{mask.W, mask.H} != size {
			t.Fatalf("card size %v resized its reusable texture to %dx%d", dimensions, mask.W, mask.H)
		}
		sc := b.Batches[0].Scissor
		area := image.Rect(int(sc.Left), int(sc.Top), int(sc.Right), int(sc.Bottom))
		want := raster.ClipMask(nil, b.maskSlots[0].shapes, area)
		at := b.Instances[0].Clip
		ox, oy := area.Min.X-int(at[0]), area.Min.Y-int(at[1])
		for y := range area.Dy() {
			got := mask.Pix[4*((oy+y)*mask.W+ox):][:4*area.Dx()]
			if !bytes.Equal(got, want.Pix[4*y*area.Dx():][:4*area.Dx()]) {
				t.Fatalf("card size %v changed coverage at row %d", dimensions, y)
			}
		}
	}
	// Only the viewport changes: an exact shape-cache hit must trim too.
	s := fixedMaskScene(600, 400, 12)
	s.Width, s.Height = 320, 240
	b.Build(s, upload)
	if mask.W > s.Width || mask.H > s.Height {
		t.Fatalf("resized viewport retained an oversized texture: %dx%d", mask.W, mask.H)
	}
}

// Reopening from a small cover must pick up a retained large buffer when the cover grows, rather than growing another full-window mask.
func TestReopenedGrowingClipUsesIdleBacking(t *testing.T) {
	var b Builder
	defer b.Release()
	var mask *scene.Image
	upload := func(m *scene.Image) uintptr { mask = m; return uintptr(m.ID()) }
	build := func(w, h int) {
		s := fixedMaskScene(w, h, 12)
		s.Width, s.Height = 3020, 2120
		b.Build(s, upload)
	}
	build(3000, 2100)
	if mask == nil {
		t.Fatal("card did not upload a mask")
	}
	id, pixels := mask.ID(), &mask.Pix[0]
	for range 3 {
		b.Build(&scene.Scene{Width: 3020, Height: 2120}, upload)
	}
	build(64, 48)
	build(2000, 1500)
	if mask.ID() != id || &mask.Pix[0] != pixels {
		t.Fatal("growing reopened card allocated another large backing buffer")
	}
}

// A perspective or gradient mask that becomes small must still shrink instead of rasterizing its old full-window area.
func TestMovingAndPaintedClipMasksStillShrink(t *testing.T) {
	for _, painted := range []bool{false, true} {
		name := "transformed"
		if painted {
			name = "gradient"
		}
		t.Run(name, func(t *testing.T) {
			var b Builder
			defer b.Release()
			var mask *scene.Image
			for _, dimensions := range [][2]int{{600, 400}, {64, 48}} {
				s := reusableMaskScene(dimensions[0], dimensions[1], 12)
				s.Width, s.Height = 640, 480
				if painted {
					clip := &s.Ops[0]
					clip.Transform = scene.Affine{}
					clip.Paint, clip.Color, clip.Color2 = scene.PaintLinear, scene.Color{}, scene.Color{A: 255}
					clip.Gradient = [4]float32{0, 0, 640, 0}
				}
				b.Build(s, func(m *scene.Image) uintptr { mask = m; return uintptr(m.ID()) })
			}
			if mask == nil || mask.W > 66 || mask.H > 50 {
				t.Fatalf("small %s clip kept its full-window mask", name)
			}
		})
	}
}
