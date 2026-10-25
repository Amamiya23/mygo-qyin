package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNowPlayingView(t *testing.T) {
	a := newTestApp()
	a.pl.at = 0
	tr := Track{ID: 1, Name: "宝船", Artists: []Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.queue = []Track{tr, {ID: 2, Name: "下一首"}}
	a.pl.order = []int{0, 1}
	a.pl.track = &a.pl.queue[0]
	a.pl.lyricsLoaded = true
	a.pl.lyrics = []LyricLine{{At: 0, Text: "第一句"}, {At: 5e9, Text: "第二句"}}
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if !a.nowPlaying {
		t.Fatal("now playing closed by itself")
	}
	for _, s := range []string{"歌词", "待播清单", "第一句", "宝船"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}
