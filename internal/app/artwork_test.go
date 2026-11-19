package app

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

// An animated WebP cover, as some playlists have, shows its first frame,
// in the bitmap too, which decodes its data again once it drops its pixels.
func TestAnimatedWebPCover(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	for _, need := range []artNeeds{artCover, artTint, artBackdrop} {
		art, err := decodeArt(b, need)
		if err != nil {
			t.Fatal(err)
		}
		img, err := decodeImage(art.data)
		if err != nil {
			t.Fatal(err)
		}
		if r := img.Bounds(); r.Dx() != 24 || r.Dy() != 24 {
			t.Errorf("first frame is %v, want 24×24", r)
		}
		if w, h := art.bmp.Size(); w != 24 || h != 24 {
			t.Errorf("bitmap is %d×%d, want 24×24", w, h)
		}
	}
}

// Once the covers take more than the limit, those asked for least
// recently go, so that a long session does not keep every cover it
// showed; one asked for again stays, however long ago it loaded.
func TestArtCacheDropsLeastRecentCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.limit = func() time.Time { return now }, 100
	for _, key := range []string{"first", "second", "third"} {
		c.add(key, &artwork{used: now, size: 30})
		now = now.Add(time.Minute)
	}
	if len(c.art) != 3 || c.size != 90 {
		t.Fatalf("evicted covers under the budget: %d covers, %d bytes", len(c.art), c.size)
	}
	c.get("first", artCover)
	c.add("fourth", &artwork{used: now, size: 30})
	if _, ok := c.art["second"]; ok {
		t.Error("kept the cover asked for least recently over the limit")
	}
	if c.size != 60 || len(c.art) != 2 {
		t.Errorf("cache keeps %d covers, %d bytes, want two covers within the 75-byte low-water mark", len(c.art), c.size)
	}
	for _, key := range []string{"first", "fourth"} {
		if _, ok := c.art[key]; !ok {
			t.Errorf("dropped %q, asked for last", key)
		}
	}
}

// Covers stay under the limit however long since they showed. They went
// after 5 minutes not asked for: going back to a page after a while on
// others, or to the window after a while away, loaded them again.
func TestArtCacheKeepsCoversUnderLimit(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now = func() time.Time { return now }
	page := &artwork{used: now, size: 100 << 10}
	c.add("page", page)
	c.add("other", &artwork{used: now, size: 100 << 10})
	for range 60 {
		now = now.Add(time.Minute)
		c.get("other", artCover)
	}
	if c.get("page", artCover) != page {
		t.Error("dropped a cover not asked for in an hour, under the limit")
	}
}

// A cover dropped while something still holds it comes back as it was.
// It loaded again: Now Playing's cover went while other pages showed,
// though Now Playing still held it, and flashed in once it opened again.
func TestArtCacheKeepsHeldCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.limit = func() time.Time { return now }, 60
	held := &artwork{used: now, size: 30}
	c.add("now playing", held)
	for _, key := range []string{"other", "another"} {
		now = now.Add(time.Minute)
		c.add(key, &artwork{used: now, size: 30})
	}
	if _, ok := c.art["now playing"]; ok {
		t.Fatal("kept a cover over the limit")
	}
	runtime.GC()
	if got := c.get("now playing", artCover); got != held {
		t.Error("loaded again a cover something still held")
	}
	if c.size != 60 || len(c.art) != 2 {
		t.Errorf("restored cache keeps %d covers, %d bytes, want two 30-byte covers", len(c.art), c.size)
	}
	runtime.KeepAlive(held)
}

// Loaded covers count their encoded data and backdrop pixels toward eviction.
func TestArtCacheCountsLoadedCovers(t *testing.T) {
	data, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()
	for _, need := range []artNeeds{artCover, artBackdrop} {
		name := "cover"
		if need == artBackdrop {
			name = "backdrop"
		}
		t.Run(name, func(t *testing.T) {
			posted := make(chan func(), 1)
			c := newArtCache(func(fn func()) { posted <- fn })
			c.get(server.URL, need)
			select {
			case fn := <-posted:
				fn()
			case <-time.After(2 * time.Second):
				t.Fatal("cover did not finish loading")
			}
			a := c.get(server.URL, need)
			if a == nil || a.failed || a.bmp == nil {
				t.Fatal("cover did not load")
			}
			want := len(data)
			if need == artBackdrop {
				if a.backdrop == nil {
					t.Fatal("backdrop did not load")
				}
				w, h := a.backdrop.Size()
				want += w * h * 4
			}
			if a.size != want || c.size != want {
				t.Errorf("cover and cache count %d and %d bytes, want %d", a.size, c.size, want)
			}
		})
	}
}

