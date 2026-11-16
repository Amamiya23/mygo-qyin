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

// The queue opens over the current page and closes through its button, Escape and outside presses.
func TestPlayerQueuePopoverInteraction(t *testing.T) {
	a := queuePopupApp(12)
	a.npPane = paneLyrics
	route := a.route
	tt := ui.NewTester(a.view, 1180, 700)
	for _, dismiss := range []string{"button", "escape", "outside"} {
		if err := tt.Click("播放列表"); err != nil {
			t.Fatal(err)
		}
		tt.Move(10, 10)
		tt.Advance(time.Second)
		if !a.queueOpen || a.nowPlaying || a.npPane != paneLyrics || a.route != route || len(a.history) != 0 {
			t.Fatalf("opening the queue changed the page or pane: open=%v, nowPlaying=%v, route=%v", a.queueOpen, a.nowPlaying, a.route)
		}
		switch dismiss {
		case "button":
			if err := tt.Click("播放列表"); err != nil {
				t.Fatal(err)
			}
		case "escape":
			tt.Key(0, ui.KeyEscape)
		case "outside":
			tt.ClickAt(10, 400)
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
			if err := tt.Click("播放列表"); err != nil {
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
	if err := tt.Click("播放列表"); err != nil {
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
	if err := tt.Click("播放列表"); err != nil {
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
	if err := tt.Click("播放列表"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	first, last = a.queuePopup.Visible()
	if first > 30 || last < 30 {
		t.Fatalf("reopening did not locate current track: %d..%d", first, last)
	}
}

// The row's right edge toggles playback; the scrollbar used to intercept clicks over its highlight.
func TestPlayerQueuePopoverScrollbarGutter(t *testing.T) {
	a := queuePopupApp(60)
	tt := ui.NewTester(a.view, 1180, 700)
	if err := tt.Click("播放列表"); err != nil {
		t.Fatal(err)
	}
	row, ok := tt.AccessByRole(ui.RoleButton, a.pl.Current().Name)
	if !ok {
		t.Fatal("cannot target the current queue row")
	}
	r := row.Bounds
	tt.ClickAt(r.X+r.W-2, r.Y+r.H/2)
	if !a.pl.Playing() || a.pl.Current().ID != 1 {
		t.Fatal("the scrollbar intercepted the song row's right edge")
	}
	first, _ := a.queuePopup.Visible()
	// The thumb sits in the gutter, outside the song's clickable rectangle.
	x, y := r.X+r.W+8, r.Y+6
	tt.Press(x, y)
	tt.Move(x, y+150)
	tt.Release(x, y+150)
	tt.Frame()
	if got, _ := a.queuePopup.Visible(); got <= first {
		t.Fatal("dragging the gutter did not scroll the queue")
	}
	if !a.pl.Playing() || a.pl.Current().ID != 1 || !a.queueOpen {
		t.Fatal("dragging the scrollbar changed playback or closed the queue")
	}
}

// Short queues do not move on wheel input; their popup used to omit padding and overflow by a few DIPs.
func TestPlayerQueuePopoverShortQueueFits(t *testing.T) {
	for _, n := range []int{1, 3} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			a := queuePopupApp(n)
			tt := ui.NewTester(a.view, 1180, 700)
			if err := tt.Click("播放列表"); err != nil {
				t.Fatal(err)
			}
			row, ok := tt.AccessByRole(ui.RoleButton, a.pl.Current().Name)
			if !ok {
				t.Fatal("cannot target the current queue row")
			}
			r := row.Bounds
			tt.Wheel(r.X+r.W/2, r.Y+r.H/2, 0, 240)
			tt.Advance(time.Second)
			after, ok := tt.AccessByRole(ui.RoleButton, a.pl.Current().Name)
			if !ok || after.Bounds.Y != r.Y {
				t.Fatal("a short queue scrolled despite fitting in the popup")
			}
		})
	}
}

// Navigating from queue artist links or opening Now Playing closes the floating queue.
func TestPlayerQueuePopoverNavigation(t *testing.T) {
	a := queuePopupApp(3)
	a.artists[2] = &artistData{}
	tt := ui.NewTester(a.view, 1180, 700)
	if err := tt.Click("播放列表"); err != nil {
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
	if err := tt.Click("播放列表"); err != nil {
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
	if err := tt.Click("播放列表"); err != nil {
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
	for _, n := range []int{3, 18} {
		for _, dark := range []bool{false, true} {
			for _, size := range [][2]int{{1180, 700}, {960, 580}} {
				a := queuePopupApp(n)
				tt := ui.NewTester(a.view, size[0], size[1])
				tt.SetDark(dark)
				if err := tt.Click("播放列表"); err != nil {
					t.Fatal(err)
				}
				for _, hovered := range []bool{false, true} {
					if hovered {
						row, ok := tt.AccessByRole(ui.RoleButton, a.pl.Queue()[1].Name)
						if !ok {
							t.Fatal("cannot target the hovered queue row")
						}
						r := row.Bounds
						tt.Move(r.X+r.W/2, r.Y+r.H/2)
					} else {
						tt.Move(10, 10)
					}
					tt.Advance(time.Second)
					tt.Frame()
					path := filepath.Join(dir, fmt.Sprintf("queue-%d-dark-%t-%dx%d-hover-%t.png", n, dark, int(size[0]), int(size[1]), hovered))
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
	}
}

func TestQueueRenameAndContextMenuRemove(t *testing.T) {
	t.Run("label renamed from 待播清单 to 播放列表", func(t *testing.T) {
		a := queuePopupApp(3)
		tt := ui.NewTester(a.view, 1180, 700)
		tt.Frame()
		if tt.HasText("待播清单") {
			t.Fatalf("unexpected 待播清单 in main view: %q", tt.Texts())
		}
		if err := tt.Click("播放列表"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if tt.HasText("待播清单") || !tt.HasText("播放列表") {
			t.Fatalf("floating queue texts mismatch: %q", tt.Texts())
		}

		// Check Now Playing tabs for both song and episode.
		a.queueOpen = false
		a.nowPlaying = true
		tt.Advance(time.Second)
		tt.Frame()
		if tt.HasText("待播清单") || !tt.HasText("播放列表") {
			t.Fatalf("now playing song tabs mismatch: %q", tt.Texts())
		}

		a.pl.Cue([]qqmusic.Track{{ID: 901, ProgramID: 55, Name: "测试播客单集", Duration: 180000}})
		a.pl.SetLyrics(nil)
		tt.Frame()
		if tt.HasText("待播清单") || !tt.HasText("播放列表") {
			t.Fatalf("now playing episode tabs mismatch: %q", tt.Texts())
		}
	})

	for _, surface := range []string{"popover", "nowplaying"} {
		t.Run("remove song in "+surface, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.route = route{page: pageCharts}
			a.pl.Cue([]qqmusic.Track{
				{ID: 1, Name: "第一首", Duration: 200000},
				{ID: 2, Name: "第二首", Duration: 200000},
				{ID: 2, Name: "第二首重复", Duration: 200000},
				{ID: 3, ProgramID: 88, PodcastID: 9, Name: "播客节目", Duration: 200000},
			})
			a.pl.SetLyrics(nil)
			a.pl.Hold(18 * time.Second)

			tt := ui.NewTester(a.view, 1180, 700)
			if surface == "popover" {
				if err := tt.Click("播放列表"); err != nil {
					t.Fatal(err)
				}
			} else {
				a.nowPlaying = true
				a.npPane = paneQueue
				tt.Frame()
				tt.Advance(time.Second)
			}
			tt.Frame()

			rightClickRow := func(name string) {
				t.Helper()
				row, ok := tt.AccessByRole(ui.RoleButton, name)
				if !ok {
					t.Fatalf("queue row %q not found", name)
				}
				tt.RightClickAt(row.Bounds.X+row.Bounds.W/2, row.Bounds.Y+row.Bounds.H/2)
			}

			// Remove a duplicate-ID non-current row; current track and its sibling must remain.
			rightClickRow("第二首重复")
			if err := tt.ChooseMenuItem("移除歌曲"); err != nil {
				t.Fatalf("%v; menu=%q", err, tt.Menu())
			}
			tt.Frame()
			if a.pl.Current() == nil || a.pl.Current().ID != 1 || len(a.pl.Queue()) != 3 || !a.pl.Playing() {
				t.Fatalf("removing duplicate track disrupted playback: cur=%v len=%d playing=%v", a.pl.Current(), len(a.pl.Queue()), a.pl.Playing())
			}
			if a.pl.Queue()[1].Name != "第二首" {
				t.Fatalf("removed wrong duplicate track: queue=%+v", a.pl.Queue())
			}

			// Remove podcast episode (ProgramID != 0) via right-click menu.
			rightClickRow("播客节目")
			if err := tt.ChooseMenuItem("移除歌曲"); err != nil {
				t.Fatalf("podcast episode missing 移除歌曲: %v; menu=%q", err, tt.Menu())
			}
			tt.Frame()
			if len(a.pl.Queue()) != 2 || a.pl.Current().ID != 1 {
				t.Fatalf("removing podcast episode failed: cur=%v len=%d", a.pl.Current(), len(a.pl.Queue()))
			}

			// Remove the current track: advances to "第二首".
			rightClickRow("第一首")
			if err := tt.ChooseMenuItem("移除歌曲"); err != nil {
				t.Fatalf("%v; menu=%q", err, tt.Menu())
			}
			tt.Frame()
			if a.pl.Current() == nil || a.pl.Current().Name != "第二首" || len(a.pl.Queue()) != 1 {
				t.Fatalf("removing current track failed: cur=%v len=%d", a.pl.Current(), len(a.pl.Queue()))
			}

			// Remove the final remaining track: clears player and closes queue/now playing.
			rightClickRow("第二首")
			if err := tt.ChooseMenuItem("移除歌曲"); err != nil {
				t.Fatalf("%v; menu=%q", err, tt.Menu())
			}
			tt.Frame()
			if a.pl.Current() != nil || len(a.pl.Queue()) != 0 || a.queueOpen || a.nowPlaying {
				t.Fatalf("removing final track left open state: cur=%v len=%d queueOpen=%v nowPlaying=%v", a.pl.Current(), len(a.pl.Queue()), a.queueOpen, a.nowPlaying)
			}
		})
	}
}

