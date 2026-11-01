package app

import (
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func songShelfApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "Amamiya"}
	tracks := make([]qqmusic.Track, 30)
	for i := range tracks {
		tracks[i] = qqmusic.Track{ID: int64(i + 1), Name: fmt.Sprintf("推荐歌曲 %02d", i+1), Artists: []qqmusic.Artist{{Name: "测试乐队"}}, Duration: 180000}
	}
	a.songShelves = &songShelfData{shelves: []qqmusic.SongShelf{
		{Title: "「Amamiya」，这是你的宝藏好歌💎", Tracks: tracks},
		{Title: "Amamiya，这是你的幸运好歌 🍀", Tracks: tracks[10:16]},
	}}
	a.featured = []qqmusic.Playlist{{ID: 900, Name: "精选歌单"}}
	a.recPlaylists = []qqmusic.Playlist{{ID: 901, Name: "为你推荐的歌单"}}
	a.daily = []qqmusic.Track{{ID: 101, Name: "今日歌曲", Duration: 180000}}
	return a
}

// Playing a home recommendation queues the whole shelf, including songs beyond its six previews.
func TestSongShelfPlayback(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(fmt.Sprint("all=", all), func(t *testing.T) {
			a := songShelfApp()
			shelf := &a.songShelves.shelves[0]
			shelf.Tracks[0].Privilege = &qqmusic.Privilege{St: -1}
			view := func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				ui.Column(c).Fill().Children(func() {
					if all {
						a.songShelfPage(c)
					} else {
						a.songShelf(c, shelf)
					}
				})
			}
			if all {
				a.open(route{page: pageSongShelf, shelf: shelf})
			}
			tt := ui.NewTester(view, 1200, 600)
			tt.Frame()
			label, want := shelf.Tracks[4].Name, shelf.Tracks[4].ID
			if all {
				label, want = "播放全部："+shelf.Title, shelf.Tracks[1].ID
				a.pl.ToggleShuffle()
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if cur := a.pl.Current(); cur == nil || cur.ID != want {
				t.Fatalf("playing %+v, want song %d", cur, want)
			}
			if !reflect.DeepEqual(a.pl.Queue(), shelf.Tracks) {
				t.Fatal("playing a preview discarded the full shelf or its order")
			}
			if all && a.pl.Shuffle() {
				t.Fatal("play all shuffled the shelf")
			}
		})
	}
}

