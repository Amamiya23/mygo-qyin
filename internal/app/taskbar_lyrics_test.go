package app

import (
	"path/filepath"
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
