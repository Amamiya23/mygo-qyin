package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// skeletonScenes are the views that load, as they show while they do.
var skeletonScenes = []struct {
	name  string
	set   func(a *app)
	texts []string // what shows for real meanwhile
}{
	{"home", func(a *app) {}, []string{"官方榜单", "推荐歌单"}},
	{"charts", func(a *app) { a.route = route{page: pageCharts} }, []string{"排行榜", "全部榜单"}},
	{"daily", func(a *app) { a.route = route{page: pageDaily} }, []string{"每日歌曲推荐"}},
	{"playlist", func(a *app) {
		a.route = route{page: pagePlaylist, playlist: 7}
		a.loadingPl[7] = true
	}, nil},
	{"search", func(a *app) {
		a.route = route{page: pageSearch}
		a.searched, a.searching = "春日影", true
	}, []string{"“春日影”"}},
	{"login", func(a *app) { a.login = loginState{open: true} }, []string{"扫码登录"}},
	{"comments", func(a *app) {
		a.nowPlaying, a.npPane = true, paneComments
		a.comments = commentState{track: 1, loading: true}
	}, nil},
	{"more-comments", func(a *app) {
		a.nowPlaying, a.npPane = true, paneComments
		a.comments = commentState{track: 1, loading: true, loaded: true, total: 1, latest: []netease.Comment{{ID: 1, Content: "好听"}}}
	}, []string{"好听"}},
	{"lyrics", func(a *app) { a.nowPlaying, a.npPane = true, paneLyrics }, nil},
	{"stage", func(a *app) { a.nowPlaying, a.npPane = true, paneStage }, nil},
}

func skeletonApp(set func(a *app)) *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.homeLoading = true
	a.pl.Cue([]netease.Track{{ID: 1, Name: "春日影"}})
	set(a)
	return a
}

// TestSkeletons shows each view that loads in its skeleton, with what it
// knows already, and the shimmer moving over it, still if asked.
func TestSkeletons(t *testing.T) {
	for _, s := range skeletonScenes {
		t.Run(s.name, func(t *testing.T) {
			a := skeletonApp(s.set)
			tt := ui.NewTester(a.view, 1200, 800)
			for _, text := range s.texts {
				if !tt.HasText(text) {
					t.Errorf("missing %q in %q", text, tt.Texts())
				}
			}
			if tt.HasText("重试") {
				t.Error("shows a retry while loading")
			}
			if !tt.FramePending() {
				t.Error("the shimmer asks for no frame")
			}
		})
	}
}

// With NM_FRAMES set, saves each skeleton in the light and the dark, the
// shimmer partway across.
func TestSkeletonFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range skeletonScenes {
		for _, dark := range []bool{false, true} {
			a := skeletonApp(s.set)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(450 * time.Millisecond)
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "skeleton-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