// Refreshing the home shelves preserves playback and previously opened details, including their scroll positions.
func TestSongShelfNavigationAndRefresh(t *testing.T) {
	requests := make(chan bool, 2)
	real := readSongShelves
	readSongShelves = func() ([]qqmusic.SongShelf, error) { requests <- true; return nil, nil }
	defer func() { readSongShelves = real }()
	a := songShelfApp()
	d := a.songShelves
	snapshot := &d.shelves[0]
	a.pl.Cue([]qqmusic.Track{{ID: 999, Name: "正在收听的歌曲", Duration: 180000}})
	queue, daily := a.pl.Queue(), a.daily
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Scroll(1000, 500, 0, 160)
	tt.Advance(time.Second)
	tt.Frame()
	homeY := a.pageState().scroll.Y
	a.open(route{page: pageSongShelf, shelf: snapshot})
	tt.Frame()
	detailRoute := a.route
	if detailRoute.page != pageSongShelf || detailRoute.shelf != snapshot {
		t.Fatal("opening the saved recommendation did not keep its snapshot")
	}
	tt.Scroll(1000, 500, 0, 320)
	tt.Advance(time.Second)
	tt.Frame()
	detailY := a.pageState().tracks.scroll.Y
	if detailY <= 0 {
		t.Fatal("detail did not scroll")
	}
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if math.Abs(float64(a.pageState().scroll.Y-homeY)) > 1 {
		t.Fatal("returning from the shelf lost the home scroll position")
	}
	tt.Scroll(1000, 400, 0, -1000)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("更换歌曲推荐"); err != nil {
		t.Fatal(err)
	}
	wait(t, requests, "recommendation refresh")
	a.loadSongShelves()
	if len(requests) != 0 || !d.loading || &d.shelves[0] != snapshot {
		t.Fatal("refresh discarded the old shelves or started a duplicate request")
	}
	updated := []qqmusic.SongShelf{{Title: "更新后的歌曲推荐", Tracks: []qqmusic.Track{{ID: 1001, Name: "新的推荐歌曲"}}}}
	a.songShelvesAnswered(d, updated, nil)
	tt.Frame()
	if !reflect.DeepEqual(a.pl.Queue(), queue) || !reflect.DeepEqual(a.daily, daily) {
		t.Fatal("refresh changed playback or daily recommendations")
	}
	if err := tt.Click("前进"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != detailRoute || len(a.route.shelf.Tracks) != 30 || math.Abs(float64(a.pageState().tracks.scroll.Y-detailY)) > 1 {
		t.Fatal("refresh replaced the previously opened shelf or its scroll position")
	}
}

// Paging a nine-song shelf backfills the last page and plays the selected original song in the whole queue.
func TestSongShelfCarouselTailPlayback(t *testing.T) {
	a := songShelfApp()
	shelf := &a.songShelves.shelves[0]
	shelf.Tracks = shelf.Tracks[:9]
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() { a.songShelf(c.Key(shelf), shelf) })
	}, 1200, 500)
	tt.Frame()
	if err := tt.Click(shelf.Tracks[8].Name); err == nil {
		t.Fatal("the last song was already on the first page")
	}
	if err := tt.Click("下一组歌曲：" + shelf.Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if err := tt.Click(shelf.Tracks[8].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != shelf.Tracks[8].ID || !reflect.DeepEqual(a.pl.Queue(), shelf.Tracks) {
		t.Fatal("the tail page played its local index or discarded the full shelf")
	}
	if a.route.page != pageHome {
		t.Fatal("paging unexpectedly navigated away from Home")
	}
	if err := tt.Click("上一组歌曲：" + shelf.Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if err := tt.Click(shelf.Tracks[0].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != shelf.Tracks[0].ID {
		t.Fatal("previous did not restore the first song")
	}
}

// Rapid arrow clicks settle on the requested boundary without wrapping or queueing delayed page changes.
func TestSongShelfCarouselRapidClicks(t *testing.T) {
	a := songShelfApp()
	shelf := &a.songShelves.shelves[0]
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() { a.songShelf(c.Key(shelf), shelf) })
	}, 1200, 500)
	tt.Frame()
	for i := 0; i < 8; i++ {
		if err := tt.Click("下一组歌曲：" + shelf.Title); err != nil {
			t.Fatal(err)
		}
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if err := tt.Click(shelf.Tracks[29].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != shelf.Tracks[29].ID {
		t.Fatal("rapid next clicks did not reach the final page")
	}
	for i := 0; i < 8; i++ {
		if err := tt.Click("上一组歌曲：" + shelf.Title); err != nil {
			t.Fatal(err)
		}
	}
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click(shelf.Tracks[0].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != shelf.Tracks[0].ID {
		t.Fatal("rapid previous clicks did not stop at the first page")
	}
}

// Separate shelves retain their own page and a new recommendation snapshot starts from its first page.
func TestSongShelfCarouselIndependentAndRefresh(t *testing.T) {
	a := songShelfApp()
	d := a.songShelves
	d.shelves[0].Tracks = d.shelves[0].Tracks[:9]
	d.shelves[1].Tracks = songShelfApp().songShelves.shelves[0].Tracks[10:19]
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() {
			for i := range d.shelves {
				shelf := &d.shelves[i]
				a.songShelf(c.Key(shelf), shelf)
			}
		})
	}, 1200, 700)
	tt.Frame()
	if err := tt.Click("下一组歌曲：" + d.shelves[0].Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if err := tt.Click(d.shelves[1].Tracks[0].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != d.shelves[1].Tracks[0].ID {
		t.Fatal("paging the first shelf changed the second shelf")
	}
	if err := tt.Click(d.shelves[0].Tracks[8].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != d.shelves[0].Tracks[8].ID {
		t.Fatal("another shelf's interaction reset the first shelf")
	}
	updated := append([]qqmusic.SongShelf(nil), d.shelves...)
	a.songShelvesAnswered(d, updated, nil)
	tt.Frame()
	if err := tt.Click(d.shelves[0].Tracks[0].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != d.shelves[0].Tracks[0].ID {
		t.Fatal("a refreshed shelf retained its old tail page")
	}
}

// Vertical scrolling and leaving Home preserve the selected shelf page and the page's vertical position.
func TestSongShelfCarouselNavigation(t *testing.T) {
	a := songShelfApp()
	shelf := &a.songShelves.shelves[0]
	shelf.Tracks = shelf.Tracks[:9]
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	if err := tt.Click("下一组歌曲：" + shelf.Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	tt.Scroll(1000, 500, 0, 80)
	tt.Advance(time.Second)
	tt.Frame()
	homeY := a.pageState().scroll.Y
	if homeY <= 0 {
		t.Fatal("the shelf intercepted ordinary vertical scrolling")
	}
	a.open(route{page: pageDaily})
	tt.Frame()
	a.back()
	tt.Frame()
	if math.Abs(float64(a.pageState().scroll.Y-homeY)) > 1 {
		t.Fatal("returning to Home changed its scroll position")
	}
	if err := tt.Click(shelf.Tracks[8].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != shelf.Tracks[8].ID {
		t.Fatal("vertical scrolling or navigation reset the selected shelf page")
	}
}

// Resizing the local carousel retains its first visible song and never changes playback or navigates away.
func TestSongShelfCarouselResize(t *testing.T) {
	a := songShelfApp()
	shelf := &a.songShelves.shelves[0]
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() { a.songShelf(c.Key(shelf), shelf) })
	}, 1200, 500)
	tt.Frame()
	if err := tt.Click("下一组歌曲：" + shelf.Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	tt.SetSize(900, 500)
	tt.Frame()
	if a.pl.Current() != nil || a.route.page != pageHome {
		t.Fatal("resizing the shelf started playback or changed the page")
	}
	if err := tt.Click(shelf.Tracks[6].Name); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != shelf.Tracks[6].ID || !reflect.DeepEqual(a.pl.Queue(), shelf.Tracks) {
		t.Fatal("resizing lost the old first song or changed its original playback index")
	}
}

// Horizontal gestures page locally, and arrow keys only page a shelf while focus remains inside it.
func TestSongShelfCarouselInput(t *testing.T) {
	a := songShelfApp()
	shelf := &a.songShelves.shelves[0]
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() {
			a.songShelf(c.Key(shelf), shelf)
			ui.ButtonBase(c.Key("outside")).Label("货架外按钮").Height(32)
		})
	}, 1200, 500)
	tt.Frame()
	r, ok := tt.Find(shelf.Tracks[0].Name)
	if !ok {
		t.Fatal("cannot target the first song for the touchpad gesture")
	}
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 60, 0)
	if a.shelfPagers[shelf].page != 1 {
		t.Fatal("horizontal scrolling did not page the hovered shelf")
	}
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 100, 0)
	if a.shelfPagers[shelf].page != 1 {
		t.Fatal("one touchpad gesture skipped multiple pages")
	}
	if err := tt.Click("下一组歌曲：" + shelf.Title); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if a.shelfPagers[shelf].page != 3 {
		t.Fatal("right arrow did not page the focused shelf")
	}
	tt.Key(0, ui.KeyLeft)
	if a.shelfPagers[shelf].page != 2 {
		t.Fatal("left arrow did not restore the preceding local page")
	}
	if err := tt.Click("货架外按钮"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if a.shelfPagers[shelf].page != 2 {
		t.Fatal("the shelf captured arrow keys after focus left it")
	}
}

