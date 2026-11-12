package app

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// taskbarLyrics owns the Explorer child used by the taskbar lyrics mode.
// Only the taskbar monitor works off-thread. Window and player state stay on main.
type taskbarLyrics struct {
	translationPolicy lyricTranslationPolicy
	a                 *app
	win               *mygo.Window
	coverWin          *mygo.Window
	controls          *mygo.Window
	controlsOpen      bool
	coverReady        bool
	scrub             scrubState
	stop              func()
	placement         taskbarLyricsPlacement
	parent            uintptr
	revision          uint64
	ready             bool
	visible           bool
	dark              bool
	lastError         string
}

func supportsTaskbarLyrics() bool { return runtime.GOOS == "windows" }

func (a *app) setTaskbarLyricsEnabled(enabled bool) {
	if !supportsTaskbarLyrics() {
		return
	}
	if enabled {
		a.applyLyricsMode(lyricsTaskbar, true)
	} else if a.settings.TaskbarLyrics {
		a.setLyricsEnabled(false)
	}
}

func (a *app) syncTaskbarLyrics() {
	if !supportsTaskbarLyrics() {
		return
	}
	if !a.settings.TaskbarLyrics || a.quitting {
		if d := a.taskbarLyrics; d != nil {
			a.taskbarLyrics = nil
			d.destroy()
		}
		return
	}
	if a.taskbarLyrics == nil {
		a.taskbarLyrics = &taskbarLyrics{a: a}
	}
	d := a.taskbarLyrics
	// UI tests exercise preferences without creating native windows or COM work.
	if a.win == nil || d.stop != nil {
		return
	}
	d.stop = startTaskbarLyricsMonitor(func(p taskbarLyricsPlacement) {
		if a.taskbarLyrics != d || !a.settings.TaskbarLyrics || a.quitting {
			return
		}
		d.updatePlacement(p)
	})
}

func (d *taskbarLyrics) updatePlacement(p taskbarLyricsPlacement) {
	changed := d.placement != p
	d.placement, d.dark = p, p.dark
	if p.parent == 0 || p.width <= 0 || p.height <= 0 {
		d.hide()
		return
	}
	if d.win != nil && (d.win.IsDestroyed() || d.parent != p.parent) {
		d.destroyWindow()
	}
	if d.win == nil {
		d.newWindow(p)
		return
	}
	if changed || !d.visible {
		d.present()
	} else if d.ready {
		// Explorer can reorder its composition bridge without changing geometry.
		// Repair child z-order without requesting another lyric paint.
		if err := placeTaskbarLyricsWindow(d.win, p); err != nil {
			d.reportError(err)
			d.hide()
		}
	}
	d.syncCoverWindow()
}

func (d *taskbarLyrics) newWindow(p taskbarLyricsPlacement) {
	dpi := max(p.dpi, 96)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: "任务栏歌词", Panel: true, Hidden: true, Frameless: true,
		Transparent: true, DisableShadow: true, DisableResize: true,
		Width: max(1, p.width*96/int(dpi)), Height: max(1, p.height*96/int(dpi)),
		Content: ui.View(d.view),
	})
	d.win, d.parent, d.ready = w, p.parent, false
	w.SetIgnoreMouseEvents(true)
	styleDesktopLyricsWindow(w)
	// Attach while hidden so even the initial transparent frame stays in Explorer.
	if err := placeTaskbarLyricsWindow(w, p); err != nil {
		d.reportError(err)
		d.destroyWindow()
		return
	}
	w.OnReadyToShow(func() {
		if d.win == w {
			d.ready = true
			d.present()
		}
	})
	w.OnClosed(func() {
		if d.win == w {
			d.win, d.ready, d.visible = nil, false, false
			d.destroyCoverWindow()
		}
	})
}

func (d *taskbarLyrics) present() {
	p := d.placement
	if d.win == nil || !d.ready || !d.a.settings.TaskbarLyrics || d.a.quitting || p.parent == 0 || p.width <= 0 || p.height <= 0 {
		return
	}
	if err := placeTaskbarLyricsWindow(d.win, p); err != nil {
		d.reportError(err)
		d.hide()
		return
	}
	d.lastError = ""
	d.visible = true
	d.win.Invalidate()
	if !d.win.IsVisible() {
		d.win.ShowInactive()
	}
	d.syncCoverWindow()
}

func (d *taskbarLyrics) reportError(err error) {
	if msg := err.Error(); msg != d.lastError {
		log.Printf("taskbar lyrics: %s", msg)
		d.lastError = msg
	}
}

func (d *taskbarLyrics) hide() {
	d.visible = false
	d.closeControls()
	if d.coverWin != nil && !d.coverWin.IsDestroyed() {
		d.coverWin.Hide()
	}
	if d.win != nil && !d.win.IsDestroyed() {
		d.win.Hide()
	}
}

func (d *taskbarLyrics) destroyWindow() {
	d.destroyCoverWindow()
	w := d.win
	d.win, d.ready, d.visible, d.parent = nil, false, false, 0
	if w != nil && !w.IsDestroyed() {
		detachTaskbarLyricsWindow(w)
		w.Destroy()
	}
}

func (d *taskbarLyrics) destroy() {
	if d.stop != nil {
		d.stop()
		d.stop = nil
	}
	d.destroyWindow()
}
