package app

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Clicking the account row drops its menu, which takes the focus so the
// arrows walk its items, and Escape or a second click puts it away.
func TestAccountMenu(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()

	if err := tt.Click("小明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.accountOpen || !tt.HasText("退出登录") {
		t.Fatalf("menu not open: %v", a.accountOpen)
	}
	if !tt.Focused("我的主页") {
		t.Fatal("the first item has no focus")
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if !tt.Focused("退出登录") {
		t.Fatal("Down did not move to signing out")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.accountOpen {
		t.Fatal("Escape left the menu open")
	}

	tt.Click("小明")
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	if a.accountOpen {
		t.Fatal("a second click left the menu open")
	}
}

// 我的主页 in the account menu opens the page of who is signed in and puts
// the menu away.
func TestAccountMenuOpensOwnPage(t *testing.T) {
	detail, playlists := userDetail, userPlaylists
	defer func() { userDetail, userPlaylists = detail, playlists }()
	userDetail = func(id int64) (*netease.User, error) { return &netease.User{UserID: id, Nickname: "小明"}, nil }
	userPlaylists = func(int64) ([]netease.Playlist, []netease.Playlist, error) { return nil, nil, nil }

	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 42, Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	if err := tt.Click("我的主页"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageUser || a.route.id != 42 {
		t.Fatalf("route = %+v, want the page of user 42", a.route)
	}
	if a.accountOpen {
		t.Fatal("the menu stayed open")
	}
}

// With NM_FRAMES set, saves the foot of the sidebar with the account menu
// open, light and dark.
func TestAccountMenuFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
		tt := ui.NewTester(a.view, 1000, 600)
		tt.SetDark(dark)
		tt.Frame()
		tt.Click("小明")
		tt.Advance(time.Second)
		tt.Frame()
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		crop := image.Rect(0, int(400*s), int((sidebarWidth+20)*s), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
		draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
		name := "account-light.png"
		if dark {
			name = "account-dark.png"
		}
		f, _ := os.Create(filepath.Join(dir, name))
		png.Encode(f, out)
		f.Close()
	}
}
