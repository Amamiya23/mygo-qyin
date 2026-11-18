package app

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
	"lilt/internal/qqmusic"
)

// Changing the taskbar panel material persists both choices without closing controls or committing a pending seek.
func TestTaskbarLyricsMaterialSettings(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a, d := taskbarControlsFixture()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	d.controlsOpen = true
	d.scrub = scrubState{v: 120, held: true, active: true}
	tt := ui.NewTester(a.settingsView, 560, 2400)
	tt.Frame()
	for _, choice := range []struct {
		label, material string
		vibrancy        mygo.Vibrancy
	}{
		{"白色", taskbarMaterialWhite, mygo.VibrancyNone},
		{"亚克力", taskbarMaterialAcrylic, mygo.VibrancyAcrylic},
		{"白色", taskbarMaterialWhite, mygo.VibrancyNone},
	} {
		if choice.material == taskbarMaterialAcrylic {
			// Move within the focused taskbar selector; the sidebar also has an Acrylic segment.
			tt.Key(0, ui.KeyLeft)
		} else if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyricsMaterial != choice.material || restored.TaskbarLyricsMaterial != choice.material || restored.taskbarControlsVibrancy() != choice.vibrancy {
			t.Fatalf("material not applied/saved: live=%q saved=%q backdrop=%q", a.settings.TaskbarLyricsMaterial, restored.TaskbarLyricsMaterial, restored.taskbarControlsVibrancy())
		}
		if !d.controlsOpen || !d.scrub.held || !d.scrub.active || !a.settings.TaskbarLyrics || a.pl.Position() != 40*time.Second {
			t.Fatal("material change closed controls, changed lyrics mode, or disturbed the pending seek")
		}
	}
}

// Choosing the taskbar edge saves the alignment and refreshes the live monitor snapshot.
func TestTaskbarLyricsAlignSettings(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.TaskbarLyrics = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.syncTaskbarLyrics()
	d := a.taskbarLyrics
	if d == nil {
		t.Fatal("taskbar mode did not create its owner")
	}
	refreshed := 0
	d.stop, d.refresh = func() {}, func() { refreshed++ }
	tt := ui.NewTester(a.settingsView, 560, 2400)
	tt.Frame()
	for _, choice := range []struct{ label, align string }{
		{"靠右", taskbarAlignRight},
		{"靠左", taskbarAlignLeft},
		{"靠右", taskbarAlignRight},
	} {
		before := refreshed
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.taskbarLyricsAlign() != choice.align || restored.taskbarLyricsAlign() != choice.align {
			t.Fatalf("alignment %q not applied/saved: live=%q saved=%q", choice.label, a.settings.TaskbarLyricsAlign, restored.TaskbarLyricsAlign)
		}
		if d.loadOptions().align != choice.align || refreshed != before+1 {
			t.Fatalf("alignment %q did not reach the monitor: options=%+v refreshed=%d", choice.label, d.loadOptions(), refreshed)
		}
		if a.settings.lyricsMode() != lyricsTaskbar {
			t.Fatal("alignment change disturbed the lyrics mode")
		}
	}
}

