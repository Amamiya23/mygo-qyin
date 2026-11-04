package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Idle, paused, hidden and inter-word-gap lyrics do not keep rebuilding themselves.
func TestDesktopLyricsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "hidden", "lrc", "gap", "timed"} {
		t.Run(mode, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
			qrc := "[1000,9000]慢(1000,3000)唱(8000,2000)"
			if mode == "lrc" {
				qrc = ""
			}
			a.pl.SetLyrics(lyrics.Build("[00:01]慢唱", "", qrc))
			if mode != "paused" {
				a.pl.Hold(2 * time.Second)
			}
			if mode == "gap" {
				a.pl.Hold(5 * time.Second)
			}
			if mode == "hidden" {
				a.settings.DesktopLyrics = false
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 760, 180)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Frame()
			before := builds
			tt.Advance(500 * time.Millisecond)
			got := builds - before
			if mode == "timed" {
				if got < 10 || got > 15 {
					t.Fatalf("active words rebuilt %d times in 500ms, want about 12", got)
				}
			} else if got != 0 {
				t.Fatalf("%s lyrics rebuilt %d times while unchanged", mode, got)
			}
		})
	}
}
