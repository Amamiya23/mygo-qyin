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

// Export the narrow player bar with disabled, enabled and locked desktop-lyrics controls.
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
		name            string
		enabled, locked bool
	}{
		{"disabled", false, false},
		{"enabled", true, false},
		{"locked", true, true},
	} {
		a.settings.DesktopLyrics, a.settings.DesktopLyricsLocked = state.enabled, state.locked
		tt := desktopLyricsEntryTester(a)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, "desktop-lyrics-player-bar-"+state.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
