package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

var white = radixPaper

var npSpring = ui.SpringOptions{Duration: 340 * time.Millisecond}

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed. Call it once a
// build, before the window's content.
func (a *app) npState(c *ui.Context) (open, settled bool) {
	open = a.nowPlaying && a.pl.Current() != nil
	if open != a.npOpen {
		a.npOpen = open
		// The window buttons light over its dark backdrop, and as the
		// theme over the pages (Windows).
		if a.win != nil {
			buttons := mygo.ThemeSystem
			if open {
				buttons = mygo.ThemeDark
			}
			a.win.SetWindowButtonsTheme(buttons)
		}
	}
	return open, a.npAtRest(open)
}

// nowPlayingLayer grows Now Playing out of the bar's cover and shrinks it
// back there, keeping the whole card over both the page and the sidebar.
// A pull follows the pointer, closing when let go far or fast enough.
func (a *app) nowPlayingLayer(c *ui.Context, open, settled bool) {
	w, h := c.Size()
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	a.npStill = c.Preferences().ReduceMotion
	spring := npSpring
	_, dy, held := layer.Dragged()
	held = held && open
	if held {
		a.npPull += dy
	}
	if _, vy, ok := layer.DragEnded(); ok && open {
		if a.npPull > h*0.2 || a.npPull > 24 && vy > 700 {
			a.nowPlaying, open = false, false
			spring.Velocity = -max(vy, 0) / h
		}
	}
	if !held {
		a.npPull = 0
	}
	a.npHeld = held
	target := float32(0)
	if open {
		target = 1
	}
	var t, shift float32
	if a.npStill {
		// MyGo 0.7 keeps color animation under ReduceMotion: use its alpha
		// for a fade without depending on the newer NonMotion API.
		fade := layer.AnimateColor("fade", white.Alpha(target), ui.Animation{Duration: 200 * time.Millisecond})
		t = float32(fade.A) / 255
	} else {
		t = layer.AnimateTo("open", target, ui.Animation{Spring: spring})
	}
	if held {
		shift = layer.Jump("pull", ui.Rubberband(a.npPull, 0, h, h))
	} else {
		shift = layer.AnimateTo("pull", 0, ui.Animation{Spring: npSpring})
	}
	fresh, was := a.npT <= 0 && t > 0, a.npT
	a.npT, a.npShift = t, shift
	if a.npAtRest(open) != settled {
		c.AnimationFrame() // let the covered page and bar come or go
	}
	if t <= 0 && !open && was <= 0 || a.pl.Current() == nil {
		// Closed: nothing to paint or click.
		layer.Invisible()
		a.queueAt, a.wallState.track = -1, 0
		return
	}
	if fresh {
		// Lay out the cover before its first visible frame.
		layer.Opacity(0)
		c.AnimationFrame()
	} else if a.npStill {
		layer.Opacity(max(t, 0))
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.IgnoreInput(true)
	}
	layer.Children(func() {
		// A clipped viewport around a fixed-size canvas is ClipInset's
		// equivalent on MyGo 0.7; the content keeps its window layout.
		card := ui.Box(c.Key("card")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
		var x, y float32
		if !a.npStill {
			k := 1 - clamp01(t)
			from := a.npFromRect(w, h)
			x, y = from.X*k, from.Y*k
			card.Top(y).Left(x).Right((w - from.X - from.W) * k).Bottom((h - from.Y - from.H) * k).
				Radius(npFromRadius*k + 28*clamp01(shift/120)).Clip()
			if shift != 0 {
				s := 1 - 0.12*clamp01(shift/h)
				card.TransformOrigin(0.5, 0).Scale(s, s).Translate(0, shift)
			}
		}
		card.Children(func() {
			ui.Box(c.Key("canvas")).Absolute().Top(-y).Left(-x).Size(w, h).
				Children(func() { a.nowPlayingView(c) })
		})
	})
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (a *app) nowPlayingView(c *ui.Context) {
	pl := a.pl
	tr := pl.Current()
	// The covered player bar stops building once this layer settles, so
	// keep our time labels ticking independently of paint-only animations.
	a.tickTimes(c)
	w, h := c.Size()
	art := a.art.get(qqmusic.Sized(tr.Album.PicURL, 600), artBackdrop)
	// The colors stay the last song's while its cover loads, rather than
	// going gray between the two.
	shown := art
	if art == nil && tr.Album.PicURL != "" {
		shown = a.npArt
	}
	a.npArt = shown
	tint := radixSage(true, 9)
	if shown != nil && !shown.failed {
		tint = shown.tint
	}
	accent := tint.Mix(white, 0.45)

	bg := radixSage(true, 1)
	if t := a.npT; t < 1 && !a.npStill {
		bg = bg.Alpha(smoothstep(0, 0.35, t))
	}
	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(bg).TextColor(white)
	// Keep the controls visible while the pointer is in their detached effect menu.
	pointerIn := root.Hovered() || a.stageStyleOpen
	root.Children(func() {
		bloom := ui.Box(c.Key("bloom")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough()
		if t := a.npT; t < 1 && !a.npStill {
			from := a.npFromRect(w, h)
			s := 1 + 0.3*(1-clamp01(t))
			bloom.Opacity(smoothstep(0.05, 0.55, t)).
				TransformOrigin((from.X+from.W/2)/w, (from.Y+from.H/2)/h).Scale(s, s)
		}
		bloom.Children(func() { a.backdrop(c, shown, tint) })
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() { a.nowPlayingBody(c, tr, art, accent, w, h, pointerIn) })
	})
}

func (a *app) nowPlayingBody(c *ui.Context, tr *qqmusic.Track, art *artwork, accent ui.Color, w, h float32, pointerIn bool) {
	pl := a.pl
	a.commentsFor(tr)
	// An episode has what it is about and its captions in place of lyrics;
	// a song, lyrics in place of those.
	switch program := tr.ProgramID != 0; {
	case program && a.npPane == paneWall:
		a.npPane = paneQueue
	case program && (a.npPane == paneLyrics || a.npPane == paneStage):
		a.npPane = paneProgram
		if a.captionsOn() {
			a.npPane = paneCaptions
		}
	case program && a.npPane == paneCaptions && !a.captionsShown():
		a.npPane = paneProgram
	case !program && (a.npPane == paneProgram || a.npPane == paneCaptions):
		a.npPane = a.lyricsPaneChosen()
	}
	if a.npPane != paneQueue {
		a.queueAt = -1
	}
	if a.npPane != paneWall {
		a.wallState.track = 0
	}
	{
		// The top strip drags the window; the chevron closes the view,
		// showing only while the pointer is in the window, as the pane tabs do.
		a.npRise(ui.Row(c).Height(topBar(c)).Justify(ui.Center).AlignItems(ui.Center).DragWindow(), 0).Children(func() {
			reveal(ui.Box(c.Key("close-reveal")), pointerIn).Children(func() {
				if npCollapseButton(c.Key("close")).Clicked() {
					a.nowPlaying = false
				}
			})
		})
		if a.npPane == paneStage {
			// The stage takes the whole view: the lyrics are the show, the
			// controls a slim bar at the foot.
			a.npRise(reveal(npTabsRow(c, w, h), pointerIn), 0).Children(func() {
				a.paneTabs(c)
				a.stageStyleButton(c)
			})
			a.stage(c, tr, accent)
			a.stageBar(c, tr, art, accent)
			return
		}
		if a.npPane == paneWall {
			// The wall takes the whole view too, under the song playing.
			a.wallPane(c, tr, art, accent, w, h, pointerIn)
			return
		}

		coverSize := npCoverSize(w, h)
		// The cover and what plays start at one edge, with the column
		// wide enough for the controls even when the cover is small.
		side := max(coverSize, 280)
		padX := npSideX(w)
		a.npRise(reveal(npTabsRow(c, w, h), pointerIn), 0).Children(func() { a.paneTabs(c) })
		ui.Row(c).Grow(1).Padding(0, padX, 44, padX).Gap(72).AlignItems(ui.Stretch).Children(func() {
			var coverTop float32
			ui.Column(c).Width(side).Justify(ui.Center).Gap(0).Children(func() {
				coverTop = ui.Box(c.Key("cover-top")).Height(0).LayoutBounds().Y
				a.npMorph(c, ui.Box(c.Key("cover-morph")).Size(coverSize, coverSize).AlignSelf(ui.Start)).Children(func() {
					a.npCover(c, tr, art, coverSize)
				})

				a.npRise(ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(36, 0, 18, 0), 1).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
						if r := pl.Radio(); r != "" {
							a.radioChip(c, r)
						}
						ui.Text(c, tr.Name).FontSize(22).FontWeight(700).LetterSpacing(-0.3).SingleLine()
						a.artistLinks(c, tr, white.Alpha(0.6), white, true).FontSize(15).SingleLine()
					})
					a.likeButton(c.Key("like"), tr, 22, white.Alpha(0.7), mediaSurface)
					a.songMenu(c.Key("more"), tr, 22, white.Alpha(0.7), mediaSurface).Margin(0, -8, 0, 0)
				})
				a.npRise(a.scrubber(c.Key("np-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 16, accent.Mix(white, 0.35)), 2)
				// As tall with the quality's badge as without it: the badge goes
				// while the next song loads, and the column would move with it.
				a.npRise(ui.Row(c).Height(16).AlignItems(ui.Center), 2).Children(func() {
					ui.Text(c, clock(pl.Position())).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
					ui.Spacer(c)
					qualityBadge(c, pl.Stream())
					ui.Spacer(c)
					status := "-" + clock(pl.Duration()-pl.Position())
					if pl.Loading() {
						status = "正在缓冲…"
					}
					ui.Text(c, status).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				})
				a.npRise(ui.Box(c).Margin(14, 0, 18, 0), 3).Children(func() {
					a.transport(c, white, white.Alpha(0.45), accent, 28, mediaSurface)
				})
				a.npRise(ui.Row(c).Justify(ui.Center), 4).Children(func() {
					a.volumeSlider(c, white.Alpha(0.7), white.Alpha(0.18), 180, mediaSurface)
				})
			})

			pane := ui.Column(c.Key("pane")).Grow(1).Basis(0).MinWidth(0)
			a.npRise(pane, 2)
			pane.Children(func() {
				// The pane starts level with the cover beside it, but below the
				// tabs, which start level with the pane's top: they come and go
				// with the pointer, so they sit out of the flow, never over it.
				b := pane.LayoutBounds()
				if b.H > 0 && coverTop > 0 {
					if top := max(coverTop-b.Y, npTabsHeight+8); top != a.npPaneTop {
						a.npPaneTop = top
						c.AnimationFrame()
					}
				} else {
					c.AnimationFrame() // not laid out yet: the next frame places it
				}
				ui.Box(c).Height(a.npPaneTop)
				switch a.npPane {
				case paneLyrics:
					a.lyricsPane(c, tr, h)
				case paneComments:
					a.commentsPane(c, tr)
				case paneProgram:
					a.programPane(c, tr)
				case paneCaptions:
					a.captionsPane(c, tr, h)
				default:
					a.queuePane(c)
				}
			})
		})
	}
}

// npMaxWidth is how wide the cover and the pane beside it go, at most.
const npMaxWidth = 1280

// npCoverSize leaves room for the controls and the strip above the cover.
func npCoverSize(w, h float32) float32 {
	return min(float32(math.Min(float64(w)*0.36, 440)), h-436)
}

// npSideX is how far in from the sides of a view w wide the cover and the
// pane beside it are: in a wide window they keep together in the middle,
// rather than the pane taking all the width the cover leaves.
func npSideX(w float32) float32 {
	return max(72, (w-npMaxWidth)/2)
}

// npTabsHeight is how tall the row of Now Playing's tabs is, for what is
// under it to start below it.
const npTabsHeight = 28

// npCollapseButton uses translucent media controls over the cover's dark
// backdrop, independently of the appearance of the pages underneath it.
func npCollapseButton(c *ui.Context) ui.Element {
	b := ui.ButtonBase(c).Size(40, 32).Center().Radius(10).Label("收起播放页").Tooltip("收起播放页")
	mediaFeedback(b, radixWhite(10))
	b.Children(func() { ui.Icon(c, iconDown).Size(20, 20) })
	return b
}

// npTabsRow is the row of Now Playing's tabs, in the same place for every
// pane, above the right-hand content. Anchoring their start keeps them
// still as the comments' count changes or the stage's style button appears.
func npTabsRow(c *ui.Context, w, h float32) ui.Element {
	left := npSideX(w) + max(npCoverSize(w, h), 280) + 72
	return ui.Row(c.Key("tabs")).Absolute().Top(topBar(c)).Left(left).Height(npTabsHeight).Gap(6).AlignItems(ui.Center)
}

// reveal fades e in while the pointer is in the window, and out when it
// leaves.
func reveal(e ui.Element, shown bool) ui.Element {
	pose := ui.Pose{Fade: 1}
	if shown {
		pose = ui.Pose{}
	}
	return e.AnimatePose(pose, ui.Animation{Duration: 200 * time.Millisecond})
}

// lyricsPaneChosen is the pane of lyrics the listener chose last: what
// the app starts on, before the player is made. Now Playing shows an
// episode's in place of either.
func (a *app) lyricsPaneChosen() int {
	if a.settings.LyricsStage {
		return paneStage
	}
	return paneLyrics
}

// The panes of Now Playing.
const (
	paneLyrics = iota
	paneQueue
	paneStage
	paneComments
	paneProgram
	paneCaptions
	paneWall
)

// npTab is a tab of Now Playing: its pane, and what it says.
type npTab struct {
	pane  int
	label string
}

// npTabs returns the tabs of Now Playing, in order, for what plays: ⌘1
// chooses the first, ⌘2 the second, and on.
func (a *app) npTabs() []npTab {
	if tr := a.pl.Current(); tr != nil && tr.ProgramID != 0 {
		tabs := []npTab{{paneProgram, "简介"}}
		if a.captionsShown() {
			tabs = append(tabs, npTab{paneCaptions, "字幕"})
		}
		return append(tabs, npTab{paneQueue, "待播清单"}, npTab{paneComments, "评论"})
	}
	return []npTab{{paneLyrics, "歌词"}, {paneStage, "律动"}, {paneQueue, "待播清单"}, {paneWall, "选歌"}, {paneComments, "评论"}}
}

func (a *app) paneTabs(c *ui.Context) {
	for _, t := range a.npTabs() {
		label := t.label
		if t.pane == paneComments {
			label = a.commentsTab()
		}
		a.paneTab(c, t.pane, label)
	}
}

// choosePane shows pane i in Now Playing, remembering lyrics or the stage,
// whichever was chosen, for the next song.
func (a *app) choosePane(i int) {
	a.npPane = i
	if (i == paneLyrics || i == paneStage) && a.settings.LyricsStage != (i == paneStage) {
		a.settings.LyricsStage = i == paneStage
		a.settings.save()
	}
}

// radioChip says which radio plays, over the title: a click on it, or on
// its cross, leaves the radio, playing on the song.
func (a *app) radioChip(c *ui.Context, name string) {
	icon, exit := iconRadio, "退出电台"
	if name == heartRadio {
		icon, exit = iconHeartFill, "退出心动模式"
	}
	chip := ui.ButtonBase(c.Key("radio-chip")).Gap(5).Padding(3, 6, 3, 8).Margin(0, 0, 4, -8).Radius(999).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	bg, fg, cross := ui.Transparent, white.Alpha(0.6), float32(0.5)
	if chip.Hovered() || chip.FocusVisible() {
		bg, fg, cross = radixWhite(2), radixWhite(12), 1
	}
	if chip.Pressed() {
		bg = radixWhite(3)
	}
	motion := ui.Animation{Duration: 140 * time.Millisecond}
	chip.Background(chip.AnimateColor("bg", bg, motion)).TextColor(chip.AnimateColor("fg", fg, motion))
	cross = chip.AnimateTo("cross", cross, motion)
	chip.Children(func() {
		ui.Icon(c, icon).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(11.5).FontWeight(700).LetterSpacing(0.3)
		ui.Icon(c, iconX).Size(11, 11).Opacity(cross)
	})
	if chip.Clicked() {
		a.exitRadio()
	}
}

// qualityBadge marks a lossless stream, as it plays.
func qualityBadge(c *ui.Context, st *qqmusic.Stream) {
	if st == nil || !qqmusic.Lossless(st.Level) {
		return
	}
	label := "无损"
	if st.Level == "hires" {
		label = "Hi-Res"
	}
	ui.Text(c, label).FontSize(9.5).FontWeight(800).LetterSpacing(0.6).TextColor(radixAmber(true, 11)).
		Padding(1, 6).Radius(4).Border(1, radixAmber(true, 8))
}

// stageBar is the slim bar along the foot of the stage: what plays, where
// it is, and the controls.
func (a *app) stageBar(c *ui.Context, tr *qqmusic.Track, art *artwork, accent ui.Color) {
	pl := a.pl
	ui.Column(c).Padding(0, 48, 28, 48).Gap(10).Children(func() {
		a.npRise(a.scrubber(c.Key("stage-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 14, accent.Mix(white, 0.35)), 2)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				if bmp := bitmap(art); bmp != nil {
					cov := a.npMorph(c, ui.Box(c.Key(tr.ID)).Size(44, 44))
					in := artIn(cov, bmp)
					cov.Children(func() { artImage(c, bmp, a.npRadius(c, 8, 44), in) })
				}
				a.npRise(ui.Row(c).Shrink(1).MinWidth(0).Gap(12).AlignItems(ui.Center), 3).Children(func() {
					ui.Column(c).Shrink(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, tr.Name).FontSize(14).FontWeight(700).SingleLine()
						a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12).SingleLine()
					})
					a.likeButton(c.Key("like"), tr, 17, white.Alpha(0.6), mediaSurface)
					a.songMenu(c.Key("more"), tr, 17, white.Alpha(0.6), mediaSurface).Margin(0, 0, 0, -10)
				})
			})
			a.npRise(ui.Row(c), 3).Children(func() { a.transport(c, white, white.Alpha(0.45), white, 22, mediaSurface) })
			a.npRise(ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End), 4).Children(func() {
				ui.Text(c, clock(pl.Position())+" / "+clock(pl.Duration())).FontSize(11.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
		})
	})
}

