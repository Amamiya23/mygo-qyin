package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"lilt/internal/player"
)

// startCacheMigration runs before boot restores playback, so no cached files are open.
func (a *app) startCacheMigration() {
	root := *a.settings.PendingCacheDir
	chosen := a.settings
	chosen.CacheDir = root
	source := a.pl.Cache
	go func() {
		target, err := prepareSongCache(chosen)
		cleanup := func() error { return nil }
		if err == nil && source != nil {
			var lastProgress int64
			cleanup, err = source.CopyTo(target, func(done, total int64) {
				if done != 0 && done != total && done-lastProgress < 16<<20 {
					return
				}
				lastProgress = done
				a.post(func() { a.cacheMoveDone, a.cacheMoveTotal = done, total })
			})
		}
		a.post(func() {
			if err == nil {
				err = a.commitCacheDirectory(root, target)
			}
			a.cacheMoving = false
			if err != nil {
				a.cacheDirError = "缓存迁移失败：" + err.Error()
				a.toast = "缓存迁移失败，继续使用原目录"
			} else {
				// Only remove originals after the new location has been saved.
				go func() {
					if err := cleanup(); err != nil {
						a.post(func() {
							a.cacheDirError = "已切换目录，部分原缓存未能删除：" + err.Error()
							a.toast = "已切换缓存目录，部分原缓存未能删除"
						})
					}
				}()
			}
			a.boot()
		})
	}()
}

// commitCacheDirectory saves the completed migration before switching the player.
func (a *app) commitCacheDirectory(root string, cache *player.Cache) error {
	next := a.settings
	next.CacheDir, next.PendingCacheDir = root, nil
	if err := next.save(); err != nil {
		return err
	}
	a.settings, a.pl.Cache = next, cache
	a.cacheDirError, a.cacheUsed = "", -1
	a.applyCacheLimit()
	return nil
}

func (a *app) cacheMigrationView(c *ui.Context) {
	p := a.pal
	c.Root().Background(p.bg)
	ui.Column(c).Fill().Center().Padding(40).Children(func() {
		ui.Column(c).Width(360).Gap(16).Children(func() {
			ui.Text(c, "正在迁移缓存").FontSize(22).FontWeight(600)
			progress := float32(0)
			if a.cacheMoveTotal > 0 {
				progress = float32(a.cacheMoveDone) / float32(a.cacheMoveTotal)
			}
			ui.Box(c).Height(6).Radius(3).Background(p.hairline).Children(func() {
				ui.Box(c).Height(6).Radius(3).WidthPercent(progress * 100).Background(p.accent)
			})
			ui.Text(c, fmt.Sprintf("%s / %s", megabytes(a.cacheMoveDone), megabytes(a.cacheMoveTotal))).FontSize(13).TextColor(p.muted)
		})
	})
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(topBar(c)).DragWindow()
}
