package app

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

// editTestApp is signed in, with liked songs, two playlists of their own
// and one saved, and shows their playlist 10.
func editTestApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "小明"}
	a.mine = []qqmusic.Playlist{
		{ID: 5, Name: "小明喜欢的音乐", SpecialType: 5},
		{ID: 10, Name: "通勤"},
		{ID: 11, Name: "睡前"},
	}
	a.saved = []qqmusic.Playlist{{ID: 20, Name: "飙升榜"}}
	a.playlists[10] = &qqmusic.Playlist{ID: 10, Name: "通勤", Tracks: []qqmusic.Track{
		{ID: 1, Name: "一张相片", Artists: []qqmusic.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []qqmusic.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, id: 10}
	return a
}

// Choosing one of the listener's playlists from a song's menu adds the
// song to it, and the menu offers neither the liked songs nor the
// playlist the song is listed in.
func TestAddSongToPlaylist(t *testing.T) {
	type add struct{ pid, song int64 }
	calls := make(chan add, 1)
	addToPlaylist = func(pid int64, songs ...int64) error { calls <- add{pid, songs[0]}; return nil }
	defer func() { addToPlaylist = qqmusic.AddToPlaylist }()

	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到歌单", "通勤"); err == nil {
		t.Error("offered to add the song to the playlist it is in")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "小明喜欢的音乐"); err == nil {
		t.Error("offered to add the song to the liked songs")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "睡前"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := <-calls; got != (add{11, 2}) {
		t.Errorf("added %+v, want song 2 to playlist 11", got)
	}
}

// Removing a song from the listener's playlist takes its row away at once,
// and asks NetEase to take it out.
func TestRemoveSongFromPlaylist(t *testing.T) {
	type del struct{ pid, song int64 }
	calls := make(chan del, 1)
	removeFromPlaylist = func(pid int64, songs ...int64) error { calls <- del{pid, songs[0]}; return nil }
	defer func() { removeFromPlaylist = qqmusic.RemoveFromPlaylist }()

	a := editTestApp()
	// Playing the id: the queue keeps the song.
	a.pl.Cue(a.playlists[10].Tracks)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从歌单中移除"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := <-calls; got != (del{10, 2}) {
		t.Errorf("removed %+v, want song 2 from playlist 10", got)
	}
	if _, ok := tt.Find("宝船"); ok {
		t.Error("the song's row is still listed")
	}
	if q := a.pl.Queue(); len(q) != 2 || q[1].ID != 2 {
		t.Errorf("removing the song changed the queue playing: %v", q)
	}
}

// A playlist the listener saved, not made, offers no removing.
func TestNoRemovingFromSavedPlaylist(t *testing.T) {
	a := editTestApp()
	a.playlists[20] = &qqmusic.Playlist{ID: 20, Name: "飙升榜", Tracks: a.playlists[10].Tracks}
	a.route = route{page: pagePlaylist, id: 20}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从歌单中移除"); err == nil {
		t.Error("offered to remove a song from a saved playlist")
	}
}

// The sidebar's plus asks for a name, and Enter makes the playlist so
// named.
func TestCreatePlaylist(t *testing.T) {
	names := make(chan string, 1)
	createPlaylist = func(name string) (*qqmusic.Playlist, error) {
		names <- name
		return &qqmusic.Playlist{ID: 12, Name: name}, nil
	}
	defer func() { createPlaylist = qqmusic.CreatePlaylist }()

	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("新建歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Type("  周末  ")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := <-names; got != "周末" {
		t.Errorf("made a playlist named %q, want 周末", got)
	}
}

// Choosing 新建歌单… from a song's menu makes the playlist, then adds the
// song to it.
func TestCreatePlaylistForSong(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到歌单", "新建歌单…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if !a.naming.open || a.naming.add == nil || a.naming.add.ID != 2 {
		t.Fatalf("the dialog is %+v, want it open to add song 2", a.naming)
	}
}

