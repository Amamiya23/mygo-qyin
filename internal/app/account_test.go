package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"rsc.io/qr"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

func newTestApp() *app {
	a := &app{playlists: map[int64]*netease.Playlist{}, loadingPl: map[int64]bool{}, plErr: map[int64]string{}, lyricAt: -1, queueAt: -1}
	a.art = newArtCache(func(func()) {})
	a.pl = player.New(func(func()) {})
	a.pl.SetVolume(0.5)
	return a
}

func TestLoginDialog(t *testing.T) {
	a := newTestApp()
	code, err := qr.Encode(netease.LoginURL("test-key"), qr.M)
	if err != nil {
		t.Fatal(err)
	}
	a.login = loginState{open: true, code: code, status: netease.QRWaiting}
	tt := ui.NewTester(a.view, 1200, 800)
	for _, s := range []string{"扫码登录", "打开网易云音乐 App，扫一扫登录", "登录网易云音乐"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
	a.login.status, a.login.scanner = netease.QRScanned, "小明"
	tt.Frame()
	if !tt.HasText("请在手机上确认登录") || !tt.HasText("小明") {
		t.Errorf("scanned: %q", tt.Texts())
	}
	a.login.status = netease.QRExpired
	tt.Frame()
	if !tt.HasText("二维码已失效") || !tt.HasText("刷新二维码") {
		t.Errorf("expired: %q", tt.Texts())
	}
}

func TestSignedInSidebar(t *testing.T) {
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	a.mine = []netease.Playlist{{ID: 10, Name: "小明喜欢的音乐"}}
	a.saved = []netease.Playlist{{ID: 11, Name: "深夜电台"}}
	tt := ui.NewTester(a.view, 1200, 800)
	for _, s := range []string{"小明", "我的歌单", "小明喜欢的音乐", "收藏的歌单", "深夜电台"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("登录网易云音乐") {
		t.Error("shows the sign-in row while signed in")
	}
}

func TestSettingsView(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.settingsView, 560, 600)
	for _, s := range []string{"刘海播放器", "显示刘海播放器", "播放时显示实时活动", "在无刘海的屏幕上常驻显示", "音质", "极高（320k）", "外观", "跟随系统", "扫码登录"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}
