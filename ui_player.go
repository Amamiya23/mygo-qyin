package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// scrubState is a progress bar's own: the value dragged, while it is, and
// the levels of the music its smoke follows.
type scrubState struct {
	v      float64
	active bool
	level  barState
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// scrubber shows how far the track is, and seeks where it is dragged or
// clicked: the bar thickens under the pointer, and its knob shows. With a
// smoke color, the bar is a comet while music plays: what has played is a
// streak brightening to the playhead, which glows and trails smoke that
// rises and fades, thicker as the music is louder.
func (a *app) scrubber(c *ui.Context, st *scrubState, track, fill ui.Color, height float32, smoke ui.Color) ui.Element {
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
	// The smoke settles away as the music pauses, and while dragging.
	smoking := float32(0)
	if smoke.A > 0 && a.settings.Smoke && pl.playing && !pl.loading && !st.active {
		smoking = 1
	}
	smoking = s.AnimateTo("smoke", smoking, ui.Animation{Duration: 600 * time.Millisecond})
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
		head, cy := r.X+r.W*frac, r.Y+r.H/2
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, track, thick/2)
		if dl := pl.stream; dl != nil {
			// What has downloaded, a shade lighter.
			if got := float32(dl.dl.Progress()); got < 1 {
				p.Fill(ui.Rect{X: r.X, Y: y, W: r.W * got, H: thick}, track, thick/2)
			}
		}
		played := ui.Rect{X: r.X, Y: y, W: max(thick, head-r.X), H: thick}
		if smoking > 0.01 {
			// A streak of light, brightest at the head.
			p.FillGradient(played, ui.LinearGradient{Angle: 90, Oklab: true,
				From: fill.Mix(smoke, smoking).Alpha(0.3), To: fill.Mix(smoke, 0.8*smoking)}, thick/2)
			lv, live := pl.levels()
			st.level.step(p.Now(), lv, live)
			bass := st.level.group(0, 6)
			drawSmoke(p, head, cy, p.Now(), bass, smoke, smoking)
			// The glow of the head, swelling with the bass, and its core.
			glow := 9 + 8*bass
			p.Shadow(ui.Rect{X: head, Y: cy}, 0, 0, 0, glow, 2.5+2.5*bass, smoke.Alpha(0.95*smoking))
			core := thick * (1 + 0.25*bass)
			p.Fill(ui.Rect{X: head - core/2, Y: cy - core/2, W: core, H: core}, white.Mix(smoke, 0.15).Alpha(smoking), core/2)
		} else {
			p.Fill(played, fill, thick/2)
		}
		if knob > 0.01 {
			k := 12 * knob
			p.Shadow(ui.Rect{X: head - k/2, Y: r.Y + (r.H-k)/2, W: k, H: k}, k/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.3))
			p.Fill(ui.Rect{X: head - k/2, Y: r.Y + (r.H-k)/2, W: k, H: k}, fill, k/2)
		}
		switch {
		case smoking > 0.01 && (pl.playing || smoking < 1):
			// The smoke drifts slowly, under a point a frame at 30 frames
			// a second, which show it smooth for a quarter of what a 120 Hz
			// display would draw.
			p.After(time.Second / 30)
		case pl.playing && !st.active && dur > 0:
			// Again when the fill has moved a pixel of the screen, which
			// takes a good part of a second, rather than every frame.
			perPixel := dur / float64(r.W*p.Scale())
			p.After(max(16*time.Millisecond, secs(perPixel)))
		}
	})
	return s
}

// Smoke from the playhead: wisps shed one after another, each drifting
// back along the bar and rising, growing and fading over its life. Where a
// wisp is follows from the time alone, so nothing is kept between frames.
const (
	smokeLife  = 1.4 // seconds
	smokeWisps = 36  // alive at once, at most
)

// drawSmoke draws the smoke of a playhead at (x, y) at now, thicker as
// loud (0 to 1) is higher, at opacity shown.
func drawSmoke(p *ui.Painter, x, y float32, now time.Time, loud float32, color ui.Color, shown float32) {
	t := float64(now.UnixMilli()) / 1000
	interval := smokeLife / smokeWisps
	first := math.Floor(t / interval)
	for j := range smokeWisps {
		k := first - float64(j)
		age := float32(t - k*interval)
		if age < 0 || age > smokeLife {
			continue
		}
		// A wisp of its own: how fast it drifts and rises, how it curls,
		// how large it grows. Quiet music sheds fewer of them.
		r := func(o float64) float32 { return lineRandom(k, o) }
		if r(5) > 0.6+0.4*loud {
			continue
		}
		f := age / smokeLife
		wx := x - age*(12+22*r(1))
		wy := y - age*(12+18*r(2)) + float32(math.Sin(float64(age)*(2.5+2*float64(r(3)))+float64(r(4))*2*math.Pi))*6*f
		size := 3 + f*(12+10*r(4))
		alpha := 0.5 * float32(math.Pow(float64(1-f), 1.4)) * shown * (0.75 + 0.5*loud)
		p.Shadow(ui.Rect{X: wx, Y: wy}, 0, 0, 0, size, size*0.45, color.Alpha(alpha))
	}
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
				a.scrubber(c.Key("bar-scrub"), &a.barScrub, p.press, p.text.Alpha(0.75), 14, p.accent).Grow(1)
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
				a.nowPlaying, a.npPane = true, a.lyricsPaneChosen()
			}
			if iconButton(c.Key("queue"), iconQueue, 16, p.muted, "播放队列").Clicked() && tr != nil {
				a.nowPlaying, a.npPane = true, paneQueue
			}
			a.volumeSlider(c, p.text.Alpha(0.75), p.press, 92)
		})
	})
}
