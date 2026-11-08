package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// NM_FRAMES saves the effect menu in both appearances, with selected effects, sparks, hover and a narrow window.
func TestStageStyleMenuFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("NM_FRAMES not set")
	}
	for _, scene := range []struct {
		name, style, hover    string
		w, h                  int
		dark, sparks, crowded bool
	}{
		{name: "light", w: 1200, h: 800},
		{name: "light-hover", w: 1200, h: 800, style: "partita", hover: "倾诉"},
		{name: "dark-hover", w: 1200, h: 800, style: "partita", hover: "倾诉", dark: true},
		{name: "sparks-on", w: 1200, h: 800, style: "tilt", hover: "火花", dark: true, sparks: true},
		{name: "narrow-light", w: 860, h: 560, hover: "云阶"},
		{name: "narrow-dark", w: 860, h: 560, style: "tilt", hover: "火花", dark: true, sparks: true},
		{name: "narrow-tall", w: 880, h: 800, dark: true, crowded: true},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.StageStyle, a.settings.StageSparks = scene.style, scene.sparks
			tr := qqmusic.Track{ID: 1, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Duration: 200000}
			a.pl.Cue([]qqmusic.Track{tr})
			a.pl.SetLyrics(lyrics.Build("[00:00.00]一二三四五六", "", ""))
			a.pl.Hold(2 * time.Second)
			if scene.crowded {
				a.comments = commentState{track: tr.ID, loaded: true, total: 12345678}
			}
			a.nowPlaying, a.npPane = true, paneStage
			tt := ui.NewTester(a.view, scene.w, scene.h)
			tt.SetDark(scene.dark)
			tt.Move(float32(scene.w)/2, float32(scene.h)/2)
			tt.Advance(time.Second)
			tt.Frame()
			if err := tt.Click("律动效果"); err != nil {
				t.Fatal(err)
			}
			if scene.hover != "" {
				r, ok := tt.Find(scene.hover)
				if !ok {
					t.Fatal("cannot target the hovered effect")
				}
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
			}
			tt.Advance(200 * time.Millisecond)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, "stage-menu-"+scene.name+".png"))
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
