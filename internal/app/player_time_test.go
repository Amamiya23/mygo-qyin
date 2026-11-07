package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Now Playing keeps its clock updating after it covers the player bar,
// without input or notifications from the player.
func TestNowPlayingTimesWithoutInput(t *testing.T) {
	for name, pane := range map[string]int{"lyrics": paneLyrics, "queue": paneQueue, "stage": paneStage, "wall": paneWall} {
		t.Run(name, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.setProgressEffect(progressStandard)
			a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 200000}})
			a.pl.SetLyrics(nil)
			out := audio.NewPlayer(&silence{})
			defer out.Close()
			out.Seek(10 * time.Second)
			a.pl.Attach(out)
			a.nowPlaying, a.npPane = true, pane
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Advance(2 * time.Second)
			_, barVisible := tt.Find("打开正在播放")
			if !a.npOpen || barVisible {
				t.Fatal("Now Playing did not cover the page")
			}
			for _, sec := range []int{11, 12, 13} {
				// Move only the audio clock, as playback does; do not force a
				// build through pl.Seek, pointer input, or Tester.Frame.
				out.Seek(time.Duration(sec) * time.Second)
				tt.Advance(1100 * time.Millisecond)
				elapsed := clock(time.Duration(sec) * time.Second)
				if pane == paneStage {
					elapsed += " / " + clock(200*time.Second)
				}
				if !tt.HasText(elapsed) {
					t.Fatalf("elapsed time did not update at %ds", sec)
				}
				if pane != paneStage && !tt.HasText("-"+clock(time.Duration(200-sec)*time.Second)) {
					t.Fatalf("remaining time did not update at %ds", sec)
				}
			}
		})
	}
}