func (a *app) paneTab(c *ui.Context, i int, label string) {
	b := ui.ButtonBase(c.Key(i)).Padding(6, 14).Radius(999).FocusRing(false).Label(label)
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if a.npPane == i {
		bg, fg = radixWhite(3), white
	}
	if b.Hovered() || b.FocusVisible() {
		bg, fg = radixWhite(2), radixWhite(12)
		if a.npPane == i {
			bg = radixWhite(4)
		}
	}
	if b.Pressed() {
		bg, fg = radixWhite(5), white
	}
	motion := ui.Animation{Duration: 140 * time.Millisecond}
	b.Background(b.AnimateColor("bg", bg, motion))
	fg = b.AnimateColor("fg", fg, motion)
	b.Children(func() {
		if i != paneComments {
			ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg)
			return
		}
		// The count rolls as it changes, from one song's to the next.
		n, text := a.commentCount(), ""
		if n > 0 {
			text = " " + count(n)
		}
		ui.Text(c, "评论").FontSize(13).FontWeight(600).TextColor(fg)
		rollCount(c, &a.tabCount, n, text, fg)
	})
	if b.Clicked() {
		a.choosePane(i)
	}
}

func (a *app) lyricsPane(c *ui.Context, tr *qqmusic.Track, h float32) {
	pl := a.pl
	lines, loaded := pl.Lyrics()
	if len(lines) == 0 {
		col := ui.Column(c).Grow(1)
		if loaded {
			col.Center()
		}
		dim, _ := a.quietShown(c, col, tr, lines, loaded)
		quietUnder(col, dim)
		col.Children(func() {
			if !loaded {
				lyricsSkeleton(c, npShimmer(c))
				return
			}
			ui.Text(c, "纯音乐，请欣赏").FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})
		a.quietCard(c, dim, h*0.18)
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	// Redraw as the next line begins.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos)
	}
	if a.lyricTrack != tr.ID {
		a.lyricTrack, a.lyricAt = tr.ID, -2
		a.lyricScroll, a.lyricGlide = ui.ScrollState{}, lyricGlide{}
	}
	lag := a.lyricGlide.step(c, &a.lyricScroll)

	scroll := ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, h*0.5, 0)
	dim, quiet := a.quietShown(c, scroll, tr, lines, loaded)
	if dim > 0.98 {
		// Under the hot comment, gone, not drawn faint.
		scroll.Invisible()
	}
	defer a.quietCard(c, dim, h*0.18)
	a.timedLines(c, scroll, lines, pos, cur, lag, linesStyle{size: 30, weight: 800, lineHeight: 1.25, quiet: quiet}, nil)
}

