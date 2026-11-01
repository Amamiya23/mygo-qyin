package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Opening the guest account menu must not start login until its login action is chosen.
func TestGuestAccountMenuLogin(t *testing.T) {
	asked := make(chan struct{}, 1)
	begin := beginLoginQR
	beginLoginQR = func() (string, error) { asked <- struct{}{}; return "", errors.New("offline") }
	t.Cleanup(func() { beginLoginQR = begin })
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("账户"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.accountOpen || a.login.open || len(asked) != 0 {
		t.Fatal("opening the account menu started login")
	}
	if !tt.Focused("登录 QQ 音乐") {
		t.Fatal("the guest menu did not focus its login action")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.accountOpen {
		t.Fatal("Escape left the guest menu open")
	}
	if err := tt.Click("账户"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("登录 QQ 音乐"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.accountOpen || !a.login.open {
		t.Fatal("login did not replace the guest menu")
	}
	wait(t, asked, "the guest login QR")
}

// Settings from either account menu uses the main route without duplicating history or creating a window.
func TestAccountMenuSettingsPage(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS opens a native settings window")
	}
	for _, signedIn := range []bool{false, true} {
		t.Run(fmt.Sprint(signedIn), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			account := "账户"
			if signedIn {
				a.profile = &qqmusic.Profile{UserID: 1, Nickname: "小明"}
				account = "小明"
			}
			tt := ui.NewTester(a.view, 1180, 780)
			for range 2 {
				if err := tt.Click(account); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if err := tt.Click("设置"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if a.route.page != pageSettings || a.settingsWin != nil || a.accountOpen || a.login.open {
					t.Fatalf("settings did not open in the main window: route %+v", a.route)
				}
				if len(a.history) != 1 {
					t.Fatalf("settings history = %v", a.history)
				}
			}
			a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "宝船"}})
			a.nowPlaying = true
			a.openSettings()
			tt.Frame()
			if a.nowPlaying || len(a.history) != 1 {
				t.Fatal("reopening settings did not dismiss Now Playing without duplicating history")
			}
		})
	}
}

// Back and Forward restore the actual scroll positions of both settings and the music page.
func TestSettingsNavigationRestoresScroll(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS opens a native settings window")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	var cards []string
	for i := range 40 {
		name := fmt.Sprintf("浏览歌单%02d", i+1)
		cards = append(cards, name)
		a.charts = append(a.charts, qqmusic.Playlist{ID: int64(i + 1), Name: name})
	}
	a.open(route{page: pageCharts})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Scroll(1000, 550, 0, 600)
	tt.Frame()
	card, cardAt := visibleNavigationItem(t, tt, cards)
	a.openSettings()
	tt.Frame()
	tt.Scroll(1000, 550, 0, 500)
	tt.Frame()
	control, controlAt := visibleNavigationItem(t, tt, []string{"更改缓存目录", "选择进度条特效", "烟雾背景"})
	for range 2 {
		tt.Key(0, ui.KeyBack)
		tt.Frame()
		if a.route.page != pageCharts {
			t.Fatalf("Back opened %+v", a.route)
		}
		assertNavigationPosition(t, tt, card, cardAt)
		tt.Key(0, ui.KeyForward)
		tt.Frame()
		if a.route.page != pageSettings {
			t.Fatalf("Forward opened %+v", a.route)
		}
		assertNavigationPosition(t, tt, control, controlAt)
	}
}

// Embedded settings must let Space toggle a focused switch rather than pause the playing song.
func TestSettingsPageSpaceTogglesControl(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.route = route{page: pageSettings}
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "宝船"}})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()
	tt := ui.NewTester(a.view, 1180, 780)
	if err := tt.Click("切换音量平衡"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	before := a.settings.Normalize
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.settings.Normalize == before || !a.pl.Playing() {
		t.Fatal("Space was taken from the focused setting by the playback shortcut")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.Normalize != a.settings.Normalize {
		t.Fatalf("keyboard change was not saved: %v", err)
	}
}

// The directory picker must be attached to whichever window hosts settings.
func TestCacheDirectoryPickerHost(t *testing.T) {
	picker := pickCacheDirectory
	defer func() { pickCacheDirectory = picker }()
	a := newTestApp()
	a.win = &mygo.Window{}
	for _, detached := range []bool{false, true} {
		parent := a.win
		if detached {
			a.settingsWin = &mygo.Window{}
			parent = a.settingsWin
		}
		pickCacheDirectory = func(opts mygo.OpenDialogOptions) ([]string, error) {
			if opts.Parent != parent {
				t.Fatal("picker was attached to the wrong settings host")
			}
			return nil, nil
		}
		a.chooseCacheDirectory()
	}
}

// With NM_FRAMES set, saves embedded settings at narrow, regular and wide window sizes.
func TestSettingsPageFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	for _, dark := range []bool{false, true} {
		for _, width := range []int{880, 1180, 1600} {
			a := newTestApp()
			a.settings = defaultSettings()
			a.tray = &mygo.Tray{}
			a.updates = true
			a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Duration: 200000}})
			a.open(route{page: pageSettings})
			tt := ui.NewTester(a.view, width, 780)
			tt.SetDark(dark)
			tt.Frame()
			for _, bottom := range []bool{false, true} {
				if bottom {
					tt.Scroll(float32(width-100), 550, 0, 2000)
					tt.Frame()
				}
				name := fmt.Sprintf("settings-page-%d-dark-%t-bottom-%t.png", width, dark, bottom)
				f, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("write frame: %v, %v", err, closeErr)
				}
			}
		}
	}
}
