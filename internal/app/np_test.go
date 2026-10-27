package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func TestNowPlayingClosesAsOneLayer(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 1, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
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

func TestQueueStaysAsATrackIsPlayed(t *testing.T) {
	a := newTestApp()
	var tracks []netease.Track
	for i := range 12 {
		tracks = append(tracks, netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌%02d", i), Duration: 200000})
	}
	a.pl.Cue(tracks)
	a.nowPlaying, a.npPane = true, paneQueue
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("歌05"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if got := a.pl.Current().Name; got != "歌05" {
		t.Fatalf("playing %q, want 歌05", got)
	}
	// The tracks before it are still listed, as they were.
	for _, s := range []string{"歌01", "歌04", "歌06", "歌09"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}

// The app chooses Now Playing's pane before it makes the player: it
// crashed on start when the choice asked the player what played.
func TestLyricsPaneBeforePlayer(t *testing.T) {
	a := &app{settings: settings{LyricsStage: true}}
	if pane := a.lyricsPaneChosen(); pane != paneStage {
		t.Errorf("pane %d", pane)
	}
}

// 上一首 plays the song before, however far this one has played: a few
// seconds in, it went back to this one's start, and took a second click.
func TestPrevGoesBackASong(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一", Duration: 200000}, {ID: 2, Name: "二", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.pl.Hold(30 * time.Second)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("上一首"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if id := a.pl.Current().ID; id != 2 {
		t.Errorf("playing %d, want the one before, 2", id)
	}
}

// Now Playing holds still as the quality's badge goes, while the next song
// loads, and comes back: the row of times grew with it, moving the column.
func TestNowPlayingHoldsStillWithoutBadge(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	var at []float32
	for _, st := range []*netease.Stream{{Level: "lossless"}, nil} {
		a.pl.SetStream(st)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		r, ok := tt.Find("专辑封面")
		if !ok {
			t.Fatal("no cover")
		}
		at = append(at, r.Y)
	}
	if at[0] != at[1] {
		t.Errorf("the cover moved from %v to %v as the badge went", at[0], at[1])
	}
}
