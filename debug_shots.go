package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/egoist/mygo"
)

// With NM_SHOTS set to a directory, the app walks through its views and
// saves what each window shows there, for checking the design.
func init() {
	dir := os.Getenv("NM_SHOTS")
	if dir == "" {
		return
	}
	shotHook = func(a *app) {
		// The scenes change settings to measure them: keep the
		// listener's own as they were.
		a.settings.path = ""
		go func() {
			shot := func(name string, w *mygo.Window) {
				png, err := w.CapturePage()
				if err == nil {
					os.WriteFile(filepath.Join(dir, name+".png"), png, 0o644)
				}
			}
			step := func(d time.Duration, fn func()) {
				time.Sleep(d)
				mygo.RunOnMain(func() { fn(); a.redraw() })
			}
			mygo.RunOnMain(func() { a.win.Show(); a.win.Focus() })
			if os.Getenv("NM_SCENE") == "perf" {
				// Muted, marks on stderr where each part begins, for the
				// frame log of MYGO_FRAME_STATS.
				mark := func(s string) { println("mark", time.Now().Format("15:04:05.000"), s) }
				// Above other windows: a covered window draws no frames.
				mygo.RunOnMain(func() {
					a.win.SetAlwaysOnTop(true)
					a.win.Show()
					// NM_SMOKE=0 and NM_NOTCH=0 turn them off, to measure them.
					a.settings.Smoke = os.Getenv("NM_SMOKE") != "0"
					a.settings.Notch = os.Getenv("NM_NOTCH") != "0"
					a.notch.setEnabled(a.settings.Notch)
				})
				for range 40 {
					var n int
					mygo.RunOnMain(func() { n = len(a.daily) })
					if n > 0 {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
				step(0, func() { a.volume = 0; a.pl.setVolume(0); a.pl.playQueue(a.daily, 0) })
				time.Sleep(4 * time.Second)
				for round := range 2 {
					if round == 0 {
						// A CPU profile of the opening, for go tool pprof.
						if f, err := os.Create(filepath.Join(dir, "open.pprof")); err == nil {
							pprof.StartCPUProfile(f)
							go func() {
								time.Sleep(1500 * time.Millisecond)
								pprof.StopCPUProfile()
								f.Close()
							}()
						}
					}
					mark(fmt.Sprint("open", round))
					step(0, func() { a.nowPlaying, a.npPane = true, paneLyrics })
					time.Sleep(3 * time.Second)
					mark(fmt.Sprint("close", round))
					step(0, func() { a.nowPlaying = false })
					time.Sleep(2 * time.Second)
				}
				for round := range 2 {
					mark(fmt.Sprint("stage", round))
					step(0, func() { a.nowPlaying, a.npPane = true, paneStage })
					time.Sleep(4 * time.Second)
					mark(fmt.Sprint("shut", round))
					step(0, func() { a.nowPlaying = false })
					time.Sleep(2 * time.Second)
				}
				mark("end")
				mygo.RunOnMain(mygo.App.Quit)
				return
			}
			if os.Getenv("NM_SCENE") == "np" {
				time.Sleep(5 * time.Second)
				shot("q-home", a.win)
				step(0, func() { a.open(route{page: pageDaily}) })
				time.Sleep(2 * time.Second)
				shot("q-daily", a.win)
				step(0, func() { a.pl.playQueue(a.daily, 0); a.volume = 0.15; a.pl.setVolume(0.15) })
				time.Sleep(4 * time.Second)
				step(0, func() { a.nowPlaying = true })
				time.Sleep(4 * time.Second)
				shot("q-np", a.win)
				mygo.RunOnMain(mygo.App.Quit)
				return
			}
			time.Sleep(5 * time.Second)
			mygo.RunOnMain(func() {
				x, y, w, h := windowFrame(a.notch.win.NativeHandle())
				println("notch frame", int(x), int(y), int(w), int(h))
			})
			shot("1-home", a.win)
			shot("1-notch-idle", a.notch.win)
			step(0, func() { a.open(route{page: pageCharts}) })
			time.Sleep(time.Second)
			shot("2-charts", a.win)
			step(0, func() { a.open(route{page: pagePlaylist, playlist: a.charts[1].ID}) })
			time.Sleep(4 * time.Second)
			shot("3-playlist", a.win)
			step(0, func() {
				pl := a.playlists[a.charts[1].ID]
				a.volume = 0.15
				a.pl.setVolume(a.volume)
				a.pl.playQueue(pl.Tracks, firstPlayable(pl.Tracks))
			})
			time.Sleep(5 * time.Second)
			shot("4-playing", a.win)
			shot("4-notch-compact", a.notch.win)
			step(0, func() { a.notch.open = true; a.notch.lastInside = time.Now().Add(time.Hour) })
			time.Sleep(2 * time.Second)
			shot("5-notch-open", a.notch.win)
			step(0, func() { a.nowPlaying = true })
			time.Sleep(4 * time.Second)
			shot("6-now-playing", a.win)
			step(0, func() { a.npPane = paneQueue })
			time.Sleep(2 * time.Second)
			shot("7-queue", a.win)
			step(0, func() { a.search("周杰伦") })
			time.Sleep(4 * time.Second)
			shot("8-search", a.win)
			mygo.RunOnMain(mygo.App.Quit)
		}()
	}
}
