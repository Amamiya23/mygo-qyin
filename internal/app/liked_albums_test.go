package app

import (
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func likedAlbumsApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 42, Nickname: "小明"}
	pl := qqmusic.Playlist{ID: 7, Name: "我喜欢", SpecialType: 5}
	a.mine = []qqmusic.Playlist{pl}
	a.playlists[7] = &pl
	al := qqmusic.AlbumProfile{Album: qqmusic.Album{ID: 8, MID: "AlbumMID", Name: "迷跡波"}, Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}}
	a.likedAlbums = &likedAlbumData{albums: []qqmusic.AlbumProfile{al}, page: paged{loaded: true}}
	a.albums[8] = &albumData{album: &al, likeUID: 42, liked: true, likeKnown: true}
	return a
}

func likedLayoutApp() *app {
	a := likedAlbumsApp()
	pl := a.playlists[7]
	pl.Description = "收藏的歌曲与专辑"
	pl.Creator.Nickname = a.profile.Nickname
	for i := range 36 {
		pl.Tracks = append(pl.Tracks, qqmusic.Track{
			ID: int64(100 + i), Name: fmt.Sprintf("喜欢的单曲%02d", i+1), Duration: 216000,
			Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Album: qqmusic.Album{Name: "迷跡波"},
		})
	}
	pl.TrackCount = len(pl.Tracks)
	for i := range 12 {
		a.likedAlbums.albums = append(a.likedAlbums.albums, qqmusic.AlbumProfile{
			Album:   qqmusic.Album{ID: int64(20 + i), Name: fmt.Sprintf("喜欢的专辑%02d", i+1)},
			Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}},
		})
	}
	return a
}

func likedBounds(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing %q", label)
	}
	return r
}

func sameLikedBounds(t *testing.T, label string, got, want ui.Rect) {
	t.Helper()
	if math.Abs(float64(got.X-want.X)) > 0.5 || math.Abs(float64(got.Y-want.Y)) > 0.5 || math.Abs(float64(got.W-want.W)) > 0.5 || math.Abs(float64(got.H-want.H)) > 0.5 {
		t.Fatalf("%s moved: got %+v; want %+v", label, got, want)
	}
}

// Switching liked songs and albums previously changed the header height and moved the tab buttons under the pointer.
func TestLikedTabsKeepHeaderGeometry(t *testing.T) {
	for _, width := range []int{1000, 1200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := likedLayoutApp()
			a.open(route{page: pagePlaylist, id: 7})
			tt := ui.NewTester(a.view, width, 800)
			tt.Frame()
			songsTab, albumsTab := likedBounds(t, tt, "单曲"), likedBounds(t, tt, "专辑")
			if err := tt.Click("专辑"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			sameLikedBounds(t, "song tab on albums", likedBounds(t, tt, "单曲"), songsTab)
			sameLikedBounds(t, "album tab on albums", likedBounds(t, tt, "专辑"), albumsTab)
			d := a.likedAlbums
			d.albums = nil
			d.page.loaded, d.page.loading = false, true
			tt.Frame()
			sameLikedBounds(t, "album tab while loading", likedBounds(t, tt, "专辑"), albumsTab)
			d.page.loaded, d.page.loading = true, false
			tt.Frame()
			sameLikedBounds(t, "album tab on empty collection", likedBounds(t, tt, "专辑"), albumsTab)
			if err := tt.Click("单曲"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			sameLikedBounds(t, "song tab after return", likedBounds(t, tt, "单曲"), songsTab)
		})
	}
}

// Each liked tab remembers its own actual scroll position, including after visiting an album detail page.
func TestLikedTabsRememberIndependentScroll(t *testing.T) {
	a := likedLayoutApp()
	a.open(route{page: pagePlaylist, id: 7})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	startSong := likedBounds(t, tt, "喜欢的单曲01")
	tt.Scroll(1000, 550, 0, 72)
	tt.Frame()
	song := likedBounds(t, tt, "喜欢的单曲01")
	if startSong.Y-song.Y < 20 {
		t.Fatalf("songs did not scroll: before %+v; after %+v", startSong, song)
	}
	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	startAlbum := likedBounds(t, tt, "迷跡波")
	tt.Scroll(1000, 550, 0, 120)
	tt.Frame()
	album := likedBounds(t, tt, "迷跡波")
	if startAlbum.Y-album.Y < 20 {
		t.Fatalf("albums did not scroll: before %+v; after %+v", startAlbum, album)
	}
	if err := tt.Click("单曲"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	sameLikedBounds(t, "song after tab return", likedBounds(t, tt, "喜欢的单曲01"), song)
	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	sameLikedBounds(t, "album after tab return", likedBounds(t, tt, "迷跡波"), album)
	if err := tt.Click("迷跡波"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageAlbum, id: 8}) {
		t.Fatalf("album route: %+v", a.route)
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	sameLikedBounds(t, "album after detail return", likedBounds(t, tt, "迷跡波"), album)
	if err := tt.Click("单曲"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	sameLikedBounds(t, "song after detail return", likedBounds(t, tt, "喜欢的单曲01"), song)
}

// The shared liked header still starts the song queue in order and honors its shuffle action.
func TestLikedHeaderPlayback(t *testing.T) {
	a := likedLayoutApp()
	a.open(route{page: pagePlaylist, id: 7})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("随机播放"); err != nil {
		t.Fatal(err)
	}
	if !a.pl.Shuffle() || len(a.pl.Queue()) != len(a.playlists[7].Tracks) {
		t.Fatal("shuffle action did not start liked songs")
	}
	tt.Frame()
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Shuffle() || a.pl.Current() == nil || a.pl.Current().ID != a.playlists[7].Tracks[0].ID || len(a.pl.Queue()) != len(a.playlists[7].Tracks) {
		t.Fatal("play action did not restore the ordered liked queue")
	}
}

// The liked playlist switches between singles and albums, and returning from an album preserves the album tab.
func TestLikedAlbumNavigation(t *testing.T) {
	a := likedAlbumsApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("我喜欢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pagePlaylist, id: 7, tab: 1}) {
		t.Fatalf("album tab route: %+v", a.route)
	}
	tt.Frame()
	if err := tt.Click("迷跡波"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageAlbum, id: 8}) {
		t.Fatalf("album route: %+v", a.route)
	}
	a.back()
	tt.Frame()
	if a.route != (route{page: pagePlaylist, id: 7, tab: 1}) {
		t.Fatalf("back route: %+v", a.route)
	}
	if err := tt.Click("单曲"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pagePlaylist, id: 7}) {
		t.Fatalf("single tab route: %+v", a.route)
	}
}

