package app

import (
	"slices"
	"time"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

const (
	maxCachedPlaylists      = 8
	maxCachedPlaylistTracks = 3000
	playlistRefreshTTL      = 5 * time.Minute
)

var playlistDetail = qqmusic.PlaylistDetail

func (a *app) playlistGen(id int64) (epoch, gen int) {
	return a.plEpoch, a.plGen[id]
}

func (a *app) samePlaylistGen(id int64, epoch, gen int) bool {
	return a.plEpoch == epoch && a.plGen[id] == gen
}

func (a *app) bumpPlaylistGen(id int64) {
	if a.plGen == nil {
		a.plGen = map[int64]int{}
	}
	a.plGen[id]++
}

func (a *app) resetPlaylistCacheState() {
	a.plEpoch++
	a.playlists = map[int64]*qqmusic.Playlist{}
	a.playlistLRU = nil
	a.plSynced = map[int64]bool{}
	a.plSyncedAt = map[int64]time.Time{}
	a.plStale = map[int64]bool{}
	a.refreshingPl = map[int64]bool{}
	a.playAfterPl = map[int64]bool{}
	a.loadingPl = map[int64]bool{}
	a.plErr = map[int64]string{}
}

func (a *app) touchPlaylist(id int64) {
	a.removePlaylistLRU(id)
	a.playlistLRU = append(a.playlistLRU, id)
}

func (a *app) removePlaylistLRU(id int64) {
	for i, k := range a.playlistLRU {
		if k == id {
			a.playlistLRU = slices.Delete(a.playlistLRU, i, i+1)
			return
		}
	}
}

func (a *app) putPlaylist(pl *qqmusic.Playlist) {
	if pl == nil {
		return
	}
	if a.playlists == nil {
		a.playlists = map[int64]*qqmusic.Playlist{}
	}
	a.playlists[pl.ID] = pl
	a.touchPlaylist(pl.ID)
	a.evictPlaylists()
}

func (a *app) evictPlaylists() {
	live := a.playlistLRU[:0]
	totalTracks := 0
	for _, id := range a.playlistLRU {
		if pl := a.playlists[id]; pl != nil {
			live = append(live, id)
			totalTracks += len(pl.Tracks)
		}
	}
	a.playlistLRU = live
	for len(a.playlistLRU) > maxCachedPlaylists || totalTracks > maxCachedPlaylistTracks {
		if len(a.playlistLRU) <= 1 {
			break
		}
		evictIdx := -1
		for i, id := range a.playlistLRU {
			if a.route.page == pagePlaylist && a.route.id == id {
				continue
			}
			evictIdx = i
			break
		}
		if evictIdx < 0 {
			break
		}
		id := a.playlistLRU[evictIdx]
		if pl := a.playlists[id]; pl != nil {
			totalTracks -= len(pl.Tracks)
		}
		delete(a.playlists, id)
		a.playlistLRU = slices.Delete(a.playlistLRU, evictIdx, evictIdx+1)
	}
}

func (a *app) markPlaylistSynced(id int64) {
	if a.plSynced == nil {
		a.plSynced = map[int64]bool{}
	}
	if a.plSyncedAt == nil {
		a.plSyncedAt = map[int64]time.Time{}
	}
	a.plSynced[id] = true
	a.plSyncedAt[id] = time.Now()
	delete(a.plStale, id)
}

func (a *app) setPlaylistStale(id int64) {
	if a.plStale == nil {
		a.plStale = map[int64]bool{}
	}
	a.plStale[id] = true
}

func (a *app) takePlayAfterPlaylist(id int64) bool {
	if !a.playAfterPl[id] {
		return false
	}
	delete(a.playAfterPl, id)
	return true
}

func (a *app) invalidatePlaylist(id int64) {
	delete(a.playlists, id)
	delete(a.plSynced, id)
	delete(a.plSyncedAt, id)
	delete(a.plStale, id)
	delete(a.loadingPl, id)
	delete(a.refreshingPl, id)
	delete(a.playAfterPl, id)
	delete(a.plErr, id)
	a.removePlaylistLRU(id)
	a.bumpPlaylistGen(id)
	if a.pl != nil && a.pl.Cache != nil {
		cache := a.pl.Cache
		go cache.DropPlaylist(id)
	}
}

func (a *app) savePlaylistCache(pl *qqmusic.Playlist) {
	if pl == nil || pl.ID <= 0 || a.pl == nil || a.pl.Cache == nil {
		return
	}
	cache := a.pl.Cache
	snapshot := *pl
	snapshot.Tracks = slices.Clone(pl.Tracks)
	go cache.KeepPlaylist(&snapshot)
}

func (a *app) saveUserPlaylistsCache() {
	if a.profile == nil || a.pl == nil || a.pl.Cache == nil {
		return
	}
	uid, cache := a.profile.UserID, a.pl.Cache
	made := slices.Clone(a.mine)
	saved := slices.Clone(a.saved)
	go cache.KeepUserPlaylists(uid, made, saved)
}

func (a *app) loadPlaylist(id int64, thenPlay bool) {
	if pl := a.playlists[id]; pl != nil {
		a.touchPlaylist(id)
		if thenPlay {
			a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
		}
		if a.plStale[id] && !a.refreshingPl[id] {
			a.refreshPlaylist(id)
		}
		return
	}
	if a.loadingPl == nil {
		a.loadingPl = map[int64]bool{}
	}
	if a.plErr == nil {
		a.plErr = map[int64]string{}
	}
	if a.loadingPl[id] {
		if thenPlay {
			if a.playAfterPl == nil {
				a.playAfterPl = map[int64]bool{}
			}
			a.playAfterPl[id] = true
		}
		return
	}
	a.loadingPl[id] = true
	delete(a.plErr, id)
	if thenPlay {
		if a.playAfterPl == nil {
			a.playAfterPl = map[int64]bool{}
		}
		a.playAfterPl[id] = true
	}
	var cache *player.Cache
	if a.pl != nil {
		cache = a.pl.Cache
	}
	epoch, gen := a.playlistGen(id)
	go func() {
		if cache != nil {
			if cachedPl, _, ok := cache.LookupPlaylist(id); ok {
				qqmusic.RememberTracks(cachedPl.Tracks)
				a.post(func() {
					if !a.samePlaylistGen(id, epoch, gen) {
						return
					}
					delete(a.loadingPl, id)
					play := a.takePlayAfterPlaylist(id)
					if a.playlists[id] == nil {
						a.putPlaylist(cachedPl)
					} else {
						a.touchPlaylist(id)
						cachedPl = a.playlists[id]
					}
					if !a.plSynced[id] || time.Since(a.plSyncedAt[id]) >= playlistRefreshTTL {
						a.setPlaylistStale(id)
						a.refreshPlaylist(id)
					}
					if play {
						a.pl.PlayQueue(cachedPl.Tracks, firstPlayable(cachedPl.Tracks))
					}
				})
				return
			}
		}
		pl, err := playlistDetail(id)
		var encoded []byte
		if err == nil && pl != nil && cache != nil {
			encoded, _ = player.MarshalPlaylist(pl)
		}
		a.post(func() {
			if !a.samePlaylistGen(id, epoch, gen) {
				return
			}
			delete(a.loadingPl, id)
			play := a.takePlayAfterPlaylist(id)
			if err != nil || pl == nil {
				a.plErr[id] = "歌单加载失败"
				return
			}
			a.markPlaylistSynced(id)
			a.putPlaylist(pl)
			if cache != nil && len(encoded) > 0 {
				go cache.KeepPlaylistData(id, encoded)
			}
			if play {
				a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
			}
		})
	}()
}

// refreshPlaylist silently updates a cached playlist in the background without
// replacing it with a loading state if the network is slow or unavailable.
func (a *app) refreshPlaylist(id int64) {
	if a.refreshingPl[id] {
		return
	}
	if a.refreshingPl == nil {
		a.refreshingPl = map[int64]bool{}
	}
	a.refreshingPl[id] = true
	var cache *player.Cache
	if a.pl != nil {
		cache = a.pl.Cache
	}
	epoch, gen := a.playlistGen(id)
	go func() {
		pl, err := playlistDetail(id)
		var encoded []byte
		if err == nil && pl != nil && cache != nil {
			encoded, _ = player.MarshalPlaylist(pl)
		}
		a.post(func() {
			delete(a.refreshingPl, id)
			if !a.samePlaylistGen(id, epoch, gen) || err != nil || pl == nil {
				return
			}
			a.markPlaylistSynced(id)
			if cache != nil && len(encoded) > 0 {
				go cache.KeepPlaylistData(id, encoded)
			}
			if a.playlists[id] != nil {
				a.playlists[id] = pl
				a.evictPlaylists()
			} else if a.route.page == pagePlaylist && a.route.id == id {
				a.putPlaylist(pl)
			}
		})
	}()
}
