package app

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Locating a distant current song scrolls to it smoothly, including while
// paused, without replacing its queue or changing playback or progress.
func TestLocateCurrentTrack(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current int
		start   int
		playing bool
		covered bool
		reduced bool
	}{
		{name: "playing below", current: 149, playing: true},
		{name: "paused above", current: 20, start: 205},
		{name: "covered by player", current: 79, playing: true, covered: true},
		{name: "reduced motion below", current: 149, playing: true, reduced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt, tracks, insets := trackLocateApp(tc.current, tc.playing)
			if tc.reduced {
				tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				tt.Frame()
				tt.Frame()
			}
			state := &a.pageState().tracks
			if tc.start > 0 {
				state.ScrollTo(tc.start+1, ui.Start)
				tt.Frame()
				tt.Frame()
			}
			if tc.covered {
				state.ScrollTo(tc.current+1, ui.Center)
				tt.Frame()
				tt.Frame()
				tt.Scroll(1000, 400, 0, -380)
				for range 30 {
					tt.Advance(16 * time.Millisecond)
					tt.Frame()
				}
			}
			queue := slices.Clone(a.pl.Queue())
			order, at := a.pl.PlayOrder()
			order = slices.Clone(order)
			position := a.pl.Position()
			first, last := state.Visible()
			inListView := tc.current+1 >= first && tc.current+1 <= last
			if tc.covered {
				r, ok := tt.Find(tracks[tc.current].Name)
				if !inListView || !ok || r.Y < 800-insets[1] {
					t.Fatalf("track was not covered by the player while still in the list viewport: %+v, found %v, rows %d-%d", r, ok, first, last)
				}
			} else if inListView {
				t.Fatal("current track was already in view")
			}
			if tc.name == "playing below" {
				saveTrackLocateFrame(t, tt, "track-locate-before.png")
			}
			if err := tt.Click("定位当前歌曲"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if r, ok := tt.Find(tracks[tc.current].Name); !tc.reduced && ok && math.Abs(float64(r.Y+r.H/2-(insets[0]+800-insets[1])/2)) < 2 {
				t.Fatal("locating jumped to the destination without a transition")
			}
			for range 90 {
				tt.Advance(16 * time.Millisecond)
				tt.Frame()
			}
			r, ok := tt.Find(tracks[tc.current].Name)
			if !ok || r.Y < insets[0] || r.Y+r.H > 800-insets[1] {
				t.Fatalf("current track not in the usable viewport: %+v, found %v, insets %v", r, ok, insets)
			}
			if !tc.reduced && math.Abs(float64(r.Y+r.H/2-(insets[0]+800-insets[1])/2)) > 2 {
				t.Fatalf("current track did not settle at the viewport centre: %+v, insets %v", r, insets)
			}
			if _, ok := tt.Find("定位当前歌曲"); ok {
				t.Error("locator remained after the current track came into view")
			}
			if tc.name == "playing below" {
				saveTrackLocateFrame(t, tt, "track-locate-after.png")
			}
			if !reflect.DeepEqual(a.pl.Queue(), queue) || a.pl.Playing() != tc.playing || a.pl.Position() != position {
				t.Fatalf("locating changed playback: playing %v, position %v, queue %+v", a.pl.Playing(), a.pl.Position(), a.pl.Queue())
			}
			gotOrder, gotAt := a.pl.PlayOrder()
			if !slices.Equal(gotOrder, order) || gotAt != at {
				t.Fatalf("locating changed play order: %v at %d", gotOrder, gotAt)
			}
			// Browsing away brings the control back without following the song.
			tt.Scroll(1000, 400, 0, 1600)
			for range 30 {
				tt.Advance(16 * time.Millisecond)
				tt.Frame()
			}
			if _, ok := tt.Find("定位当前歌曲"); !ok {
				t.Error("locator did not return after scrolling away")
			}
		})
	}
}

// A list only offers to locate its current song when that song belongs to it.
func TestTrackLocatorOnlyForItsList(t *testing.T) {
	a, tt, _, _ := trackLocateApp(149, false)
	a.pl.Cue([]qqmusic.Track{{ID: 999, Name: "其他列表的歌曲", Duration: 200000}})
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("定位当前歌曲"); ok {
		t.Fatal("locator shown for a song outside this list")
	}
}

func trackLocateApp(current int, playing bool) (*app, *ui.Tester, []qqmusic.Track, *[2]float32) {
	a := newTestApp()
	a.settings = defaultSettings()
	tracks := make([]qqmusic.Track, 220)
	for i := range tracks {
		tracks[i] = qqmusic.Track{ID: int64(i + 1), Name: fmt.Sprintf("定位曲目%03d", i+1), Duration: 200000}
	}
	a.pl.Cue([]qqmusic.Track{tracks[current], {ID: 998, Name: "原队列下一首", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.pl.ToggleShuffle()
	if playing {
		a.pl.Hold(73 * time.Second)
	} else {
		a.pl.Seek(73 * time.Second)
	}
	insets := new([2]float32)
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		insets[0], insets[1] = a.insets(c)
		ui.Column(c).Fill().Children(func() {
			a.trackRows(c, tracks, true, func(c *ui.Context) { ui.Box(c).Height(280) })
		})
	}, 1200, 800)
	tt.Frame()
	tt.Frame()
	return a, tt, tracks, insets
}

func saveTrackLocateFrame(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, tt.Image())
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
}
