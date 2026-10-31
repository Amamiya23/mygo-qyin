package app

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// searchResults and searchSuggest search QQ Music; tests replace them.
var (
	searchResults     = qqmusic.SearchPage
	searchResultsFrom = qqmusic.SearchPageSource
	searchSuggest     = qqmusic.SearchSuggestContext
)

// searchLimit is how many results load at a time.
const searchLimit = 20

// searchKinds are the kinds of search, as their tabs show them.
var searchKinds = []struct {
	kind        qqmusic.SearchType
	label, unit string
}{
	{qqmusic.SearchSongs, "单曲", "首单曲"},
	{qqmusic.SearchArtists, "歌手", "位歌手"},
	{qqmusic.SearchAlbums, "专辑", "张专辑"},
}

// searchData is what a search found, as far as its pages have loaded.
type searchData struct {
	res  qqmusic.SearchResult
	page paged
}

// search searches for q, of the kind last searched for.
func (a *app) search(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	kind := a.searched.kind
	if kind == 0 {
		kind = qqmusic.SearchSongs
	}
	a.searchFor(route{page: pageSearch, query: q, kind: kind})
}

// searchFor opens the page of a search, remembering its query. Searched
// for again, it loads anew.
func (a *app) searchFor(r route) {
	a.rememberSearch(r.query)
	a.dismissSuggestions()
	if r == a.route {
		a.nowPlaying = false
		a.loadSearch(r, true)
		return
	}
	a.open(r)
}

// loadSearch loads the first page of a search, unless it has, or again.
func (a *app) loadSearch(r route, again bool) {
	if d := a.searches[r]; d != nil && !again {
		// A failed first page is not a cached result. Reopening it retries,
		// while loaded pages (including failed pagination) and active loads stay.
		if d.page.loaded || d.page.loading || d.page.err == "" {
			return
		}
	}
	d := &searchData{}
	a.searches[r] = d
	a.moreResults(r, d)
}

// moreResults loads the next page of a search. A search done again has
// new data, so the last one's pages land where nothing shows them.
func (a *app) moreResults(r route, d *searchData) {
	if d.page.loading {
		return
	}
	d.page.loading, d.page.err = true, ""
	offset := d.res.NextOffset
	if offset <= 0 {
		offset = d.res.Len()
	}
	source, fetch, fetchFrom := d.res.Source, searchResults, searchResultsFrom
	go func() {
		var res qqmusic.SearchResult
		var err error
		if source == qqmusic.SearchAuto {
			res, err = fetch(r.query, r.kind, searchLimit, offset)
		} else {
			res, err = fetchFrom(r.query, r.kind, searchLimit, offset, source)
		}
		a.post(func() {
			d.finishPage(res, err)
		})
	}()
}

func (d *searchData) finishPage(res qqmusic.SearchResult, err error) {
	d.page.loading = false
	if err != nil {
		d.page.err = searchErrorMessage(err)
		return
	}
	d.res.Append(res)
	offset := d.res.NextOffset
	if offset <= 0 {
		offset = d.res.Len()
	}
	// Follow the upstream page boundary when unusable entries were filtered.
	// A server can still claim more while sending none.
	d.page.loaded, d.page.more = true, res.Len() > 0 && offset < d.res.Total
}

func searchErrorMessage(err error) string {
	var cooldown *qqmusic.SearchCooldownError
	var protocol *qqmusic.ProtocolError
	var filtered *qqmusic.SearchFilterError
	var networkError net.Error
	switch {
	case errors.As(err, &cooldown):
		return "搜索暂时不可用，请稍后重试"
	case errors.As(err, &protocol) && protocol.Code == 2001:
		return "QQ 音乐拒绝搜索（2001），请确认登录后重试"
	case errors.As(err, &filtered):
		return fmt.Sprintf("QQ 音乐过滤了搜索结果（%d），请稍后重试", filtered.Filter)
	case errors.As(err, &networkError) && networkError.Timeout():
		return "搜索超时，请检查网络后重试"
	default:
		return "搜索失败，请稍后再试"
	}
}

