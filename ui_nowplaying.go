package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

var white = ui.RGB(255, 255, 255)

var npMotion = ui.ElementTransition{
	Animation: &ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond, Bounce: 0.08}},
	Enter:     &ui.Motion{Y: 60, Opacity: 0},
	Exit:      &ui.Motion{Y: 60, Opacity: 0},
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (a *app) nowPlayingView(c *ui.Context) {
	pl := a.pl
	tr := pl.current()
	w, h := c.Size()
	art := a.art.get(sized(tr.Album.PicURL, 600), true)
	tint := ui.RGB(120, 120, 130)
	if art != nil && !art.failed {
		tint = art.tint
	}
	accent := tint.Mix(white, 0.45)

	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.Hex("#0b0b0d")).Clip().Transition(npMotion).TextColor(white)
	root.Children(func() {
		a.backdrop(c, art, tint)
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() { a.nowPlayingBody(c, tr, art, accent, w, h) })
	})
}

func (a *app) nowPlayingBody(c *ui.Context, tr *Track, art *artwork, accent ui.Color, w, h float32) {
	pl := a.pl
	bar := c.TitleBar()
	{
		// The top strip drags the window; the chevron closes the view.
		ui.Row(c).Height(max(bar.Height, 28) + 20).Justify(ui.Center).AlignItems(ui.Center).DragWindow().Children(func() {
			if iconButton(c.Key("close"), iconDown, 22, white.Alpha(0.7), "关闭").Clicked() {
				a.nowPlaying = false
			}
		})

		side := float32(math.Min(float64(w)*0.36, 440))
		coverSize := min(side, h-360)
		ui.Row(c).Grow(1).Padding(0, 72, 44, 72).Gap(72).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(side).Justify(ui.Center).Gap(0).Children(func() {
				// The cover breathes: smaller while paused.
				scale := float32(1)
				if !pl.playing {
					scale = 0.88
				}
				box := ui.Box(c.Key(tr.ID)).Size(coverSize, coverSize).AlignSelf(ui.Center).Radius(16).
					Shadow(0, 30, 60, -12, ui.RGBA(0, 0, 0, 0.55))
				box.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
				box.Children(func() {
					if art != nil && art.bmp != nil {
						ui.Image(c, art.bmp).Fill().Fit(ui.Cover).Radius(16)
					} else {
						ui.Box(c).Fill().Radius(16).Background(white.Alpha(0.08))
					}
				})

				ui.Column(c).Gap(4).Margin(36, 0, 18, 0).Children(func() {
					ui.Text(c, tr.Name).FontSize(22).FontWeight(700).LetterSpacing(-0.3).SingleLine()
					ui.Text(c, tr.ArtistNames()+" — "+tr.Album.Name).FontSize(15).TextColor(white.Alpha(0.6)).SingleLine()
				})
				a.scrubber(c.Key("np-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 16)
				ui.Row(c).Children(func() {
					ui.Text(c, clock(pl.position())).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
					ui.Spacer(c)
					status := "-" + clock(pl.duration()-pl.position())
					if pl.loading {
						status = "正在缓冲…"
					}
					ui.Text(c, status).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				})
				ui.Box(c).Margin(14, 0, 18, 0).Children(func() {
					a.transport(c, white, white.Alpha(0.45), accent, 28)
				})
				ui.Row(c).Justify(ui.Center).Children(func() {
					a.volumeSlider(c, white.Alpha(0.7), white.Alpha(0.18), 180)
				})
			})

			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				ui.Row(c).Gap(6).Margin(0, 0, 8, 0).Children(func() {
					a.paneTab(c, 0, "歌词")
					a.paneTab(c, 1, "待播清单")
				})
				if a.npPane == 0 {
					a.lyricsPane(c, tr, h)
				} else {
					a.queuePane(c)
				}
			})
		})
	}
}

func (a *app) paneTab(c *ui.Context, i int, label string) {
	b := ui.ButtonBase(c.Key(label)).Padding(6, 14).Radius(999).FocusRing(false)
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if a.npPane == i {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		bg = white.Alpha(0.06)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() { ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg) })
	if b.Clicked() {
		a.npPane = i
	}
}

