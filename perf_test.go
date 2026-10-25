package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// playingApp is an app playing a track with lyrics, at a position the test
// sets with pl.pending, and the number of times its view was built.
func playingApp() (*app, *ui.Tester, *int) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := Track{ID: 7, Name: "宝船", Artists: []Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.queue, a.pl.order, a.pl.at = []Track{tr}, []int{0}, 0
	a.pl.track = &a.pl.queue[0]
	a.pl.lyricsLoaded = true
	// A line every 10 seconds, sung over the first 5 of them.
	var lrc string
	for i := range 12 {
		lrc += "[0" + string(rune('0'+i*10/60)) + ":" + string(rune('0'+i*10%60/10)) + "0.00]一二三四五六\n"
	}
	a.pl.lyrics = buildLyrics(lrc, "", "")
	for i := range a.pl.lyrics {
		l := &a.pl.lyrics[i]
		l.End = l.At + 5*time.Second
		l.Words = spreadWords(l.Text, l.At, l.End)
	}
	a.pl.playing = true
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.view(c) }, 1200, 800)
	return a, tt, &builds
}

// framesAt counts the frames the view asks for over a second, at 60 a
// second, with playback held at pos.
func framesAt(a *app, tt *ui.Tester, pos time.Duration) int {
	a.pl.pending = pos
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	asked := 0
	for range 60 {
		tt.Advance(time.Second / 60)
		if tt.FramePending() {
			asked++
			tt.Frame()
		}
	}
	return asked
}

// Now Playing asks for frames only as fast as what it shows changes. The
// Tester runs timers as it settles a frame, so this counts only what does
// not change for a while: a frame log of the app (MYGO_FRAME_STATS) measures
// the rest.
func TestNowPlayingFrameBudget(t *testing.T) {
	a, tt, _ := playingApp()
	if n := framesAt(a, tt, 12*time.Second); n > 3 {
		t.Errorf("a page under the player: %d frames a second", n)
	}
	a.nowPlaying, a.npPane = true, paneStage
	if n := framesAt(a, tt, 18*time.Second); n > 12 {
		t.Errorf("the stage between lines: %d frames a second", n)
	}
}
