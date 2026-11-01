package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

var readSongShelves = qqmusic.RecommendedSongShelves

// The request and its old shelves share an identity until the listener changes.
type songShelfData struct {
	shelves []qqmusic.SongShelf
	loading bool
	err     string
}

func (a *app) loadSongShelves() {
	if a.profile == nil || a.songShelves != nil && a.songShelves.loading {
		return
	}
	if a.songShelves == nil {
		a.songShelves = &songShelfData{}
	}
	d := a.songShelves
	d.loading, d.err = true, ""
	fetch := readSongShelves
	go func() {
		shelves, err := fetch()
		a.post(func() { a.songShelvesAnswered(d, shelves, err) })
	}()
}

func (a *app) songShelvesAnswered(d *songShelfData, shelves []qqmusic.SongShelf, err error) {
	if a.songShelves != d {
		return
	}
	d.loading = false
	if err != nil {
		d.err = "歌曲推荐加载失败"
		return
	}
	d.shelves, d.err = shelves, ""
	a.shelfPagers = nil
}

// Account changes discard personal snapshots, including those in Back and Forward.
func (a *app) clearSongShelves() {
	a.songShelves = nil
	a.shelfPagers = nil
	if a.route.page == pageSongShelf {
		a.route = route{page: pageHome}
	}
	isShelf := func(r route) bool { return r.page == pageSongShelf }
	a.history = slices.DeleteFunc(a.history, isShelf)
	a.forward = slices.DeleteFunc(a.forward, isShelf)
	for r := range a.pageStates {
		if isShelf(r) {
			delete(a.pageStates, r)
		}
	}
}

func (a *app) songShelfColumns(c *ui.Context) int {
	w, _ := c.Size()
	return min(3, max(1, int((w-sidebarWidth-2*pagePad+16)/240)))
}

func (a *app) shelfAction(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c).Height(32).PaddingX(8).Gap(5).Radius(6).AlignItems(ui.Center).Label(label)
	color := a.pal.muted
	if b.Hovered() || b.FocusVisible() {
		b.Background(a.pal.hover)
		color = a.pal.text
	}
	b.TextColor(color).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14)
		}
		ui.Text(c, label).FontSize(12).SingleLine()
	})
	return b
}

func (a *app) songShelfSection(c *ui.Context, start, end int) {
	if a.profile == nil {
		if start != 0 {
			return
		}
		ui.Row(c).Margin(28, 0, 0, 0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "歌曲推荐").FontSize(19).FontWeight(700).Grow(1)
			a.shelfAction(c, iconUser, "登录查看推荐").OnClick(a.openLogin)
		})
		return
	}
	d := a.songShelves
	if d == nil {
		return
	}
	if len(d.shelves) == 0 && d.loading {
		if start == 0 {
			a.songShelfSkeleton(c, a.pageShimmer(c))
		}
		return
	}
	for i := start; i < min(end, len(d.shelves)); i++ {
		shelf := &d.shelves[i]
		if len(shelf.Tracks) == 0 {
			continue
		}
		a.songShelf(c.Key(shelf), shelf)
	}
	if start == 0 && (d.err != "" || len(d.shelves) == 0) {
		ui.Row(c.Key("status")).Margin(24, 0, 0, 0).Gap(10).AlignItems(ui.Center).Children(func() {
			message := d.err
			if message == "" {
				message = "暂无歌曲推荐"
			}
			ui.Text(c, message).FontSize(12).TextColor(a.pal.muted)
			a.shelfAction(c, iconReload, "重试").Label("重试歌曲推荐").Disabled(d.loading).OnClick(a.loadSongShelves)
		})
	}
}