// The mirror switch exists only beside the right edge, saves, and stays inert on the left.
func TestTaskbarLyricsMirrorSettings(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.TaskbarLyrics = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.syncTaskbarLyrics()
	d := a.taskbarLyrics
	refreshed := 0
	d.stop, d.refresh = func() {}, func() { refreshed++ }
	tt := ui.NewTester(a.settingsView, 560, 2400)
	tt.Frame()
	if _, ok := tt.Find("靠右时镜像布局"); ok {
		t.Fatal("mirror switch visible while left-aligned")
	}
	if err := tt.Click("靠右"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []bool{true, false, true} {
		before := refreshed
		if err := tt.Click("切换靠右时镜像布局"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyricsMirror != want || restored.TaskbarLyricsMirror != want {
			t.Fatalf("mirror not applied/saved: live=%t saved=%t want=%t", a.settings.TaskbarLyricsMirror, restored.TaskbarLyricsMirror, want)
		}
		if a.settings.taskbarLyricsMirrored() != want || refreshed <= before {
			t.Fatalf("mirror %t did not reach the monitor: mirrored=%t refreshed=%d", want, a.settings.taskbarLyricsMirrored(), refreshed)
		}
	}
	if err := tt.Click("靠左"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("靠右时镜像布局"); ok {
		t.Fatal("mirror switch survived the left edge")
	}
	if a.settings.taskbarLyricsMirrored() {
		t.Fatal("saved mirror stayed active on the left edge")
	}
}

// A taskbar without room hides the lyrics and says so once, re-arming after recovery.
func TestTaskbarLyricsNoSpaceToast(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.TaskbarLyrics = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000}})
	a.syncTaskbarLyrics()
	d := a.taskbarLyrics
	d.stop, d.refresh = func() {}, func() {}
	noSpace := taskbarLyricsPlacement{noSpace: true}
	usable := taskbarLyricsPlacement{parent: 1, taskbar: 2, width: 280, height: 42, dpi: 96}
	const msg = "任务栏空间不足，无法显示任务栏歌词，可改用桌面歌词"
	d.updatePlacement(noSpace)
	if a.toast != msg || d.visible {
		t.Fatalf("no space did not warn once: toast=%q visible=%t", a.toast, d.visible)
	}
	a.toast = ""
	d.updatePlacement(noSpace)
	d.updatePlacement(taskbarLyricsPlacement{})
	if a.toast != "" {
		t.Fatalf("repeat or unrelated samples re-warned: %q", a.toast)
	}
	if d.notePlacementSpace(usable) {
		t.Fatal("usable placement asked for a warning")
	}
	d.updatePlacement(noSpace)
	if a.toast != msg {
		t.Fatalf("recovered space did not re-arm the warning: %q", a.toast)
	}
	a.toast = ""
	d.spaceWarned = false
	trackless := newTestApp()
	trackless.settings.TaskbarLyrics = true
	d.a = trackless
	d.updatePlacement(noSpace)
	if trackless.toast != "" {
		t.Fatalf("warned without a track: %q", trackless.toast)
	}
	d.a = a
	a.settings.TaskbarLyrics = false
	d.updatePlacement(noSpace)
	if a.toast != "" {
		t.Fatalf("warned while disabled: %q", a.toast)
	}
}

