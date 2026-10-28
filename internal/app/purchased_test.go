package app

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

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
