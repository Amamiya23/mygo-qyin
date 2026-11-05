package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Paused, hidden, missing and completed taskbar lyrics stay idle; active highlighting and long-line scrolling are bounded.
func TestTaskbarLyricsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "hidden", "disabled", "missing", "prelude", "lrc-short", "lrc-long", "lrc-translation", "preview-long", "timed", "gap", "outro"} {
		t.Run(mode, func(t *testing.T) {
			a, d := taskbarLyricsViewFixture()
			qrc := "[1000,9000]我们一起走过漫长道路(1000,3000)直到音乐带我们走向远方(8000,2000)"
			if mode == "lrc-short" || mode == "lrc-long" || mode == "lrc-translation" || mode == "preview-long" {
				qrc = ""
			}
			lrc := "[00:01]我们一起走过漫长道路，直到音乐带我们走向远方"
			if mode == "lrc-short" {
				lrc = "[00:01]我们\n[00:20]再见"
			}
			if mode == "preview-long" {
				lrc = "[00:01]我们\n[00:20]让音乐陪伴我们走过漫长道路，直到歌声带我们走向远方"
			}
			translation := ""
			if mode == "lrc-translation" {
				lrc, translation = "[00:01]Stay", "[00:01]留在我身边，陪伴我们走过漫长道路直到远方"
			}
			a.pl.SetLyrics(lyrics.Build(lrc, translation, qrc))
			position := 2 * time.Second
			if mode == "gap" {
				position = 5 * time.Second
			}
			if mode == "outro" {
				position = 12 * time.Second
			}
			if mode == "prelude" {
				position = 0
			}
			if mode == "paused" {
				a.pl.Seek(position)
			} else {
				a.pl.Hold(position)
			}
			if mode == "hidden" {
				d.visible = false
			}
			if mode == "disabled" {
				a.settings.TaskbarLyrics = false
			}
			if mode == "missing" {
				a.pl.SetLyrics(nil)
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 160, 42)
			tt.Frame()
			before := builds
			tt.Advance(500 * time.Millisecond)
			got := builds - before
			if mode == "timed" || mode == "lrc-long" || mode == "lrc-translation" {
				if got < 10 || got > 15 {
					t.Fatalf("%s rebuilt %d times in 500ms, want about 12", mode, got)
				}
			} else if got != 0 {
				t.Fatalf("%s taskbar lyrics rebuilt %d times while unchanged", mode, got)
			}
		})
	}
}