// Keyboard paging remains available after a focused boundary button disables or a focused song leaves the page.
func TestSongShelfCarouselFocusRetention(t *testing.T) {
	for _, count := range []int{9, 30} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			a := songShelfApp()
			shelf := &a.songShelves.shelves[0]
			shelf.Tracks = shelf.Tracks[:count]
			tt := ui.NewTester(func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				ui.Column(c).Fill().Children(func() { a.songShelf(c.Key(shelf), shelf) })
			}, 1200, 500)
			tt.Frame()
			if count == 9 {
				if err := tt.Click("下一组歌曲：" + shelf.Title); err != nil {
					t.Fatal(err)
				}
				if a.shelfPagers[shelf].page != 1 {
					t.Fatal("next did not reach the tail page")
				}
				tt.Key(0, ui.KeyLeft)
				if a.shelfPagers[shelf].page != 0 {
					t.Fatal("disabling the focused next button lost keyboard paging")
				}
			} else {
				if err := tt.Click(shelf.Tracks[0].Name); err != nil {
					t.Fatal(err)
				}
				tt.Key(0, ui.KeyRight)
				tt.Key(0, ui.KeyRight)
				if a.shelfPagers[shelf].page != 2 {
					t.Fatal("removing the focused song lost subsequent keyboard paging")
				}
				tt.Key(0, ui.KeyLeft)
				if a.shelfPagers[shelf].page != 1 {
					t.Fatal("the carousel lost reverse keyboard paging after replacing songs")
				}
			}
		})
	}
}

