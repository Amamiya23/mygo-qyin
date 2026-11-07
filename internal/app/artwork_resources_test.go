package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"
)

func coverData(t testing.TB) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.SetRGBA(x, y, color.RGBA{R: 180, G: 40, B: 80, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func finishArtWork(t *testing.T, posted <-chan func()) {
	t.Helper()
	synctest.Wait()
	select {
	case fn := <-posted:
		fn()
	default:
		t.Fatal("artwork worker did not publish a result")
	}
}

// Display-only covers used to decode and measure a second full image they never used.
func TestArtCoverNeedsNoDerivedImage(t *testing.T) {
	art, err := decodeArt(coverData(t), artCover)
	if err != nil {
		t.Fatal(err)
	}
	if art.bmp == nil || len(art.data) == 0 {
		t.Fatal("cover lost its bitmap or compressed restore source")
	}
	if art.tint != (ui.Color{}) || art.backdrop != nil || art.ready != artCover {
		t.Fatal("display-only cover generated unrequested derived resources")
	}
	if w, h := art.bmp.Size(); w != 16 || h != 16 {
		t.Fatalf("cover is %d×%d, want 16×16", w, h)
	}
}

// Display and backdrop requests used separate downloads and bitmaps, even for the same URL.
func TestArtCacheSharesCoverAndBackdrop(t *testing.T) {
	for _, order := range []struct {
		name   string
		first  artNeeds
		during bool
	}{
		{"cover-then-backdrop", artCover, false},
		{"backdrop-during-cover", artCover, true},
		{"cover-during-backdrop", artBackdrop, true},
		{"tint-then-backdrop", artTint, false},
	} {
		t.Run(order.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				data := coverData(t)
				posted := make(chan func(), 1)
				c := newArtCache(func(fn func()) { posted <- fn })
				fetches, releases := 0, 0
				c.fetch = func(string) ([]byte, error) {
					fetches++
					return data, nil
				}
				c.release = func(*ui.Bitmap) { releases++ }
				c.get("cover", order.first)
				if order.during {
					c.get("cover", artCover)
					c.get("cover", artBackdrop)
				}
				finishArtWork(t, posted)
				base := c.get("cover", artCover)
				if base == nil || base.failed || base.bmp == nil {
					t.Fatal("base cover did not load")
				}
				bmp := base.bmp
				if !order.during {
					if c.get("cover", artBackdrop) != nil {
						t.Fatal("returned incomplete backdrop resources")
					}
				}
				for len(c.busy) != 0 {
					finishArtWork(t, posted)
				}
				art := c.get("cover", artBackdrop)
				if art != base || art.bmp != bmp || c.get("cover", artCover) != art {
					t.Fatal("deriving a backdrop replaced the shared cover or bitmap")
				}
				if art.backdrop == nil || art.tint == (ui.Color{}) {
					t.Fatal("backdrop request did not produce colors and a blurred image")
				}
				w, h := art.backdrop.Size()
				want := len(data) + w*h*4
				if fetches != 1 || releases != 1 || len(c.art) != 1 || c.size != want || art.size != want {
					t.Fatalf("shared cover: fetches=%d releases=%d entries=%d bytes=%d/%d, want 1/1/1/%d", fetches, releases, len(c.art), c.size, art.size, want)
				}
			})
		})
	}
}

// Color-only users such as the notch retain their tint without allocating a blurred background.
func TestArtCacheTintNeedsNoBackdrop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		posted := make(chan func(), 1)
		c := newArtCache(func(fn func()) { posted <- fn })
		data := coverData(t)
		c.fetch = func(string) ([]byte, error) { return data, nil }
		c.release = func(*ui.Bitmap) {}
		c.get("cover", artTint)
		finishArtWork(t, posted)
		art := c.get("cover", artTint)
		if art == nil || art.failed || art.tint == (ui.Color{}) || art.backdrop != nil {
			t.Fatal("color-only request lost its tint or generated an unused backdrop")
		}
		if art.size != len(data) || c.size != len(data) {
			t.Fatal("color-only request counted unused backdrop pixels")
		}
	})
}

// An upgrade finishing after eviction must revive the same held cover and count its new pixels once.
func TestArtCacheCountsEvictedBackdropUpgrade(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		posted := make(chan func(), 1)
		c := newArtCache(func(fn func()) { posted <- fn })
		data := coverData(t)
		fetches := 0
		c.fetch = func(string) ([]byte, error) { fetches++; return data, nil }
		c.release = func(*ui.Bitmap) {}
		now := time.Unix(1e9, 0)
		c.now = func() time.Time { return now }
		c.get("cover", artCover)
		finishArtWork(t, posted)
		held := c.get("cover", artCover)
		c.get("cover", artBackdrop)
		c.limit = len(data) * 3 / 2
		now = now.Add(time.Minute)
		c.add("other", &artwork{used: now, size: len(data)})
		if c.art["cover"] != nil {
			t.Fatal("fixture did not evict the cover while its backdrop was pending")
		}
		c.limit = artLimit
		finishArtWork(t, posted)
		if got := c.get("cover", artBackdrop); got != held || got.backdrop == nil {
			t.Fatal("upgrade lost a held cover or its derived backdrop")
		}
		want := c.art["other"].size + held.size
		if c.size != want || fetches != 1 {
			t.Fatalf("upgraded cache keeps %d bytes after %d fetches, want %d bytes after one fetch", c.size, fetches, want)
		}
	})
}

// Reviving a held cover under pressure must update recency before choosing which entries to evict.
func TestArtCacheRevivesCoverBeforeEviction(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.limit = func() time.Time { return now }, 100
	held := &artwork{used: now, size: 30}
	c.add("held", held)
	now = now.Add(time.Minute)
	c.add("older", &artwork{used: now, size: 40})
	now = now.Add(time.Minute)
	c.add("newer", &artwork{used: now, size: 40})
	if c.art["held"] != nil {
		t.Fatal("fixture did not evict the held cover")
	}
	now = now.Add(time.Minute)
	c.add("fresh", &artwork{used: now, size: 40})
	now = now.Add(time.Minute)
	if got := c.get("held", artCover); got != held || c.art["held"] != held {
		t.Fatal("revived cover was immediately evicted using its old access time")
	}
}

// Compare display-only decoding with the color measurement ordinary covers previously performed.
func BenchmarkArtDecode(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 800))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:i+4], []byte{180, 40, 80, 255})
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 85}); err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		need artNeeds
	}{{"cover", artCover}, {"with-tint", artTint}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				art, err := decodeArt(encoded.Bytes(), tc.need)
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(art)
			}
		})
	}
}
