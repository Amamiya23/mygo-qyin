package app

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func desktopLyricsEntryTester(a *app) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		a.playerBar(c)
	}, 880, int(playerBarHeight))
}

func readDesktopLyricsEntrySettings(t *testing.T, a *app) settings {
	t.Helper()
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var restored settings
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	return restored
}

// The bottom-right switch toggles desktop lyrics and persists the choice even before a track is cued.
func TestDesktopLyricsPlayerEntryToggle(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := desktopLyricsEntryTester(a)
	tt.Frame()
	for _, enabled := range []bool{true, false, true} {
		if err := tt.Click("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.DesktopLyrics != enabled || readDesktopLyricsEntrySettings(t, a).DesktopLyrics != enabled {
			t.Fatalf("desktop lyrics did not persist enabled=%t", enabled)
		}
	}
}

// A single right click directly locks or unlocks lyrics without a menu or opening Settings.
func TestDesktopLyricsPlayerEntryLockAndUnlock(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := desktopLyricsEntryTester(a)
	tt.Frame()
	if err := tt.Click("桌面歌词"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, locked := range []bool{true, false, true, false} {
		if err := tt.RightClick("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if !a.settings.DesktopLyrics || !restored.DesktopLyrics || a.settings.DesktopLyricsLocked != locked || restored.DesktopLyricsLocked != locked {
			t.Fatalf("right click did not persist enabled lyrics with locked=%t", locked)
		}
		if tt.Menu() != nil || a.settingsWin != nil {
			t.Fatal("right click opened a menu or Settings")
		}
	}
}

// Right-clicking a disabled lyric switch preserves its saved lock choice without enabling lyrics.
func TestDesktopLyricsPlayerEntryDisabledRightClick(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	for _, locked := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.path = filepath.Join(t.TempDir(), "settings.json")
		a.settings.DesktopLyricsLocked = locked
		a.settings.save()
		tt := desktopLyricsEntryTester(a)
		tt.Frame()
		if err := tt.RightClick("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.DesktopLyrics || restored.DesktopLyrics || a.settings.DesktopLyricsLocked != locked || restored.DesktopLyricsLocked != locked {
			t.Fatalf("disabled right click changed the saved feature or lock state: locked=%t", locked)
		}
		if tt.Menu() != nil {
			t.Fatal("disabled right click opened a context menu")
		}
	}
}

// The taskbar player switch preserves its mode across toggles and never applies desktop locking on right click.
func TestTaskbarLyricsPlayerEntryToggleAndRightClick(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "unlocked", true: "locked"}[locked], func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.path = filepath.Join(t.TempDir(), "settings.json")
			a.settings.DesktopLyricsLocked = locked
			a.settings.DesktopLyricsDoubleLine = true
			a.settings.DesktopLyricsFontSize = 44
			a.setLyricsMode("taskbar")
			tt := desktopLyricsEntryTester(a)
			tt.Frame()
			for _, enabled := range []bool{true, false, true} {
				if err := tt.Click("任务栏歌词"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if err := tt.RightClick("任务栏歌词"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				restored := readDesktopLyricsEntrySettings(t, a)
				if a.settings.TaskbarLyrics != enabled || restored.TaskbarLyrics != enabled || a.settings.DesktopLyrics || restored.DesktopLyrics {
					t.Fatalf("taskbar player switch did not exclusively persist enabled=%t", enabled)
				}
				if a.settings.lyricsMode() != "taskbar" || restored.lyricsMode() != "taskbar" {
					t.Fatal("turning taskbar lyrics off lost the chosen mode")
				}
				if a.settings.DesktopLyricsLocked != locked || restored.DesktopLyricsLocked != locked {
					t.Fatalf("taskbar right click changed desktop locked=%t", locked)
				}
				if tt.Menu() != nil || a.settingsWin != nil {
					t.Fatal("taskbar right click opened a menu or Settings")
				}
			}
			a.setLyricsMode("desktop")
			tt.Frame()
			if !a.settings.DesktopLyrics || a.settings.TaskbarLyrics || a.settings.DesktopLyricsLocked != locked || !a.settings.DesktopLyricsDoubleLine || a.settings.DesktopLyricsFontSize != 44 {
				t.Fatal("returning to desktop mode lost its lock, line, or font preferences")
			}
			if err := tt.RightClick("桌面歌词"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.settings.DesktopLyricsLocked == locked {
				t.Fatal("returning to desktop mode did not restore right-click locking")
			}
		})
	}
}

// The tray switches the selected taskbar lyrics without exposing an enabled desktop unlock action.
func TestTaskbarLyricsTrayToggle(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.settings.DesktopLyricsLocked = true
	a.setLyricsMode("taskbar")
	for _, enabled := range []bool{true, false, true} {
		clicked := false
		for _, item := range a.trayMenu().Items() {
			if item.Label == "解锁桌面歌词" && !item.Disabled {
				t.Fatal("taskbar mode exposes a desktop unlock action")
			}
			if item.Label == "任务栏歌词" {
				if item.Checked == enabled {
					t.Fatalf("tray checked state did not match the previous taskbar state before enabled=%t", enabled)
				}
				item.Click(item, nil)
				clicked = true
			}
		}
		if !clicked {
			t.Fatal("tray has no taskbar lyric switch")
		}
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyrics != enabled || restored.TaskbarLyrics != enabled || a.settings.DesktopLyrics || restored.lyricsMode() != "taskbar" || !a.settings.DesktopLyricsLocked {
			t.Fatalf("tray did not persist taskbar enabled=%t while preserving the desktop lock preference", enabled)
		}
	}
}

// Export both lyric modes and their settings to inspect the controls at narrow widths.
func TestDesktopLyricsEntryFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect desktop lyrics controls")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	for _, state := range []struct {
		name, mode      string
		enabled, locked bool
	}{
		{"disabled", lyricsDesktop, false, false},
		{"enabled", lyricsDesktop, true, false},
		{"locked", lyricsDesktop, true, true},
		{"disabled", lyricsTaskbar, false, true},
		{"enabled", lyricsTaskbar, true, true},
	} {
		if state.mode == lyricsTaskbar && !supportsTaskbarLyrics() {
			continue
		}
		a.settings.LyricsMode = state.mode
		a.settings.DesktopLyrics = state.enabled && state.mode == lyricsDesktop
		a.settings.TaskbarLyrics = state.enabled && state.mode == lyricsTaskbar
		a.settings.DesktopLyricsLocked = state.locked
		tt := desktopLyricsEntryTester(a)
		tt.Frame()
		saveLyricsEntryFrame(t, dir, state.mode+"-lyrics-player-bar-"+state.name+".png", tt)
		if state.name == "enabled" {
			for _, dark := range []bool{false, true} {
				settingsView := ui.NewTester(a.settingsView, 480, 1300)
				settingsView.SetDark(dark)
				settingsView.Frame()
				appearance := "light"
				if dark {
					appearance = "dark"
				}
				saveLyricsEntryFrame(t, dir, state.mode+"-lyrics-settings-480-"+appearance+".png", settingsView)
			}
		}
	}
}

func saveLyricsEntryFrame(t *testing.T, dir, name string, tt *ui.Tester) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, tt.Image())
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("save lyrics frame: %v / %v", err, closeErr)
	}
}
