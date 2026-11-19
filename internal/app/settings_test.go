package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Choosing how the main panel closes updates and saves the choice in both directions.
func TestQuitOnCloseSettings(t *testing.T) {
	a := newTestApp()
	a.tray = &mygo.Tray{} // A successfully created tray makes hiding available.
	a.settings = defaultSettings()
	a.settings.QuitOnClose = false
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	keepRunning := "最小化到托盘"
	switch runtime.GOOS {
	case "darwin":
		keepRunning = "保持后台运行"
	case "linux":
		keepRunning = "隐藏到托盘"
	}
	for _, choice := range []struct {
		label       string
		quitOnClose bool
	}{
		{"退出应用", true},
		{keepRunning, false},
		{"退出应用", true},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.QuitOnClose != choice.quitOnClose {
			t.Fatalf("after choosing %q, QuitOnClose = %t, want %t", choice.label, a.settings.QuitOnClose, choice.quitOnClose)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		restored.QuitOnClose = !choice.quitOnClose
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.QuitOnClose != choice.quitOnClose {
			t.Fatalf("after choosing %q, restored QuitOnClose = %t, want %t", choice.label, restored.QuitOnClose, choice.quitOnClose)
		}
	}
}

// Toggling playback fading saves the selected behavior for the next launch.
func TestPlaybackFadeSettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	for _, want := range []bool{false, true, false} {
		if err := tt.Click("切换播放操作淡入淡出"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if a.settings.PlaybackFade != want || restored.PlaybackFade != want {
			t.Fatalf("playback fade: live=%t saved=%t, want %t", a.settings.PlaybackFade, restored.PlaybackFade, want)
		}
	}
}

// Choosing a volume-balance mode applies it to the current player and persists across launches.
func TestVolumeBalanceSettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.setProgressEffect(progressSquiggly)
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	var applied []string
	a.pl.Normalize = func() string {
		mode := a.settings.volumeBalance()
		applied = append(applied, mode)
		return mode
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	for _, choice := range []struct{ label, mode string }{
		{"关闭", volumeBalanceOff},
		{"高音量", volumeBalanceHigh},
		{"标准", volumeBalanceStandard},
		{"安静", volumeBalanceQuiet},
	} {
		before := len(applied)
		if err := tt.Click("选择音量平衡"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := a.settings.volumeBalance(); got != choice.mode {
			t.Fatalf("volume balance = %q, want %q", got, choice.mode)
		}
		if len(applied) <= before || applied[len(applied)-1] != choice.mode {
			t.Fatalf("current player did not apply volume balance %q: %v", choice.mode, applied)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		restored.Normalize = choice.mode != volumeBalanceOff
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if got := restored.volumeBalance(); got != choice.mode {
			t.Fatalf("restored volume balance = %q, want %q", got, choice.mode)
		}
	}
}

// The old volume-balance switch becomes the standard mode when it was on.
func TestVolumeBalanceMigration(t *testing.T) {
	for _, tc := range []struct {
		legacy bool
		want   string
	}{
		{true, volumeBalanceStandard},
		{false, volumeBalanceOff},
	} {
		s := defaultSettings()
		s.VolumeBalance = ""
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"volumeBalance":%t}`, tc.legacy)), &s); err != nil {
			t.Fatal(err)
		}
		if got := s.volumeBalance(); got != tc.want {
			t.Fatalf("legacy switch %t selects %q, want %q", tc.legacy, got, tc.want)
		}
	}
}

// With NM_FRAMES set, saves settings and startup migration progress in both themes.
func TestSettingsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	for _, dark := range []bool{false, true} {
		for _, width := range []int{480, 560, 880} {
			a := newTestApp()
			a.tray = &mygo.Tray{}
			a.settings = defaultSettings()
			a.settings.QuitOnClose = dark
			a.settings.CacheDir = filepath.Join(t.TempDir(), "音乐缓存")
			a.settings.CacheMB = 5120
			a.pl.Cache = newSongCache(a.settings)
			a.cacheUsed = 5 << 30
			if dark {
				pending := filepath.Join(t.TempDir(), "另一个磁盘")
				a.settings.PendingCacheDir = &pending
			}
			a.updates = true
			view, height := a.settingsView, 1400
			if width == 880 {
				a.cacheMoving = true
				a.cacheMoveDone, a.cacheMoveTotal = 2<<30, 5<<30
				view, height = a.view, 580
			}
			tt := ui.NewTester(view, width, height)
			tt.SetScale(2)
			tt.SetDark(dark)
			tt.Frame()
			appearance := "light"
			if dark {
				appearance = "dark"
			}
			name := fmt.Sprintf("settings-%d-%s.png", width, appearance)
			if a.cacheMoving {
				name = "cache-migration-" + appearance + ".png"
			}
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
	}
}

// Switching between sidebar materials saves the chosen material for the next launch.
func TestSidebarMaterialSettings(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("desktop sidebar material settings")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	choices := []struct {
		label    string
		material string
		vibrancy mygo.Vibrancy
	}{
		{"Mica", "mica", mygo.VibrancyMica},
		{"纯色", "solid", mygo.VibrancyNone},
		{"亚克力", "acrylic", mygo.VibrancyAcrylic},
		{"Mica", "mica", mygo.VibrancyMica},
	}
	if runtime.GOOS == "darwin" {
		choices = []struct {
			label    string
			material string
			vibrancy mygo.Vibrancy
		}{
			{"纯色", "solid", mygo.VibrancyNone},
			{"系统材质", "sidebar", mygo.VibrancySidebar},
			{"纯色", "solid", mygo.VibrancyNone},
		}
	}
	for _, choice := range choices {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.SidebarMaterial != choice.material {
			t.Fatalf("selected %q, want %q", a.settings.SidebarMaterial, choice.material)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.SidebarMaterial != choice.material || restored.sidebarVibrancy() != choice.vibrancy {
			t.Fatalf("restored material %q (%q), want %q (%q)", restored.SidebarMaterial, restored.sidebarVibrancy(), choice.material, choice.vibrancy)
		}
	}
}

// Choosing a progress effect from the dropdown saves it across launches and replaces the old smoke-only choice.
func TestProgressEffectSettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.setVolumeBalance(volumeBalanceHigh)
	if err := json.Unmarshal([]byte("{\"smoke\":true}"), &a.settings); err != nil {
		t.Fatal(err)
	}
	if got := a.settings.progressEffect(); got != progressSmoke {
		t.Fatalf("old smoke setting selects %q, want %q", got, progressSmoke)
	}
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	for _, choice := range []struct{ label, effect string }{
		{"波浪", progressSquiggly},
		{"标准", progressStandard},
		{"烟雾", progressSmoke},
		{"波浪", progressSquiggly},
	} {
		if err := tt.Click("选择进度条特效"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := a.settings.progressEffect(); got != choice.effect {
			t.Fatalf("after choosing %q, progress effect = %q, want %q", choice.label, got, choice.effect)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if got := restored.progressEffect(); got != choice.effect {
			t.Fatalf("after choosing %q, restored effect = %q, want %q", choice.label, got, choice.effect)
		}
		if restored.Smoke != (choice.effect == progressSmoke) {
			t.Fatalf("after choosing %q, saved smoke compatibility = %t", choice.label, restored.Smoke)
		}
	}
}

func TestHiResSettingsPersist(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 900, 2000)
	tt.Frame()
	for _, choice := range []struct{ label, level string }{{"Hi-Res（FLAC）", "hires"}, {"无损（FLAC）", "lossless"}, {"Hi-Res（FLAC）", "hires"}} {
		if err := tt.Click("选择音质"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved settings
		if err := json.Unmarshal(b, &saved); err != nil {
			t.Fatal(err)
		}
		if a.settings.Quality != choice.level || saved.Quality != choice.level {
			t.Fatalf("live=%s saved=%s want=%s", a.settings.Quality, saved.Quality, choice.level)
		}
	}
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		if err := tt.Click("选择音质"); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "hires-settings.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQualityBadgeUsesActualStreamLevel(t *testing.T) {
	st := &qqmusic.Stream{}
	tt := ui.NewTester(func(c *ui.Context) { qualityBadge(c, st) }, 200, 80)
	for _, level := range []string{"hires", "lossless", "standard"} {
		st.Level = level
		tt.Frame()
		_, hiRes := tt.Find("Hi-Res")
		_, lossless := tt.Find("无损")
		if hiRes != (level == "hires") || lossless != (level == "lossless") {
			t.Fatalf("incorrect badge for %s", level)
		}
	}
}

// fakeUpdater swaps the update plugin's preferences for ones in memory,
// counting the checks asked for, until the returned function puts them
// back.
func fakeUpdater(checks, downloads *bool, checked *int) func() {
	check, getChecks, setChecks := checkForUpdates, updateChecks, setUpdateChecks
	getDownloads, setDownloads, last := updateDownloads, setUpdateDownloads, lastUpdateCheck
	checkForUpdates = func() { *checked++ }
	updateChecks = func() bool { return *checks }
	setUpdateChecks = func(on bool) { *checks = on }
	updateDownloads = func() bool { return *downloads }
	setUpdateDownloads = func(on bool) { *downloads = on }
	lastUpdateCheck = func() time.Time { return time.Time{} }
	return func() {
		checkForUpdates, updateChecks, setUpdateChecks = check, getChecks, setChecks
		updateDownloads, setUpdateDownloads, lastUpdateCheck = getDownloads, setDownloads, last
	}
}

// Turning off checking for updates turns it off in the updater, and
// installing updates without asking can't be turned on without it.
func TestTurnOffUpdateChecks(t *testing.T) {
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	a := newTestApp()
	a.settings = defaultSettings()
	a.updates = true
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	// The row's title is labelled as its switch is: click the switch, at
	// the row's end.
	toggle := func(title string) {
		t.Helper()
		bounds, ok := tt.Find(title)
		if !ok {
			t.Fatalf("no row %s", title)
		}
		tt.ClickAt(560-28-16-12, bounds.Y+bounds.H/2)
		tt.Frame()
	}
	toggle("自动检查更新")
	if checks {
		t.Fatal("checks for updates still")
	}
	toggle("自动下载并安装")
	if downloads {
		t.Fatal("turned on installing updates without checking for them")
	}
	// It does turn on while the updater checks.
	checks = true
	tt.Frame()
	toggle("自动下载并安装")
	if !downloads {
		t.Fatal("didn't turn on installing updates")
	}
}

// 检查更新 in the settings checks for updates as the user asked.
func TestCheckForUpdatesFromSettings(t *testing.T) {
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	a := newTestApp()
	a.settings = defaultSettings()
	a.updates = true
	a.profile = &qqmusic.Profile{UserID: 1}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked %d times", checked)
	}
}
