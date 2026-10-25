package main

import (
	"testing"
	"time"

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

func TestNowPlayingClosesAsOneLayer(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := Track{ID: 1, Name: "宝船", Artists: []Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.queue, a.pl.order, a.pl.at = []Track{tr}, []int{0}, 0
	a.pl.track = &a.pl.queue[0]
	a.pl.lyricsLoaded = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetDark(false)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("待播清单") {
		t.Fatal("now playing did not open")
	}
	// Mid-way out, it still covers the page and the sidebar alike.
	a.nowPlaying = false
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	tt.Frame()
	img := tt.Image()
	sx, sy := img.Bounds().Dx(), img.Bounds().Dy()
	dark := func(x, y float32) bool {
		r, g, b, _ := img.At(int(x*float32(sx)/1200), int(y*float32(sy)/800)).RGBA()
		return (r+g+b)/3 < 0x9000
	}
	if !dark(900, 400) || !dark(100, 400) {
		t.Errorf("page %v, sidebar %v: the closing layer should cover both", dark(900, 400), dark(100, 400))
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.HasText("待播清单") {
		t.Error("now playing still shows after closing")
	}
}