// searchHistoryLimit is how many searches the history keeps.
const searchHistoryLimit = 20

// rememberSearch puts q first in the history of searches, once.
func (a *app) rememberSearch(q string) {
	history := []string{q}
	for _, s := range a.settings.SearchHistory {
		if s != q && len(history) < searchHistoryLimit {
			history = append(history, s)
		}
	}
	a.settings.SearchHistory = history
	a.settings.save()
}

func (a *app) clearSearchHistory() {
	a.settings.SearchHistory = nil
	a.settings.save()
}

func (a *app) searchPage(c *ui.Context) {
	r := a.route
	d := a.searches[r]
	header := func(c *ui.Context) { a.searchHeader(c, d) }
	more := func() { a.moreResults(r, d) }
	if r.kind == qqmusic.SearchSongs {
		if !d.page.loaded && d.page.loading {
			a.trackSkeleton(c, a.pageShimmer(c), true, header)
			return
		}
		a.trackRows(c, d.res.Songs, true, header, func(c *ui.Context) {
			a.pageFoot(c, &d.page, more)
			a.searchEmpty(c, d)
		})
		return
	}
	a.scrollPage(c, func() {
		header(c)
		res := &d.res
		switch r.kind {
		case qqmusic.SearchArtists:
			ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
				for i := range res.Artists {
					a.artistCard(c.Key(res.Artists[i].ID), &res.Artists[i])
				}
			})
		case qqmusic.SearchAlbums:
			a.albumGrid(c, res.Albums)
		case qqmusic.SearchUsers:
			for i := range res.Users {
				a.userRow(c.Key(res.Users[i].UserID), &res.Users[i])
			}
		case qqmusic.SearchPodcasts:
			for i := range res.Podcasts {
				a.podcastRow(c.Key(res.Podcasts[i].ID), &res.Podcasts[i])
			}
		}
		if !d.page.loaded && d.page.loading {
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 156), 156, false)
			return
		}
		a.pageFoot(c, &d.page, more)
		a.searchEmpty(c, d)
	})
}

func (a *app) searchEmpty(c *ui.Context, d *searchData) {
	if d.page.loaded && d.res.Len() == 0 {
		a.empty(c, "没有找到相关结果")
	}
}

// searchHeader is what was searched for, how many it found, and the tabs
// of what it may look for instead.
func (a *app) searchHeader(c *ui.Context, d *searchData) {
	r := a.route
	sub := "搜索"
	at := 0
	for i, k := range searchKinds {
		if k.kind == r.kind {
			at = i
			if d.page.loaded {
				sub = fmt.Sprintf("搜索 · 找到 %d %s", d.res.Total, k.unit)
			}
		}
	}
	a.heading(c, "“"+r.query+"”", sub)
	ui.Row(c).Margin(0, 0, 18, 0).Children(func() {
		labels := make([]string, len(searchKinds))
		for i, k := range searchKinds {
			labels[i] = k.label
		}
		if segmented(c.Key("kind"), &at, labels...) {
			r.kind = searchKinds[at].kind
			a.show(r)
		}
	})
}

// artistCard is an artist's round photo over their name, opening their page.
func (a *app) artistCard(c *ui.Context, ar *qqmusic.ArtistProfile) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(ar.Name)
	hovered := card.Hovered()
	card.Children(func() {
		photo := ar.Avatar
		if photo == "" {
			photo = ar.PicURL
		}
		art := ui.Box(c).AspectRatio(1).Radius(999)
		if hovered {
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.Children(func() { a.coverFill(c, photo, 400, 999) })
		ui.Text(c, ar.Name).FontSize(13).FontWeight(500).SingleLine().AlignSelf(ui.Center)
	})
	if card.Clicked() {
		a.open(route{page: pageArtist, id: ar.ID})
	}
}
