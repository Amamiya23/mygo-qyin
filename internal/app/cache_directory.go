package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lilt/internal/player"
)

// Tests replace the native picker without opening a system dialog.
var pickCacheDirectory = mygo.Dialog.Open

// cacheDirectoryRow shows the actual song directory and its picker.
func (a *app) cacheDirectoryRow(c *ui.Context) {
	p := paletteFor(c.Theme())
	path := "默认目录"
	if cache := a.pl.Cache; cache != nil {
		path = cache.Directory()
	}
	if pending := a.settings.PendingCacheDir; pending != nil {
		path = "默认目录"
		if *pending != "" {
			path = filepath.Join(*pending, "Lilt", "songs")
		}
	}
	ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, "缓存目录").FontSize(13.5).FontWeight(500)
			ui.Text(c, path).FontSize(12).TextColor(p.muted).SingleLine().Tooltip(path)
			if a.settings.PendingCacheDir != nil {
				ui.Text(c, "重启后迁移并生效").FontSize(12).TextColor(p.muted)
			}
			if a.cacheDirError != "" {
				ui.Text(c, a.cacheDirError).FontSize(12).TextColor(radixRed(c.Theme().Dark, 11)).LineHeight(1.4)
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if a.settings.PendingCacheDir != nil {
				ui.Button(c.Key("cancel-cache-dir"), "取消更改").Label("取消缓存目录更改").Disabled(a.cacheMoving).OnClick(func() {
					a.changeCacheDirectory(a.settings.CacheDir)
				})
			} else if a.settings.CacheDir != "" {
				ui.Button(c.Key("default-cache-dir"), "恢复默认").Label("恢复默认缓存目录").Disabled(a.cacheMoving).OnClick(func() {
					a.changeCacheDirectory("")
				})
			}
			ui.Button(c.Key("cache-dir"), "更改").Label("更改缓存目录").Disabled(a.cacheMoving).OnClick(a.chooseCacheDirectory)
		})
	})
}

func (a *app) chooseCacheDirectory() {
	path := a.settings.CacheDir
	if a.settings.PendingCacheDir != nil {
		path = *a.settings.PendingCacheDir
	}
	if path == "" && a.pl.Cache != nil {
		path = filepath.Dir(a.pl.Cache.Directory())
	}
	paths, err := pickCacheDirectory(mygo.OpenDialogOptions{
		Parent:            a.settingsWin,
		Title:             "选择缓存目录",
		ButtonLabel:       "选择",
		DefaultPath:       path,
		Directory:         true,
		CreateDirectories: true,
	})
	if err != nil {
		a.cacheDirError = "无法打开文件夹选择器"
		return
	}
	if len(paths) == 0 {
		return
	}
	a.changeCacheDirectory(paths[0])
}

func (a *app) changeCacheDirectory(dir string) {
	if err := a.setCacheDirectory(dir); err != nil {
		a.cacheDirError = err.Error()
	}
}

// setCacheDirectory stages a validated directory for migration before the next playback session.
func (a *app) setCacheDirectory(dir string) error {
	if a.cacheMoving {
		return fmt.Errorf("正在迁移缓存")
	}
	if dir != "" {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("请选择完整的文件夹路径")
		}
		dir = filepath.Clean(dir)
	}
	next := a.settings
	if dir == next.CacheDir {
		next.PendingCacheDir = nil
	} else {
		chosen := next
		chosen.CacheDir = dir
		if _, err := prepareSongCache(chosen); err != nil {
			return err
		}
		next.PendingCacheDir = &dir
	}
	if err := next.save(); err != nil {
		return fmt.Errorf("无法保存缓存目录：%w", err)
	}
	a.settings = next
	a.cacheDirError = ""
	return nil
}

func prepareSongCache(s settings) (*player.Cache, error) {
	cache := newSongCache(s)
	if cache == nil {
		return nil, fmt.Errorf("无法获取默认缓存目录")
	}
	if err := os.MkdirAll(cache.Directory(), 0o700); err != nil {
		return nil, fmt.Errorf("无法使用此目录：%w", err)
	}
	probe, err := os.CreateTemp(cache.Directory(), ".write-check-*")
	if err != nil {
		return nil, fmt.Errorf("此目录不可写：%w", err)
	}
	path := probe.Name()
	err = probe.Close()
	removeErr := os.Remove(path)
	if err != nil {
		return nil, fmt.Errorf("无法写入此目录：%w", err)
	}
	if removeErr != nil {
		return nil, fmt.Errorf("无法清理此目录：%w", removeErr)
	}
	return cache, nil
}