// linesStyle is how timedLines sets its lines: the size, weight and
// leading of their type; quiet nearly hides them, under a hot comment.
type linesStyle struct {
	size       float32
	weight     int
	lineHeight float32
	quiet      bool
}

// timedLines fills scroll with lines that follow the track playing at
// pos: the line at cur lit, and brought to a third of the way down once
// it changes, but none with cur -1 before the first; a click on one seeks
// to it. The lines are held back by lag, how far the glide to the line
// lit has still to go (lyricGlide). foot, if set, follows the lines.
func (a *app) timedLines(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, cur int, lag float32, st linesStyle, foot func()) {
	pl := a.pl
	var active ui.Element
	scroll.Children(func() {
		ui.Column(c).Gap(6).Translate(0, -lag).Children(func() {
			for i, l := range lines {
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
				on := i == cur
				alpha := float32(0.28)
				if on {
					alpha = 1
				} else if line.Hovered() {
					alpha = 0.55
				}
				if st.quiet {
					// Nearly gone under a hot comment.
					alpha *= quietFaded
				}
				if line.Hovered() {
					line.Background(white.Alpha(0.06))
				}
				scale := float32(0.94)
				if on {
					scale = 1
				}
				line.TransformOrigin(0, 0.5).AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}})
				line.Children(func() {
					// The line fades as a pose, in frames that only paint, rather
					// than building the view at the display's rate as it does.
					text := ui.Column(c).AlignItems(ui.Start).Gap(6)
					text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
					text.Children(func() {
						if on && l.Timed && len(l.Words) > 0 {
							// The line sung lights up word by word as it is sung.
							ui.RichText(c.Key("words")).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).Children(func() {
								for wi, wd := range l.Words {
									lit := wd.Progress(pos)
									ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
								}
							})
							if pl.Playing() {
								// Once a word ends, sleep until another starts instead of
								// rebuilding unchanged lyrics through a gap or the outro.
								if next := desktopLyricWake(desktopLyricContent{index: cur, line: l}, pos); next > 0 {
									c.After(next)
								}
							}
						} else {
							ui.Text(c, l.Text).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(white)
						}
						if l.Translation != "" {
							ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7))
						}
					})
				})
				if line.Clicked() {
					pl.Seek(l.At)
				}
				if on {
					active = line
				}
			}
		})
		if foot != nil {
			foot()
		}
	})
	// Bring the line sung to a third of the way down, once it changed.
	if cur != a.lyricAt {
		box := scroll.Bounds()
		target := float32(0)
		if active.Valid() {
			// Where the glide is going, without where it still is.
			if lb := active.LayoutBounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + a.lyricScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		a.lyricAt = cur
		a.lyricGlide.to(c, &a.lyricScroll, min(max(0, target), a.lyricScroll.MaxY))
	}
}