// A temporary cover failure retries after its cooldown instead of staying cached forever.
func TestArtCacheRetriesFailedCover(t *testing.T) {
	data, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()
	posted := make(chan func(), 1)
	c := newArtCache(func(fn func()) { posted <- fn })
	now := time.Unix(1e9, 0)
	c.now = func() time.Time { return now }
	failed := &artwork{failed: true, used: now}
	c.add(server.URL, failed)
	now = now.Add(artRetry - time.Second)
	if c.get(server.URL, artCover) != failed || c.busy[server.URL] != nil {
		t.Fatal("retried the failed cover before its cooldown elapsed")
	}
	now = now.Add(time.Second)
	if c.get(server.URL, artCover) != nil || c.busy[server.URL] == nil {
		t.Fatal("did not retry the failed cover after its cooldown elapsed")
	}
	select {
	case fn := <-posted:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("cover retry did not finish loading")
	}
	if a := c.get(server.URL, artCover); a == nil || a.failed || a.bmp == nil {
		t.Fatal("successful retry did not replace the failed cover")
	}
	if c.size != len(data) {
		t.Errorf("retry counts %d bytes, want %d", c.size, len(data))
	}
}

func TestArtCacheDiskPersistence(t *testing.T) {
	data := coverData(t)
	dir := t.TempDir()
	disk := player.NewCache(dir, 10<<20)

	posted1 := make(chan func(), 1)
	c1 := newArtCache(func(fn func()) { posted1 <- fn })
	c1.disk = disk
	fetches := 0
	c1.fetch = func(string) ([]byte, error) {
		fetches++
		return data, nil
	}
	c1.get("https://example.com/album.jpg", artCover)
	select {
	case fn := <-posted1:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("first load timed out")
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
	if cached, ok := disk.LookupCover("https://example.com/album.jpg"); !ok || !bytes.Equal(cached, data) {
		t.Fatalf("cover was not persisted to disk: ok=%v", ok)
	}

	// Second artCache loads from disk even when network fails.
	posted2 := make(chan func(), 1)
	c2 := newArtCache(func(fn func()) { posted2 <- fn })
	c2.disk = player.NewCache(dir, 10<<20)
	c2.fetch = func(string) ([]byte, error) {
		fetches++
		return nil, errors.New("offline")
	}
	c2.get("https://example.com/album.jpg", artCover)
	select {
	case fn := <-posted2:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("disk load timed out")
	}
	if fetches != 1 {
		t.Fatalf("disk hit still called fetch (%d times)", fetches)
	}
	if a := c2.get("https://example.com/album.jpg", artCover); a == nil || a.failed || a.bmp == nil {
		t.Fatalf("disk cover did not load cleanly: %+v", a)
	}

	// Corrupt disk file is dropped and re-fetched from network.
	disk.KeepCover("https://example.com/album.jpg", []byte("not-an-image"))
	posted3 := make(chan func(), 1)
	c3 := newArtCache(func(fn func()) { posted3 <- fn })
	c3.disk = disk
	c3.fetch = func(string) ([]byte, error) {
		fetches++
		return data, nil
	}
	c3.get("https://example.com/album.jpg", artCover)
	select {
	case fn := <-posted3:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("corrupt recovery timed out")
	}
	if fetches != 2 {
		t.Fatalf("fetches after corrupt disk cache = %d, want 2", fetches)
	}
	if cached, ok := disk.LookupCover("https://example.com/album.jpg"); !ok || !bytes.Equal(cached, data) {
		t.Fatalf("corrupt disk cover was not replaced: ok=%v", ok)
	}
}

type genericImage struct{ image.Image }

func TestShrinkPreservesSampleColors(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(3, 5, 51, 53))
	nrgba := image.NewNRGBA(rgba.Bounds())
	ycbcr := image.NewYCbCr(rgba.Bounds(), image.YCbCrSubsampleRatio420)
	for y := 5; y < 53; y++ {
		for x := 3; x < 51; x++ {
			c := color.NRGBA{R: uint8(x * 3), G: uint8(y * 4), B: 80, A: uint8(x + y)}
			rgba.Set(x, y, c)
			nrgba.SetNRGBA(x, y, c)
			ycbcr.Y[ycbcr.YOffset(x, y)] = uint8(x * 3)
			i := ycbcr.COffset(x, y)
			ycbcr.Cb[i], ycbcr.Cr[i] = uint8(y*3), 150
		}
	}
	for _, img := range []image.Image{rgba, nrgba, ycbcr} {
		fast, generic := shrink(img, 24), shrink(genericImage{img}, 24)
		if !slices.Equal(fast.Pix, generic.Pix) {
			t.Fatalf("typed sampling changed %T colors", img)
		}
	}
	if allocs := testing.AllocsPerRun(10, func() { shrink(ycbcr, 24) }); allocs > 4 {
		t.Fatalf("YCbCr sampling allocates %.0f objects", allocs)
	}
}

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

