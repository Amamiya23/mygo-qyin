package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Paused, hidden, inter-word-gap and completed lyrics do not keep rebuilding themselves.
func TestDesktopLyricsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "hidden", "lrc", "gap", "interlude", "outro", "timed"} {
		t.Run(mode, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
			qrc := "[1000,9000]慢(1000,3000)唱(8000,2000)"
			if mode == "lrc" {
				qrc = ""
			}
			if mode == "interlude" {
				qrc += "\n[20000,2000]后句(20000,2000)"
			}
			a.pl.SetLyrics(lyrics.Build("[00:01]慢唱", "", qrc))
			if mode != "paused" {
				a.pl.Hold(2 * time.Second)
			}
			if mode == "gap" {
				a.pl.Hold(5 * time.Second)
			}
			if mode == "interlude" || mode == "outro" {
				a.pl.Hold(12 * time.Second)
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

// A sentence change requests a bounded burst of frames and becomes idle after both layers settle.
func TestDesktopLyricsTransitionFrameBudget(t *testing.T) {
	var state desktopLyricViewState
	started, builds := false, 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		if !started {
			state.changed, started = c.Now(), true
		}
		state.blend(c)
	}, 100, 100)
	tt.Frame()
	before := builds
	tt.Advance(desktopLyricsFade + 40*time.Millisecond)
	if got := builds - before; got < 12 || got > 18 {
		t.Fatalf("sentence transition rebuilt %d times, want a short 60 Hz burst", got)
	}
	before = builds
	tt.Advance(time.Second)
	if builds != before {
		t.Fatalf("settled transition kept requesting frames: %d", builds-before)
	}
}

// Previewing the next Chinese lyric must not animate paused, LRC-only or completed words.
func TestDesktopLyricsDoubleLineFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "lrc", "gap", "interlude", "outro", "timed"} {
		t.Run(mode, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
			a.settings.DesktopLyricsDoubleLine = true
			qrc := "[1000,9000]我(1000,3000)们(8000,2000)\n[20000,2000]再见(20000,2000)"
			if mode == "lrc" {
				qrc = ""
			}
			a.pl.SetLyrics(lyrics.Build("[00:01]我们\n[00:20]再见", "", qrc))
			position := 2 * time.Second
			switch mode {
			case "gap":
				position = 5 * time.Second
			case "interlude":
				position = 12 * time.Second
			case "outro":
				position = 23 * time.Second
			}
			if mode == "paused" {
				a.pl.Seek(position)
			} else {
				a.pl.Hold(position)
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 760, 240)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Frame()
			before := builds
			tt.Advance(500 * time.Millisecond)
			got := builds - before
			if mode == "timed" {
				if got < 10 || got > 15 {
					t.Fatalf("double-line word highlighting rebuilt %d times, want about 12", got)
				}
			} else if got != 0 {
				t.Fatalf("%s preview kept rebuilding: %d", mode, got)
			}
		})
	}
}
