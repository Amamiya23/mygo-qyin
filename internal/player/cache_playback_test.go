package player

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"lilt/internal/qqmusic"
)

func TestQueueUsesCompleteCacheWhenQualityRequestStalls(t *testing.T) {
	cache, _ := cachePlaybackFixture(t, "lossless", 1, 2)
	previous := songURL
	t.Cleanup(func() { songURL = previous })
	songURL = func(ctx context.Context, _ int64, requested, cached string) (*qqmusic.Stream, error) {
		if requested != "hires" || cached != "lossless" {
			t.Errorf("online lookup got requested=%q, cached=%q", requested, cached)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	downloadBody(t, func(*http.Request) io.ReadCloser {
		t.Error("downloaded a song with a complete fallback cache")
		return io.NopCloser(bytes.NewReader(nil))
	})
	synctest.Test(t, func(t *testing.T) {
		post := make(chan func(), 16)
		p := &Player{Cache: cache, post: func(f func()) { post <- f }, Quality: func() string { return "hires" }}
		defer stopCachePlayback(p)
		p.PlayQueue([]qqmusic.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}}, 0)
		for _, id := range []int64{1, 2} {
			if id == 2 {
				p.Next()
			}
			start := time.Now()
			time.Sleep(2*time.Second - time.Millisecond)
			synctest.Wait()
			for len(post) != 0 {
				(<-post)()
			}
			if !p.Loading() || p.Ready() {
				t.Fatalf("track %d did not wait for the Hi-Res lookup", id)
			}
			p.TogglePlay() // Check the playback source without opening a sound device.
			awaitCachePlayback(t, p, post, id, "lossless")
			if waited := time.Since(start); waited != 2*time.Second {
				t.Fatalf("track %d waited %s, want 2s", id, waited)
			}
		}
	})
}

func TestCacheFallbackKeepsAvailableUpgrade(t *testing.T) {
	cache, data := cachePlaybackFixture(t, "lossless", 1)
	upgrade := &qqmusic.Stream{URL: "https://audio.test/hires.flac", Size: int64(len(data)), Level: "hires", Type: "flac"}
	downloadBody(t, func(r *http.Request) io.ReadCloser {
		if r.URL.String() != upgrade.URL || r.Context().Err() != nil {
			t.Error("the lookup deadline affected the selected higher-quality download")
		}
		return io.NopCloser(bytes.NewReader(data))
	})
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		resolve := func(context.Context, int64, string, string) (*qqmusic.Stream, error) {
			time.Sleep(1500 * time.Millisecond)
			return upgrade, nil
		}
		info, s, _, err := openTrack(context.Background(), cache, qqmusic.Track{ID: 1, Duration: 5000}, "hires", false, resolve)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if info.Level != "hires" || time.Since(start) >= 2*time.Second {
			t.Fatalf("timely upgrade was not used: level=%s, elapsed=%s", info.Level, time.Since(start))
		}
	})
}

func TestSwitchTrackCancelsPendingSource(t *testing.T) {
	cache, _ := cachePlaybackFixture(t, "hires", 2)
	if !cache.KeepLyrics(1, "[00:00.00]first track", "", "") {
		t.Fatal("could not cache lyrics")
	}
	previous := songURL
	t.Cleanup(func() { songURL = previous })
	started := make(chan struct{})
	result := make(chan error, 1)
	songURL = func(ctx context.Context, id int64, _, cached string) (*qqmusic.Stream, error) {
		if id != 1 || cached != "" {
			t.Error("unexpected source request")
		}
		if _, ok := ctx.Deadline(); ok {
			t.Error("a track without a cache should not have an upgrade deadline")
		}
		close(started)
		<-ctx.Done()
		result <- ctx.Err()
		return nil, ctx.Err()
	}
	synctest.Test(t, func(t *testing.T) {
		post := make(chan func(), 16)
		p := &Player{Cache: cache, post: func(f func()) { post <- f }, Quality: func() string { return "hires" }}
		defer stopCachePlayback(p)
		p.PlayQueue([]qqmusic.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}}, 0)
		<-started
		p.Next()
		p.TogglePlay()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatal("previous track stopped without cancellation:", err)
			}
		case <-time.After(time.Second):
			t.Fatal("previous track's source request continued after switching")
		}
		awaitCachePlayback(t, p, post, 2, "hires")
		synctest.Wait()
		for len(post) != 0 {
			(<-post)()
		}
		if p.Current().ID != 2 || p.Err() != "" || !p.Ready() {
			t.Fatal("the canceled request changed the new track")
		}
	})
}

func cachePlaybackFixture(t *testing.T, level string, ids ...int64) (*Cache, []byte) {
	t.Helper()
	data, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCache(t.TempDir(), 32<<20)
	st := &qqmusic.Stream{Size: int64(len(data)), Level: level, Type: "flac"}
	for _, id := range ids {
		f := cache.part(st)
		if f == nil {
			t.Fatal("could not create cache file")
		}
		_, err := f.Write(data)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		if _, ok := cache.keep(id, st, f.Name(), st.Size); !ok || !cache.KeepLyrics(id, "[00:00.00]fixture", "", "") {
			t.Fatalf("could not cache track %d", id)
		}
	}
	return cache, data
}

func awaitCachePlayback(t *testing.T, p *Player, post <-chan func(), id int64, level string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !p.Ready() || !p.lyricsLoaded {
		if err := p.Err(); err != "" {
			t.Fatal(err)
		}
		select {
		case f := <-post:
			f()
		case <-deadline:
			t.Fatalf("track %d did not load", id)
		}
	}
	dl, _ := p.stream.source()
	_, complete := dl.completedSize()
	if p.Current().ID != id || p.Stream().Level != level || p.Loading() || p.buffering() || !complete {
		t.Fatalf("track %d did not open its complete %s cache: current=%d, level=%s, loading=%v, buffering=%v, complete=%v", id, level, p.Current().ID, p.Stream().Level, p.Loading(), p.buffering(), complete)
	}
}

func stopCachePlayback(p *Player) {
	out := p.out
	p.stop()
	if out != nil {
		out.Close()
	}
}