// Renaming a playlist from the sidebar's menu shows its new name at once,
// and asks NetEase to rename it.
func TestRenamePlaylist(t *testing.T) {
	type rename struct {
		id   int64
		name string
	}
	calls := make(chan rename, 1)
	renamePlaylist = func(id int64, name string) error { calls <- rename{id, name}; return nil }
	defer func() { renamePlaylist = qqmusic.RenamePlaylist }()

	a := editTestApp()
	a.route = route{page: pageHome}
	a.homeLoading = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("睡前"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("助眠")
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-calls; got != (rename{11, "助眠"}) {
		t.Errorf("renamed %+v, want playlist 11 to 助眠", got)
	}
	if a.mine[2].Name != "助眠" {
		t.Errorf("the sidebar names it %q", a.mine[2].Name)
	}
	if _, ok := tt.Find("助眠"); !ok {
		t.Error("the new name does not show")
	}
}

// The liked songs can't be renamed: they offer no menu.
func TestNoRenamingLikedSongs(t *testing.T) {
	a := editTestApp()
	a.route = route{page: pageHome}
	a.homeLoading = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.RightClick("小明喜欢的音乐")
	if err := tt.ChooseMenuItem("重命名…"); err == nil {
		t.Error("offered to rename the liked songs")
	}
}

// Deleting the playlist shown, once confirmed, takes it from the sidebar
// and goes back to the page before, with no way forward to it.
func TestDeletePlaylist(t *testing.T) {
	ids := make(chan int64, 1)
	deletePlaylist = func(id int64) error { ids <- id; return nil }
	defer func() { deletePlaylist = qqmusic.DeletePlaylist }()

	a := editTestApp()
	a.homeLoading = true
	a.route = route{page: pageCharts}
	a.open(route{page: pagePlaylist, id: 10})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("删除歌单…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	select {
	case <-ids:
		t.Fatal("deleted before it was confirmed")
	default:
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-ids; got != 10 {
		t.Errorf("deleted playlist %d, want 10", got)
	}
	if a.editable(10) != nil {
		t.Error("the sidebar still lists it")
	}
	if a.route.page != pageCharts || len(a.history) != 0 || len(a.forward) != 0 {
		t.Errorf("on %+v, history %v, forward %v; want the charts with nowhere to go", a.route, a.history, a.forward)
	}
}

// Cancelling the confirmation deletes nothing.
func TestCancelDeletingPlaylist(t *testing.T) {
	deletePlaylist = func(int64) error { t.Error("deleted the playlist"); return nil }
	defer func() { deletePlaylist = qqmusic.DeletePlaylist }()

	a := editTestApp()
	a.route = route{page: pageHome}
	a.homeLoading = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("睡前"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("删除歌单…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if a.editable(11) == nil {
		t.Error("the playlist left the sidebar")
	}
}

// Saved playlists previously had no removal action; both menus must unfavorite only after confirmation and preserve the queue.
func TestUnfavoritePlaylist(t *testing.T) {
	for _, entry := range []string{"sidebar", "header"} {
		t.Run(entry, func(t *testing.T) {
			type call struct {
				id    int64
				owned bool
			}
			calls := make(chan call, 1)
			unfavoritePlaylist = func(id int64) error { calls <- call{id: id}; return nil }
			deletePlaylist = func(id int64) error { calls <- call{id: id, owned: true}; return nil }
			defer func() { unfavoritePlaylist, deletePlaylist = qqmusic.UnfavoritePlaylist, qqmusic.DeletePlaylist }()

			a := editTestApp()
			a.playlists[20] = &qqmusic.Playlist{ID: 20, Name: "飙升榜", Tracks: a.playlists[10].Tracks}
			a.pl.Cue(a.playlists[20].Tracks)
			a.route = route{page: pagePlaylist, id: 20}
			a.history = []route{{page: pageCharts}, a.route}
			a.forward = []route{a.route}
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			if entry == "sidebar" {
				if err := tt.RightClick("飙升榜"); err != nil {
					t.Fatal(err)
				}
			} else if err := tt.Click("更多"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("取消收藏…"); err != nil {
				t.Fatalf("%v; the menu shows %q", err, tt.Menu())
			}
			tt.Advance(300 * time.Millisecond)
			tt.Frame()
			select {
			case <-calls:
				t.Fatal("removed the playlist before confirmation")
			default:
			}
			if err := tt.Click("取消收藏"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if len(a.saved) != 0 || len(a.mine) != 3 {
				t.Fatalf("wrong library changed: saved=%v, mine=%v", a.saved, a.mine)
			}
			if got := <-calls; got != (call{id: 20}) {
				t.Errorf("wrong operation: %+v", got)
			}
			if a.route.page != pageCharts || len(a.history) != 0 || len(a.forward) != 0 {
				t.Errorf("removed playlist still in navigation: route=%+v, history=%v, forward=%v", a.route, a.history, a.forward)
			}
			if q := a.pl.Queue(); len(q) != 2 || q[0].ID != 1 || q[1].ID != 2 {
				t.Errorf("unfavoriting changed the playing queue: %v", q)
			}
		})
	}
}

// Cancelling the saved-playlist confirmation leaves its membership and current page intact without contacting QQ Music.
func TestCancelUnfavoritePlaylist(t *testing.T) {
	calls := make(chan int64, 1)
	unfavoritePlaylist = func(id int64) error { calls <- id; return nil }
	defer func() { unfavoritePlaylist = qqmusic.UnfavoritePlaylist }()
	a := editTestApp()
	before := a.route
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("飙升榜"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("取消收藏…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if len(a.saved) != 1 || a.saved[0].ID != 20 || a.route != before || a.deleting.open {
		t.Fatal("cancelling changed the library or page, or left confirmation open")
	}
	select {
	case <-calls:
		t.Fatal("cancel contacted QQ Music")
	default:
	}
}

// A refused unfavorite restores the saved entry and its cache, while a confirmed removal clears the cache.
func TestUnfavoritePlaylistResult(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		a := editTestApp()
		pl := a.saved[0]
		a.saved = nil // The pending action has already removed the sidebar row.
		a.playlists[pl.ID] = &pl
		var err error
		if rejected {
			err = errors.New("QQ Music refused the request")
		}
		a.finishPlaylistRemoval(pl, true, 0, err)
		if rejected {
			if len(a.saved) != 1 || a.saved[0].ID != pl.ID || a.saved[0].Name != pl.Name || a.playlists[pl.ID] == nil {
				t.Fatal("refused unfavorite did not restore the saved entry and retain its cache")
			}
		} else if len(a.saved) != 0 || a.playlists[pl.ID] != nil {
			t.Fatal("successful unfavorite retained membership or cached detail")
		}
		if len(a.mine) != 3 {
			t.Fatal("the favorite result changed owned playlists")
		}
	}
}

// With NM_FRAMES set, saves the listener's playlist with its sidebar, and
// the dialogs that name and delete playlists, to look at.
func TestPlaylistEditFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	save := func(name string) {
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, tt.Image())
	}
	for _, dark := range []bool{false, true} {
		suffix := "-light"
		if dark {
			suffix = "-dark"
		}
		tt.SetDark(dark)
		save("playlist" + suffix)
		a.newPlaylist(nil)
		save("create" + suffix)
		a.naming.open = false
		a.startRename(a.editable(11))
		save("rename" + suffix)
		a.naming.open = false
		a.confirmDelete(a.editable(11))
		save("delete" + suffix)
		a.deleting.open = false
		a.confirmUnfavorite(&a.saved[0])
		save("unfavorite" + suffix)
		a.deleting.open = false
	}
}

func pumpPosted(t *testing.T, ch chan func(), until func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !until() {
		select {
		case fn := <-ch:
			fn()
		case <-deadline:
			t.Fatal("timed out waiting for posted callback")
		}
	}
}

func drainPosted(ch chan func(), wait time.Duration) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case fn := <-ch:
			fn()
		case <-timer.C:
			return
		}
	}
}

