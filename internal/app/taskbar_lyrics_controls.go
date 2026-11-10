package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	taskbarControlsWidth   = 360
	taskbarControlsHeight  = 124
	taskbarMaterialWhite   = "white"
	taskbarMaterialAcrylic = "acrylic"
)

func (s settings) taskbarControlsVibrancy() mygo.Vibrancy {
	if s.TaskbarLyricsMaterial == taskbarMaterialWhite {
		return mygo.VibrancyNone
	}
	return mygo.VibrancyAcrylic
}

func (a *app) setTaskbarLyricsMaterial(material string) {
	a.settings.TaskbarLyricsMaterial = material
	a.settings.save()
	if d := a.taskbarLyrics; d != nil && d.controls != nil && !d.controls.IsDestroyed() {
		d.controls.SetVibrancy(a.settings.taskbarControlsVibrancy())
	}
	a.redraw()
}

// Only the cover owns an interactive taskbar rectangle; the lyric host stays
// transparent to input, including clicks sent to Explorer's other threads.
func (d *taskbarLyrics) syncCoverWindow() {
	if d.a.win == nil {
		return
	}
	tr := d.a.pl.Current()
	if !d.visible || !d.a.settings.TaskbarLyrics || d.win == nil || d.win.IsDestroyed() || tr == nil || tr.Album.PicURL == "" {
		d.destroyCoverWindow()
		return
	}
	if d.coverWin == nil || d.coverWin.IsDestroyed() {
		p := taskbarLyricsCoverPlacement(d.placement)
		if p.width <= 0 || p.height <= 0 {
			return
		}
		dpi := max(p.dpi, 96)
		w := mygo.NewWindow(mygo.WindowOptions{
			Title: "任务栏歌曲封面", Panel: true, Hidden: true, Frameless: true,
			Transparent: true, DisableShadow: true, DisableResize: true,
			Width: max(1, p.width*96/int(dpi)), Height: max(1, p.height*96/int(dpi)),
			Content: ui.View(d.coverView),
		})
		d.coverWin, d.coverReady = w, false
		w.SetIgnoreMouseEvents(false)
		styleDesktopLyricsWindow(w)
		if err := placeTaskbarLyricsCover(w, d.placement); err != nil {
			d.reportError(err)
			d.destroyCoverWindow()
			return
		}
		w.OnReadyToShow(func() {
			if d.coverWin == w {
				d.coverReady = true
				d.syncCoverWindow()
			}
		})
		w.OnClosed(func() {
			if d.coverWin == w {
				d.coverWin, d.coverReady = nil, false
				d.closeControls()
			}
		})
		return
	}
	if err := placeTaskbarLyricsCover(d.coverWin, d.placement); err != nil {
		d.reportError(err)
		d.destroyCoverWindow()
		return
	}
	if d.coverReady && !d.coverWin.IsVisible() {
		d.coverWin.Invalidate()
		d.coverWin.ShowInactive()
	}
}

func (d *taskbarLyrics) destroyCoverWindow() {
	d.closeControls()
	w := d.coverWin
	d.coverWin, d.coverReady = nil, false
	if w != nil && !w.IsDestroyed() {
		detachTaskbarLyricsWindow(w)
		w.Destroy()
	}
}

func (d *taskbarLyrics) toggleControls() {
	if d.controlsOpen {
		d.closeControls()
		return
	}
	tr := d.a.pl.Current()
	if !d.visible || !d.a.settings.TaskbarLyrics || tr == nil || tr.Album.PicURL == "" || d.a.quitting {
		return
	}
	d.controlsOpen = true
	// The same actions work in Tester without creating native windows.
	if d.coverWin == nil {
		d.a.redraw()
		return
	}
	bounds := d.coverWin.Bounds()
	w := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: d.coverWin, Anchor: mygo.Rectangle{Width: bounds.Width, Height: bounds.Height},
		Placement: mygo.PlacementTopStart, Gap: 16, Width: taskbarControlsWidth, Height: taskbarControlsHeight,
		Focusable: true, Shadow: true, Hidden: true, Vibrancy: mygo.VibrancyAcrylic, Content: ui.View(d.controlsView),
	})
	// Windows can enable a backdrop later only when the window was created
	// with material support. Disable it before showing the opaque white panel.
	w.SetVibrancy(d.a.settings.taskbarControlsVibrancy())
	d.controls = w
	w.OnClosed(func() {
		if d.controls == w {
			d.controls, d.controlsOpen, d.scrub = nil, false, scrubState{}
			d.a.redraw()
		}
	})
	w.OnReadyToShow(func() {
		if d.controls == w && d.controlsOpen && d.visible && !d.a.quitting {
			w.Show()
		}
	})
	d.a.redraw()
}

func (d *taskbarLyrics) closeControls() {
	open := d.controlsOpen
	w := d.controls
	d.controls, d.controlsOpen, d.scrub = nil, false, scrubState{}
	if w != nil && !w.IsDestroyed() {
		w.Destroy()
	}
	if open {
		d.a.redraw()
	}
}
