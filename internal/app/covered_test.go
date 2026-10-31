package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// The page under Now Playing is not built while Now Playing covers it, and
// shows again where it was scrolled once Now Playing closes.
func TestCoveredPageKeepsItsPlace(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tracks := make([]qqmusic.Track, 200)
	for i := range tracks {
		tracks[i] = qqmusic.Track{ID: int64(i + 1), Name: fmt.Sprintf("曲目%d", i+1)}
	}
	a.playlists[42] = &qqmusic.Playlist{ID: 42, Name: "飙升榜", Tracks: tracks}
	for i := range 40 {
		a.charts = append(a.charts, qqmusic.Playlist{ID: int64(100 + i), Name: fmt.Sprintf("榜单%d", i+1)})
	}
	a.pl.Cue([]qqmusic.Track{{ID: 9999, Name: "一张相片"}})
	tt := ui.NewTester(a.view, 1200, 800)

	for _, r := range []route{{page: pagePlaylist, id: 42}, {page: pageCharts}} {
		a.open(r)
		tt.Frame()
		var names []string
		if r.page == pagePlaylist {
			for _, track := range tracks {
				names = append(names, track.Name)
			}
		} else {
			for _, chart := range a.charts {
				names = append(names, chart.Name)
			}
		}
		initialName, initialAt := visibleNavigationItem(t, tt, names)
		tt.Wheel(700, 400, 0, 2400)
		tt.Advance(time.Second)
		tt.Frame()
		name, before := visibleNavigationItem(t, tt, names)
		if name == initialName && before == initialAt {
			t.Fatalf("%+v: the wheel did not scroll it", r)
		}

		a.nowPlaying = true
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if tt.HasText("为你推荐") {
			t.Errorf("%+v: the sidebar is built under Now Playing", r)
		}

		a.nowPlaying = false
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		// Check the content's actual anchor, not the incidental phase of
		// every hovered heart and animated control elsewhere in the window.
		assertNavigationPosition(t, tt, name, before)
	}
}