func TestPlaylistCacheOfflineFallbackAndSilentRefresh(t *testing.T) {
	origDetail, origUser := playlistDetail, userPlaylists
	t.Cleanup(func() { playlistDetail, userPlaylists = origDetail, origUser })

	cache := player.NewCache(t.TempDir(), 4<<20)
	cachedPl := &qqmusic.Playlist{
		ID:         101,
		Name:       "缓存歌单",
		TrackCount: 1,
		Tracks:     []qqmusic.Track{{ID: 5001, MID: "mid5001", Name: "离线曲目", Duration: 180000}},
	}
	if !cache.KeepPlaylist(cachedPl) {
		t.Fatal("KeepPlaylist failed")
	}
	if !cache.KeepUserPlaylists(888, []qqmusic.Playlist{{ID: 101, Name: "缓存歌单", TrackCount: 1}}, []qqmusic.Playlist{{ID: 202, Name: "收藏歌单"}}) {
		t.Fatal("KeepUserPlaylists failed")
	}

	// Simulate an offline/weak-network environment first.
	detailGate := make(chan struct{})
	detailErr := errors.New("offline")
	freshName := "同步后歌单"
	userCalls, detailCalls := 0, 0
	userPlaylists = func(uid int64) ([]qqmusic.Playlist, []qqmusic.Playlist, error) {
		userCalls++
		return nil, nil, errors.New("offline")
	}
	playlistDetail = func(id int64) (*qqmusic.Playlist, error) {
		detailCalls++
		<-detailGate
		if detailErr != nil {
			return nil, detailErr
		}
		return &qqmusic.Playlist{
			ID:         id,
			Name:       freshName,
			TrackCount: 2,
			Tracks: []qqmusic.Track{
				{ID: 5001, MID: "mid5001", Name: "离线曲目", Duration: 180000},
				{ID: 5002, MID: "mid5002", Name: "新增曲目", Duration: 210000},
			},
		}, nil
	}

	posted := make(chan func(), 32)
	a := newTestApp()
	a.pl.Cache = cache
	a.postFn = func(fn func()) { posted <- fn }
	a.profile = &qqmusic.Profile{UserID: 888, Nickname: "测试用户"}
	a.route = route{page: pagePlaylist, id: 101}

	a.loadMine(888)
	pumpPosted(t, posted, func() bool { return len(a.mine) == 1 && len(a.saved) == 1 })
	if a.mine[0].ID != 101 || a.saved[0].ID != 202 {
		t.Fatalf("restored user playlists mismatch: mine=%+v saved=%+v", a.mine, a.saved)
	}

	// Loading the playlist while offline serves disk cache immediately and starts playback.
	a.loadPlaylist(101, true)
	pumpPosted(t, posted, func() bool { return a.playlists[101] != nil })
	if a.plErr[101] != "" || a.playlists[101].Name != "缓存歌单" {
		t.Fatalf("unexpected cached playlist state: err=%q pl=%+v", a.plErr[101], a.playlists[101])
	}
	if a.pl.Current() == nil || a.pl.Current().ID != 5001 {
		t.Fatalf("expected immediate playback from cached playlist, got %+v", a.pl.Current())
	}

	// Let the background stale refresh finish with an error: cached view must stay intact without error banner.
	close(detailGate)
	pumpPosted(t, posted, func() bool { return !a.refreshingPl[101] })
	if a.plErr[101] != "" || a.playlists[101] == nil || a.playlists[101].Name != "缓存歌单" {
		t.Fatalf("failed background refresh cleared or errored cached playlist: err=%q pl=%+v", a.plErr[101], a.playlists[101])
	}
	if !a.plStale[101] {
		t.Fatal("expected playlist to remain marked stale after failed background refresh")
	}

	// When network recovers, revisiting triggers silent background refresh and updates both RAM and disk.
	detailErr = nil
	a.loadPlaylist(101, false)
	pumpPosted(t, posted, func() bool { return a.playlists[101] != nil && a.playlists[101].Name == freshName })
	if a.plStale[101] || !a.plSynced[101] || len(a.playlists[101].Tracks) != 2 {
		t.Fatalf("background refresh did not sync playlist: stale=%v synced=%v tracks=%d", a.plStale[101], a.plSynced[101], len(a.playlists[101].Tracks))
	}
	drainPosted(posted, 50*time.Millisecond)

	// Wait briefly for the async disk write of refreshed playlist.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if onDisk, _, ok := cache.LookupPlaylist(101); ok && onDisk.Name == freshName {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("disk cache was not updated after background refresh (detailCalls=%d, userCalls=%d)", detailCalls, userCalls)
}