// The drift eases in as the music plays and glides to a stop as it
// pauses; the bass swells and settles.
func TestBackdropDrifts(t *testing.T) {
	var bd backdropCache
	now := time.Unix(0, 0)
	var quiet [player.Bands]float32
	for range 120 {
		now = now.Add(backdropFrame)
		bd.step(now, true, quiet, true)
	}
	if bd.speed != 1 || bd.drift <= 0 {
		t.Fatalf("playing: speed %v, drift %v", bd.speed, bd.drift)
	}
	var loud [player.Bands]float32
	for i := range loud {
		loud[i] = 1
	}
	now = now.Add(backdropFrame)
	bd.step(now, true, loud, true)
	if bd.level.group(0, 6) <= 0.2 {
		t.Errorf("bass %v on a beat", bd.level.group(0, 6))
	}
	moving := true
	for range 120 {
		now = now.Add(backdropFrame)
		moving = bd.step(now, false, quiet, false)
	}
	if moving || bd.speed != 0 {
		t.Errorf("paused: still moving, speed %v", bd.speed)
	}
	at := bd.drift
	now = now.Add(time.Second)
	bd.step(now, false, quiet, false)
	if bd.drift != at {
		t.Error("drifted while paused")
	}
}

// With NM_FRAMES set, saves the fog of a few covers as it drifts,
// and on a beat, one row a cover.
func TestBackdropDriftFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	const w, h = 300, 420
	var rows []*image.RGBA
	for _, q := range []string{"春日影 MyGO", "晴天 周杰伦", "夜に駆ける"} {
		hits, err := qqmusic.Search(q, 1)
		if err != nil || len(hits) == 0 {
			t.Fatal(q, err)
		}
		img, err := fetchImage(qqmusic.Sized(hits[0].Album.PicURL, 600))
		if err != nil {
			t.Fatal(err)
		}
		art := &artwork{tint: dominantColor(img)}
		shots := []struct{ drift, bass float32 }{{0, 0}, {2, 0}, {4, 0}, {4, 0.9}}
		row := image.NewRGBA(image.Rect(0, 0, w*len(shots), h))
		for i, s := range shots {
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { drawFog(p, r, art.tint, s.drift, s.bass) })
			}, w, h)
			tt.Frame()
			shot := tt.Image()
			draw.Draw(row, image.Rect(i*w, 0, (i+1)*w, h), shot, shot.Bounds().Min, draw.Src)
		}
		rows = append(rows, row)
	}
	out := image.NewRGBA(image.Rect(0, 0, rows[0].Bounds().Dx(), h*len(rows)))
	for i, r := range rows {
		draw.Draw(out, image.Rect(0, i*h, r.Bounds().Dx(), (i+1)*h), r, image.Point{}, draw.Src)
	}
	f, _ := os.Create(filepath.Join(dir, "drift.png"))
	defer f.Close()
	png.Encode(f, out)
}

// A cover that loads while its place shows fades in; one ready as its
// place first shows is there at once.
func TestArtFadesIn(t *testing.T) {
	var bmp *ui.Bitmap
	var in float32
	tt := ui.NewTester(func(c *ui.Context) { in = artIn(ui.Box(c).Size(40, 40), bmp) }, 100, 100)
	tt.Frame()
	if in != 0 {
		t.Fatalf("shown %v while loading", in)
	}
	bmp = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	tt.Frame()
	tt.Advance(artFade.Duration / 3)
	if in <= 0 || in >= 1 {
		t.Errorf("shown %v a third of the way", in)
	}
	tt.Advance(artFade.Duration)
	if in != 1 {
		t.Errorf("shown %v once faded in", in)
	}

	in = 0
	ready := ui.NewTester(func(c *ui.Context) { in = artIn(ui.Box(c).Size(40, 40), bmp) }, 100, 100)
	ready.Frame()
	if in != 1 {
		t.Errorf("a cover ready at once shown %v", in)
	}
}
