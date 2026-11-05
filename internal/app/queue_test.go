package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func queuePopupApp(n int) *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.route = route{page: pageCharts}
	tracks := make([]qqmusic.Track, n)
	for i := range tracks {
		tracks[i] = qqmusic.Track{ID: int64(i + 1), Name: fmt.Sprintf("队列歌曲 %02d", i+1), Duration: 200000,
			Artists: []qqmusic.Artist{{ID: int64(i + 1), Name: fmt.Sprintf("歌手 %02d", i+1)}}}
	}
	if n > 0 {
		a.pl.Cue(tracks)
		a.pl.SetLyrics(nil)
	}
	return a
}

// The queue opens over the current page and closes through its button, Escape, outside presses and its close control.
func TestPlayerQueuePopoverInteraction(t *testing.T) {
	a := queuePopupApp(12)
	a.npPane = paneLyrics
	route := a.route
	tt := ui.NewTester(a.view, 1180, 700)
	for _, dismiss := range []string{"button", "escape", "outside", "close"} {
		if err := tt.Click("待播清单"); err != nil {
			t.Fatal(err)
		}
		tt.Move(10, 10)
		tt.Advance(time.Second)
		if !a.queueOpen || a.nowPlaying || a.npPane != paneLyrics || a.route != route || len(a.history) != 0 {
			t.Fatalf("opening the queue changed the page or pane: open=%v, nowPlaying=%v, route=%v", a.queueOpen, a.nowPlaying, a.route)
		}
		switch dismiss {
		case "button":
			if err := tt.Click("待播清单"); err != nil {
				t.Fatal(err)
			}
		case "escape":
			tt.Key(0, ui.KeyEscape)
		case "outside":
			tt.ClickAt(10, 400)
		case "close":
			if err := tt.Click("关闭待播清单"); err != nil {
				t.Fatal(err)
			}
		}
		if a.queueOpen || a.pl.Playing() || a.pl.Current().ID != 1 || a.route != route {
			t.Fatalf("%s did not dismiss locally", dismiss)
		}
	}
}

// Queue clicks use the actual play order without replacing the queue or opening Now Playing.
func TestPlayerQueuePopoverPlaysTracks(t *testing.T) {
	for _, shuffled := range []bool{false, true} {
		t.Run(fmt.Sprint(shuffled), func(t *testing.T) {
			a := queuePopupApp(12)
			if shuffled {
				a.pl.ToggleShuffle()
			}
			order, at := a.pl.PlayOrder()
			before := slices.Clone(order)
			i := order[(at+1)%len(order)]
			target := a.pl.Queue()[i]
			tt := ui.NewTester(a.view, 1180, 700)
			if err := tt.Click("待播清单"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(target.Name); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			after, _ := a.pl.PlayOrder()
			if a.pl.Current().ID != target.ID || !slices.Equal(before, after) || len(a.pl.Queue()) != 12 || !a.queueOpen || a.nowPlaying {
				t.Fatal("queue selection did not play the selected track in place")
			}
			// Focus remains on this row when another control advances the track.
			a.pl.Jump(order[(at+2)%len(order)])
			tt.Frame()
			current := a.pl.Current().ID
			playing := a.pl.Playing()
			tt.Key(0, ui.KeySpace)
			if a.pl.Current().ID != current || a.pl.Playing() == playing {
				t.Fatal("Space activated the old focused row instead of toggling the current track")
			}
		})
	}
}

// An unavailable queue entry does not replace the current track.
func TestPlayerQueuePopoverUnavailableTrack(t *testing.T) {
	a := queuePopupApp(3)
	a.pl.Queue()[1].Privilege = &qqmusic.Privilege{St: -1}
	tt := ui.NewTester(a.view, 1180, 700)
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(a.pl.Queue()[1].Name); err != nil {
		t.Fatal(err)
	}
	if a.pl.Current().ID != 1 || a.pl.Playing() || !a.queueOpen {
		t.Fatal("an unavailable queue entry changed playback")
	}
}

// Manual queue scrolling survives rebuilds, and reopening locates the current track.
func TestPlayerQueuePopoverScroll(t *testing.T) {
	a := queuePopupApp(60)
	a.pl.Jump(30)
	tt := ui.NewTester(a.view, 1180, 700)
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	first, last := a.queuePopup.Visible()
	if first > 30 || last < 30 {
		t.Fatalf("current track is outside opened queue: %d..%d", first, last)
	}
	r, ok := tt.Find(a.pl.Queue()[31].Name)
	if !ok {
		t.Fatal("cannot target a queue row")
	}
	tt.Wheel(r.X+r.W/2, r.Y+r.H/2, 0, 720)
	tt.Advance(time.Second)
	tt.Frame()
	scrolled, _ := a.queuePopup.Visible()
	if scrolled <= first {
		t.Fatal("wheel input did not scroll the queue")
	}
	for range 3 {
		tt.Frame()
	}
	if got, _ := a.queuePopup.Visible(); got != scrolled {
		t.Fatalf("rebuilding reset queue scroll: %d to %d", scrolled, got)
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	first, last = a.queuePopup.Visible()
	if first > 30 || last < 30 {
		t.Fatalf("reopening did not locate current track: %d..%d", first, last)
	}
}

// Navigating from queue artist links or opening Now Playing closes the floating queue.
func TestPlayerQueuePopoverNavigation(t *testing.T) {
	a := queuePopupApp(3)
	a.artists[2] = &artistData{}
	tt := ui.NewTester(a.view, 1180, 700)
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	r, ok := tt.Find("歌手 02")
	if !ok {
		t.Fatal("cannot target the queue artist")
	}
	// A single artist's rich paragraph spans the row; click its first word.
	tt.ClickAt(r.X+5, r.Y+r.H/2)
	tt.Frame()
	if a.queueOpen || a.route.page != pageArtist || a.route.id != 2 {
		t.Fatalf("artist navigation: queue open=%v, route=%+v", a.queueOpen, a.route)
	}
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.nowPlaying || a.queueOpen {
		t.Fatal("opening Now Playing left the floating queue open")
	}
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	tt.Frame()
	if a.nowPlaying || a.queueOpen {
		t.Fatal("closing Now Playing restored the floating queue")
	}
}

// The empty player cannot open a queue without a current track.
func TestPlayerQueuePopoverEmpty(t *testing.T) {
	a := queuePopupApp(0)
	tt := ui.NewTester(a.view, 1180, 700)
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	if a.queueOpen || a.nowPlaying {
		t.Fatal("the empty player opened a queue")
	}
}

// NM_FRAMES saves floating queues for visual review in both themes and window sizes.
func TestPlayerQueuePopoverFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("NM_FRAMES not set")
	}
	for _, dark := range []bool{false, true} {
		for _, size := range [][2]int{{1180, 700}, {960, 580}} {
			a := queuePopupApp(18)
			tt := ui.NewTester(a.view, size[0], size[1])
			tt.SetDark(dark)
			if err := tt.Click("待播清单"); err != nil {
				t.Fatal(err)
			}
			tt.Move(10, 10)
			tt.Advance(time.Second)
			tt.Frame()
			path := filepath.Join(dir, fmt.Sprintf("queue-dark-%t-%dx%d.png", dark, int(size[0]), int(size[1])))
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
