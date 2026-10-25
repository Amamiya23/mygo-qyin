package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
)

// silence is a song of silence, to play without a stream.
type silence struct{ pos int64 }

func (s *silence) SampleRate() int           { return 48000 }
func (s *silence) Channels() int             { return 2 }
func (s *silence) Len() int64                { return 48000 * 200 }
func (s *silence) SetPosition(f int64) error { s.pos = f; return nil }
func (s *silence) Read(buf []float32) (int, error) {
	clear(buf)
	s.pos += int64(len(buf) / 2)
	return len(buf), nil
}

// Space pauses right after a click opened Now Playing: the button clicked
// has the focus then, and would take Space as another click.
func TestSpaceAfterOpeningNowPlaying(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := Track{ID: 7, Name: "宝船", Artists: []Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.queue, a.pl.order, a.pl.at = []Track{tr}, []int{0}, 0
	a.pl.track = &a.pl.queue[0]
	a.pl.lyricsLoaded = true
	a.pl.out = audio.NewPlayer(&silence{})
	a.pl.out.SetVolume(0)
	a.pl.out.Play()
	a.pl.playing = true
	defer a.pl.out.Close()

	tt := ui.NewTester(a.view, 1200, 800)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !a.nowPlaying {
		t.Fatal("now playing did not open")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.pl.playing {
		t.Error("Space did not pause")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.pl.playing {
		t.Error("Space did not play again")
	}
}
