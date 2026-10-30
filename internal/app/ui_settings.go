package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.measureCache()
		return
	}
	a.measureCache()
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "设置",
		Width:           560,
		Height:          720,
		MinWidth:        480,
		MinHeight:       420,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		// The app's menu has nothing for this window: on Linux and
		// Windows it stays out of sight until Alt shows it.
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f7f9f8, #171918)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		a.syncDock()
	})
	a.syncDock()
}

// changed applies the settings everywhere and keeps them.
func (a *app) changed() {
	a.settings.save()
	a.settings.applyAppearance()
	if a.notch != nil {
		a.notch.setEnabled(a.settings.Notch)
	}
	a.redraw()
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAccent
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.settingsWin.Close() })
	s := &a.settings

	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		row := func(label, desc string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, label).FontSize(13.5).FontWeight(500)
					if desc != "" {
						ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
					}
				})
				control()
			})
		}

		group("通用", func() {
			row("外观", "", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.changed()
				}
			})
			if runtime.GOOS == "windows" {
				row("侧边栏材质", "", func() {
					i := 0
					if s.sidebarVibrancy() == mygo.VibrancyMica {
						i = 1
					}
					if segmented(c.Key("sidebar-material"), &i, "亚克力", "Mica") {
						s.SidebarMaterial = []string{"acrylic", "mica"}[i]
						if a.win != nil {
							a.win.SetVibrancy(s.sidebarVibrancy())
						}
						s.save()
						a.redraw()
					}
				})
			}

			if a.canHideOnClose() {
				row("关闭主窗口时", "", func() {
					i := 0
					if s.QuitOnClose {
						i = 1
					}
					keepRunning := "最小化到托盘"
					switch runtime.GOOS {
					case "darwin":
						keepRunning = "保持后台运行"
					case "linux":
						keepRunning = "隐藏到托盘"
					}
					if segmented(c.Key("close-action"), &i, keepRunning, "退出应用") {
						s.QuitOnClose = i == 1
						s.save()
					}
				})
				// Only on macOS: elsewhere the icon is always in the notification
				// area.
				if runtime.GOOS == "darwin" {
					row("应用图标", "显示在菜单栏时，关闭窗口后可从菜单栏图标打开", func() {
						labels := make([]string, len(appIcons))
						chosen := appIcons[0].label
						for i, ic := range appIcons {
							labels[i] = ic.label
							if ic.id == s.AppIcon {
								chosen = ic.label
							}
						}
						if ui.Select(c.Key("app-icon"), &chosen, labels).Label("应用图标").Width(150).Changed() {
							for _, ic := range appIcons {
								if ic.label == chosen {
									s.AppIcon = ic.id
								}
							}
							a.changed()
							a.changeAppIcon()
						}
					})
				}
			}

			// Only on macOS, the one with a notch player.
			if a.notch != nil {
				row("显示刘海播放器", "在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开", func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label("显示刘海播放器").Changed() {
						a.changed()
					}
				})
				row("播放时显示实时活动", "在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标", func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label("播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row("在无刘海的屏幕上常驻显示", "关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏", func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label("在无刘海的屏幕上常驻显示").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			}

			row("音质", "按 QQ 音乐账号权限播放 MP3 或 FLAC，下一首生效", func() {
				labels := make([]string, len(qualityLevels))
				chosen := qualityLevels[1].label
				for i, q := range qualityLevels {
					labels[i] = q.label
					if q.level == s.Quality {
						chosen = q.label
					}
				}
				if ui.Select(c.Key("quality"), &chosen, labels).Label("音质").Width(150).Changed() {
					for _, q := range qualityLevels {
						if q.label == chosen {
							s.Quality = q.level
						}
					}
					a.changed()
				}
			})
			row("间奏时显示热评", "前奏、间奏与尾奏时，歌词处换成这首歌的热门评论", func() {
				if ui.Switch(c.Key("hot-comments"), &s.HotComments).Label("间奏时显示热评").Changed() {
					a.changed()
				}
			})

			row("歌曲缓存", a.cacheUsage(), func() {
				labels := make([]string, len(cacheLimits))
				chosen := cacheLimits[2].label
				for i, l := range cacheLimits {
					labels[i] = l.label
					if l.mb == s.CacheMB {
						chosen = l.label
					}
				}
				if ui.Select(c.Key("cache"), &chosen, labels).Label("歌曲缓存").Width(150).Disabled(a.cacheMoving).Changed() {
					for _, l := range cacheLimits {
						if l.label == chosen {
							s.CacheMB = l.mb
						}
					}
					a.changed()
					a.applyCacheLimit()
				}
			})
			a.cacheDirectoryRow(c)
			row("清除缓存", "", func() {
				if ui.Button(c.Key("clear-cache"), "清除").Disabled(a.cacheMoving || a.cacheUsed <= 0).Clicked() {
					a.clearCache()
				}
			})
		})

		group("实验性功能", func() {
			row("进度条烟雾特效", "播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量", func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label("进度条烟雾特效").Changed() {
					a.changed()
				}
			})
			row("烟雾背景", "播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量", func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label("烟雾背景").Changed() {
					a.changed()
				}
			})
		})

		group("关于与更新", func() {
			row("当前版本 "+mygo.App.Version(), "", func() {
				if a.updates {
					if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
						checkForUpdates()
					}
				}
			})
			if a.updates {
				desc := ""
				if t := lastUpdateCheck(); !t.IsZero() {
					desc = "上次检查：" + commentTime(t, time.Now())
				}
				row("自动检查更新", desc, func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
						setUpdateChecks(on)
					}
				})
				row("自动下载并安装", "", func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
			}
		})
	})
}
