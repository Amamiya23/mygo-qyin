package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

// RunDesktopLyricsProbe runs the real lyrics panel with offline fixtures and
// isolated preferences, for native-window checks without changing a listener's data.
func RunDesktopLyricsProbe(dir string, duration time.Duration, check bool) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0700); err != nil {
		return err
	}
	mygo.App.SetName("Lilt Lyrics Probe")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, "data"))
	mygo.App.SetPath(mygo.PathCache, filepath.Join(dir, "cache"))
	a := &app{settings: defaultSettings()}
	a.settings.path = filepath.Join(dir, "settings.json")
	a.pl = player.New(a.post)
	a.pl.Changed = func() { a.redraw() }
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Lilt Lyrics Probe", Width: 580, Height: 300,
			Content: ui.View(func(c *ui.Context) {
				ui.Column(c).Fill().Padding(24).Gap(12).Background(radixSage(c.Theme().Dark, 2)).Children(func() {
					ui.Text(c, "桌面歌词 · 原生窗口验证").FontSize(20)
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, "打开歌词").OnClick(func() { a.setDesktopLyricsEnabled(true) })
						ui.Button(c, "锁定歌词").OnClick(func() { a.desktopLyrics.setLocked(true) })
						ui.Button(c, "解锁歌词").OnClick(func() { a.desktopLyrics.setLocked(false) })
						ui.Button(c, "关闭歌词").OnClick(func() { a.setDesktopLyricsEnabled(false) })
						ui.Button(c, "测试菜单").OnClick(func() { a.trayMenu().Popup(a.win) })
					})
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, "隐藏主窗 3 秒").OnClick(func() {
							a.win.Hide()
							time.AfterFunc(3*time.Second, func() { mygo.RunOnMain(a.showMain) })
						})
						ui.Button(c, "英文长句").OnClick(func() {
							a.pl.SetLyrics(lyrics.Build("[00:01.00]Let every small and ordinary moment become a melody that stays with us through the long night", "[00:01.00]让每个平凡瞬间化作旋律，陪伴我们走过漫长的夜晚", ""))
							a.redraw()
						})
						ui.Button(c, "退出验证").OnClick(mygo.App.Quit)
					})
				})
			})})
		a.win.OnReadyToShow(func() { a.win.Show(); a.win.Focus() })
		a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "歌词验证", Duration: 60000}})
		a.pl.SetLyrics(lyrics.Build("[00:01.00]风吹过遥远的山海", "", "[1000,10000]风(1000,1200)吹(2200,1200)过(3400,1200)遥(4600,1200)远(5800,1200)的(7000,1200)山(8200,1200)海(9400,1600)"))
		a.pl.Hold(4500 * time.Millisecond)
		a.setDesktopLyricsEnabled(true)
		a.applyTray()
		if check {
			runDesktopLyricsNativeChecks(a, dir)
		}
		if duration > 0 {
			time.AfterFunc(duration, mygo.App.Quit)
		}
		go func() {
			time.Sleep(time.Second)
			mygo.RunOnMain(func() {
				if d := a.desktopLyrics; d != nil && d.win != nil {
					if data, err := d.win.CapturePage(); err == nil {
						os.WriteFile(filepath.Join(dir, "desktop-lyrics.png"), data, 0600)
					}
				}
			})
		}()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		if a.desktopLyrics != nil {
			a.desktopLyrics.destroy()
		}
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
	})
	return mygo.App.Run()
}

func runDesktopLyricsNativeChecks(a *app, dir string) {
	results := map[string]any{}
	stages := []struct {
		name   string
		action func()
	}{
		{"shown", func() {}},
		{"resized", func() { a.desktopLyrics.resizeWidth(40) }},
		{"long-line", func() {
			a.pl.SetLyrics(lyrics.Build("[00:01]让每个平凡瞬间化作旋律，陪伴我们走过漫长的夜晚，一起听见那些熟悉的声音", "", ""))
			a.redraw()
		}},
		{"short-line", func() {
			a.pl.SetLyrics(lyrics.Build("[00:01]我们一起听音乐", "", ""))
			a.redraw()
		}},
		{"double-line", func() {
			a.settings.DesktopLyricsDoubleLine = true
			a.pl.SetLyrics(lyrics.Build("[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", "", ""))
			a.redraw()
		}},
		{"single-line", func() {
			a.settings.DesktopLyricsDoubleLine = false
			a.redraw()
		}},
		{"translated-double-line", func() {
			a.settings.DesktopLyricsDoubleLine = true
			a.pl.SetLyrics(lyrics.Build("[00:01]Let the music carry us home\n[00:12]Stay with me", "[00:01]让音乐陪伴我们回家\n[00:12]留在我身边", ""))
			a.redraw()
		}},
		{"translated-single-line", func() {
			a.settings.DesktopLyricsDoubleLine = false
			a.redraw()
		}},
		{"main-hidden", func() { a.win.Hide() }},
		{"locked", func() { a.desktopLyrics.setLocked(true) }},
		{"locked-moved", func() {
			r := a.desktopLyrics.win.Bounds()
			r.X += 32
			r.Y -= 32
			a.desktopLyrics.win.SetBounds(r)
		}},
		{"unlocked", func() { a.desktopLyrics.setLocked(false) }},
		{"closed", func() { a.setDesktopLyricsEnabled(false) }},
		{"reopened", func() { a.setDesktopLyricsEnabled(true); a.win.ShowInactive() }},
		{"locked-again", func() { a.setDesktopLyricsLocked(true) }},
		{"closed-while-locked", func() { a.setDesktopLyricsEnabled(false) }},
		{"restored-locked", func() { a.setDesktopLyricsEnabled(true) }},
		{"bottom-edge", func() {
			w := a.desktopLyrics.win
			r := w.Bounds()
			work := a.desktopLyrics.workArea(r)
			r.Y = work.Y + work.Height - r.Height
			w.SetBounds(r)
		}},
		{"unlocked-bottom-edge", func() { a.setDesktopLyricsLocked(false); a.desktopLyrics.fitWorkArea() }},
	}
	for i, stage := range stages {
		time.AfterFunc(time.Duration(i+1)*time.Second, func() {
			mygo.RunOnMain(func() {
				stage.action()
				// Newly created panels show only after their first rendered frame.
				time.AfterFunc(300*time.Millisecond, func() {
					mygo.RunOnMain(func() {
						results[stage.name] = desktopLyricsNativeSnapshot(a)
						fmt.Fprintf(os.Stderr, "lyrics-probe snapshot=%s locked=%t\n", stage.name, a.settings.DesktopLyricsLocked)
						if strings.HasSuffix(stage.name, "-line") && a.desktopLyrics.win != nil {
							if data, err := a.desktopLyrics.win.CapturePage(); err == nil {
								os.WriteFile(filepath.Join(dir, stage.name+".png"), data, 0600)
							}
						}
						if b, err := json.MarshalIndent(results, "", "  "); err == nil {
							os.WriteFile(filepath.Join(dir, "native-report.json"), b, 0600)
						}
					})
				})
			})
		})
	}
}
