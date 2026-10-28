package app

import (
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic-mygo/internal/qqmusic"
)

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
