package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const sidebarWidth = 232

func (a *app) view(c *ui.Context) {
	t := *c.Theme()
	a.pal = paletteFor(&t)
	p := a.pal
	t.Accent, t.AccentText = p.accent, p.onAccent
	t.Selection = p.accent.Alpha(0.3)
	c.SetTheme(&t)

	a.shortcuts(c)

	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	root := ui.Box(c).Fill()
	if !c.Focused(a.searchBox) {
		// Space plays and pauses wherever the focus is, as in Music. On
		// the container around everything, it comes before a button the
		// pointer focused, which would take Space as a click; on the
		// window, it would come after.
		root.OnShortcut(0, ui.KeySpace, a.pl.togglePlay)
		// And where nothing has the focus, as once Now Playing covers the
		// button that had it.
		c.OnShortcut(0, ui.KeySpace, a.pl.togglePlay)
	}
	npOpen, npSettled := a.npState(c)
	root.Children(func() {
		page := ui.Row(c).Fill().AlignItems(ui.Stretch)
		if npOpen && npSettled {
			// Covered by Now Playing: kept, with its scrolling and state, but
			// not painted, so that nothing in it asks for frames.
			page.Invisible()
		}
		page.Children(func() {
			side := ui.Column(c).Width(sidebarWidth)
			if !c.Vibrancy() {
				side.Background(p.sidebar)
			}
			side.Children(func() { a.sidebar(c) })
			ui.Box(c).Grow(1).Background(p.bg).BorderWidth(0, 0, 0, 1).BorderColor(p.hairline).Children(func() {
				a.content(c)
				// The bars float over the page, which scrolls under them:
				// positioned, they paint over it.
				a.pageTop(c)
				a.playerBar(c)
			})
		})
		a.nowPlayingLayer(c, npOpen, npSettled)
	})
	a.loginDialog(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.syncWindows()
}

func (a *app) shortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyF, func() {
		a.nowPlaying = false
		a.searchBox.Focus(c)
	})
	if a.nowPlaying {
		c.OnShortcut(0, ui.KeyEscape, func() { a.nowPlaying = false })
	}
}

// go opens a page, remembering the one before for the back button.
func (a *app) open(r route) {
	if r == a.route {
		return
	}
	a.history = append(a.history, a.route)
	a.route = r
	a.nowPlaying = false
	if r.page == pagePlaylist {
		a.trackList = ui.ListState{}
		a.loadPlaylist(r.playlist, false)
	}
}

func (a *app) back() {
	if len(a.history) == 0 {
		return
	}
	a.route = a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.trackList = ui.ListState{}
}

func (a *app) sidebar(c *ui.Context) {
	p := a.pal
	bar := c.TitleBar()
	ui.Column(c).Fill().Padding(max(bar.Height, 28)+14, 14, 14, 14).Gap(4).Children(func() {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(max(bar.Height, 28) + 8).DragWindow()

		// The search field.
		field := ui.Row(c).Height(32).PaddingX(10).Gap(7).Radius(9).AlignItems(ui.Center).Margin(0, 0, 14, 0)
		focused := c.Focused(a.searchBox)
		if focused {
			field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
		} else {
			field.Background(p.hover).Border(1, p.hairline)
		}
		field.Children(func() {
			ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.muted)
			in := ui.TextInputBase(c.Key("search"), &a.query).Bind(&a.searchBox).Grow(1).
				Placeholder("搜索歌曲、歌手").FontSize(13).Label("搜索")
			if in.Submitted() && strings.TrimSpace(a.query) != "" {
				a.search(strings.TrimSpace(a.query))
			}
		})

		ui.Scroll(c.Key("nav")).Grow(1).Margin(0, -6).Padding(0, 6).Gap(4).Children(func() {
			a.navItem(c, iconHome, "为你推荐", route{page: pageHome})
			a.navItem(c, iconDaily, "每日推荐", route{page: pageDaily})
			a.navItem(c, iconCharts, "排行榜", route{page: pageCharts})
			if a.searched != "" {
				a.navItem(c, iconSearch, "搜索结果", route{page: pageSearch})
			}
			a.myPlaylists(c, "我的歌单", a.mine)
			a.myPlaylists(c, "收藏的歌单", a.saved)
		})

		// What plays, as a card at the foot of the sidebar, over who listens.
		if tr := a.pl.current(); tr != nil {
			a.sidebarNowPlaying(c, tr)
		}
		a.accountRow(c)
	})
}

func (a *app) navItem(c *ui.Context, icon *ui.SVG, label string, r route) {
	p := a.pal
	selected := a.route.page == r.page && !a.nowPlaying
	b := ui.ButtonBase(c.Key(label)).Height(34).PaddingX(10).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	bg := ui.Transparent
	switch {
	case selected:
		bg = p.press
	case b.Hovered():
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	color := p.text
	if selected {
		color = p.accent
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(color)
		ui.Text(c, label).FontSize(13.5).FontWeight(500).TextColor(p.text)
	})
	if b.Clicked() {
		a.open(r)
	}
}

func (a *app) sidebarNowPlaying(c *ui.Context, tr *Track) {
	p := a.pal
	card := ui.ButtonBase(c.Key("np-card")).Gap(10).Padding(8).Radius(12).AlignItems(ui.Center).FocusRing(false).Label("正在播放")
	bg := p.hover
	if card.Hovered() {
		bg = p.press
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	card.Children(func() {
		a.cover(c, tr.Album.PicURL, 96, 40, 6).Shadow(0, 2, 6, 0, p.cardShade)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, tr.Name).FontSize(12.5).FontWeight(600).SingleLine()
			ui.Text(c, tr.ArtistNames()).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
	})
	if card.Clicked() {
		a.nowPlaying = true
	}
}