// lyricGlide scrolls the lyrics to the line sung, as a critically damped
// spring, as ScrollTo's would. ScrollTo moves the offset in every frame,
// and MyGo builds the view again in the frame after each that moved it,
// and its spring creeps over the last pixel for half a second: with a line
// following another every second or two, most of a song. So the offset
// goes to the line at once, in one frame, and the lines are held back by
// what the glide has still to go, its lag, which settles to nothing within
// half a pixel.
type lyricGlide struct {
	on bool
	// The lag t after start: (a + b·t)·e^(−ωt), in DIPs.
	a, b  float64
	start time.Time
	// The offset it goes to, set in the next frame (pending), with the lag
	// that holds the lines back from it: the layout of the frame that
	// chose it takes the offset too, the lines built without the lag.
	target  float32
	pending bool
	set     float32 // the offset it set
}

// lyricOmega is the spring's ω: a line or two away, it comes within half a
// pixel in about 800 ms.
const lyricOmega = 10

// at returns the lag at now, and how fast it changes, in DIPs a second.
func (g *lyricGlide) at(now time.Time) (lag, v float64) {
	if !g.on {
		return 0, 0
	}
	t := now.Sub(g.start).Seconds()
	e := math.Exp(-lyricOmega * t)
	return (g.a + g.b*t) * e, (g.b - lyricOmega*(g.a+g.b*t)) * e
}

