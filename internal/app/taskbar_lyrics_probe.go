package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

// RunTaskbarLyricsCheck validates the production owner without login, network or user preferences.
func RunTaskbarLyricsCheck(dir string, duration time.Duration, coverPath string) error {
	if !supportsTaskbarLyrics() {
		return fmt.Errorf("taskbar lyrics are Windows-only")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0700); err != nil {
		return err
	}
	mygo.App.SetName("Lilt Taskbar Lyrics Check")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, "data"))
	mygo.App.SetPath(mygo.PathCache, filepath.Join(dir, "cache"))
	a := &app{settings: defaultSettings()}
	a.settings.path = filepath.Join(dir, "settings.json")
	a.art = newArtCache(a.post)
	coverURL := ""
	if coverPath != "" {
		data, err := os.ReadFile(coverPath)
		if err != nil {
			return err
		}
		bmp, _, err := decodeArt(data)
		if err != nil {
			return err
		}
		coverURL = "taskbar-cover-fixture"
		a.art.add(qqmusic.Sized(coverURL, 96), &artwork{bmp: bmp, size: len(data), used: a.art.now()})
	}
	a.pl = player.New(a.post)
	a.pl.Changed = a.redraw
	results := map[string]any{}
	write := func(stage string) {
		sample := taskbarLyricsNativeSnapshot(a)
		sample["coverFixture"] = coverPath != ""
		if d := a.taskbarLyrics; d != nil && d.win != nil && !d.win.IsDestroyed() {
			if data, err := d.win.CapturePage(); err == nil {
				os.WriteFile(filepath.Join(dir, stage+".png"), data, 0600)
			}
			if err := captureTaskbarLyricsScreen(d.win, filepath.Join(dir, stage+"-screen.png")); err != nil {
				sample["screenCaptureError"] = err.Error()
			}
			if err := captureTaskbarLyricsBar(d.placement.taskbar, filepath.Join(dir, stage+"-taskbar.png")); err != nil {
				sample["taskbarCaptureError"] = err.Error()
			}
			if d.controls != nil && !d.controls.IsDestroyed() {
				if data, err := d.controls.CapturePage(); err == nil {
					os.WriteFile(filepath.Join(dir, stage+"-controls.png"), data, 0600)
				}
				if err := captureTaskbarLyricsScreen(d.controls, filepath.Join(dir, stage+"-controls-screen.png")); err != nil {
					sample["controlsCaptureError"] = err.Error()
				}
			}
			if d.coverWin != nil && !d.coverWin.IsDestroyed() {
				if data, err := d.coverWin.CapturePage(); err == nil {
					os.WriteFile(filepath.Join(dir, stage+"-cover.png"), data, 0600)
				}
				if err := captureTaskbarLyricsScreen(d.coverWin, filepath.Join(dir, stage+"-cover-screen.png")); err != nil {
					sample["coverCaptureError"] = err.Error()
				}
			}
		}
		results[stage] = sample
		if b, err := json.MarshalIndent(results, "", "  "); err == nil {
			os.WriteFile(filepath.Join(dir, "native-report.json"), b, 0600)
		}
	}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Lilt Taskbar Lyrics Check", Hidden: true, Width: 420, Height: 150,
			Content: ui.View(func(c *ui.Context) {
				ui.Column(c).Fill().Center().Gap(12).Background(radixSage(c.Theme().Dark, 2)).Children(func() {
					ui.Text(c, "任务栏歌词验证").FontSize(20)
					ui.Button(c, "结束验证").OnClick(mygo.App.Quit)
				})
			})})
		a.win.OnReadyToShow(a.win.ShowInactive)
		a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "离线歌词验证", Duration: 60000, Album: qqmusic.Album{PicURL: coverURL}}})
		a.pl.SetLyrics(lyrics.Build("[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", "", "[1000,10000]我(1000,1500)们(2500,1500)一(4000,1500)起(5500,1500)听(7000,1500)音(8500,1200)乐(9700,1300)\n[12000,10000]让(12000,1500)声(13500,1500)音(15000,1500)陪(16500,1500)伴(18000,1500)远(19500,1200)方(20700,1300)"))
		a.pl.Seek(4500 * time.Millisecond)
		a.setTaskbarLyricsEnabled(true)
		stages := []struct {
			name string
			run  func()
		}{
			{"shown", func() {}},
			{"paused-seek", func() { a.pl.Seek(15 * time.Second); a.redraw() }},
			{"controls-open", func() {
				if coverPath != "" && a.taskbarLyrics != nil {
					if err := clickTaskbarLyricsCover(a.taskbarLyrics); err != nil {
						a.taskbarLyrics.reportError(err)
					}
				}
			}},
			{"z-order-repair", func() {
				if coverPath != "" && a.taskbarLyrics != nil {
					if err := raiseTaskbarLyricsHost(a.taskbarLyrics); err != nil {
						a.taskbarLyrics.reportError(err)
					}
				}
			}},
			{"translated", func() {
				a.pl.SetLyrics(lyrics.Build("[00:01]Let the music carry us home", "[00:01]让音乐陪伴我们回家", ""))
				a.pl.Seek(4500 * time.Millisecond)
				a.redraw()
			}},
			{"controls-close", func() {
				if coverPath != "" && a.taskbarLyrics != nil {
					if err := escapeTaskbarLyricsControls(a.taskbarLyrics); err != nil {
						a.taskbarLyrics.reportError(err)
					}
				}
			}},
			{"long-line", func() {
				a.pl.SetLyrics(lyrics.Build("[00:01]Let every small and ordinary moment become a melody that stays with us through the long night", "[00:01]让每个平凡瞬间化作旋律，陪伴我们走过漫长的夜晚", ""))
				a.pl.Seek(4500 * time.Millisecond)
				a.redraw()
			}},
			{"main-hidden", func() { a.win.Hide() }},
			{"disabled", func() { a.setTaskbarLyricsEnabled(false) }},
			{"reopened", func() { a.setTaskbarLyricsEnabled(true) }},
		}
		for i, s := range stages {
			time.AfterFunc(time.Duration(3+i*4)*time.Second, func() {
				mygo.RunOnMain(func() {
					if a.quitting {
						return
					}
					s.run()
					time.AfterFunc(1800*time.Millisecond, func() {
						mygo.RunOnMain(func() {
							if !a.quitting {
								write(s.name)
							}
						})
					})
				})
			})
		}
		time.AfterFunc(duration, mygo.App.Quit)
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		if a.taskbarLyrics != nil {
			a.taskbarLyrics.destroy()
		}
		write("cleanup")
	})
	return mygo.App.Run()
}
