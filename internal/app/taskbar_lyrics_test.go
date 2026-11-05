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

// The settings switch persists both directions, stops monitoring, and leaves desktop lyrics independent.
func TestTaskbarLyricsSettingsToggle(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.setDesktopLyricsEnabled(true)
	desktop := a.desktopLyrics
	tt := ui.NewTester(a.settingsView, 560, 2200)
	tt.Frame()
	stopped := 0
	for _, enabled := range []bool{true, false, true, false} {
		if err := tt.Click("启用任务栏歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyrics != enabled || restored.TaskbarLyrics != enabled {
			t.Fatalf("taskbar switch did not apply/persist %t: live=%t saved=%t owner=%t", enabled, a.settings.TaskbarLyrics, restored.TaskbarLyrics, a.taskbarLyrics != nil)
		}
		if (a.taskbarLyrics != nil) != enabled {
			t.Fatalf("taskbar owner lifecycle did not follow enabled=%t", enabled)
		}
		if enabled {
			a.taskbarLyrics.stop = func() { stopped++ }
		}
		if a.desktopLyrics != desktop || !a.settings.DesktopLyrics || !restored.DesktopLyrics {
			t.Fatal("taskbar switch changed desktop lyrics")
		}
	}
	if stopped != 2 {
		t.Fatalf("disabling did not stop each monitor: stopped=%d", stopped)
	}
	// Startup uses the saved choice without needing a click in Settings.
	a.settings.TaskbarLyrics = true
	a.settings.save()
	b := newTestApp()
	b.settings = readDesktopLyricsEntrySettings(t, a)
	b.syncTaskbarLyrics()
	if b.taskbarLyrics == nil {
		t.Fatal("restored preference did not recreate taskbar lyrics")
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
