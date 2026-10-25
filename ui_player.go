package main

import (
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// scrubState is a progress bar's own: the value dragged, while it is.
type scrubState struct {
	v      float64
	active bool
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// scrubber shows how far the track is, and seeks where it is dragged or
// clicked: the bar thickens under the pointer, and its knob shows.
func (a *app) scrubber(c *ui.Context, st *scrubState, track, fill ui.Color, height float32) ui.Element {
	pl := a.pl
	dur := pl.duration().Seconds()
	if !st.active {
		st.v = pl.position().Seconds()
	}
	s := ui.SliderBase(c, &st.v, 0, max(dur, 1)).Height(height).FocusRing(false).Label("播放进度").Cursor(ui.CursorPointer)
	pressed := s.Pressed()
	switch {
	case pressed:
		st.active = true
	case st.active:
		st.active = false
		pl.seek(secs(st.v))
	case s.Changed():
		pl.seek(secs(st.v))
	}
	hot := s.Hovered() || pressed
	thick := float32(4)
	if hot {
		thick = 6
	}
	thick = s.AnimateTo("thick", thick, ui.Snappy)
	knob := float32(0)
	if hot {
		knob = 1
	}
	knob = s.AnimateTo("knob", knob, ui.Snappy)
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		at := st.v
		if !st.active {
			at = pl.position().Seconds()
		}
		frac := float32(0)
		if dur > 0 {
			frac = float32(min(1, max(0, at/dur)))
		}
		y := r.Y + (r.H-thick)/2
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, track, thick/2)
		if dl := pl.stream; dl != nil {
			// What has downloaded, a shade lighter.
			if got := float32(dl.dl.Progress()); got < 1 {
				p.Fill(ui.Rect{X: r.X, Y: y, W: r.W * got, H: thick}, track, thick/2)
			}
		}
		p.Fill(ui.Rect{X: r.X, Y: y, W: max(thick, r.W*frac), H: thick}, fill, thick/2)
		if knob > 0.01 {
			k := 12 * knob
			p.Shadow(ui.Rect{X: r.X + r.W*frac - k/2, Y: r.Y + (r.H-k)/2, W: k, H: k}, k/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.3))
			p.Fill(ui.Rect{X: r.X + r.W*frac - k/2, Y: r.Y + (r.H-k)/2, W: k, H: k}, fill, k/2)
		}
		if pl.playing && !st.active {
			p.AnimationFrame()
		}
	})
	return s
}

// tickTimes redraws the view as the next second of the track begins, for
// the times shown.
func (a *app) tickTimes(c *ui.Context) {
	if a.pl.playing && a.pl.out != nil {
		pos := a.pl.position()
		c.After(time.Second - pos%time.Second + 10*time.Millisecond)
	}
}

// volumeSlider is a slim slider with the speaker before it, which mutes.
func (a *app) volumeSlider(c *ui.Context, color, track ui.Color, width float32) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		icon := iconVolume
		if a.volume == 0 {
			icon = iconMute
		}
		if iconButton(c.Key("mute"), icon, 16, color, "静音").Clicked() {
			if a.volume > 0 {
				a.mutedFrom, a.volume = a.volume, 0
			} else {
				a.volume = max(a.mutedFrom, 0.5)
			}
			a.pl.setVolume(a.volume)
		}
		s := ui.SliderBase(c.Key("volume"), &a.volume, 0, 1).Width(width).Height(16).FocusRing(false).Label("音量")
		if s.Changed() {
			a.pl.setVolume(a.volume)
		}
		hot := s.Hovered() || s.Pressed()
		thick := float32(4)
		if hot {
			thick = 5
		}
		thick = s.AnimateTo("thick", thick, ui.Snappy)
		v := float32(a.volume)
		s.Draw(func(p *ui.Painter, r ui.Rect) {
			y := r.Y + (r.H-thick)/2
			p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, track, thick/2)
			p.Fill(ui.Rect{X: r.X, Y: y, W: max(thick, r.W*v), H: thick}, color, thick/2)
		})
	})
}

