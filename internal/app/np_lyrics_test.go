package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

func lyricsAlignmentApp(w, h int, first, translation string, pos time.Duration) (*app, *ui.Tester) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := qqmusic.Track{ID: 1, Name: "滚滚红尘", Artists: []qqmusic.Artist{{Name: "陈淑桦"}}, Album: qqmusic.Album{PicURL: "lyrics-cover"}, Duration: 200000}
	a.pl.Cue([]qqmusic.Track{tr})
	texts := []string{first, "本应属于你的心", "它依然护紧我胸口", "为只为那尘世转变的面孔后的翻云覆雨手", "来易来去难去", "数十载的人世游", "分易分聚难聚", "爱与恨的千古愁", "于是不得不离开"}
	var lrc string
	for i, text := range texts {
		lrc += fmt.Sprintf("[00:%02d.00]%s\n", (i+1)*5, text)
	}
	if translation != "" {
		translation = "[00:05.00]" + translation
	}
	a.pl.SetLyrics(lyrics.Build(lrc, translation, ""))
	a.pl.Hold(pos)
	a.comments = commentState{track: tr.ID, loaded: true}
	a.nowPlaying, a.npPane = true, paneLyrics
	art := &artwork{bmp: solidBitmap(radixSage(true, 9)), tint: radixSage(true, 9)}
	for _, size := range []int{96, 120, 240, 600} {
		a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
	}
	a.art.art[qqmusic.Sized(tr.Album.PicURL, 600)+"#backdrop"] = art
	tt := ui.NewTester(a.view, w, h)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt
}

// The first lyric's upper padding remains clickable after its row moves above the cover's top.
func TestLyricsTopLineSeeks(t *testing.T) {
	a, tt := lyricsAlignmentApp(1200, 800, "爱与恨的千古愁", "", 0)
	r, ok := tt.Find("爱与恨的千古愁")
	if !ok {
		t.Fatal("first lyric is missing")
	}
	tt.Press(r.X+r.W/2, r.Y+2)
	tt.Release(r.X+r.W/2, r.Y+2)
	tt.Frame()
	if got := a.pl.Position(); got != 5*time.Second {
		t.Fatalf("clicking the first lyric sought to %s, want 5s", got)
	}
}

// With NM_FRAMES set, saves the first lyric, its hover, and scrolling beside the full-size and paused covers.
func TestLyricsAlignmentFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, scene := range []struct {
		name, first, translation string
		w, h                     int
		pos                      time.Duration
		hover, paused            bool
	}{
		{"regular", "爱与恨的千古愁", "", 1200, 800, 0, false, false},
		{"active", "爱与恨的千古愁", "", 1200, 800, 5 * time.Second, false, false},
		{"hover", "爱与恨的千古愁", "", 1200, 800, 0, true, false},
		{"paused", "爱与恨的千古愁", "", 1200, 800, 0, false, true},
		{"english", "Love and hate through the years", "爱与恨的千古愁", 1200, 800, 0, false, false},
		{"scrolled", "爱与恨的千古愁", "", 1200, 800, 35 * time.Second, false, false},
		{"short", "爱与恨的千古愁", "", 860, 560, 0, true, false},
		{"wide", "爱与恨的千古愁", "", 1920, 1080, 0, false, false},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a, tt := lyricsAlignmentApp(scene.w, scene.h, scene.first, scene.translation, scene.pos)
			if scene.paused {
				out := audio.NewPlayer(&silence{})
				out.SetVolume(0)
				defer out.Close()
				a.pl.Attach(out)
				a.pl.TogglePlay()
				a.pl.Seek(scene.pos)
			}
			if scene.hover {
				r, ok := tt.Find(scene.first)
				if !ok {
					t.Fatal("first lyric is missing")
				}
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
			} else {
				tt.Move(float32(scene.w)-20, 20)
			}
			tt.Frame()
			tt.Advance(time.Second)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, "lyrics-alignment-"+scene.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
