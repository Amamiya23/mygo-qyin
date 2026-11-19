package app

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
		if tt.HasText("首页") {
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

func TestScrollEdgesFollowContent(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	for i := range 40 {
		a.featured = append(a.featured, qqmusic.Playlist{ID: int64(i + 1), Name: "歌单"})
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.underTop {
		t.Error("top edge shows before scrolling")
	}
	if !a.underBottom {
		t.Error("no bottom edge over a long page")
	}
	tt.Scroll(800, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	if !a.underTop {
		t.Error("no top edge after scrolling")
	}
	if a.underBottom {
		t.Error("bottom edge shows at the end of the page")
	}
}

func TestShortPageHasNoEdges(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.featured = []qqmusic.Playlist{{ID: 1, Name: "歌单"}}
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if a.underTop || a.underBottom {
		t.Errorf("edges on a page that fits: top %v bottom %v", a.underTop, a.underBottom)
	}
}

// Opening or closing the account menu leaves the player's edge as it was:
// the content's box had no key, so the menu coming or going made it anew,
// and the edge faded in again from nothing, showing the page bare under
// the player until something asked for another frame.
func TestAccountMenuKeepsPlayerEdge(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "小明"}
	for i := range 40 {
		a.featured = append(a.featured, qqmusic.Playlist{ID: int64(i + 1), Name: "歌单"})
	}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	// The player's foot, right of where the menu's shadow reaches.
	bar := func() *image.RGBA {
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		r := image.Rect(int((sidebarWidth+60)*s), int((700-playerBarHeight)*s), img.Bounds().Dx(), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
		return out
	}
	want := bar()
	for _, step := range []string{"open", "close"} {
		if err := tt.Click("小明"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !bytes.Equal(bar().Pix, want.Pix) {
			t.Fatalf("the player's edge changed on %s", step)
		}
	}
}

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &qqmusic.Playlist{ID: 42, Name: "飙升榜", Tracks: []qqmusic.Track{{ID: 1, Name: "一张相片"}}}
	tt := ui.NewTester(a.view, 1200, 800)
	charts, list := route{page: pageCharts}, route{page: pagePlaylist, id: 42}
	a.open(charts)
	a.open(list)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != charts {
		t.Fatalf("back went to %+v", a.route)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	a.open(route{page: pageHome})
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != (route{page: pageHome}) {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}

// With NM_FRAMES set, previews Acrylic controls over a synthetic soft backdrop, without claiming to render the Windows material.
func TestAcrylicSidebarFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" || runtime.GOOS != "windows" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := editTestApp()
		a.settings.SidebarMaterial = "acrylic"
		a.pl.Cue(a.playlists[10].Tracks)
		tt := ui.NewTester(func(c *ui.Context) {
			p := paletteFor(c.Theme())
			ui.Box(c).Fill().Draw(func(painter *ui.Painter, r ui.Rect) {
				painter.FillGradient(r, ui.LinearGradient{Angle: 160, Stops: []ui.GradientStop{
					{Color: radixGreen(dark, 4).Mix(p.sidebar, 0.7), At: 0},
					{Color: radixAmber(dark, 4).Mix(p.sidebar, 0.7), At: 0.5},
					{Color: radixRed(dark, 4).Mix(p.sidebar, 0.7), At: 1},
				}}, 0)
			}).Children(func() { a.view(c) })
		}, 1180, 780)
		tt.SetDark(dark)
		tt.SetVibrancy(true)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		name := "acrylic-sidebar-light.png"
		if dark {
			name = "acrylic-sidebar-dark.png"
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}

// Each playlist header hides only its own rows without changing the page
// or queue, and Enter restores their navigation.
func TestSidebarPlaylistCollapse(t *testing.T) {
	for _, tc := range []struct {
		name, header, row, other string
		playlist                 int64
	}{
		{"mine", "我的歌单", "睡前", "飙升榜", 11},
		{"saved", "收藏的歌单", "飙升榜", "睡前", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := editTestApp()
			a.playlists[tc.playlist] = &qqmusic.Playlist{ID: tc.playlist, Name: tc.row}
			a.pl.Cue(a.playlists[10].Tracks)
			before := a.route
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()

			if err := tt.Click(tc.header); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if _, ok := tt.Find(tc.row); ok {
				t.Error("the collapsed section still builds its playlist rows")
			}
			if _, ok := tt.Find(tc.other); !ok {
				t.Error("collapsing one section hid the other section's rows")
			}
			if a.route != before || len(a.pl.Queue()) != 2 || len(a.mine) != 3 || len(a.saved) != 1 {
				t.Error("collapsing the section changed the page, queue or playlists")
			}

			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			if err := tt.Click(tc.row); err != nil {
				t.Fatalf("the restored playlist cannot be opened: %v", err)
			}
			tt.Frame()
			if a.route != (route{page: pagePlaylist, id: tc.playlist}) {
				t.Errorf("opened %+v, want playlist %d", a.route, tc.playlist)
			}
		})
	}
}

// The plus still opens the naming dialog with both sections collapsed, without expanding either one.
func TestNewPlaylistInCollapsedSidebar(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, header := range []string{"我的歌单", "收藏的歌单"} {
		if err := tt.Click(header); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if !a.mineCollapsed || !a.savedCollapsed {
		t.Fatal("the sections did not collapse independently")
	}
	if err := tt.Click("新建歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.naming.open {
		t.Error("the plus did not open the naming dialog")
	}
	if !a.mineCollapsed || !a.savedCollapsed {
		t.Error("the plus also toggled a playlist section")
	}
}
