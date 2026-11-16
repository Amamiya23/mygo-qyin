package app

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

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
