package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/player"
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

// Settings from the sidebar uses the main route without duplicating history or creating a window.
func TestSidebarSettingsPage(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS opens a native settings window")
	}
	for _, signedIn := range []bool{false, true} {
		t.Run(fmt.Sprint(signedIn), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			if signedIn {
				a.profile = &qqmusic.Profile{UserID: 1, Nickname: "小明"}
			}
			tt := ui.NewTester(a.view, 1180, 780)
			for range 2 {
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
	if err := tt.Click("切换热评"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	before := a.settings.HotComments
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.settings.HotComments == before || !a.pl.Playing() {
		t.Fatal("Space was taken from the focused setting by the playback shortcut")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.HotComments != a.settings.HotComments {
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

// Choosing a directory stages a migration; the originals are removed only after the copied cache is committed.
func TestCacheDirectorySettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	oldCache := a.pl.Cache
	writeCacheFixture(t, filepath.Join(oldCache.Directory(), "1-exhigh.mp3"), "old song")
	writeCacheFixture(t, filepath.Join(oldCache.Directory(), "1-exhigh.json"), `{"level":"exhigh","type":"mp3","size":8}`)

	selected := filepath.Join(t.TempDir(), "音乐缓存")
	writeCacheFixture(t, filepath.Join(selected, "Lilt", "songs", "2-exhigh.mp3"), "new song")
	userSong := filepath.Join(selected, "my-music.mp3")
	writeCacheFixture(t, userSong, "personal music")
	picker := pickCacheDirectory
	defer func() { pickCacheDirectory = picker }()
	pickCacheDirectory = func(opts mygo.OpenDialogOptions) ([]string, error) {
		if !opts.Directory || opts.Multiple || opts.DefaultPath != a.settings.CacheDir {
			t.Fatalf("picker options: %+v", opts)
		}
		return []string{selected}, nil
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("更改缓存目录"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PendingCacheDir == nil || *a.settings.PendingCacheDir != selected || a.pl.Cache != oldCache {
		t.Fatal("selection did not stage migration while keeping the current cache")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	restored := defaultSettings()
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.PendingCacheDir == nil || *restored.PendingCacheDir != selected || restored.CacheDir != a.settings.CacheDir {
		t.Fatalf("restored cache choices: %+v", restored)
	}
	restored.CacheDir = *restored.PendingCacheDir
	target := newSongCache(restored)
	cleanup, err := oldCache.CopyTo(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if oldCache.Size() != int64(len("old song")) {
		t.Fatal("copying removed the source before committing settings")
	}
	if err := a.commitCacheDirectory(selected, target); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if a.settings.CacheDir != selected || a.settings.PendingCacheDir != nil || a.pl.Cache != target || oldCache.Size() != 0 {
		t.Fatal("completed migration did not select the destination and clean the originals")
	}
	b, err = os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.CacheDir != selected || saved.PendingCacheDir != nil {
		t.Fatalf("committed settings: %+v, %v", saved, err)
	}
	if target.Size() != int64(len("new song")+len("old song")) {
		t.Fatalf("migrated cache size = %d", target.Size())
	}
	target.Clear()
	if b, err := os.ReadFile(userSong); err != nil || string(b) != "personal music" {
		t.Fatalf("user music changed: %q, %v", b, err)
	}
}

// Cancelled, failed, or unwritable selections leave the live cache and persisted path unchanged.
func TestCacheDirectorySelectionFailure(t *testing.T) {
	for _, scenario := range []string{"cancel", "picker error", "blocked directory", "save error"} {
		t.Run(scenario, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.CacheDir = t.TempDir()
			a.settings.path = filepath.Join(t.TempDir(), "settings.json")
			if err := a.settings.save(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(a.settings.path)
			if err != nil {
				t.Fatal(err)
			}
			a.pl.Cache = newSongCache(a.settings)
			previousCache, previousDir := a.pl.Cache, a.settings.CacheDir
			selected := t.TempDir()
			if scenario == "blocked directory" {
				writeCacheFixture(t, filepath.Join(selected, "Lilt"), "a file blocks the directory")
			}
			if scenario == "save error" {
				blocker := filepath.Join(t.TempDir(), "file")
				writeCacheFixture(t, blocker, "a file blocks settings writes")
				a.settings.path = filepath.Join(blocker, "settings.json")
			}
			picker := pickCacheDirectory
			defer func() { pickCacheDirectory = picker }()
			pickCacheDirectory = func(mygo.OpenDialogOptions) ([]string, error) {
				switch scenario {
				case "cancel":
					return nil, nil
				case "picker error":
					return nil, errors.New("picker unavailable")
				default:
					return []string{selected}, nil
				}
			}
			tt := ui.NewTester(a.settingsView, 560, 2000)
			tt.Frame()
			if err := tt.Click("更改缓存目录"); err != nil {
				t.Fatal(err)
			}
			if a.pl.Cache != previousCache || a.settings.CacheDir != previousDir {
				t.Fatal("unsuccessful selection changed the cache")
			}
			if scenario != "cancel" && a.cacheDirError == "" {
				t.Fatal("the failed selection was not reported")
			}
			if scenario != "save error" {
				after, err := os.ReadFile(a.settings.path)
				if err != nil || string(after) != string(before) {
					t.Fatalf("persisted settings changed: %v", err)
				}
			}
		})
	}
}

// Restoring the default stages migration to the platform cache without interrupting current playback.
func TestRestoreDefaultCacheDirectory(t *testing.T) {
	defaultRoot := t.TempDir()
	mygo.App.SetPath(mygo.PathCache, defaultRoot)
	defer mygo.App.SetPath(mygo.PathCache, "")
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("恢复默认缓存目录"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PendingCacheDir == nil || *a.settings.PendingCacheDir != "" || a.pl.Cache.Directory() == filepath.Join(defaultRoot, "songs") {
		t.Fatal("restoring the default did not defer migration until restart")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.PendingCacheDir == nil || *saved.PendingCacheDir != "" || saved.CacheDir != a.settings.CacheDir {
		t.Fatalf("saved custom directory = %q, %v", saved.CacheDir, err)
	}
}

// A measurement finishing after a directory switch must not replace the new directory's usage.
func TestCacheMeasurementAfterDirectoryChange(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	oldCache := newSongCache(a.settings)
	a.settings.CacheDir = t.TempDir()
	a.pl.Cache = newSongCache(a.settings)
	a.cacheMeasured(a.pl.Cache, 15)
	a.cacheMeasured(oldCache, 99)
	if a.cacheUsed != 15 {
		t.Fatalf("usage = %d, want the current cache's 15", a.cacheUsed)
	}
}

// Cancelling a pending directory change leaves playback and saved settings on the original cache.
func TestCancelCacheDirectoryChange(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	original := a.pl.Cache
	if err := a.setCacheDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("取消缓存目录更改"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PendingCacheDir != nil || a.pl.Cache != original {
		t.Fatal("cancellation did not retain the original cache")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var restored settings
	if err := json.Unmarshal(b, &restored); err != nil || restored.PendingCacheDir != nil {
		t.Fatalf("restored pending directory: %v, %v", restored.PendingCacheDir, err)
	}
}

// If the completed migration cannot be saved, the original cache and pending choice stay selected.
func TestCacheDirectoryCommitFailure(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	destination := t.TempDir()
	a.settings.PendingCacheDir = &destination
	blocker := filepath.Join(t.TempDir(), "file")
	writeCacheFixture(t, blocker, "settings writes fail here")
	a.settings.path = filepath.Join(blocker, "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	original := a.pl.Cache
	chosen := a.settings
	chosen.CacheDir = destination
	if err := a.commitCacheDirectory(destination, newSongCache(chosen)); err == nil {
		t.Fatal("the failed settings write was ignored")
	}
	if a.pl.Cache != original || a.settings.PendingCacheDir == nil || a.settings.CacheDir == destination {
		t.Fatal("failed commit changed the selected cache")
	}
}

// Clearing from settings during migration used to delete the source while it was being copied.
func TestCacheClearDuringMigration(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.pl.Cache = newSongCache(a.settings)
	writeCacheFixture(t, filepath.Join(a.pl.Cache.Directory(), "1-exhigh.mp3"), "source")
	a.cacheUsed, a.cacheMoving = 6, true
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("清除"); err != nil {
		t.Fatal(err)
	}
	if a.cacheUsed != 6 || a.pl.Cache.Size() != 6 {
		t.Fatal("clearing during migration modified the source cache")
	}
}

func writeCacheFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCacheLimitSettingsLabelAndSharedUsage(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	a.art.disk = a.pl.Cache
	if !a.pl.Cache.KeepCover("https://example.com/cover.jpg", []byte("cover-bytes")) {
		t.Fatal("failed to cache cover")
	}
	if !a.pl.Cache.KeepLyrics(1, "[00:01.00]test", "", "") {
		t.Fatal("failed to cache lyrics")
	}
	a.cacheUsed = a.pl.Cache.Size()
	if a.cacheUsed <= 0 {
		t.Fatalf("expected shared cache usage > 0, got %d", a.cacheUsed)
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if _, ok := tt.Find("歌曲缓存"); ok {
		t.Fatal("old label 歌曲缓存 is still present")
	}
	if _, ok := tt.Find("缓存上限"); !ok {
		t.Fatal("new label 缓存上限 not found")
	}
	if err := tt.Click("选择缓存上限"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.CacheMB != 0 {
		t.Fatalf("CacheMB = %d, want 0", a.settings.CacheMB)
	}
}

// The playback fade switch must control quitting as well as pause and resume.
func TestQuitAudioFollowsPlaybackFadeSetting(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	previous := shutdownAudio
	defer func() { shutdownAudio = previous }()
	var durations []time.Duration
	shutdownAudio = func(ctx context.Context, fade time.Duration) error {
		if ctx.Err() != nil {
			t.Fatal("shutdown received an expired context")
		}
		durations = append(durations, fade)
		return nil
	}
	a.finishAudio()
	if err := tt.Click("切换播放操作淡入淡出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.finishAudio()
	if len(durations) != 2 || durations[0] != player.FadeOutDuration || durations[1] != 0 {
		t.Fatalf("shutdown fade durations = %v", durations)
	}
}

func TestMainWindowCloseUsesConfiguredExitBehavior(t *testing.T) {
	a := newTestApp()
	a.tray = &mygo.Tray{}
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	// Verify both the default hide behavior and the real settings control.
	for _, exit := range []bool{false, true} {
		if exit {
			if err := tt.Click("退出应用"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
		}
		hides, quits := 0, 0
		e := &mygo.CloseEvent{}
		a.mainWindowClose(e, func() { hides++ }, func() { quits++ })
		if !e.DefaultPrevented() || (exit && (quits != 1 || hides != 0)) || (!exit && (hides != 1 || quits != 0)) {
			t.Fatalf("exit=%t: hides=%d quits=%d", exit, hides, quits)
		}
	}
	// Without a usable tray/Dock, hiding must never strand the application.
	a.tray = nil
	a.settings.QuitOnClose = false
	if !a.canHideOnClose() {
		quits := 0
		a.mainWindowClose(&mygo.CloseEvent{}, func() { t.Fatal("hid without a tray") }, func() { quits++ })
		if quits != 1 {
			t.Fatal("did not quit without a way to restore the window")
		}
	}
	// Closing windows during a real quit must neither hide nor re-enter Quit.
	a.quitting = true
	e := &mygo.CloseEvent{}
	a.mainWindowClose(e, func() { t.Fatal("hid during quit") }, func() { t.Fatal("recursive quit") })
	if e.DefaultPrevented() {
		t.Fatal("window prevented final quit")
	}
}
