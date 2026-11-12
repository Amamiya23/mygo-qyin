package app

import (
	"math"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const desktopLyricsWidth = 760

// desktopLyrics owns a panel independently of the main window's visibility.
// All its state and callbacks belong to MyGo's main thread.
type desktopLyrics struct {
	translationPolicy lyricTranslationPolicy
	a                 *app
	win               *mygo.Window
	offDisplays       func()
	width             float64
	desiredHeight     int
	revision          uint64
	workAreaOverride  func(mygo.Rectangle) mygo.Rectangle
	stopWorkArea      func()
}

func supportsDesktopLyrics() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

func (a *app) setDesktopLyricsEnabled(enabled bool) {
	if !supportsDesktopLyrics() {
		return
	}
	if enabled {
		a.applyLyricsMode(lyricsDesktop, true)
	} else if a.settings.DesktopLyrics {
		a.setLyricsEnabled(false)
	}
}

func (a *app) setDesktopLyricsLocked(locked bool) {
	if !supportsDesktopLyrics() || !a.settings.DesktopLyrics {
		return
	}
	if a.desktopLyrics == nil {
		a.syncDesktopLyrics()
	}
	if a.desktopLyrics != nil {
		a.desktopLyrics.setLocked(locked)
	}
}

func (a *app) syncDesktopLyrics() {
	if !supportsDesktopLyrics() {
		return
	}
	if a.desktopLyrics == nil && a.settings.DesktopLyrics {
		a.desktopLyrics = &desktopLyrics{a: a, width: desktopLyricsWidth}
	}
	d := a.desktopLyrics
	if d == nil {
		return
	}
	if !a.settings.DesktopLyrics {
		d.destroy()
		return
	}
	// Tests can exercise actions without opening an operating-system window.
	if a.win == nil || d.win != nil {
		return
	}
	work := mygo.Screen.PrimaryDisplay().WorkArea
	bounds := clampDesktopLyricsBounds(mygo.Rectangle{
		X:     work.X + (work.Width-desktopLyricsWidth)/2,
		Y:     work.Y + work.Height - 220,
		Width: desktopLyricsWidth, Height: 180,
	}, work)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: "桌面歌词", Panel: true, Hidden: true, Frameless: true,
		Transparent: true, DisableShadow: true, DisableResize: true,
		X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
		StateKey: "desktop-lyrics", Content: ui.View(d.view),
	})
	d.win = w
	styleDesktopLyricsWindow(w)
	watchDesktopLyricsWorkArea(d)
	d.desiredHeight = 0
	d.width = float64(w.Bounds().Width)
	setDesktopLyricsIgnoreMouse(w, a.settings.DesktopLyricsLocked)
	w.OnReadyToShow(func() {
		if d.win == w && a.settings.DesktopLyrics && !a.quitting {
			w.ShowInactive()
		}
	})
	w.OnClose(func(*mygo.CloseEvent) {
		if !a.quitting {
			a.settings.DesktopLyrics = false
			a.settings.save()
			a.refreshLyricsTray()
			a.redraw()
		}
	})
	w.OnClosed(func() {
		if d.win == w {
			d.clearWorkAreaWatcher()
			d.win = nil
			if d.offDisplays != nil {
				d.offDisplays()
				d.offDisplays = nil
			}
		}
	})
	d.offDisplays = mygo.Screen.OnDisplaysChanged(func() {
		if d.win == nil {
			return
		}
		r := d.win.Bounds()
		work := d.workArea(r)
		d.win.SetBounds(clampDesktopLyricsBounds(r, work))
	})
}

func (d *desktopLyrics) destroy() {
	d.clearWorkAreaWatcher()
	if d.offDisplays != nil {
		d.offDisplays()
		d.offDisplays = nil
	}
	if w := d.win; w != nil {
		// Keep the enabled preference when the whole application quits.
		d.win = nil
		w.Close()
	}
	d.desiredHeight = 0
}

func (d *desktopLyrics) clearWorkAreaWatcher() {
	if d.stopWorkArea != nil {
		d.stopWorkArea()
		d.stopWorkArea = nil
	}
	d.workAreaOverride = nil
}

func (d *desktopLyrics) workArea(r mygo.Rectangle) mygo.Rectangle {
	work := mygo.Screen.DisplayMatching(r).WorkArea
	if d.workAreaOverride != nil {
		return d.workAreaOverride(work)
	}
	return work
}

func (d *desktopLyrics) fitWorkArea() {
	if d.win == nil {
		return
	}
	r := d.win.Bounds()
	if next := clampDesktopLyricsBounds(r, d.workArea(r)); next != r {
		d.win.SetBounds(next)
	}
}

func (d *desktopLyrics) close() { d.a.setDesktopLyricsEnabled(false) }

func (d *desktopLyrics) setLocked(locked bool) {
	d.a.settings.DesktopLyricsLocked = locked
	d.a.settings.save()
	if d.win != nil {
		setDesktopLyricsIgnoreMouse(d.win, locked)
	}
	d.a.refreshLyricsTray()
	d.a.redraw()
}

func (d *desktopLyrics) fontSize() float32 {
	size := d.a.settings.DesktopLyricsFontSize
	if size == 0 || math.IsNaN(float64(size)) || math.IsInf(float64(size), 0) {
		return 32
	}
	return min(64, max(18, size))
}

func (d *desktopLyrics) setFontSize(size float32) {
	if math.IsNaN(float64(size)) || math.IsInf(float64(size), 0) {
		return
	}
	d.a.settings.DesktopLyricsFontSize = min(64, max(18, size))
	d.a.settings.save()
	d.a.redraw()
}

// fitHeight follows measured content once it changes; resizing is deferred
// until the current build finishes, and keeps the lyrics' bottom anchored.
func (d *desktopLyrics) fitHeight(height float64) {
	if d.win == nil || math.IsNaN(height) || math.IsInf(height, 0) {
		return
	}
	w := d.win
	work := d.workArea(w.Bounds())
	h := min(work.Height, max(100, int(math.Ceil(height))))
	if h == d.desiredHeight {
		return
	}
	d.desiredHeight = h
	w.Update(func() {
		if d.win != w || d.desiredHeight != h {
			return
		}
		r := w.Bounds()
		if r.Height == h {
			return
		}
		r.Y += r.Height - h
		r.Height = h
		w.SetBounds(clampDesktopLyricsBounds(r, work))
	})
}

func (d *desktopLyrics) resizeWidth(delta float64) {
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return
	}
	if d.width == 0 {
		d.width = desktopLyricsWidth
	}
	if d.win == nil {
		d.width = min(1800, max(320, d.width+delta))
		return
	}
	r := d.win.Bounds()
	work := d.workArea(r)
	r.Width = min(work.Width, max(320, r.Width+int(math.Round(delta))))
	r = clampDesktopLyricsBounds(r, work)
	d.width = float64(r.Width)
	d.win.SetBounds(r)
}

func clampDesktopLyricsBounds(r, work mygo.Rectangle) mygo.Rectangle {
	if work.Width <= 0 || work.Height <= 0 {
		return r
	}
	r.Width = min(work.Width, max(1, r.Width))
	r.Height = min(work.Height, max(1, r.Height))
	r.X = min(work.X+work.Width-r.Width, max(work.X, r.X))
	r.Y = min(work.Y+work.Height-r.Height, max(work.Y, r.Y))
	return r
}

func (a *app) refreshLyricsTray() {
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
}