func TestPlaylistCacheBoundedMemoryLRU(t *testing.T) {
	origDetail := playlistDetail
	t.Cleanup(func() { playlistDetail = origDetail })

	cache := player.NewCache(t.TempDir(), 8<<20)
	netCalls := map[int64]int{}
	playlistDetail = func(id int64) (*qqmusic.Playlist, error) {
		netCalls[id]++
		return &qqmusic.Playlist{
			ID:         id,
			Name:       fmt.Sprintf("歌单 %d", id),
			TrackCount: 1,
			Tracks:     []qqmusic.Track{{ID: id*10 + 1, Name: "歌曲"}},
		}, nil
	}

	posted := make(chan func(), 64)
	a := newTestApp()
	a.pl.Cache = cache
	a.postFn = func(fn func()) { posted <- fn }

	total := int64(maxCachedPlaylists + 3)
	for id := int64(1); id <= total; id++ {
		a.route = route{page: pagePlaylist, id: id}
		a.loadPlaylist(id, false)
		pumpPosted(t, posted, func() bool { return a.playlists[id] != nil })
	}
	if len(a.playlists) > maxCachedPlaylists {
		t.Fatalf("in-memory playlists exceeded cap: got %d, max %d", len(a.playlists), maxCachedPlaylists)
	}
	if a.playlists[total] == nil {
		t.Fatal("active route playlist was evicted")
	}
	if a.playlists[1] != nil {
		t.Fatal("oldest playlist 1 should have been evicted from RAM")
	}

	// Wait for background KeepPlaylistData for playlist 1 to finish writing to disk.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, _, ok := cache.LookupPlaylist(1); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Reopening evicted playlist 1 within TTL reloads from disk without another network call.
	beforeCalls := netCalls[1]
	a.route = route{page: pagePlaylist, id: 1}
	a.loadPlaylist(1, false)
	pumpPosted(t, posted, func() bool { return a.playlists[1] != nil })
	drainPosted(posted, 40*time.Millisecond)
	if netCalls[1] != beforeCalls {
		t.Fatalf("reopening evicted playlist within TTL hit network: %d -> %d", beforeCalls, netCalls[1])
	}

	// Verify track-count ceiling also evicts old playlists while preserving the active route.
	bigTracks := make([]qqmusic.Track, maxCachedPlaylistTracks/2+10)
	for i := range bigTracks {
		bigTracks[i] = qqmusic.Track{ID: int64(i + 1), Name: "曲"}
	}
	a.route = route{page: pagePlaylist, id: 901}
	a.putPlaylist(&qqmusic.Playlist{ID: 901, Name: "大歌单1", Tracks: bigTracks})
	a.route = route{page: pagePlaylist, id: 902}
	a.putPlaylist(&qqmusic.Playlist{ID: 902, Name: "大歌单2", Tracks: bigTracks})
	if a.playlists[902] == nil || a.playlists[901] != nil {
		t.Fatalf("track-count LRU eviction failed: has901=%v has902=%v", a.playlists[901] != nil, a.playlists[902] != nil)
	}
}

