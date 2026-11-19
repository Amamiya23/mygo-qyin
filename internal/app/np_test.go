package app

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
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
	if !tt.HasText("收起播放页") {
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
	if tt.HasText("收起播放页") {
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
	}{{"律动", paneStage}, {"播放列表", paneQueue}, {"选歌", paneWall}, {"评论", paneComments}, {"歌词", paneLyrics}} {
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
		{name: "tab-hover", w: 1200, h: 800, hover: "播放列表"},
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
			art := &artwork{bmp: solidBitmap(radixSage(true, 9)), tint: radixSage(true, 9), ready: artBackdrop}
			for _, size := range []int{96, 120, 240, 600} {
				a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
			}
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

// transitionApp opens Now Playing by clicking the player bar, as a listener does.
func transitionApp(t *testing.T, reduceMotion bool) (*app, *ui.Tester) {
	t.Helper()
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.npPane = paneQueue
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	return a, tt
}

// Escape during opening and another bar click during closing leave the new opening usable.
func TestNowPlayingReopensDuringTransition(t *testing.T) {
	a, tt := transitionApp(t, false)
	tt.Advance(80 * time.Millisecond)
	tt.Key(0, ui.KeyEscape)
	if a.nowPlaying {
		t.Fatal("Escape did not close the opening view")
	}
	tt.Advance(60 * time.Millisecond)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.nowPlaying {
		t.Fatal("the bar click did not reopen Now Playing")
	}
	if _, visible := tt.Find("打开正在播放"); visible {
		t.Fatal("the reopened view did not cover the player bar")
	}
	if err := tt.Click("收起播放页"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.nowPlaying {
		t.Fatal("the reopened view could not be closed")
	}
	if _, visible := tt.Find("打开正在播放"); !visible {
		t.Fatal("closing did not restore the player bar")
	}
}

// pullTransition drags the blank gap beside the cover, avoiding its own fling gesture.
func pullTransition(t *testing.T, dy float32, steps int, rest time.Duration) (*app, *ui.Tester, ui.Rect) {
	t.Helper()
	a, tt := transitionApp(t, false)
	tt.Advance(time.Second)
	cover, ok := tt.Find("专辑封面")
	if !ok {
		t.Fatal("Now Playing did not show its cover")
	}
	x, y := float32(450), float32(300)
	tt.Press(x, y)
	for i := 1; i <= steps; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Move(x, y+dy*float32(i)/float32(steps))
	}
	tt.Advance(rest)
	tt.Release(x, y+dy)
	tt.Advance(time.Second)
	return a, tt, cover
}

// A long downward pull closes the page and returns input to the player bar.
func TestNowPlayingPullDownCloses(t *testing.T) {
	a, tt, _ := pullTransition(t, 320, 12, 300*time.Millisecond)
	if a.nowPlaying {
		t.Fatal("Now Playing stayed open after a long pull")
	}
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	if !a.nowPlaying {
		t.Fatal("the player bar did not receive input after the pull")
	}
}

// A short pull released at rest returns the cover to its place and keeps the page open.
func TestNowPlayingShortPullSpringsBack(t *testing.T) {
	a, tt, before := pullTransition(t, 60, 6, 300*time.Millisecond)
	if !a.nowPlaying {
		t.Fatal("a short pull closed Now Playing")
	}
	after, ok := tt.Find("专辑封面")
	if !ok || math.Abs(float64(after.X-before.X))+math.Abs(float64(after.Y-before.Y)) > 1 {
		t.Fatalf("the cover did not return to its place: before %v, after %v", before, after)
	}
	if err := tt.Click("收起播放页"); err != nil {
		t.Fatal(err)
	}
	if a.nowPlaying {
		t.Fatal("the returned page did not receive its collapse click")
	}
}

// A quick downward flick closes the page before it travels the long-pull distance.
func TestNowPlayingFlickDownCloses(t *testing.T) {
	a, tt, _ := pullTransition(t, 80, 3, 0)
	if a.nowPlaying {
		t.Fatal("Now Playing stayed open after a downward flick")
	}
	if _, visible := tt.Find("打开正在播放"); !visible {
		t.Fatal("the flick did not restore the player bar")
	}
}

// Closing used to hide both covers in the landing frame; the bar must stay covered by artwork throughout the handoff.
func TestNowPlayingClosingCoverDoesNotBlink(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := qqmusic.Track{ID: 7, Name: "宝船", Album: qqmusic.Album{PicURL: "transition-cover"}, Duration: 200000}
	art := &artwork{bmp: solidBitmap(radixAmber(false, 9)), tint: radixAmber(false, 9), ready: artBackdrop}
	for _, size := range []int{120, 600} {
		a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
	}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(nil)
	a.npPane = paneQueue
	tt := ui.NewTester(a.view, 1200, 800)
	bar, ok := tt.Find("打开正在播放")
	if !ok {
		t.Fatal("the bar did not show its track")
	}
	x, y := bar.X+24, bar.Y+bar.H/2
	saveTransitionFrame(t, tt, "bar")
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	saveTransitionFrame(t, tt, "open")
	tt.Key(0, ui.KeyEscape)
	for ms := range 1000 {
		tt.Advance(time.Millisecond)
		if a.npT > 0.05 {
			continue // The moving cover has not reached the bar yet.
		}
		img := tt.Image()
		scale := float32(img.Bounds().Dx()) / 1200
		r, g, b, _ := img.At(int(x*scale), int(y*scale)).RGBA()
		if max(r, g, b)-min(r, g, b) < 0x2000 {
			saveTransitionFrame(t, tt, "missing-cover")
			t.Fatalf("at %d ms after closing, the bar's cover was missing at (%v,%v): t=%v, from=%v, pixel=%x/%x/%x", ms+1, x, y, a.npT, a.npFrom, r, g, b)
		}
	}
}

// Reduced motion keeps opening and closing operable while the view fades in place.
func TestNowPlayingReducedMotionInput(t *testing.T) {
	a, tt := transitionApp(t, true)
	tt.Advance(time.Second)
	if !a.nowPlaying {
		t.Fatal("the bar did not open Now Playing with reduced motion")
	}
	if err := tt.Click("收起播放页"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(80 * time.Millisecond)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.nowPlaying {
		t.Fatal("the fading view blocked the bar's reopen click")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if a.nowPlaying {
		t.Fatal("Escape did not close Now Playing with reduced motion")
	}
}

// With NM_FRAMES set, saves the actual bar-to-page transition to inspect its cover and clipping.
func TestNowPlayingTransitionFrames(t *testing.T) {
	if os.Getenv("NM_FRAMES") == "" {
		t.Skip()
	}
	for _, scene := range []struct {
		name string
		pane int
		w, h int
	}{
		{"queue", paneQueue, 1200, 800},
		{"stage", paneStage, 1200, 800},
		{"wall", paneWall, 1200, 800},
		{"short", paneQueue, 860, 560},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			tr := qqmusic.Track{ID: 7, Name: "宝船", Album: qqmusic.Album{PicURL: "transition-frames"}, Duration: 200000}
			art := &artwork{bmp: solidBitmap(radixAmber(false, 9)), tint: radixAmber(false, 9), ready: artBackdrop}
			for _, size := range []int{120, 600} {
				a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
			}
			a.pl.Cue([]qqmusic.Track{tr})
			a.pl.SetLyrics(nil)
			a.npPane = scene.pane
			tt := ui.NewTester(a.view, scene.w, scene.h)
			if err := tt.Click("打开正在播放"); err != nil {
				t.Fatal(err)
			}
			for i, advance := range []time.Duration{80 * time.Millisecond, 100 * time.Millisecond, time.Second} {
				tt.Advance(advance)
				saveTransitionFrame(t, tt, scene.name+"-"+[]string{"opening-80ms", "opening-180ms", "opened"}[i])
			}
			tt.Key(0, ui.KeyEscape)
			for i, advance := range []time.Duration{80 * time.Millisecond, 100 * time.Millisecond, time.Second} {
				tt.Advance(advance)
				saveTransitionFrame(t, tt, scene.name+"-"+[]string{"closing-80ms", "closing-180ms", "closed"}[i])
			}
		})
	}
}

func saveTransitionFrame(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, "transition-"+name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// Now Playing keeps its clock updating after it covers the player bar,
// without input or notifications from the player.
func TestNowPlayingTimesWithoutInput(t *testing.T) {
	for name, pane := range map[string]int{"lyrics": paneLyrics, "queue": paneQueue, "stage": paneStage, "wall": paneWall} {
		t.Run(name, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.setProgressEffect(progressStandard)
			a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 200000}})
			a.pl.SetLyrics(nil)
			out := audio.NewPlayer(&silence{})
			defer out.Close()
			out.Seek(10 * time.Second)
			a.pl.Attach(out)
			a.nowPlaying, a.npPane = true, pane
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Advance(2 * time.Second)
			_, barVisible := tt.Find("打开正在播放")
			if !a.npOpen || barVisible {
				t.Fatal("Now Playing did not cover the page")
			}
			for _, sec := range []int{11, 12, 13} {
				// Move only the audio clock, as playback does; do not force a
				// build through pl.Seek, pointer input, or Tester.Frame.
				out.Seek(time.Duration(sec) * time.Second)
				tt.Advance(1100 * time.Millisecond)
				elapsed := clock(time.Duration(sec) * time.Second)
				if pane == paneStage {
					elapsed += " / " + clock(200*time.Second)
				}
				if !tt.HasText(elapsed) {
					t.Fatalf("elapsed time did not update at %ds", sec)
				}
				if pane != paneStage && !tt.HasText("-"+clock(time.Duration(200-sec)*time.Second)) {
					t.Fatalf("remaining time did not update at %ds", sec)
				}
			}
		})
	}
}

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
	tr := qqmusic.Track{ID: 7, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(nil)
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

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
	if a.pl.Playing() {
		t.Error("Space did not pause")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.pl.Playing() {
		t.Error("Space did not play again")
	}
}
