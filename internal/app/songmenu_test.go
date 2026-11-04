package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	tr := qqmusic.Track{ID: 1859245776, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}, {Name: "小明"}}, Duration: 200000}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	for _, c := range []struct{ item, want string }{
		{"复制歌名", "宝船"},
		{"复制歌名和歌手", "宝船 - DOUDOU, 小明"},
		{"复制链接", "https://y.qq.com/n/ryqq/songDetail/1859245776"},
	} {
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &qqmusic.Playlist{ID: 42, Name: "飙升榜", Tracks: []qqmusic.Track{
		{ID: 1, Name: "一张相片", Artists: []qqmusic.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, id: 42}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := "https://y.qq.com/n/ryqq/songDetail/2"; copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}

// Choosing play-next from song menus inserts after the current song without
// restarting it, replacing its queue, or resuming paused playback.
func TestSongMenuPlayNext(t *testing.T) {
	for _, source := range []string{"playlist", "daily picks", "song shelf", "queue", "more"} {
		for _, playing := range []bool{false, true} {
			name := source + "/paused"
			if playing {
				name = source + "/playing"
			}
			t.Run(name, func(t *testing.T) {
				a := newTestApp()
				a.settings = defaultSettings()
				tracks := []qqmusic.Track{
					{ID: 1, Name: "当前歌曲", Duration: 200000},
					{ID: 2, Name: "原下一首", Duration: 200000},
					{ID: 3, Name: "选择的歌曲", Duration: 200000},
				}
				a.pl.Cue(tracks)
				const position = 73 * time.Second
				a.pl.Seek(position)
				if playing {
					a.pl.Hold(position)
				}
				current := a.pl.Current()
				view := func(c *ui.Context) {
					a.pal = paletteFor(c.Theme())
					ui.Column(c).Fill().Children(func() {
						switch source {
						case "playlist":
							a.trackRows(c, tracks, true, func(c *ui.Context) {})
						case "daily picks":
							a.dailyPicks(c, tracks)
						case "song shelf":
							a.songShelf(c, &qqmusic.SongShelf{Title: "歌曲推荐", Tracks: tracks})
						case "queue":
							a.queuePane(c)
						case "more":
							a.songMenu(c, &tracks[2], 20, a.pal.text)
						}
					})
				}
				tt := ui.NewTester(view, 1200, 800)
				tt.Frame()
				var err error
				if source == "more" {
					err = tt.Click("更多")
				} else {
					err = tt.RightClick(tracks[2].Name)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem("下一首播放"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				tt.Frame()
				order, at := a.pl.PlayOrder()
				ids := make([]int64, len(order))
				for i, k := range order {
					ids[i] = a.pl.Queue()[k].ID
				}
				if !slices.Equal(ids, []int64{1, 3, 2, 3}) || at != 0 {
					t.Fatalf("play order = %v at %d", ids, at)
				}
				if a.pl.Current() != current || a.pl.Playing() != playing || a.pl.Loading() || a.pl.Position() != position {
					t.Fatalf("playback changed: current %+v, playing %v, loading %v, position %v", a.pl.Current(), a.pl.Playing(), a.pl.Loading(), a.pl.Position())
				}
			})
		}
	}
}

// Unplayable songs cannot be inserted through their context menu.
func TestSongMenuPlayNextUnavailable(t *testing.T) {
	a := newTestApp()
	tr := qqmusic.Track{ID: 7, Name: "不可播放", Privilege: &qqmusic.Privilege{St: -1}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, tr.Name).ContextMenu(a.songItems(&tr, 0))
	}, 400, 200)
	tt.Frame()
	if err := tt.RightClick(tr.Name); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("下一首播放"); err == nil {
		t.Fatal("unplayable song's play-next action was enabled")
	}
	tt.Frame()
	if len(a.pl.Queue()) != 0 {
		t.Fatal("unplayable song was queued")
	}
}