// Typing a horizontal offset saves whole pixels and refreshes the live monitor snapshot.
func TestTaskbarLyricsOffsetSettings(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.TaskbarLyrics = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.syncTaskbarLyrics()
	d := a.taskbarLyrics
	refreshed := 0
	d.stop, d.refresh = func() {}, func() { refreshed++ }
	tt := ui.NewTester(a.settingsView, 560, 2400)
	tt.Frame()
	for _, want := range []float64{-120, 80, 0} {
		before := refreshed
		if err := tt.Click("输入歌词水平偏移"); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(strconv.FormatFloat(want, 'f', -1, 64))
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyricsOffsetX != want || restored.TaskbarLyricsOffsetX != want {
			t.Fatalf("offset not applied/saved: live=%v saved=%v, want %v", a.settings.TaskbarLyricsOffsetX, restored.TaskbarLyricsOffsetX, want)
		}
		if d.loadOptions().offsetX != int(want) || refreshed <= before {
			t.Fatalf("offset %v did not reach the monitor: options=%+v refreshed=%d", want, d.loadOptions(), refreshed)
		}
	}
	// Out-of-range typing keeps the last in-range prefix; losing focus reformats the field.
	tt.Key(ui.Cmd, ui.KeyA)
	for _, r := range "5001" {
		tt.Type(string(r))
		tt.Frame()
	}
	if a.settings.TaskbarLyricsOffsetX != 500 {
		t.Fatalf("out-of-range offset applied: %v", a.settings.TaskbarLyricsOffsetX)
	}
	if err := tt.Click("歌词水平偏移"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// The saved monitor name resolves to its current handle; unknown names keep the primary taskbar.
func TestTaskbarLyricsDisplayID(t *testing.T) {
	displays := []mygo.Display{
		{ID: 11, Label: `\\.\DISPLAY1`, Primary: true},
		{ID: 22, Label: `\\.\DISPLAY2`},
	}
	for _, tc := range []struct {
		label string
		want  int64
	}{
		{"", 0},
		{`\\.\DISPLAY2`, 22},
		{`\\.\DISPLAY1`, 11},
		{`\\.\DISPLAY3`, 0},
	} {
		if got := taskbarLyricsDisplayID(displays, tc.label); got != tc.want {
			t.Fatalf("label %q resolved to %d, want %d", tc.label, got, tc.want)
		}
	}
	if got := taskbarLyricsDisplayID(nil, `\\.\DISPLAY2`); got != 0 {
		t.Fatalf("disconnected monitor resolved to %d, want the primary taskbar", got)
	}
}

// Monitor labels name the primary screen and show native resolutions.
func TestTaskbarDisplayLabel(t *testing.T) {
	primary := mygo.Display{ID: 1, Primary: true, Bounds: mygo.Rectangle{Width: 1920, Height: 1080}, ScaleFactor: 1}
	secondary := mygo.Display{ID: 2, Bounds: mygo.Rectangle{Width: 1280, Height: 720}, ScaleFactor: 2}
	if got := taskbarDisplayLabel(0, primary); got != "主屏幕（1920×1080）" {
		t.Fatalf("primary label = %q", got)
	}
	if got := taskbarDisplayLabel(1, secondary); got != "屏幕 2（2560×1440）" {
		t.Fatalf("secondary label = %q", got)
	}
	if got := taskbarDisplayLabel(1, mygo.Display{}); got != "屏幕 2" {
		t.Fatalf("label without bounds = %q", got)
	}
}

// Choosing a screen saves its monitor name, retargets the live monitor, and falls back when it disconnects.
func TestTaskbarLyricsDisplaySettings(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.TaskbarLyrics = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.taskbarDisplays = []mygo.Display{
		{ID: 11, Label: `\\.\DISPLAY1`, Primary: true, Bounds: mygo.Rectangle{Width: 1920, Height: 1080}, ScaleFactor: 1},
		{ID: 22, Label: `\\.\DISPLAY2`, Bounds: mygo.Rectangle{Width: 2560, Height: 1440}, ScaleFactor: 1},
	}
	a.syncTaskbarLyrics()
	d := a.taskbarLyrics
	refreshed := 0
	d.stop, d.refresh = func() {}, func() { refreshed++ }
	tt := ui.NewTester(a.settingsView, 560, 2400)
	tt.Frame()
	for _, choice := range []struct {
		label, saved string
		display      int64
	}{
		{"屏幕 2（2560×1440）", `\\.\DISPLAY2`, 22},
		{"主屏幕（1920×1080）", "", 0},
		{"屏幕 2（2560×1440）", `\\.\DISPLAY2`, 22},
	} {
		before := refreshed
		if err := tt.Click("选择任务栏歌词屏幕"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyricsDisplay != choice.saved || restored.TaskbarLyricsDisplay != choice.saved {
			t.Fatalf("screen %q not applied/saved: live=%q saved=%q", choice.label, a.settings.TaskbarLyricsDisplay, restored.TaskbarLyricsDisplay)
		}
		if d.loadOptions().display != choice.display || refreshed != before+1 {
			t.Fatalf("screen %q did not reach the monitor: options=%+v refreshed=%d", choice.label, d.loadOptions(), refreshed)
		}
	}
	// The chosen monitor disconnects: the picker shows the primary and the monitor falls back.
	a.taskbarDisplays = a.taskbarDisplays[:1]
	a.taskbarLyricsOptionsChanged()
	tt.Frame()
	if d.loadOptions().display != 0 {
		t.Fatalf("disconnected monitor kept handle %d", d.loadOptions().display)
	}
	if err := tt.Click("选择任务栏歌词屏幕"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("主屏幕（1920×1080）"); err != nil {
		t.Fatalf("picker did not fall back to the primary screen: %v", err)
	}
	tt.Frame()
}

// Choosing a lyric position swaps the active owner, saves the mode, and stops the previous taskbar monitor.
func TestTaskbarLyricsSettingsModeSwitch(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.setDesktopLyricsEnabled(true)
	tt := ui.NewTester(a.settingsView, 560, 2200)
	tt.Frame()
	stopped := 0
	for _, choice := range []struct {
		label, mode string
		taskbar     bool
	}{
		{"任务栏歌词", "taskbar", true},
		{"桌面歌词", "desktop", false},
		{"任务栏歌词", "taskbar", true},
		{"桌面歌词", "desktop", false},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.lyricsMode() != choice.mode || restored.lyricsMode() != choice.mode {
			t.Fatalf("position %q was not applied and saved: live=%q saved=%q", choice.label, a.settings.lyricsMode(), restored.lyricsMode())
		}
		if a.settings.TaskbarLyrics != choice.taskbar || restored.TaskbarLyrics != choice.taskbar || a.settings.DesktopLyrics == choice.taskbar || restored.DesktopLyrics == choice.taskbar {
			t.Fatalf("position %q did not exclusively enable its lyrics: live=%+v saved=%+v", choice.label, a.settings, restored)
		}
		if (a.taskbarLyrics != nil) != choice.taskbar {
			t.Fatalf("taskbar owner lifecycle did not follow position %q", choice.label)
		}
		if choice.taskbar {
			a.taskbarLyrics.stop = func() { stopped++ }
		}
	}
	if stopped != 2 {
		t.Fatalf("disabling did not stop each monitor: stopped=%d", stopped)
	}
	a.setLyricsMode("taskbar")
	b := newTestApp()
	b.settings = readDesktopLyricsEntrySettings(t, a)
	b.settings.normalizeLyrics()
	b.syncTaskbarLyrics()
	if b.taskbarLyrics == nil {
		t.Fatal("restored preference did not recreate taskbar lyrics")
	}
}

// Choosing a taskbar position while lyrics are off saves the preference without opening either lyric window.
func TestTaskbarLyricsSettingsModeWhileDisabled(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2200)
	tt.Frame()
	for _, label := range []string{"任务栏歌词", "桌面歌词", "任务栏歌词"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.lyricsEnabled() || a.desktopLyrics != nil || a.taskbarLyrics != nil {
			t.Fatalf("choosing %q opened disabled lyrics", label)
		}
	}
	restored := readDesktopLyricsEntrySettings(t, a)
	if restored.lyricsMode() != "taskbar" || restored.lyricsEnabled() {
		t.Fatal("disabled taskbar preference did not survive saving")
	}
	player := desktopLyricsEntryTester(a)
	player.Frame()
	if err := player.Click("任务栏歌词"); err != nil {
		t.Fatal(err)
	}
	player.Frame()
	if !a.settings.TaskbarLyrics || a.settings.DesktopLyrics || a.taskbarLyrics == nil {
		t.Fatal("player switch did not open the saved taskbar mode")
	}
}

// Paused seeks invalidate visible taskbar lyrics, while a hidden taskbar requests no redraw.
func TestTaskbarLyricsPausedSeekRedraw(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]qqmusic.Track{{ID: 1, Duration: 60000}})
	out := audio.NewPlayer(&silence{})
	defer out.Close()
	a.pl.Attach(out)
	a.pl.TogglePlay()
	a.settings.TaskbarLyrics = true
	a.taskbarLyrics = &taskbarLyrics{a: a, win: &mygo.Window{}, visible: true}
	defer func(f func(*mygo.Window)) { invalidateWindow = f }(invalidateWindow)
	seen := 0
	invalidateWindow = func(w *mygo.Window) {
		if w == a.taskbarLyrics.win {
			seen++
		}
	}
	defer func(f func(time.Duration, func())) { sessionAfter = f }(sessionAfter)
	sessionAfter = func(time.Duration, func()) {}
	a.pl.Changed = a.playerChanged
	a.pl.Seek(12 * time.Second)
	if seen == 0 || a.taskbarLyrics.revision == 0 {
		t.Fatal("paused seek did not refresh taskbar lyrics")
	}
	a.taskbarLyrics.visible = false
	seen = 0
	a.pl.Seek(20 * time.Second)
	if seen != 0 {
		t.Fatal("hidden taskbar lyrics still invalidate")
	}
}
