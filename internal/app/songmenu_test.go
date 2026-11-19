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

	"github.com/egoist/mygo/audio"
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

// Clicking the current entry pauses and resumes without reloading its audio or
// replacing the queue: song and episode rows used to start it over from zero.
func TestCurrentEntryTogglesPlayback(t *testing.T) {
	for _, page := range []string{"playlist", "daily picks", "song shelf", "queue", "podcast"} {
		t.Run(page, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			tr := qqmusic.Track{ID: 7, Name: "宝船", Duration: 200000}
			if page == "podcast" {
				tr.ProgramID, tr.PodcastID = 70, 5
			}
			// The same song can be clicked in a list other than its queue.
			listed := []qqmusic.Track{tr, {ID: 8, Name: "列表中的另一首", Duration: 200000}}
			queued := []qqmusic.Track{tr, {ID: 9, Name: "原队列的下一首", Duration: 200000}}
			a.pl.Cue(queued)
			a.pl.SetLyrics(nil)
			a.pl.ToggleShuffle()
			order, at := a.pl.PlayOrder()
			order = slices.Clone(order)
			out := audio.NewPlayer(&silence{})
			out.SetVolume(0)
			const position = 73 * time.Second
			out.Seek(position)
			out.Play()
			a.pl.Attach(out)
			defer out.Close()

			pod := &qqmusic.Podcast{ID: 5, Name: "测试播客"}
			pr := qqmusic.Program{ID: 70, Name: tr.Name, Duration: tr.Duration}
			pr.MainSong.ID = tr.ID
			d := &podcastData{podcast: pod, programs: []qqmusic.Program{pr}}
			view := func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				// Keep other copies of the title in the sidebar and player bar
				// out of the lookup, so the click targets the entry itself.
				ui.Column(c).Fill().Children(func() {
					switch page {
					case "playlist":
						a.trackRows(c, listed, true, func(c *ui.Context) {})
					case "daily picks":
						a.dailyPicks(c, listed)
					case "song shelf":
						a.songShelf(c, &qqmusic.SongShelf{Title: "歌曲推荐", Tracks: listed})
					case "queue":
						a.queuePane(c)
					case "podcast":
						a.programRow(c, d, 0, a.pl.Current())
					}
				})
			}
			tt := ui.NewTester(view, 1200, 800)
			tt.Advance(time.Second)
			tt.Frame()
			for _, wantPlaying := range []bool{false, true} {
				if err := tt.Click(tr.Name); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if got := a.pl.Playing(); got != wantPlaying {
					t.Fatalf("playing = %v, want %v", got, wantPlaying)
				}
				if !a.pl.Ready() || a.pl.Position() < position {
					t.Fatalf("audio reloaded or progress lost: ready %v, position %v", a.pl.Ready(), a.pl.Position())
				}
				if !reflect.DeepEqual(a.pl.Queue(), queued) {
					t.Fatal("click replaced the original queue")
				}
				gotOrder, gotAt := a.pl.PlayOrder()
				if !slices.Equal(gotOrder, order) || gotAt != at {
					t.Fatalf("play order changed: %v at %d, want %v at %d", gotOrder, gotAt, order, at)
				}
				if dir := os.Getenv("NM_FRAMES"); dir != "" && page == "playlist" {
					name := "track-row-paused.png"
					if wantPlaying {
						name = "track-row-playing-hover.png"
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
			}
		})
	}
}

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