// A failed refresh leaves the old recommendations usable, and an old account's answer cannot restore them.
func TestSongShelfRefreshFailureAndAccountChange(t *testing.T) {
	a := songShelfApp()
	d := a.songShelves
	snapshot := &d.shelves[0]
	pager := &songShelfPager{}
	pager.layout(len(snapshot.Tracks), 6)
	pager.move(1)
	a.shelfPagers = map[*qqmusic.SongShelf]*songShelfPager{snapshot: pager}
	d.loading = true
	a.songShelvesAnswered(d, nil, errors.New("offline"))
	if d.loading || d.err == "" || &d.shelves[0] != snapshot || a.shelfPagers[snapshot] != pager || pager.page != 1 {
		t.Fatal("failed refresh discarded the previous recommendations")
	}
	a.route = route{page: pageSongShelf, shelf: snapshot}
	a.history = []route{{page: pageHome}, a.route}
	a.forward = []route{a.route}
	a.pageState()
	a.clearSongShelves()
	a.songShelvesAnswered(d, []qqmusic.SongShelf{{Title: "Old account"}}, nil)
	if a.songShelves != nil || a.shelfPagers != nil || a.route.page != pageHome || len(a.history) != 1 || len(a.forward) != 0 || len(a.pageStates) != 0 {
		t.Fatal("account change retained personal recommendations or their navigation")
	}
}

// Liking and right-clicking a shelf entry act on that song without starting playback.
func TestSongShelfLikeAndMenu(t *testing.T) {
	a := songShelfApp()
	shelf := &a.songShelves.shelves[0]
	calls := make(chan int64, 2)
	realLike, realCopy := likeSong, copyText
	likeSong = func(id int64, like bool) error { calls <- id; return nil }
	var copied string
	copyText = func(text string) error { copied = text; return nil }
	defer func() { likeSong, copyText = realLike, realCopy }()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() { a.songShelfRow(c, shelf, 0) })
	}, 400, 200)
	tt.Frame()
	if err := tt.Click("喜欢"); err != nil {
		t.Fatal(err)
	}
	if id := wait(t, calls, "like song"); id != shelf.Tracks[0].ID || !a.liked[id] {
		t.Fatal("shelf heart liked the wrong song")
	}
	tt.Frame()
	if err := tt.RightClick(shelf.Tracks[0].Name); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	if copied != shelf.Tracks[0].URL() || a.pl.Current() != nil {
		t.Fatal("the shelf menu used the wrong song or an auxiliary action started playback")
	}
}

// Saved frames show the native shelves and their details at wide and narrow window sizes in both appearances.
func TestSongShelfFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, width := range []int{1200, 900} {
		for _, dark := range []bool{false, true} {
			for _, detail := range []bool{false, true} {
				a := songShelfApp()
				colors := []ui.Color{radixGreen(false, 9), radixAmber(false, 9), radixSage(true, 9), radixRed(false, 9)}
				for i := range a.songShelves.shelves[0].Tracks {
					tr := &a.songShelves.shelves[0].Tracks[i]
					tr.Album.PicURL = fmt.Sprintf("shelf-cover-%d", i)
					color := colors[i%len(colors)]
					art := &artwork{bmp: solidBitmap(color), tint: color}
					for _, px := range []int{96, 120, 200, 400, 600} {
						a.art.art[qqmusic.Sized(tr.Album.PicURL, px)] = art
					}
				}
				if detail {
					a.open(route{page: pageSongShelf, shelf: &a.songShelves.shelves[0]})
				} else {
					a.songShelves.shelves[0].Tracks = a.songShelves.shelves[0].Tracks[:9]
				}
				tt := ui.NewTester(a.view, width, 900)
				tt.SetDark(dark)
				tt.Advance(time.Second)
				tt.Frame()
				tt.Advance(time.Second)
				tt.Frame()
				name := fmt.Sprintf("song-shelves-%d-dark-%t-detail-%t.png", width, dark, detail)
				save := func(name string) {
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
				save(name)
				if !detail {
					for i := 0; i < 3; i++ {
						if err := tt.Click("下一组歌曲：" + a.songShelves.shelves[0].Title); err != nil {
							t.Fatal(err)
						}
					}
					tt.Advance(250 * time.Millisecond)
					tt.Frame()
					save(fmt.Sprintf("song-shelves-%d-dark-%t-tail.png", width, dark))
					tt.Scroll(float32(width-100), 600, 0, 520)
					tt.Advance(time.Second)
					tt.Frame()
					save(fmt.Sprintf("song-shelves-%d-dark-%t-scrolled.png", width, dark))
				}
			}
		}
	}
}
