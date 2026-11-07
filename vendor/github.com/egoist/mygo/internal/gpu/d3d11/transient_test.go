//go:build windows

package d3d11

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// Retired temporary images and canvases used to wait hundreds of frames before freeing their textures.
func TestTransientResourcesRetireAfterDraw(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r := transientRenderer(t)
	defer r.Release()
	mask, cover, layer, kept := transientResources()
	s := transientResourceScene(mask, cover, layer, kept)
	if err := r.draw(s); err != nil {
		t.Fatal(err)
	}
	if r.images[mask.ID()] == nil || r.images[cover.ID()] == nil || r.canvases[layer.ID()] == nil || r.canvases[kept.ID()] == nil {
		t.Fatal("a texture was freed in the frame that used it")
	}
	maskTexture, layerTexture := r.images[mask.ID()], r.canvases[layer.ID()]
	want, stride, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.draw(&scene.Scene{Width: 64, Height: 64}); err != nil {
		t.Fatal(err)
	}
	if r.images[mask.ID()] != nil || r.canvases[layer.ID()] != nil {
		t.Fatal("retired temporary resources kept their textures until the periodic sweep")
	}
	if maskTexture.tex != 0 || maskTexture.srv != 0 || layerTexture.tex != 0 || layerTexture.srv != 0 || layerTexture.rtv != 0 {
		t.Fatal("retired resources left their native texture handles owned by the cache")
	}
	if r.images[cover.ID()] == nil || r.canvases[kept.ID()] == nil {
		t.Fatal("retiring temporary resources also evicted a normal image or kept canvas")
	}
	if err := r.draw(s); err != nil {
		t.Fatal(err)
	}
	got, gotStride, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	if gotStride != stride || !bytes.Equal(got, want) {
		t.Fatal("reopening a temporary image or canvas changed its pixels")
	}
	runtime.KeepAlive(mask)
	runtime.KeepAlive(cover)
	runtime.KeepAlive(layer)
	runtime.KeepAlive(kept)
}

// A frame that skips rebuilding an unchanged scene must not evict temporary textures still shown there.
func TestTransientResourcesSurviveUnchangedFrame(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r := transientRenderer(t)
	defer r.Release()
	if r.ctx1 == 0 {
		t.Skip("no Direct3D 11.1 partial redraw")
	}
	mask, cover, layer, kept := transientResources()
	s := transientResourceScene(mask, cover, layer, kept)
	if drawn, err := r.drawFrame(s); err != nil || !drawn {
		t.Fatalf("first frame: drawn=%v error=%v", drawn, err)
	}
	r.held = true // the swap chain holds this frame, as after presenting
	imageTexture, canvasTexture := r.images[mask.ID()], r.canvases[layer.ID()]
	if imageTexture == nil || canvasTexture == nil {
		t.Fatal("the first frame did not create its temporary textures")
	}
	if drawn, err := r.drawFrame(s); err != nil || drawn {
		t.Fatalf("unchanged frame: drawn=%v error=%v", drawn, err)
	}
	if r.images[mask.ID()] != imageTexture || r.canvases[layer.ID()] != canvasTexture {
		t.Fatal("an unchanged frame evicted temporary textures still in its scene")
	}
	if imageTexture.tex == 0 || canvasTexture.tex == 0 {
		t.Fatal("an unchanged frame released a current temporary texture")
	}
	if drawn, err := r.drawFrame(&scene.Scene{Width: 64, Height: 64}); err != nil || !drawn {
		t.Fatalf("retirement frame: drawn=%v error=%v", drawn, err)
	}
	if r.images[mask.ID()] != nil || r.canvases[layer.ID()] != nil {
		t.Fatal("temporary textures survived the first changed frame after their removal")
	}
	runtime.KeepAlive(mask)
	runtime.KeepAlive(cover)
	runtime.KeepAlive(layer)
	runtime.KeepAlive(kept)
}

func transientRenderer(t *testing.T) *Renderer {
	t.Helper()
	hwnd, err := newHiddenWindow(64, 64)
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

func transientResources() (mask, cover, layer, kept *scene.Image) {
	pixels := func(v byte) []byte {
		p := make([]byte, 8*8*4)
		for i := 0; i < len(p); i += 4 {
			p[i], p[i+1], p[i+2], p[i+3] = v, v, v, 255
		}
		return p
	}
	mask = scene.NewImageRGBA(8, 8, pixels(77))
	mask.Transient = true
	cover = scene.NewImageRGBA(8, 8, pixels(133))
	layer, kept = scene.NewCanvas(8, 8), scene.NewCanvas(8, 8)
	layer.Transient = true
	for _, c := range []*scene.Image{layer, kept} {
		c.Scene.Ops = []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{W: 8, H: 8}, Color: scene.Color{R: 200, G: 200, B: 200, A: 255}}}
	}
	return
}

func transientResourceScene(images ...*scene.Image) *scene.Scene {
	s := &scene.Scene{Width: 64, Height: 64}
	for i, img := range images {
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Image: img, Rect: scene.Rect{X: float32(i * 12), W: 8, H: 8}, Src: scene.Rect{W: 8, H: 8}})
	}
	return s
}