func (a *app) songShelf(c *ui.Context, shelf *qqmusic.SongShelf) {
	if a.shelfPagers == nil {
		a.shelfPagers = map[*qqmusic.SongShelf]*songShelfPager{}
	}
	pager := a.shelfPagers[shelf]
	if pager == nil {
		pager = &songShelfPager{}
		a.shelfPagers[shelf] = pager
	}
	columns := a.songShelfColumns(c)
	resized := pager.size != columns*2
	pager.layout(len(shelf.Tracks), columns*2)
	section := ui.Column(c).Margin(28, 0, 0, 0).Gap(10)
	section.HandleInput(pager.scroll)
	if pager.pages() > 1 {
		moveByKey := func(direction int) {
			if !pager.move(direction) {
				return
			}
			// A song can leave the grid after a key turn. Keep keyboard focus
			// on an enabled, stable arrow so subsequent keys stay in this shelf.
			if pager.page == 0 || direction > 0 && pager.page < pager.pages()-1 {
				pager.next.Focus(c)
			} else {
				pager.previous.Focus(c)
			}
		}
		section.OnShortcut(0, ui.KeyLeft, func() { moveByKey(-1) }).
			OnShortcut(0, ui.KeyRight, func() { moveByKey(1) })
	}
	section.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			songShelfTitle(c, shelf.Title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).MaxLines(2).Grow(1).Shrink(1).MinWidth(0)
			if pager.pages() > 1 {
				a.shelfPageButton(c.Key("previous"), iconBack, "上一组歌曲："+shelf.Title, "上一组", pager.page == 0).
					Bind(&pager.previous).OnClick(func() { pager.move(-1) })
				a.shelfPageButton(c.Key("next"), iconForward, "下一组歌曲："+shelf.Title, "下一组", pager.page == pager.pages()-1).
					Bind(&pager.next).OnClick(func() { pager.move(1) })
			}
		})
		ui.Box(c.Key("viewport")).Clip().Children(func() {
			grid := ui.Grid(c.Key("songs")).Columns(columns).GapX(16).GapY(4)
			start := pager.start()
			if resized {
				grid.Jump("start", float32(start))
			}
			at := grid.AnimateTo("start", float32(start), ui.Animation{Duration: 200 * time.Millisecond})
			lag := min(float32(1), max(float32(-1), (float32(start)-at)/float32(pager.size)))
			fade := lag
			if fade < 0 {
				fade = -fade
			}
			grid.Translate(lag*20, 0).Opacity(1 - 0.22*fade).Children(func() {
				for i := start; i < min(start+pager.size, len(shelf.Tracks)); i++ {
					a.songShelfRow(c.Key(shelf.Tracks[i].ID), shelf, i)
				}
			})
		})
	})
}