// transport is the row of controls: shuffle, previous, play, next, repeat.
func (a *app) transport(c *ui.Context, color, dim, on ui.Color, size float32) {
	pl := a.pl
	ui.Row(c).Gap(size * 0.5).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		shuffle := dim
		if pl.shuffle {
			shuffle = on
		}
		if iconButton(c.Key("shuffle"), iconShuffle, size*0.62, shuffle, "随机播放").Clicked() {
			pl.toggleShuffle()
		}
		if iconButton(c.Key("prev"), iconPrev, size*0.85, color, "上一首").Clicked() {
			pl.prev()
		}
		icon := iconPlay
		if pl.playing {
			icon = iconPause
		}
		if iconButton(c.Key("play"), icon, size*1.25, color, "播放/暂停").Clicked() {
			pl.togglePlay()
		}
		if iconButton(c.Key("next"), iconNext, size*0.85, color, "下一首").Clicked() {
			pl.next()
		}
		repeat, ricon := dim, iconRepeat
		switch pl.repeat {
		case repeatAll:
			repeat = on
		case repeatOne:
			repeat, ricon = on, iconRepeat1
		}
		if iconButton(c.Key("repeat"), ricon, size*0.62, repeat, "循环").Clicked() {
			pl.cycleRepeat()
		}
	})
}

// playerBar runs along the foot of the content: what plays, its controls
// and its progress, and the volume.
func (a *app) playerBar(c *ui.Context) {
	p := a.pal
	pl := a.pl
	tr := pl.current()
	a.tickTimes(c)
	// Over the foot of the page: while content is under it, its edge frosts
	// it, with a hairline along the top.
	edge := ui.Box(c.Key("bottom-edge")).Absolute().Left(0).Right(0).Bottom(0).Height(playerBarHeight).
		Material(glass.ScrollEdge{Hard: true, Bottom: true, Background: p.bg})
	edge.Opacity(edgeShown(edge, a.underBottom))
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(0).Height(playerBarHeight).PaddingX(20).Gap(20).AlignItems(ui.Center).Children(func() {
		// What plays.
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			if tr == nil {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(8).Background(p.hover).Center().Children(func() {
						ui.Icon(c, iconNote).Size(18, 18).TextColor(p.faint)
					})
					ui.Text(c, "未在播放").FontSize(13).TextColor(p.muted)
				})
				return
			}
			info := ui.ButtonBase(c.Key("info")).Gap(12).AlignItems(ui.Center).FocusRing(false).Shrink(1).MinWidth(0).Label("打开正在播放")
			hovered := info.Hovered()
			info.Children(func() {
				ui.Box(c).Size(48, 48).Children(func() {
					a.cover(c, tr.Album.PicURL, 120, 48, 8).Shadow(0, 2, 8, 0, p.cardShade)
					if hovered {
						ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(8).Center().
							Background(ui.RGBA(0, 0, 0, 0.35)).TextColor(ui.RGB(255, 255, 255)).Children(func() {
							ui.Icon(c, iconDown).Size(18, 18).Rotate(180)
						})
					}
				})
				ui.Column(c).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, tr.Name).FontSize(13.5).FontWeight(600).SingleLine()
					sub := tr.ArtistNames()
					color := p.muted
					switch {
					case pl.loading:
						sub = "正在缓冲…"
					case pl.err != "":
						sub, color = pl.err, p.accent
					}
					ui.Text(c, sub).FontSize(12).TextColor(color).SingleLine()
				})
			})
			if info.Clicked() {
				a.nowPlaying = true
			}
			a.likeButton(c.Key("like"), tr, 16, p.muted).Margin(0, 0, 0, 4)
		})

		// Controls over the progress.
		ui.Column(c).Width(440).Shrink(1).Gap(0).AlignItems(ui.Stretch).Children(func() {
			a.transport(c, p.text, p.muted, p.accent, 18)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(-4, 0, 0, 0).Children(func() {
				ui.Text(c, clock(pl.position())).FontSize(10.5).FontFeatures("tnum").TextColor(p.faint).Width(34).TextAlign(ui.End)
				a.scrubber(c.Key("bar-scrub"), &a.barScrub, p.press, p.text.Alpha(0.75), 14).Grow(1)
				ui.Text(c, clock(pl.duration())).FontSize(10.5).FontFeatures("tnum").TextColor(p.faint).Width(34)
			})
		})

		// Lyrics and volume.
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).AlignItems(ui.Center).Gap(4).Children(func() {
			lyr := p.muted
			if a.nowPlaying {
				lyr = p.accent
			}
			if iconButton(c.Key("lyrics"), iconLyrics, 16, lyr, "歌词").Clicked() && tr != nil {
				a.nowPlaying, a.npPane = true, 0
			}
			if iconButton(c.Key("queue"), iconQueue, 16, p.muted, "播放队列").Clicked() && tr != nil {
				a.nowPlaying, a.npPane = true, 1
			}
			a.volumeSlider(c, p.text.Alpha(0.75), p.press, 92)
		})
	})
}