// to scrolls s to target from the next frame, gliding from where it
// shows, at the speed it went if it was on its way.
func (g *lyricGlide) to(c *ui.Context, s *ui.ScrollState, target float32) {
	now := c.Now()
	lag, v := g.at(now)
	shown := float64(s.Y) + lag
	g.target, g.pending = target, true
	g.on = !c.Preferences().ReduceMotion
	g.start, g.a = now, shown-float64(target)
	g.b = v + lyricOmega*g.a
	c.AnimationFrame()
}

// step returns the lag of the lines in this frame, and asks for the next
// while there is one. The listener scrolling stops the glide where it
// shows.
func (g *lyricGlide) step(c *ui.Context, s *ui.ScrollState) float32 {
	if g.pending {
		s.Y, g.set, g.pending = g.target, g.target, false
	}
	if !g.on {
		return 0
	}
	lag, v := g.at(c.Now())
	if s.Y != g.set {
		// The listener took over: the offset takes what showed.
		s.Y, g.on = max(0, min(s.Y+float32(lag), s.MaxY)), false
		return 0
	}
	if math.Abs(lag) < 0.5 && math.Abs(v) < 20 {
		g.on = false
		return 0
	}
	c.AnimationFrame()
	return float32(lag)
}

// queuePane lists the whole queue in play order, the track playing marked
// among the others: a click plays one there, and the list stays as it is.
func (a *app) queuePane(c *ui.Context) {
	a.queueTracks(c, &a.queueList, &a.queueAt, palette{
		text: white, muted: white.Alpha(0.55), faint: white.Alpha(0.4),
		hover: white.Alpha(0.08), press: white.Alpha(0.12),
	}).Padding(8, 0, 40, 0)
}

