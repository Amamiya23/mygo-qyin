package app

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"

	"lilt/internal/player"
)

// newSongCache returns the cache of songs played, in the app's cache
// directory, or nil without one.
func newSongCache(s settings) *player.Cache {
	if s.CacheDir != "" {
		return player.NewCache(filepath.Join(s.CacheDir, "Lilt", "songs"), int64(s.CacheMB)<<20)
	}
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return nil
	}
	return player.NewCache(filepath.Join(dir, "songs"), int64(s.CacheMB)<<20)
}

// applyCacheLimit gives the cache the limit the settings say.
func (a *app) applyCacheLimit() {
	if cache := a.pl.Cache; cache != nil {
		limit := int64(a.settings.CacheMB) << 20
		go func() {
			cache.SetLimit(limit)
			n := cache.Size()
			a.post(func() { a.cacheMeasured(cache, n) })
		}()
	}
}

// measureCache finds how much the cache takes, on a goroutine, for the
// settings to show.
func (a *app) measureCache() {
	cache := a.pl.Cache
	if cache == nil {
		return
	}
	go func() {
		n := cache.Size()
		a.post(func() { a.cacheMeasured(cache, n) })
	}()
}

// cacheMeasured ignores a measurement from a directory no longer in use.
func (a *app) cacheMeasured(cache *player.Cache, n int64) {
	if a.pl.Cache != cache {
		return
	}
	a.cacheUsed = n
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

func (a *app) clearCache() {
	if cache := a.pl.Cache; cache != nil {
		a.cacheUsed = 0
		go func() {
			cache.Clear()
			n := cache.Size()
			a.post(func() { a.cacheMeasured(cache, n) })
		}()
	}
}

// cacheUsage says how much the cache takes, once measured.
func (a *app) cacheUsage() string {
	switch n := a.cacheUsed; {
	case n < 0:
		return "正在计算已用空间…"
	case n >= 1<<30:
		return fmt.Sprintf("已用 %.1f GB", float64(n)/(1<<30))
	default:
		return fmt.Sprintf("已用 %d MB", n>>20)
	}
}
