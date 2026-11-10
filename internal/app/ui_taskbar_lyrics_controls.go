package app

import (
	"github.com/egoist/mygo/ui"
	"lilt/internal/qqmusic"
)

type taskbarControlsTrack struct {
	id  int64
	mid string
}

// controlsView is a small player above the taskbar cover, independent of the main window.
func (d *taskbarLyrics) controlsView(c *ui.Context) {
	theme := *c.Theme()
	white := d.a.settings.TaskbarLyricsMaterial == taskbarMaterialWhite
	material := !white && c.Vibrancy()
	// The material follows app appearance, which can differ from the taskbar.
	if white {
		theme.Dark = false
	} else if !material {
		theme.Dark = d.dark
	}
	c.SetTheme(&theme)
	dark := theme.Dark
	background, text := radixGray(dark, 2), radixGray(dark, 12)
	muted := radixGray(dark, 11)
	if material {
		background = ui.Transparent
	}
	c.Root().Background(background)
	c.OnShortcut(0, ui.KeyEscape, d.closeControls)
	tr := d.a.pl.Current()
	if !d.controlsOpen || tr == nil {
		return
	}
	identity := taskbarControlsTrack{id: tr.ID, mid: tr.MID}
	previous := ui.Local(c.Root(), "taskbar-controls-track", func() taskbarControlsTrack { return identity })
	if *previous != identity {
		d.scrub = scrubState{}
		*previous = identity
	}
	a, pl := d.a, d.a.pl
	p := paletteFor(&theme)
	a.tickTimes(c)
	ui.Column(c.Key(identity)).Fill().Padding(12).Gap(8).TextColor(text).Children(func() {
		ui.Row(c).Height(64).Gap(12).AlignItems(ui.Center).Children(func() {
			cover := ui.Box(c.Key("artwork")).Size(64, 64).Shrink(0).Radius(7).Background(radixGray(dark, 4)).Center()
			bmp := bitmap(a.art.get(qqmusic.Sized(tr.Album.PicURL, 120), artCover))
			cover.Children(func() {
				if bmp != nil {
					ui.Image(c, bmp).Size(64, 64).Fit(ui.Cover).Radius(7)
				} else {
					ui.Icon(c, iconNote).Size(24, 24).TextColor(muted)
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).LineHeight(1.15).SingleLine().Tooltip(tr.Name)
				artist := tr.ArtistNames()
				ui.Text(c, artist).FontSize(13).LineHeight(1.15).TextColor(muted).SingleLine().Tooltip(artist)
			})
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
				iconButton(c.Key("previous"), iconPrev, 18, text, "上一首", playerSurface).Size(28, 28).Shrink(0).
					OnClick(func() { pl.Prev(); a.redraw() })
				playButton(c.Key("play"), pl.Playing(), 20, text, playerSurface).Size(36, 36).Shrink(0).
					OnClick(func() { pl.TogglePlay(); a.redraw() })
				iconButton(c.Key("next"), iconNext, 18, text, "下一首", playerSurface).Size(28, 28).Shrink(0).
					OnClick(func() { pl.Next(); a.redraw() })
			})
		})
		ui.Column(c).Gap(0).Children(func() {
			a.scrubber(c.Key("progress"), &d.scrub, radixGray(dark, 8), p.text.Alpha(0.75), 14, p.accent).FillWidth()
			position := pl.Position()
			if d.scrub.active || d.scrub.held {
				position = secs(d.scrub.v)
			}
			ui.Row(c).Height(14).Justify(ui.SpaceBetween).TextColor(muted).Children(func() {
				ui.Text(c, clock(position)).FontSize(11.5).FontFeatures("tnum").TextColor(muted)
				ui.Text(c, clock(pl.Duration())).FontSize(11.5).FontFeatures("tnum").TextColor(muted)
			})
		})
	})
}
