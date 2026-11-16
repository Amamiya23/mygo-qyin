package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

type purchaseRequest struct {
	tab, offset int
}

func fakePurchases(t *testing.T) chan purchaseRequest {
	t.Helper()
	asked := make(chan purchaseRequest, 8)
	songs, albums := readPurchasedSongs, readPurchasedAlbums
	t.Cleanup(func() { readPurchasedSongs, readPurchasedAlbums = songs, albums })
	readPurchasedSongs = func(_, offset int) ([]qqmusic.Track, bool, error) {
		asked <- purchaseRequest{purchasedSongs, offset}
		return nil, false, errors.New("offline")
	}
	readPurchasedAlbums = func(_, offset int) ([]qqmusic.AlbumProfile, bool, error) {
		asked <- purchaseRequest{purchasedAlbums, offset}
		return nil, false, errors.New("offline")
	}
	return asked
}

func purchasedApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 42, Nickname: "小明"}
	a.purchased[purchasedSongs] = &purchasedData{
		songs: []qqmusic.Track{
			{ID: 1, MID: "songOne", Name: "春日影", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Album: qqmusic.Album{Name: "迷跡波"}, Duration: 253000},
			{ID: 2, MID: "songTwo", Name: "迷星叫", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Album: qqmusic.Album{Name: "迷跡波"}, Duration: 216000},
		},
		page: paged{loaded: true},
	}
	a.purchased[purchasedAlbums] = &purchasedData{
		albums: []qqmusic.AlbumProfile{{Album: qqmusic.Album{ID: 8, MID: "albumMid", Name: "迷跡波"}, Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}}},
		page:   paged{loaded: true},
	}
	a.albums[8] = &albumData{album: &qqmusic.AlbumProfile{Album: qqmusic.Album{ID: 8, Name: "迷跡波"}}}
	return a
}

// The sidebar opens purchased albums by default, and switching tabs and opening an album preserves navigation.
func TestPurchasedNavigation(t *testing.T) {
	asked := fakePurchases(t)
	a := purchasedApp()
	a.purchased = [2]*purchasedData{}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("已购音乐"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asked, "purchased albums"); got != (purchaseRequest{purchasedAlbums, 0}) {
		t.Fatalf("request %+v", got)
	}
	tt.Frame()
	if err := tt.Click("单曲"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asked, "purchased singles"); got != (purchaseRequest{purchasedSongs, 0}) {
		t.Fatalf("request %+v", got)
	}
	a.purchased[purchasedAlbums] = purchasedApp().purchased[purchasedAlbums]
	tt.Frame()
	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("迷跡波"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageAlbum, id: 8}) {
		t.Fatalf("album route %+v", a.route)
	}
	a.back()
	tt.Frame()
	if a.route != (route{page: pagePurchased, tab: purchasedAlbums}) || len(asked) != 0 {
		t.Fatalf("back route %+v; repeated requests %d", a.route, len(asked))
	}
}

// A purchase page retries from the same offset after failure, while refresh replaces it from the start.
func TestPurchasedPaginationAndRefresh(t *testing.T) {
	asked := fakePurchases(t)
	a := purchasedApp()
	d := a.purchased[purchasedSongs]
	d.page.more = true
	a.open(route{page: pagePurchased, tab: purchasedSongs})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if got := wait(t, asked, "next purchases"); got != (purchaseRequest{purchasedSongs, 2}) {
		t.Fatalf("request %+v", got)
	}
	d.page.loading, d.page.err = false, "offline"
	tt.Frame()
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asked, "purchase retry"); got != (purchaseRequest{purchasedSongs, 2}) {
		t.Fatalf("retry %+v", got)
	}
	d.page = paged{loaded: true}
	tt.Frame()
	if err := tt.Click("刷新已购音乐"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asked, "purchase refresh"); got != (purchaseRequest{purchasedSongs, 0}) || a.purchased[purchasedSongs] == d {
		t.Fatalf("refresh %+v; kept old data %v", got, a.purchased[purchasedSongs] == d)
	}
}