// backdrop paints the cover, blurred, in two layers turning slowly against
// each other, darkened so what is over it reads.
func (a *app) backdrop(c *ui.Context, art *artwork, tint ui.Color) {
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, tint.Mix(ui.RGB(0, 0, 0), 0.55), 0)
		if art == nil || art.backdrop == nil {
			return
		}
		t := float64(p.Now().UnixMilli()%240000) / 240000 * 2 * math.Pi
		side := float32(math.Hypot(float64(r.W), float64(r.H))) * 1.25
		layer := func(angle float32, scale float32, dx, dy float32, alpha float32) {
			s := side * scale
			cx, cy := r.X+r.W/2+dx, r.Y+r.H/2+dy
			p.Transform(ui.Rotation(angle), cx, cy, func() {
				p.ImageRegion(art.backdrop, ui.Rect{W: 48, H: 48}, ui.Rect{X: cx - s/2, Y: cy - s/2, W: s, H: s},
					ui.ImageOptions{Tint: white.Alpha(alpha)})
			})
		}
		layer(float32(t*180/math.Pi), 1, 0, 0, 1)
		layer(-float32(t*180/math.Pi)*1.5+90, 0.8, r.W*0.18*float32(math.Cos(t*2)), r.H*0.15*float32(math.Sin(t*3)), 0.55)
		// Darker toward the edges and the foot, where the controls are.
		p.Fill(r, ui.RGBA(0, 0, 0, 0.32), 0)
		p.FillGradient(r, ui.LinearGradient{
			Angle: 180,
			Stops: []ui.GradientStop{{Color: ui.RGBA(0, 0, 0, 0), At: 0.4}, {Color: ui.RGBA(0, 0, 0, 0.35), At: 1}},
		}, 0)
		// The drift is slow: a frame every 50 ms shows it smooth.
		p.After(50 * time.Millisecond)
	})
}

func (a *app) lyricsPane(c *ui.Context, tr *Track, h float32) {
	pl := a.pl
	lines := pl.lyrics
	if len(lines) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			if !pl.lyricsLoaded {
				ui.Spinner(c)
				return
			}
			ui.Text(c, "纯音乐，请欣赏").FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})
		return
	}
	pos := pl.position()
	cur := lyricIndex(lines, pos)
	// Redraw as the next line begins.
	if pl.playing && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if a.lyricTrack != tr.ID {
		a.lyricTrack, a.lyricAt = tr.ID, -2
		a.lyricScroll = ui.ScrollState{}
	}

	scroll := ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(h*0.22, 8, h*0.5, 0).Gap(6)
	var active ui.Element
	scroll.Children(func() {
		for i, l := range lines {
			line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
			on := i == cur
			alpha := float32(0.28)
			if on {
				alpha = 1
			} else if line.Hovered() {
				alpha = 0.55
			}
			alpha = line.AnimateTo("alpha", alpha, ui.Animation{Duration: 380 * time.Millisecond})
			if line.Hovered() {
				line.Background(white.Alpha(0.06))
			}
			scale := float32(0.94)
			if on {
				scale = 1
			}
			line.TransformOrigin(0, 0.5).AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}})
			line.Children(func() {
				ui.Text(c, l.Text).FontSize(30).FontWeight(800).LineHeight(1.25).LetterSpacing(-0.4).TextColor(white.Alpha(alpha))
				if l.Translation != "" {
					ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(alpha * 0.7))
				}
			})
			if line.Clicked() {
				pl.seek(l.At)
			}
			if on {
				active = line
			}
		}
	})
	// Bring the line sung to a third of the way down, once it changed.
	if cur != a.lyricAt {
		box := scroll.Bounds()
		target := float32(0)
		if active.Valid() {
			if lb := active.Bounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + a.lyricScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		a.lyricAt = cur
		a.lyricScroll.ScrollTo(0, max(0, target), ui.Animation{Spring: ui.SpringOptions{Duration: 800 * time.Millisecond}})
	}
}

func (a *app) queuePane(c *ui.Context) {
	pl := a.pl
	next := pl.upNext(80)
	if len(next) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "播放队列已到末尾").FontSize(15).TextColor(white.Alpha(0.5))
		})
		return
	}
	ui.Scroll(c.Key("queue")).Grow(1).Padding(8, 0, 40, 0).Gap(2).Children(func() {
		for _, i := range next {
			tr := &pl.queue[i]
			row := ui.ButtonBase(c.Key(tr.ID)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
			if row.Hovered() {
				row.Background(white.Alpha(0.08))
			}
			if !tr.Playable() {
				row.Opacity(0.4)
			}
			row.Children(func() {
				a.cover(c, tr.Album.PicURL, 96, 42, 7)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
					ui.Text(c, tr.ArtistNames()).FontSize(12.5).TextColor(white.Alpha(0.55)).SingleLine()
				})
				ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.4))
			})
			if row.Clicked() && tr.Playable() {
				i := i
				row.OnClick(func() { pl.jump(i) })
			}
		}
	})
}
