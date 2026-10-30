package app

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Returning from a scrolled playlist used to reset the home and chart grids, and Forward lost the playlist's place too.
func TestNavigationRestoresScroll(t *testing.T) {
	for _, page := range []pageKind{pageHome, pageCharts} {
		t.Run(fmt.Sprint(page), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			var cards, songs []string
			var tracks []qqmusic.Track
			for i := range 100 {
				name := fmt.Sprintf("曲目%03d", i+1)
				songs = append(songs, name)
				tracks = append(tracks, qqmusic.Track{ID: int64(i + 1), Name: name})
			}
			for i := range 40 {
				name := fmt.Sprintf("浏览歌单%02d", i+1)
				cards = append(cards, name)
				pl := qqmusic.Playlist{ID: int64(1000 + i), Name: name, Tracks: tracks}
				a.playlists[pl.ID] = &pl
				if page == pageHome {
					a.featured = append(a.featured, pl)
				} else {
					a.charts = append(a.charts, pl)
				}
			}
			a.open(route{page: page})
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			tt.Scroll(1000, 550, 0, 600)
			tt.Frame()
			card, cardAt := visibleNavigationItem(t, tt, cards)
			if !a.underTop {
				t.Fatal("grid did not scroll")
			}
			if err := tt.Click(card); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.route.page != pagePlaylist {
				t.Fatalf("card did not open a playlist: %+v", a.route)
			}
			playlistRoute := a.route
			tt.Scroll(1000, 550, 0, 700)
			tt.Frame()
			song, songAt := visibleNavigationItem(t, tt, songs)
			if !a.underTop {
				t.Fatal("playlist did not scroll")
			}
			for range 2 {
				if err := tt.Click("返回"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if a.route.page != page {
					t.Fatalf("returned to %+v", a.route)
				}
				assertNavigationPosition(t, tt, card, cardAt)
				if err := tt.Click("前进"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if a.route != playlistRoute {
					t.Fatalf("forward opened %+v, want %+v", a.route, playlistRoute)
				}
				assertNavigationPosition(t, tt, song, songAt)
			}
		})
	}
}

// Different playlists keep independent row anchors, and a newly opened playlist starts at its top.
func TestPlaylistsKeepIndependentScroll(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	names := make([][]string, 2)
	for i := range 2 {
		pl := &qqmusic.Playlist{ID: int64(i + 1), Name: fmt.Sprintf("歌单%d", i+1)}
		for j := range 100 {
			name := fmt.Sprintf("歌单%d曲目%03d", i+1, j+1)
			names[i] = append(names[i], name)
			pl.Tracks = append(pl.Tracks, qqmusic.Track{ID: int64(i*100 + j + 1), Name: name})
		}
		a.playlists[pl.ID] = pl
	}
	a.open(route{page: pagePlaylist, id: 1})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Scroll(1000, 550, 0, 700)
	tt.Frame()
	first, firstAt := visibleNavigationItem(t, tt, names[0])
	a.open(route{page: pagePlaylist, id: 2})
	tt.Frame()
	if a.underTop {
		t.Fatal("a new playlist inherited the previous playlist's scroll")
	}
	tt.Scroll(1000, 550, 0, 1000)
	tt.Frame()
	second, secondAt := visibleNavigationItem(t, tt, names[1])
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	assertNavigationPosition(t, tt, first, firstAt)
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	assertNavigationPosition(t, tt, second, secondAt)
}

// visibleNavigationItem finds a fully visible card or row below the navigation bar and above the player.
func visibleNavigationItem(t *testing.T, tt *ui.Tester, names []string) (string, ui.Rect) {
	t.Helper()
	for _, name := range names {
		if r, ok := tt.Find(name); ok && r.Y > 90 && r.Y+r.H < 670 {
			return name, r
		}
	}
	t.Fatal("no visible navigation target")
	return "", ui.Rect{}
}

// assertNavigationPosition checks the actual restored geometry rather than a saved offset alone.
func assertNavigationPosition(t *testing.T, tt *ui.Tester, name string, want ui.Rect) {
	t.Helper()
	got, ok := tt.Find(name)
	if !ok || math.Abs(float64(got.Y-want.Y)) > 1 || math.Abs(float64(got.X-want.X)) > 1 {
		t.Fatalf("%s moved after navigation: got %+v (visible %v), want %+v", name, got, ok, want)
	}
}
