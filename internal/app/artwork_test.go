package app

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"lilt/internal/player"
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
