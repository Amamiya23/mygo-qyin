package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"lilt/internal/qqmusic"
)

// Locking never removes the external tray action that restores pointer input.
func TestDesktopLyricsTrayUnlock(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	a := newTestApp()
	a.setDesktopLyricsEnabled(true)
	a.desktopLyrics.setLocked(true)
	menu := a.trayMenu()
	for _, item := range menu.Items() {
		if item.Label == "解锁桌面歌词" {
			if item.Disabled {
				t.Fatal("locked lyrics cannot be unlocked from tray")
			}
			item.Click(item, nil)
			if a.settings.DesktopLyricsLocked {
				t.Fatal("tray unlock kept mouse passthrough enabled")
			}
			return
		}
	}
	t.Fatal("no external unlock action")
}

// A paused seek redraws the enabled lyrics panel along with the main player.
func TestDesktopLyricsPausedSeekRedraw(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]qqmusic.Track{{ID: 1, Duration: 60000}})
	out := audio.NewPlayer(&silence{})
	defer out.Close()
	a.pl.Attach(out)
	a.pl.TogglePlay()
	a.settings.DesktopLyrics = true
	a.win = &mygo.Window{}
	a.settingsWin = &mygo.Window{}
	a.desktopLyrics = &desktopLyrics{a: a, win: &mygo.Window{}}
	defer func(f func(*mygo.Window)) { invalidateWindow = f }(invalidateWindow)
	seen := map[*mygo.Window]int{}
	invalidateWindow = func(w *mygo.Window) { seen[w]++ }
	defer func(f func(time.Duration, func())) { sessionAfter = f }(sessionAfter)
	sessionAfter = func(time.Duration, func()) {}
	a.pl.Changed = a.playerChanged
	a.pl.Seek(12 * time.Second)
	if seen[a.desktopLyrics.win] == 0 || seen[a.win] == 0 || seen[a.settingsWin] == 0 {
		t.Fatal("paused seek did not redraw both windows")
	}
	a.settings.DesktopLyrics = false
	seen = map[*mygo.Window]int{}
	a.pl.Seek(20 * time.Second)
	if seen[a.desktopLyrics.win] != 0 {
		t.Fatal("disabled lyrics still invalidates")
	}
}

// Bounds remain reachable after a display disappears or becomes smaller.
func TestDesktopLyricsBoundsRecovery(t *testing.T) {
	work := mygo.Rectangle{X: -1280, Y: 40, Width: 1280, Height: 680}
	r := clampDesktopLyricsBounds(mygo.Rectangle{X: 2000, Y: 1600, Width: 1600, Height: 900}, work)
	if r.X != work.X || r.Y != work.Y || r.Width != work.Width || r.Height != work.Height {
		t.Fatalf("off-screen bounds were not fitted: %+v", r)
	}
}