// queueTracks shares playback and menus between the page and its floating queue.
func (a *app) queueTracks(c *ui.Context, list *ui.ListState, lastAt *int, p palette) ui.Element {
	pl := a.pl
	order, at := pl.PlayOrder()
	// Opened, it shows the track playing at its top; after, it follows it
	// as little as keeps it in view.
	if at >= 0 && at != *lastAt {
		if *lastAt < 0 {
			list.ScrollTo(at, ui.Start)
		} else {
			list.ScrollIntoView(at)
		}
		*lastAt = at
	}
	list.Key = func(row int) any { return order[row] }
	return ui.List(c.Key("queue"), list, len(order), func(r int) {
		i := order[r]
		tr := &pl.Queue()[i]
		playing := r == at
		row := ui.ButtonBase(c.Key(i)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(a.songItems(tr, 0))
		switch {
		case playing:
			row.Background(p.press)
		case row.Hovered():
			row.Background(p.hover)
		}
		if !tr.Playable() {
			row.Opacity(0.4)
		}
		row.Children(func() {
			a.cover(c, tr.Album.PicURL, 96, 42, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
				a.artistLinks(c, tr, p.muted, p.text).FontSize(12.5).SingleLine()
			})
			ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(p.faint)
		})
		if row.Clicked() && tr.Playable() {
			if playing {
				pl.TogglePlay()
			} else {
				pl.Jump(i)
			}
		}
	}).Grow(1).MinHeight(0).TextColor(p.text).Gap(2)
}