// Album pagination retries its failed offset, and refreshing starts a new collection from zero.
func TestLikedAlbumPaginationAndRefresh(t *testing.T) {
	old := readLikedAlbums
	defer func() { readLikedAlbums = old }()
	asks := make(chan int, 4)
	readLikedAlbums = func(_, offset int) ([]qqmusic.AlbumProfile, bool, error) {
		asks <- offset
		return nil, false, errors.New("offline")
	}
	a := likedAlbumsApp()
	d := a.likedAlbums
	d.page.more = true
	a.open(route{page: pagePlaylist, id: 7, tab: 1})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if offset := wait(t, asks, "next liked albums"); offset != 1 {
		t.Fatalf("next offset: %d", offset)
	}
	d.page.loading, d.page.err = false, "offline"
	tt.Frame()
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if offset := wait(t, asks, "retry liked albums"); offset != 1 {
		t.Fatalf("retry offset: %d", offset)
	}
	d.page = paged{loaded: true}
	tt.Frame()
	if err := tt.Click("刷新收藏专辑"); err != nil {
		t.Fatal(err)
	}
	if offset := wait(t, asks, "refresh liked albums"); offset != 0 || a.likedAlbums == d {
		t.Fatalf("refresh offset: %d; old data retained: %v", offset, a.likedAlbums == d)
	}
}

// The album heart sends its MID, commits only confirmed changes, and ignores replies for a previous account.
func TestAlbumHeartWritesAndConfirms(t *testing.T) {
	old := likeAlbum
	defer func() { likeAlbum = old }()
	type request struct {
		mid   string
		liked bool
	}
	asks := make(chan request, 4)
	likeAlbum = func(mid string, liked bool) error { asks <- request{mid, liked}; return nil }
	a := likedAlbumsApp()
	a.open(route{page: pageAlbum, id: 8})
	d := a.albums[8]
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("取消收藏专辑"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asks, "unfavorite album"); got != (request{"AlbumMID", false}) {
		t.Fatalf("request: %+v", got)
	}
	if !d.liked || !d.likeLoading {
		t.Fatal("changed collection before confirmation")
	}
	a.albumLikeAnswered(42, d, false, errors.New("offline"))
	if !d.liked || d.likeLoading {
		t.Fatal("failure changed collection state")
	}
	a.toggleAlbumLike(d)
	wait(t, asks, "retry album")
	a.albumLikeAnswered(42, d, false, nil)
	if d.liked || !d.likeKnown || a.likedAlbums != nil {
		t.Fatal("confirmed unfavorite did not update collection")
	}
	tt.Frame()
	if err := tt.Click("收藏专辑"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asks, "favorite album"); got != (request{"AlbumMID", true}) {
		t.Fatalf("request: %+v", got)
	}
	a.profile = &qqmusic.Profile{UserID: 99}
	a.albumLikeAnswered(42, d, true, nil)
	if d.liked {
		t.Fatal("old account changed album status")
	}
}

// A failed album status read offers a read retry, so an already saved album is never accidentally collected again.
func TestAlbumHeartRetriesUnknownStatus(t *testing.T) {
	oldRead, oldWrite := readAlbumLiked, likeAlbum
	defer func() { readAlbumLiked, likeAlbum = oldRead, oldWrite }()
	reads := make(chan string, 2)
	writes := make(chan bool, 2)
	readAlbumLiked = func(mid string) (bool, error) { reads <- mid; return false, errors.New("offline") }
	likeAlbum = func(_ string, liked bool) error { writes <- liked; return nil }
	a := likedAlbumsApp()
	d := a.albums[8]
	d.likeKnown = false
	a.open(route{page: pageAlbum, id: 8})
	if mid := wait(t, reads, "album status"); mid != "AlbumMID" {
		t.Fatalf("read MID: %s", mid)
	}
	a.albumLikeStatusAnswered(42, d, false, errors.New("offline"))
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("重试专辑收藏状态"); err != nil {
		t.Fatal(err)
	}
	if mid := wait(t, reads, "retry album status"); mid != "AlbumMID" || len(writes) != 0 {
		t.Fatalf("retry MID: %s; writes: %d", mid, len(writes))
	}
	a.albumLikeStatusAnswered(42, d, true, nil)
	tt.Frame()
	if err := tt.Click("取消收藏专辑"); err != nil {
		t.Fatal(err)
	}
	if liked := wait(t, writes, "unfavorite after status retry"); liked {
		t.Fatal("re-collected an already saved album")
	}
}

// With NM_FRAMES set, saves both liked tabs in both appearances to inspect their shared header and content alignment.
func TestLikedAlbumFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for tab, name := range []string{"songs", "albums"} {
		for _, dark := range []bool{false, true} {
			a := likedLayoutApp()
			a.open(route{page: pagePlaylist, id: 7, tab: tab})
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("liked-%s-dark-%t.png", name, dark)))
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