// Opening purchases while signed out waits for login without requesting personal data.
func TestPurchasedRequiresLogin(t *testing.T) {
	asked := fakePurchases(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.open(route{page: pagePurchased})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.purchased[purchasedSongs] != nil || len(asked) != 0 {
		t.Fatal("requested purchases without an account")
	}
	started := make(chan bool, 1)
	begin := beginLoginQR
	beginLoginQR = func() (string, error) { started <- true; return "", errors.New("offline") }
	defer func() { beginLoginQR = begin }()
	if err := tt.Click("登录 QQ 音乐"); err != nil {
		t.Fatal(err)
	}
	wait(t, started, "login")
	if !a.login.open {
		t.Fatal("login did not open")
	}
}

// With NM_FRAMES set, saves both purchase tabs in light and dark for visual inspection.
func TestPurchasedFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, tab := range []int{purchasedSongs, purchasedAlbums} {
		for _, dark := range []bool{false, true} {
			a := purchasedApp()
			a.open(route{page: pagePurchased, tab: tab})
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("purchased-%d-dark-%t.png", tab, dark)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPurchasedDiskCacheOfflineAndRefresh(t *testing.T) {
	origSongs, origAlbums := readPurchasedSongs, readPurchasedAlbums
	t.Cleanup(func() { readPurchasedSongs, readPurchasedAlbums = origSongs, origAlbums })

	cacheDir := t.TempDir()
	cache := player.NewCache(cacheDir, 4<<20)

	songCalls, albumCalls := 0, 0
	offline := false
	readPurchasedSongs = func(limit, offset int) ([]qqmusic.Track, bool, error) {
		songCalls++
		if offline {
			return nil, false, errors.New("offline")
		}
		if offset == 0 {
			return []qqmusic.Track{{ID: 101, MID: "s101", Name: "已购一"}}, true, nil
		}
		return []qqmusic.Track{{ID: 102, MID: "s102", Name: "已购二"}}, false, nil
	}
	readPurchasedAlbums = func(limit, offset int) ([]qqmusic.AlbumProfile, bool, error) {
		albumCalls++
		if offline {
			return nil, false, errors.New("offline")
		}
		return []qqmusic.AlbumProfile{{Album: qqmusic.Album{ID: 201, MID: "a201", Name: "已购专辑一"}}}, false, nil
	}

	posted := make(chan func(), 32)
	a1 := newTestApp()
	a1.pl.Cache = cache
	a1.postFn = func(fn func()) { posted <- fn }
	a1.profile = &qqmusic.Profile{UserID: 77}

	// First load fetches page 0 and persists to disk.
	a1.loadPurchased(purchasedSongs, false)
	pumpPosted(t, posted, func() bool {
		d := a1.purchased[purchasedSongs]
		return d != nil && d.page.loaded && !d.page.loading
	})
	a1.morePurchased(purchasedSongs, a1.purchased[purchasedSongs])
	pumpPosted(t, posted, func() bool {
		d := a1.purchased[purchasedSongs]
		return d != nil && !d.page.loading && len(d.songs) == 2
	})
	a1.loadPurchased(purchasedAlbums, false)
	pumpPosted(t, posted, func() bool {
		d := a1.purchased[purchasedAlbums]
		return d != nil && d.page.loaded && !d.page.loading
	})

	// Wait briefly for async disk writes to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s, _, _, okS := cache.LookupPurchasedSongs(77)
		al, _, _, okA := cache.LookupPurchasedAlbums(77)
		if okS && len(s) == 2 && okA && len(al) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Fresh app in offline mode loads both tabs from disk without network calls.
	offline = true
	beforeSongs, beforeAlbums := songCalls, albumCalls
	a2 := newTestApp()
	a2.pl.Cache = player.NewCache(cacheDir, 4<<20)
	a2.postFn = func(fn func()) { posted <- fn }
	a2.profile = &qqmusic.Profile{UserID: 77}

	a2.loadPurchased(purchasedSongs, false)
	pumpPosted(t, posted, func() bool {
		d := a2.purchased[purchasedSongs]
		return d != nil && d.page.loaded && !d.page.loading
	})
	a2.loadPurchased(purchasedAlbums, false)
	pumpPosted(t, posted, func() bool {
		d := a2.purchased[purchasedAlbums]
		return d != nil && d.page.loaded && !d.page.loading
	})
	drainPosted(posted, 40*time.Millisecond)
	if songCalls != beforeSongs || albumCalls != beforeAlbums {
		t.Fatalf("offline load within TTL hit network: songs %d->%d albums %d->%d", beforeSongs, songCalls, beforeAlbums, albumCalls)
	}
	if len(a2.purchased[purchasedSongs].songs) != 2 || a2.purchased[purchasedSongs].page.err != "" {
		t.Fatalf("offline songs mismatch: %+v", a2.purchased[purchasedSongs])
	}
	if len(a2.purchased[purchasedAlbums].albums) != 1 || a2.purchased[purchasedAlbums].page.err != "" {
		t.Fatalf("offline albums mismatch: %+v", a2.purchased[purchasedAlbums])
	}

	// Simulate an expired cache entry on disk (cachedAt older than TTL):
	// loads cached data immediately, attempts silent refresh while offline without
	// clearing cached items or setting an error, then succeeds when online.
	expiredData, err := jsonMarshalExpiredPurchasedSongs([]qqmusic.Track{{ID: 101, MID: "s101", Name: "旧已购"}}, false, time.Now().Add(-purchasedRefreshTTL-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !a2.pl.Cache.KeepPurchasedSongsData(77, expiredData) {
		t.Fatal("KeepPurchasedSongsData failed")
	}

	a3 := newTestApp()
	a3.pl.Cache = player.NewCache(cacheDir, 4<<20)
	a3.postFn = func(fn func()) { posted <- fn }
	a3.profile = &qqmusic.Profile{UserID: 77}

	a3.loadPurchased(purchasedSongs, false)
	pumpPosted(t, posted, func() bool {
		d := a3.purchased[purchasedSongs]
		return d != nil && d.page.loaded && !d.refreshing
	})
	if a3.purchased[purchasedSongs].page.err != "" || len(a3.purchased[purchasedSongs].songs) != 1 || a3.purchased[purchasedSongs].songs[0].Name != "旧已购" {
		t.Fatalf("failed background refresh disturbed cached purchased songs: %+v", a3.purchased[purchasedSongs])
	}

	// Recover network and revisit stale tab: silently refreshes to latest songs.
	offline = false
	a3.loadPurchased(purchasedSongs, false)
	pumpPosted(t, posted, func() bool {
		d := a3.purchased[purchasedSongs]
		return d != nil && !d.refreshing && !d.stale && len(d.songs) > 0 && d.songs[0].Name == "已购一"
	})
}

func jsonMarshalExpiredPurchasedSongs(songs []qqmusic.Track, more bool, at time.Time) ([]byte, error) {
	return json.Marshal(player.CachedPurchasedSongs{Songs: songs, More: more, CachedAt: at})
}

