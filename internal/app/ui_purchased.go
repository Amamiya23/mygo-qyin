package app

import (
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

const (
	purchasedAlbums = iota
	purchasedSongs
	purchasedLimit = 30
)

// These read the current account's purchases; tests replace them.
var (
	readPurchasedSongs  = qqmusic.PurchasedSongs
	readPurchasedAlbums = qqmusic.PurchasedAlbums
)

type purchasedData struct {
	songs  []qqmusic.Track
	albums []qqmusic.AlbumProfile
	page   paged
}

func (a *app) loadPurchased(tab int, again bool) {
	if a.profile == nil {
		return
	}
	if a.purchased[tab] != nil && !again {
		return
	}
	d := &purchasedData{}
	a.purchased[tab] = d
	a.morePurchased(tab, d)
}

func (a *app) morePurchased(tab int, d *purchasedData) {
	if a.profile == nil || d.page.loading {
		return
	}
	d.page.loading, d.page.err = true, ""
	offset := len(d.songs) + len(d.albums)
	go func() {
		var songs []qqmusic.Track
		var albums []qqmusic.AlbumProfile
		var more bool
		var err error
		if tab == purchasedSongs {
			songs, more, err = readPurchasedSongs(purchasedLimit, offset)
		} else {
			albums, more, err = readPurchasedAlbums(purchasedLimit, offset)
		}
		a.post(func() {
			d.page.loading = false
			if err != nil {
				d.page.err = "已购音乐加载失败"
				return
			}
			d.songs = append(d.songs, songs...)
			d.albums = append(d.albums, albums...)
			d.page.loaded, d.page.more = true, more && len(songs)+len(albums) > 0
		})
	}()
}

func (a *app) purchasedPage(c *ui.Context) {
	if a.profile == nil {
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center().Gap(12).Children(func() {
			ui.Text(c, "登录后查看已购音乐").FontSize(13).TextColor(a.pal.muted)
			if ui.Button(c, "登录 QQ 音乐").Clicked() {
				a.openLogin()
			}
		})
		return
	}
	tab := a.route.tab
	d := a.purchased[tab]
	content := route{page: pagePurchased}
	header := func(c *ui.Context) { a.purchasedHeader(c, d) }
	foot := func(c *ui.Context) {
		a.pageFoot(c, &d.page, func() { a.morePurchased(tab, d) })
		if d.page.loaded && len(d.songs)+len(d.albums) == 0 {
			label := "暂无已购单曲"
			if tab == purchasedAlbums {
				label = "暂无已购专辑"
			}
			a.empty(c, label)
		}
	}
	if tab == purchasedSongs {
		if !d.page.loaded && d.page.loading {
			a.listPageWithKey(c, content, &a.purchasedScroll[purchasedSongs], func() {
				a.underTop, a.underBottom = false, true
				a.trackSkeletonRows(c, a.pageShimmer(c), true, header)
			})
			return
		}
		a.trackRowsWithKey(c, content, &a.purchasedScroll[purchasedSongs], d.songs, true, header, foot)
		return
	}
	a.listPageWithKey(c, content, &a.purchasedScroll[tab], func() {
		header(c)
		a.albumGrid(c, d.albums)
		if !d.page.loaded && d.page.loading {
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 156), 156, false)
			return
		}
		foot(c)
	})
}

func (a *app) purchasedHeader(c *ui.Context, d *purchasedData) {
	a.heading(c, "已购音乐", "")
	ui.Row(c).AlignItems(ui.Center).Gap(12).Margin(0, 0, 18, 0).Children(func() {
		tab := a.route.tab
		if segmented(c.Key("purchased-kind"), &tab, "专辑", "单曲") {
			r := a.route
			r.tab = tab
			a.show(r)
		}
		ui.Box(c).Grow(1)
		if len(d.songs) > 0 {
			pillButton(c.Key("play-purchased"), iconPlay, "播放", a.pal.accent, a.pal.onAccent).OnClick(func() {
				if a.pl.Shuffle() {
					a.pl.ToggleShuffle()
				}
				a.pl.PlayQueue(d.songs, firstPlayable(d.songs))
			})
		}
		iconButton(c.Key("refresh-purchased"), iconReload, 17, a.pal.muted, "刷新已购音乐").Disabled(d.page.loading).
			OnClick(func() { a.loadPurchased(a.route.tab, true) })
	})
}