// playerBarHeight is the height of the player along the foot of the pages.
const playerBarHeight = 84

// pageGap is the room between the bar over a page and its content.
const pageGap = 12

// insets returns the room the bars over a page take at its top and its
// bottom: pages pad their content by it, and scroll under the bars.
func (a *app) insets(c *ui.Context) (top, bottom float32) {
	return max(c.TitleBar().Height, 28) + 16, playerBarHeight
}

// startMark marks where a page's content starts, first in what scrolls:
// once it has moved up, content is under the bar at the top.
func (a *app) startMark(c *ui.Context) {
	b := ui.Box(c.Key("start-mark")).Height(0).Bounds()
	if b.W > 0 {
		top, _ := a.insets(c)
		a.underTop = b.Y < top+pageGap-0.5
	}
}

// endMark marks where a page's content ends, last in what scrolls: while it
// is below the top of the player, content is under the player.
func (a *app) endMark(c *ui.Context) {
	b := ui.Box(c.Key("end-mark")).Height(0).Bounds()
	if b.W > 0 {
		_, h := c.Size()
		_, bottom := a.insets(c)
		a.underBottom = b.Y > h-bottom+0.5
	}
}

// edgeShown returns how much of a scroll edge shows over el: fully while
// content is under its bar, and none at rest, as AppKit shows its edges.
func edgeShown(el ui.Element, under bool) float32 {
	target := float32(0)
	if under {
		target = 1
	}
	return el.AnimateTo("shown", target, ui.Animation{Duration: 180 * time.Millisecond})
}

// pageTop is the strip over the top of a page, frosting what scrolls under
// it as macOS 26's hard scroll edge does: it drags the window, and holds
// the back button once there is a page to go back to.
func (a *app) pageTop(c *ui.Context) {
	top, _ := a.insets(c)
	edge := ui.Box(c.Key("top-edge")).Absolute().Top(0).Left(0).Right(0).Height(top).DragWindow().
		Material(glass.ScrollEdge{Hard: true, Background: a.pal.bg})
	edge.Opacity(edgeShown(edge, a.underTop))
	if len(a.history) > 0 {
		ui.Row(c).Absolute().Top(0).Left(20).Height(top).AlignItems(ui.Center).Children(func() {
			if iconButton(c.Key("back"), iconBack, 18, a.pal.muted, "返回").Clicked() {
				a.back()
			}
		})
	}
}

func (a *app) content(c *ui.Context) {
	// Pages mark where their content is, as they scroll.
	a.underTop, a.underBottom = false, false
	switch a.route.page {
	case pageHome:
		a.homePage(c)
	case pageDaily:
		a.dailyPage(c)
	case pageCharts:
		a.chartsPage(c)
	case pagePlaylist:
		a.playlistPage(c)
	case pageSearch:
		a.searchPage(c)
	}
}

// centered shows a spinner, or a message with a retry button.
func (a *app) centered(c *ui.Context, loading bool, err string, retry func()) {
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center().Gap(12).Children(func() {
		if loading {
			ui.Spinner(c)
			return
		}
		ui.Text(c, err).TextColor(a.pal.muted).FontSize(13)
		if retry != nil && ui.Button(c, "重试").Clicked() {
			retry()
		}
	})
}

// Loading what the pages show.

func (a *app) loadHome() {
	a.homeLoading, a.homeErr = true, ""
	signedIn := a.profile != nil
	go func() {
		featured, err1 := FeaturedPlaylists(30)
		charts, err2 := Charts()
		daily, _ := DailySongs()
		var rec []Playlist
		if signedIn {
			rec, _ = RecommendedPlaylists()
		}
		a.post(func() {
			a.homeLoading = false
			a.featured, a.charts, a.daily, a.recPlaylists = featured, charts, daily, rec
			if err1 != nil && err2 != nil {
				a.homeErr = "无法连接网易云音乐，请检查网络"
			}
		})
	}()
}

func (a *app) loadPlaylist(id int64, thenPlay bool) {
	if pl := a.playlists[id]; pl != nil {
		if thenPlay {
			a.pl.playQueue(pl.Tracks, firstPlayable(pl.Tracks))
		}
		return
	}
	if a.loadingPl[id] {
		return
	}
	a.loadingPl[id] = true
	delete(a.plErr, id)
	go func() {
		pl, err := PlaylistDetail(id)
		a.post(func() {
			delete(a.loadingPl, id)
			if err != nil {
				a.plErr[id] = "歌单加载失败"
				return
			}
			a.playlists[id] = pl
			if thenPlay {
				a.pl.playQueue(pl.Tracks, firstPlayable(pl.Tracks))
			}
		})
	}()
}

func firstPlayable(tracks []Track) int {
	for i := range tracks {
		if tracks[i].Playable() {
			return i
		}
	}
	return 0
}

func (a *app) search(q string) {
	a.history = append(a.history, a.route)
	a.route = route{page: pageSearch}
	a.trackList = ui.ListState{}
	a.nowPlaying = false
	a.runSearch(q)
}

// runSearch loads the results of q, wherever the listener is.
func (a *app) runSearch(q string) {
	a.searched, a.searching, a.searchErr, a.results = q, true, "", nil
	go func() {
		res, err := Search(q, 60)
		a.post(func() {
			if q != a.searched {
				return
			}
			a.searching = false
			a.results = res
			if err != nil {
				a.searchErr = "搜索失败，请稍后再试"
			}
		})
	}()
}