func (a *app) shelfPageButton(c *ui.Context, icon *ui.SVG, label, tooltip string, disabled bool) ui.Element {
	color := a.pal.muted
	if disabled {
		color = radixSage(a.pal.dark, 8)
	}
	b := ui.ButtonBase(c).Size(32, 32).Center().Radius(999).Label(label).Tooltip(tooltip).Disabled(disabled).TextColor(color)
	bg := ui.Transparent
	if !disabled && (b.Hovered() || b.FocusVisible()) {
		bg = a.pal.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if !disabled && b.Pressed() {
		pose.Scale = 0.9
	}
	b.AnimatePose(pose, ui.Animation{Duration: 100 * time.Millisecond})
	b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	return b
}

// Narrow corner quotes avoid the full-width opening quote's leading blank.
// Keep the original title in the model and accessibility label.
func songShelfTitle(c *ui.Context, title string) ui.Element {
	if rest, ok := strings.CutPrefix(title, "「"); ok {
		return ui.Text(c, "｢"+strings.Replace(rest, "」", "｣", 1)).Label(title)
	}
	return ui.Text(c, title)
}

func shelfHasPlayable(shelf *qqmusic.SongShelf) bool {
	return len(shelf.Tracks) > 0 && shelf.Tracks[firstPlayable(shelf.Tracks)].Playable()
}

func (a *app) playSongShelf(shelf *qqmusic.SongShelf) {
	if !shelfHasPlayable(shelf) {
		return
	}
	if a.pl.Shuffle() {
		a.pl.ToggleShuffle()
	}
	a.pl.PlayQueue(shelf.Tracks, firstPlayable(shelf.Tracks))
}

func (a *app) playShelfSong(shelf *qqmusic.SongShelf, i int) {
	tr := &shelf.Tracks[i]
	if !tr.Playable() {
		return
	}
	if cur := a.pl.Current(); cur != nil && cur.ID == tr.ID {
		a.pl.TogglePlay()
		return
	}
	a.pl.PlayQueue(shelf.Tracks, i)
}

func (a *app) songShelfRow(c *ui.Context, shelf *qqmusic.SongShelf, i int) {
	p := a.pal
	tr := &shelf.Tracks[i]
	cur := a.pl.Current()
	current := cur != nil && cur.ID == tr.ID
	row := ui.Row(c).Height(76).Padding(6).Gap(3).Radius(8).AlignItems(ui.Center).ContextMenu(a.songItems(tr, 0))
	row.Children(func() {
		b := ui.ButtonBase(c.Key("song")).Grow(1).Shrink(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Label(tr.Name).
			Disabled(!tr.Playable()).OnClick(func() { a.playShelfSong(shelf, i) })
		hovered := row.Hovered() || b.FocusVisible()
		bg := ui.Transparent
		if current {
			bg = radixGreen(p.dark, 3)
		} else if hovered {
			bg = p.hover
		}
		row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
		if !tr.Playable() {
			b.Opacity(0.42)
		}
		b.Children(func() {
			ui.Box(c).Size(48, 48).Radius(6).Children(func() {
				a.coverFill(c, tr.Album.PicURL, 120, 6)
				if hovered || current {
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(6).Center().Background(radixBlack(5)).PassThrough().Children(func() {
						if current && a.pl.Playing() {
							a.nowPlayingBars(c, radixPaper)
						} else {
							ui.Icon(c, iconPlay).Size(15, 15).TextColor(radixPaper)
						}
					})
				}
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				name := p.text
				if current {
					name = radixGreen(p.dark, 11)
				}
				ui.Text(c, tr.Name).FontSize(13.5).FontWeight(500).TextColor(name).SingleLine()
				ui.Text(c, tr.ArtistNames()).FontSize(12).TextColor(p.muted).SingleLine()
			})
		})
		a.likeButton(c.Key("like"), tr, 14, p.muted)
	})
}

func (a *app) songShelfPage(c *ui.Context) {
	shelf := a.route.shelf
	if shelf == nil || a.profile == nil {
		a.centered(c, "暂无歌曲推荐", nil)
		return
	}
	a.trackRows(c, shelf.Tracks, true, func(c *ui.Context) {
		ui.Column(c).Padding(0, 12, 26, 12).Gap(8).Children(func() {
			songShelfTitle(c, shelf.Title).FontSize(28).FontWeight(700).LetterSpacing(-0.6).MaxLines(3).LineHeight(1.25)
			ui.Text(c, fmt.Sprintf("%d 首歌曲", len(shelf.Tracks))).FontSize(13).TextColor(a.pal.muted)
			pillButton(c.Key("play-all"), iconPlay, "播放全部", a.pal.accent, a.pal.onAccent).
				AlignSelf(ui.Start).Margin(8, 0, 0, 0).Label("播放全部：" + shelf.Title).Disabled(!shelfHasPlayable(shelf)).
				OnClick(func() { a.playSongShelf(shelf) })
		})
	})
}

func (a *app) songShelfSkeleton(c *ui.Context, s shimmer) {
	ui.Column(c).Margin(28, 0, 0, 0).Gap(10).Children(func() {
		s.text(c, 19, 1.4).WidthPercent(40)
		ui.Grid(c).Columns(a.songShelfColumns(c)).GapX(16).GapY(4).Children(func() {
			for i := range a.songShelfColumns(c) * 2 {
				ui.Row(c.Key(i)).Height(76).Padding(6).Gap(10).AlignItems(ui.Center).Children(func() {
					s.bone(c, 6).Size(48, 48)
					ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
						s.text(c, 13.5, 1.3).WidthPercent(boneWidth(i))
						s.text(c, 12, 1.3).WidthPercent(45)
					})
				})
			}
		})
	})
}
