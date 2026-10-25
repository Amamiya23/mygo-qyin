package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestLikeFromPlayerBar(t *testing.T) {
	calls := make(chan bool, 4)
	likeSong = func(id int64, like bool) error { calls <- like; return nil }
	defer func() { likeSong = Like }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &Profile{UserID: 1, Nickname: "小明"}
	tr := Track{ID: 7, Name: "宝船", Artists: []Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.queue, a.pl.order, a.pl.at = []Track{tr}, []int{0}, 0
	a.pl.track = &a.pl.queue[0]
	a.pl.lyricsLoaded = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if err := tt.Click("喜欢"); err != nil {
		t.Fatal(err)
	}
	if !a.liked[7] {
		t.Fatal("the heart did not fill at once")
	}
	if like := <-calls; !like {
		t.Error("asked NetEase to unlike")
	}
	tt.Frame()
	if err := tt.Click("取消喜欢"); err != nil {
		t.Fatal(err)
	}
	if a.liked[7] {
		t.Error("still liked after unliking")
	}
	if like := <-calls; like {
		t.Error("asked NetEase to like again")
	}
}

// With NM_FRAMES set to a directory, saves the like burst at moments after
// the click there, side by side, to look at.
func TestLikeBurstFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	likeSong = func(int64, bool) error { return nil }
	defer func() { likeSong = Like }()
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &Profile{UserID: 1}
	tr := Track{ID: 7, Name: "宝船", Duration: 200000}
	a.pl.queue, a.pl.order, a.pl.at = []Track{tr}, []int{0}, 0
	a.pl.track = &a.pl.queue[0]
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	r, ok := tt.Find("喜欢")
	if !ok {
		t.Fatal("no heart")
	}
	tt.Click("喜欢")
	moments := []time.Duration{0, 60, 120, 200, 300, 420, 600}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1200
	cell := int(90 * scale)
	strip := image.NewRGBA(image.Rect(0, 0, cell*len(moments), cell))
	at := time.Duration(0)
	for i, m := range moments {
		tt.Advance(m*time.Millisecond - at)
		at = m * time.Millisecond
		tt.Frame()
		img := tt.Image()
		cx, cy := int((r.X+r.W/2)*scale), int((r.Y+r.H/2)*scale)
		draw.Draw(strip, image.Rect(i*cell, 0, (i+1)*cell, cell), img, image.Pt(cx-cell/2, cy-cell/2), draw.Src)
	}
	f, _ := os.Create(filepath.Join(dir, "burst.png"))
	defer f.Close()
	png.Encode(f, strip)
}
