package app

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

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
	art := &artwork{bmp: solidBitmap(radixAmber(false, 9)), tint: radixAmber(false, 9)}
	for _, size := range []int{120, 600} {
		a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
	}
	a.art.art[qqmusic.Sized(tr.Album.PicURL, 600)+"#backdrop"] = art
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
			art := &artwork{bmp: solidBitmap(radixAmber(false, 9)), tint: radixAmber(false, 9)}
			for _, size := range []int{120, 600} {
				a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
			}
			a.art.art[qqmusic.Sized(tr.Album.PicURL, 600)+"#backdrop"] = art
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
