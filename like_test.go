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

// With NM_FRAMES set, saves the lyrics stage at moments through a line.
func TestStageFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := newTestApp()
	a.settings = defaultSettings()
	tr := Track{ID: 7, Name: "海屿你", Artists: []Artist{{Name: "马也_Crabbit"}}, Duration: 200000}
	a.pl.queue, a.pl.order, a.pl.at = []Track{tr}, []int{0}, 0
	a.pl.track = &a.pl.queue[0]
	a.pl.lyricsLoaded = true
	a.pl.lyrics = buildLyrics("[00:13.00]我从不主动示弱\n[00:30.56]曾经的那些发生过的开心和难过",
		"[00:13.00]I never show weakness first\n[00:30.56]The joy and sorrow that once happened",
		`[13010,4780](13010,660,0)从(13670,670,0)不(14340,30,0) (14370,480,0)主(14850,610,0)动(15460,30,0) (15490,330,0)示(15820,1070,0)弱
[17790,1770](17790,170,0)我(17960,450,0)们(18410,290,0)的(18700,470,0)过(19170,390,0)去
[30560,6580](30560,220,0)曾(30780,430,0)经(31210,340,0)的(31550,350,0)那(31900,470,0)些(32370,400,0)发(32770,320,0)生(33090,470,0)过(33560,30,0) (33590,210,0)的(33800,550,0)开(34350,250,0)心(34600,360,0)和(34960,640,0)难(35600,1500,0)过`)
	a.nowPlaying, a.npPane = true, paneStage
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetDark(true)
	tt.Advance(time.Second)
	moments := []time.Duration{12900, 13300, 13800, 14600, 15900, 17300, 17900, 18500, 26000}
	img := tt.Image()
	cw, ch := img.Bounds().Dx()/3, img.Bounds().Dy()/3
	grid := image.NewRGBA(image.Rect(0, 0, cw*3, ch*3))
	for i, m := range moments {
		a.pl.pending = m * time.Millisecond
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
		shot := tt.Image()
		small := image.NewRGBA(image.Rect(0, 0, cw, ch))
		for y := range ch {
			for x := range cw {
				small.Set(x, y, shot.At(x*3, y*3))
			}
		}
		draw.Draw(grid, image.Rect((i%3)*cw, (i/3)*ch, (i%3+1)*cw, (i/3+1)*ch), small, image.Point{}, draw.Src)
	}
	f, _ := os.Create(filepath.Join(dir, "stage.png"))
	defer f.Close()
	png.Encode(f, grid)
}
