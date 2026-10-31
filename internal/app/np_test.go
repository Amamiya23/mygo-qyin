package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Clicking the collapse control dismisses Now Playing as one layer over the page.
func TestNowPlayingClosesAsOneLayer(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := qqmusic.Track{ID: 1, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.Cue([]qqmusic.Track{tr})
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
	tt.Move(600, 400)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("收起播放页"); err != nil {
		t.Fatal(err)
	}
	if a.nowPlaying {
		t.Fatal("the collapse control did not dismiss Now Playing")
	}
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
	var tracks []qqmusic.Track
	for i := range 12 {
		tracks = append(tracks, qqmusic.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌%02d", i), Duration: 200000})
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
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "一", Duration: 200000}, {ID: 2, Name: "二", Duration: 200000}})
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
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "一", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	var at []float32
	for _, st := range []*qqmusic.Stream{{Level: "lossless"}, nil} {
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

// ⌘ and a number choose Now Playing's tab in that place, for a song and
// for an episode, whose tabs are others: its last is the comments, wherever
// the captions it may have put it.
func TestCmdNumberChoosesTab(t *testing.T) {
	for _, c := range []struct {
		track qqmusic.Track
		keys  []int // from 1, 0 for the last tab
		panes []int
	}{
		{qqmusic.Track{ID: 1, Name: "宝船", Duration: 200000}, []int{4, 2, 1}, []int{paneWall, paneStage, paneLyrics}},
		{qqmusic.Track{ID: 2, Name: "节目", ProgramID: 9, Duration: 200000}, []int{0, 1}, []int{paneComments, paneProgram}},
	} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.pl.Cue([]qqmusic.Track{c.track})
		a.pl.SetLyrics(nil)
		a.nowPlaying = true
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		for i, n := range c.keys {
			if n == 0 {
				n = len(a.npTabs())
			}
			tt.Key(ui.Cmd, ui.Key1+ui.Key(n-1))
			tt.Frame()
			if a.npPane != c.panes[i] {
				t.Errorf("%s: ⌘%d chose pane %d, want %d", c.track.Name, n, a.npPane, c.panes[i])
			}
		}
	}
}

// Now Playing's tabs used to jump between panes and shift as the comments' count grew; they stay in place after either change.
func TestNowPlayingTabsStayPut(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "一", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.comments = commentState{track: 1, loaded: true}
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	settle := func() {
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
	}
	settle()
	first, ok := tt.Find("歌词")
	if !ok {
		t.Fatal("no 歌词 tab")
	}
	checkPosition := func() {
		t.Helper()
		r, ok := tt.Find("歌词")
		if !ok {
			t.Fatal("no 歌词 tab")
		}
		if r.X != first.X || r.Y != first.Y {
			t.Errorf("in pane %d, with %d comments, the 歌词 tab is at %v,%v, was at %v,%v", a.npPane, a.comments.total, r.X, r.Y, first.X, first.Y)
		}
	}
	for _, tab := range []struct {
		label string
		pane  int
	}{{"律动", paneStage}, {"待播清单", paneQueue}, {"选歌", paneWall}, {"评论", paneComments}, {"歌词", paneLyrics}} {
		if err := tt.Click(tab.label); err != nil {
			t.Fatal(err)
		}
		settle()
		if a.npPane != tab.pane {
			t.Fatalf("%s chose pane %d, want %d", tab.label, a.npPane, tab.pane)
		}
		checkPosition()
	}
	for _, pane := range []int{paneStage, paneLyrics} {
		a.npPane = pane
		a.comments.total = 0
		settle()
		a.comments.total = 12345678
		settle()
		checkPosition()
	}
}

// With NM_FRAMES set, saves Now Playing's layouts and control states to inspect alignment and media contrast in both appearances.
func TestNowPlayingFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, scene := range []struct {
		name    string
		w, h    int
		pane    int
		hover   string
		pressed bool
		dark    bool
	}{
		{name: "short", w: 860, h: 560},
		{name: "regular", w: 1200, h: 800},
		{name: "collapse-hover-light", w: 1200, h: 800, hover: "收起播放页"},
		{name: "collapse-hover-dark", w: 1200, h: 800, hover: "收起播放页", dark: true},
		{name: "collapse-pressed", w: 1200, h: 800, hover: "收起播放页", pressed: true},
		{name: "play-hover-light", w: 1200, h: 800, hover: "播放/暂停"},
		{name: "play-hover-dark", w: 1200, h: 800, hover: "播放/暂停", dark: true},
		{name: "play-pressed", w: 1200, h: 800, hover: "播放/暂停", pressed: true},
		{name: "previous-hover", w: 1200, h: 800, hover: "上一首"},
		{name: "next-hover", w: 1200, h: 800, hover: "下一首"},
		{name: "shuffle-hover", w: 1200, h: 800, hover: "随机播放"},
		{name: "shuffle-on-hover", w: 1200, h: 800, hover: "随机播放"},
		{name: "repeat-hover", w: 1200, h: 800, hover: "循环"},
		{name: "like-hover", w: 1200, h: 800, hover: "喜欢"},
		{name: "liked-hover", w: 1200, h: 800, hover: "取消喜欢"},
		{name: "more-hover", w: 1200, h: 800, hover: "更多"},
		{name: "mute-hover", w: 1200, h: 800, hover: "静音"},
		{name: "volume-hover", w: 1200, h: 800, hover: "音量"},
		{name: "tab-hover", w: 1200, h: 800, hover: "待播清单"},
		{name: "wide", w: 1920, h: 1080},
		{name: "stage", w: 1200, h: 800, pane: paneStage},
		{name: "stage-play-hover", w: 1200, h: 800, pane: paneStage, hover: "播放/暂停"},
		{name: "wall", w: 1200, h: 800, pane: paneWall},
		{name: "wall-play-hover", w: 1200, h: 800, pane: paneWall, hover: "播放/暂停"},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			tr := qqmusic.Track{ID: 1, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Album: qqmusic.Album{PicURL: "layout-cover"}, Duration: 200000}
			a.pl.Cue([]qqmusic.Track{tr})
			a.pl.SetLyrics(nil)
			a.volume = 0.72
			a.pl.SetVolume(a.volume)
			if scene.hover == "取消喜欢" {
				a.liked = map[int64]bool{tr.ID: true}
			}
			if scene.name == "shuffle-on-hover" {
				a.pl.ToggleShuffle()
			}
			a.comments = commentState{track: tr.ID, loaded: true, total: 12345678}
			a.nowPlaying, a.npPane = true, scene.pane
			art := &artwork{bmp: solidBitmap(radixSage(true, 9)), tint: radixSage(true, 9)}
			for _, size := range []int{96, 120, 240, 600} {
				a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
			}
			a.art.art[qqmusic.Sized(tr.Album.PicURL, 600)+"#backdrop"] = art
			tt := ui.NewTester(a.view, scene.w, scene.h)
			tt.SetDark(scene.dark)
			tt.Frame()
			tt.Move(float32(scene.w)/2, float32(scene.h)/2)
			tt.Advance(time.Second)
			tt.Frame()
			if scene.hover != "" {
				b, ok := tt.Find(scene.hover)
				if !ok {
					t.Fatalf("no %s control", scene.hover)
				}
				tt.Move(b.X+b.W/2, b.Y+b.H/2)
				if scene.pressed {
					tt.Press(b.X+b.W/2, b.Y+b.H/2)
				}
				tt.Advance(200 * time.Millisecond)
				tt.Frame()
			}
			f, err := os.Create(filepath.Join(dir, "now-playing-"+scene.name+".png"))
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