func TestSearchMineUsesDiskCacheAndBoundsMemory(t *testing.T) {
	origMine := mineDetail
	t.Cleanup(func() { mineDetail = origMine })

	cache := player.NewCache(t.TempDir(), 8<<20)
	posted := make(chan func(), 32)
	a := newTestApp()
	a.pl.Cache = cache
	a.postFn = func(fn func()) { posted <- fn }
	a.profile = &qqmusic.Profile{UserID: 1}

	total := maxCachedPlaylists + 2
	for i := 1; i <= total; i++ {
		id := int64(1000 + i)
		a.mine = append(a.mine, qqmusic.Playlist{ID: id, Name: fmt.Sprintf("我的%d", i)})
		if i <= 2 {
			if !cache.KeepPlaylist(&qqmusic.Playlist{
				ID:     id,
				Name:   fmt.Sprintf("我的%d", i),
				Tracks: []qqmusic.Track{{ID: id * 10, Name: fmt.Sprintf("关键字%d", i)}},
			}) {
				t.Fatal("KeepPlaylist failed")
			}
		}
	}

	fetched := map[int64]bool{}
	mineDetail = func(id int64) (*qqmusic.Playlist, error) {
		fetched[id] = true
		return &qqmusic.Playlist{
			ID:     id,
			Name:   fmt.Sprintf("网络%d", id),
			Tracks: []qqmusic.Track{{ID: id * 10, Name: fmt.Sprintf("关键字%d", id)}},
		}, nil
	}

	r := route{page: pageSearch, kind: searchMine, query: "关键字"}
	d := &searchData{}
	a.searchMine(r, d)
	pumpPosted(t, posted, func() bool { return d.page.loaded })

	if fetched[1001] || fetched[1002] {
		t.Fatalf("searchMine hit network for disk-cached playlists: %+v", fetched)
	}
	if len(d.res.Songs) != total {
		t.Fatalf("matched %d songs, want %d", len(d.res.Songs), total)
	}
	if len(a.playlists) > maxCachedPlaylists {
		t.Fatalf("searchMine exceeded memory playlist cap: %d > %d", len(a.playlists), maxCachedPlaylists)
	}
}
