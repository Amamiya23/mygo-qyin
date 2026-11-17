package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

var (
	readSquareTabs        = qqmusic.PlaylistSquareTabs
	readSquareAllTags     = qqmusic.PlaylistAllTags
	readSquareFeed        = qqmusic.PlaylistSquareFeed
	readCategoryBasic     = qqmusic.CategoryBasic
	readCategoryPlaylists = qqmusic.CategoryPlaylists
)

const (
	squareTabRecommendID = -1
	squareTabEditorialID = -2
	squareOfficialCatID  = 3317
	squareFeedSize       = 40
	squareCategorySize   = 60
)

type squareTabKey struct {
	id int64
	ai bool
}

type squareCategoryData struct {
	titleID int64
	items   []qqmusic.Playlist
	seen    map[int64]bool
	fetched int
	total   int
	lastID  string
	pageNum int
	page    paged
}

type squareState struct {
	gen         int
	tabs        []qqmusic.SquareTab
	tabsLoading bool
	tabsErr     string

	groups      []qqmusic.CategoryGroup
	tagsLoading bool
	tagsErr     string
	tagsOpen    bool
	tagsTrigger ui.Handle

	activeID   int64
	activeName string
	activeAI   bool
	titleID    int64

	feedHead  []qqmusic.Playlist
	headSeen  map[int64]bool
	feedItems []qqmusic.Playlist
	feedSeen  map[int64]bool
	feedFrom  int
	feedLimit int
	feedPage  paged

	cats         map[squareTabKey]*squareCategoryData
	basics       map[int64]*qqmusic.CategoryInfo
	basicLoading map[int64]bool

	scrolls map[squareTabKey]*ui.ScrollState
}

func (a *app) squareEntry(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("open-square")).Height(30).Padding(0, 6, 0, 10).Gap(3).Radius(6).
		AlignItems(ui.Center).FocusRing(false).Label("歌单广场")
	fg, bg := p.muted, ui.Transparent
	if b.Hovered() || b.FocusVisible() {
		fg, bg = p.text, p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).
		TextColor(b.AnimateColor("fg", fg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, "歌单广场").FontSize(12.5).FontWeight(500).SingleLine()
		ui.Icon(c, iconForward).Size(13, 13)
	})
	if b.Clicked() {
		a.open(route{page: pageSquare})
	}
}

func (a *app) ensureSquareInit() {
	if a.square.activeID == 0 {
		a.square.activeID = -1
		a.square.titleID = -1
		a.square.activeName = "推荐"
	}
	if a.square.cats == nil {
		a.square.cats = map[squareTabKey]*squareCategoryData{}
	}
	if a.square.basics == nil {
		a.square.basics = map[int64]*qqmusic.CategoryInfo{}
	}
	if a.square.basicLoading == nil {
		a.square.basicLoading = map[int64]bool{}
	}
	if a.square.scrolls == nil {
		a.square.scrolls = map[squareTabKey]*ui.ScrollState{}
	}
}

func (a *app) squareScrollState() *ui.ScrollState {
	a.ensureSquareInit()
	key := squareTabKey{id: a.square.activeID, ai: a.square.activeAI}
	st := a.square.scrolls[key]
	if st == nil {
		st = &ui.ScrollState{}
		a.square.scrolls[key] = st
	}
	return st
}

func (a *app) loadSquare() {
	a.ensureSquareInit()
	a.loadSquareTabs()
	a.loadSquareAllTags()
	a.loadSquareCategoryBasic(squareOfficialCatID)
	if a.square.activeID < 0 {
		if !a.square.feedPage.loaded && !a.square.feedPage.loading {
			a.loadMoreSquareFeed()
		}
		return
	}
	a.loadSquareCategoryBasic(a.square.titleID)
	a.ensureSquareCategoryLoaded(squareTabKey{id: a.square.activeID, ai: a.square.activeAI}, a.square.titleID)
}

func (a *app) loadSquareTabs() {
	if len(a.square.tabs) > 0 || a.square.tabsLoading {
		return
	}
	a.square.tabsLoading, a.square.tabsErr = true, ""
	gen := a.square.gen
	fetch := readSquareTabs
	go func() {
		tabs, err := fetch()
		a.post(func() {
			if a.square.gen != gen {
				return
			}
			a.square.tabsLoading = false
			if err != nil {
				a.square.tabsErr = "分类标签加载失败"
				return
			}
			a.square.tabs = tabs
		})
	}()
}

func (a *app) loadSquareAllTags() {
	if len(a.square.groups) > 0 || a.square.tagsLoading {
		return
	}
	a.square.tagsLoading, a.square.tagsErr = true, ""
	gen := a.square.gen
	fetch := readSquareAllTags
	go func() {
		groups, err := fetch()
		a.post(func() {
			if a.square.gen != gen {
				return
			}
			a.square.tagsLoading = false
			if err != nil {
				a.square.tagsErr = "全部分类加载失败"
				return
			}
			a.square.groups = groups
		})
	}()
}

func (a *app) loadMoreSquareFeed() {
	if a.square.feedPage.loading {
		return
	}
	a.square.feedPage.loading, a.square.feedPage.err = true, ""
	gen := a.square.gen
	from := a.square.feedFrom
	includeHead := from == 0
	fetch := readSquareFeed
	go func() {
		page, err := fetch(from, squareFeedSize, includeHead)
		a.post(func() {
			if a.square.gen != gen {
				return
			}
			a.square.feedPage.loading = false
			if err != nil {
				a.square.feedPage.err = "歌单加载失败"
				return
			}
			if a.square.headSeen == nil {
				a.square.headSeen = map[int64]bool{}
			}
			if a.square.feedSeen == nil {
				a.square.feedSeen = map[int64]bool{}
			}
			if includeHead {
				for _, pl := range page.Head {
					if pl.ID > 0 && !a.square.headSeen[pl.ID] {
						a.square.headSeen[pl.ID] = true
						a.square.feedHead = append(a.square.feedHead, pl)
					}
				}
			}
			for _, pl := range page.Items {
				if pl.ID > 0 && !a.square.feedSeen[pl.ID] {
					a.square.feedSeen[pl.ID] = true
					a.square.feedItems = append(a.square.feedItems, pl)
				}
			}
			a.square.feedFrom = from + len(page.Items)
			if page.Limit > 0 {
				a.square.feedLimit = page.Limit
			}
			a.square.feedPage.loaded = true
			a.square.feedPage.more = page.HasMore && len(page.Items) > 0 &&
				(a.square.feedLimit == 0 || a.square.feedFrom < a.square.feedLimit)
		})
	}()
}

func (a *app) selectSquareCategory(catID, titleID int64, name string, ai bool) {
	a.ensureSquareInit()
	a.square.tagsOpen = false
	if catID <= 0 {
		id := catID
		if id == 0 {
			id = squareTabRecommendID
			name = "推荐"
		} else if id == squareTabEditorialID && name == "" {
			name = "编辑精选"
		}
		a.square.activeID, a.square.titleID, a.square.activeName, a.square.activeAI = id, id, name, false
		if !a.square.feedPage.loaded && !a.square.feedPage.loading {
			a.loadMoreSquareFeed()
		}
		return
	}
	if titleID <= 0 {
		titleID = catID
	}
	a.square.activeID = catID
	a.square.titleID = titleID
	a.square.activeName = name
	a.square.activeAI = ai
	a.loadSquareCategoryBasic(titleID)
	a.ensureSquareCategoryLoaded(squareTabKey{id: catID, ai: ai}, titleID)
}

func (a *app) loadSquareCategoryBasic(titleID int64) {
	if titleID <= 0 || a.square.basics[titleID] != nil || a.square.basicLoading[titleID] {
		return
	}
	a.square.basicLoading[titleID] = true
	gen := a.square.gen
	fetch := readCategoryBasic
	go func() {
		info, err := fetch(titleID)
		a.post(func() {
			if a.square.gen != gen {
				return
			}
			delete(a.square.basicLoading, titleID)
			if err != nil {
				return
			}
			a.square.basics[titleID] = &info
		})
	}()
}

func (a *app) ensureSquareCategoryLoaded(key squareTabKey, titleID int64) {
	cat := a.square.cats[key]
	if cat == nil {
		cat = &squareCategoryData{titleID: titleID, seen: map[int64]bool{}}
		a.square.cats[key] = cat
	}
	if !cat.page.loaded && !cat.page.loading {
		a.loadMoreSquareCategory(key)
	}
}

func (a *app) loadMoreSquareCategory(key squareTabKey) {
	cat := a.square.cats[key]
	if cat == nil || cat.page.loading {
		return
	}
	cat.page.loading, cat.page.err = true, ""
	gen := a.square.gen
	titleID := cat.titleID
	if titleID <= 0 {
		titleID = key.id
	}
	pageNum := cat.pageNum
	lastID := cat.lastID
	fetch := readCategoryPlaylists
	go func() {
		page, err := fetch(key.id, titleID, pageNum, lastID, squareCategorySize, key.ai)
		a.post(func() {
			if a.square.gen != gen || a.square.cats[key] != cat {
				return
			}
			cat.page.loading = false
			if err != nil {
				cat.page.err = "歌单加载失败"
				return
			}
			if cat.seen == nil {
				cat.seen = map[int64]bool{}
			}
			added := 0
			for _, pl := range page.Items {
				if pl.ID > 0 && !cat.seen[pl.ID] {
					cat.seen[pl.ID] = true
					cat.items = append(cat.items, pl)
					added++
				}
			}
			cat.fetched += len(page.Items)
			if page.Total > 0 {
				cat.total = page.Total
			}
			cat.page.loaded = true
			if key.ai {
				cat.pageNum = pageNum + 1
				cat.page.more = page.HasMore && added > 0
			} else {
				cat.lastID = page.LastID
				cat.page.more = page.HasMore && added > 0 && (cat.total == 0 || cat.fetched < cat.total)
			}
		})
	}()
}

func (a *app) squareSubTags(titleID int64) []qqmusic.CategoryTag {
	info := a.square.basics[titleID]
	if info == nil || len(info.Children) == 0 {
		return nil
	}
	hasParent := false
	for _, ch := range info.Children {
		if ch.ID == titleID {
			hasParent = true
			break
		}
	}
	if hasParent {
		return info.Children
	}
	out := make([]qqmusic.CategoryTag, 0, len(info.Children)+1)
	out = append(out, qqmusic.CategoryTag{ID: titleID, Name: "全部"})
	out = append(out, info.Children...)
	return out
}

func (a *app) squareActiveTitleLabel() string {
	if info := a.square.basics[a.square.titleID]; info != nil && info.Name != "" {
		return info.Name
	}
	for _, g := range a.square.groups {
		for _, t := range g.Tags {
			if t.ID == a.square.titleID {
				return t.Name
			}
		}
	}
	return a.square.activeName
}

func (a *app) squareDisplayTabs() []qqmusic.SquareTab {
	rec := qqmusic.SquareTab{ID: squareTabRecommendID, Name: "推荐", Type: 1}
	official := qqmusic.SquareTab{ID: squareOfficialCatID, Name: "官方歌单", Type: 3}
	editorial := qqmusic.SquareTab{ID: squareTabEditorialID, Name: "编辑精选", Type: 1}

	var childIDs map[int64]bool
	var childNames map[string]bool
	if info := a.square.basics[squareOfficialCatID]; info != nil && len(info.Children) > 0 {
		childIDs = make(map[int64]bool, len(info.Children))
		childNames = make(map[string]bool, len(info.Children))
		for _, ch := range info.Children {
			if ch.ID != squareOfficialCatID {
				childIDs[ch.ID] = true
				childNames[ch.Name] = true
			}
		}
	}

	var rest []qqmusic.SquareTab
	for _, tab := range a.square.tabs {
		switch {
		case tab.IsRecommend():
			if tab.Name != "" {
				rec.Name = tab.Name
			}
		case tab.ID == squareOfficialCatID:
			official = tab
		case tab.ID == squareTabEditorialID:
		case childIDs[tab.ID] || childNames[tab.Name]:
		default:
			rest = append(rest, tab)
		}
	}
	out := make([]qqmusic.SquareTab, 0, 3+len(rest))
	out = append(out, rec, official, editorial)
	out = append(out, rest...)
	return out
}

func (a *app) squareTabMatches(tab qqmusic.SquareTab) bool {
	if tab.ID == squareTabRecommendID {
		return a.square.activeID == squareTabRecommendID
	}
	if tab.ID == squareTabEditorialID {
		return a.square.activeID == squareTabEditorialID
	}
	return a.square.activeID > 0 && a.square.titleID == tab.ID
}

func (a *app) squarePage(c *ui.Context) {
	a.ensureSquareInit()
	top, bottom := a.insets(c)
	s := a.pageShimmer(c)

	ui.Scroll(c.Key("square")).TrackScroll(a.squareScrollState()).Grow(1).
		Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).
		Children(func() {
			a.startMark(c)
			defer a.endMark(c)

			a.heading(c, "歌单广场", "")
			a.squareTabBar(c, s)

			switch {
			case a.square.activeID == squareTabEditorialID:
				a.squareEditorialContent(c, s)
			case a.square.activeID <= 0:
				a.squareRecommendContent(c, s)
			default:
				a.squareCategoryContent(c, s)
			}
		})
}

func (a *app) squareTabBar(c *ui.Context, s shimmer) {
	subTags := []qqmusic.CategoryTag(nil)
	if a.square.activeID > 0 {
		subTags = a.squareSubTags(a.square.titleID)
	}
	barMarginBottom := float32(20)
	if len(subTags) > 0 {
		barMarginBottom = 10
	}

	ui.Row(c.Key("tabs-row")).AlignItems(ui.Start).Gap(10).Margin(0, 0, barMarginBottom, 0).Children(func() {
		ui.Row(c.Key("tabs")).Grow(1).Shrink(1).MinWidth(0).Wrap().Gap(6).Children(func() {
			tabs := a.squareDisplayTabs()
			matchedTab := false
			for _, tab := range tabs {
				selected := a.squareTabMatches(tab)
				if selected {
					matchedTab = true
				}
				if a.squareTabPill(c.Key(tab.ID), tab.Name, selected).Clicked() {
					if tab.ID == squareTabRecommendID || tab.ID == squareTabEditorialID {
						a.selectSquareCategory(tab.ID, tab.ID, tab.Name, false)
					} else {
						a.selectSquareCategory(tab.ID, tab.ID, tab.Name, tab.IsAI())
					}
				}
			}
			if a.square.tabsLoading && len(a.square.tabs) == 0 {
				for i, w := range []float32{64, 54, 54, 54} {
					s.bone(c.Key(fmt.Sprintf("bone:%d", i)), 15).Size(w, 30)
				}
			}
			if a.square.activeID > 0 && !matchedTab {
				label := a.squareActiveTitleLabel()
				if label != "" {
					if a.squareTabPill(c.Key("custom-tab"), label, true).Clicked() {
						a.selectSquareCategory(a.square.titleID, a.square.titleID, label, a.square.activeAI)
					}
				}
			}
		})
		a.squareAllTagsButton(c, s)
	})

	if len(subTags) > 0 {
		ui.Row(c.Key("sub-tags")).Wrap().Gap(6).Margin(0, 0, 18, 0).Children(func() {
			for _, ch := range subTags {
				selected := ch.ID == a.square.activeID
				if a.squareSubChip(c.Key(ch.ID), ch.Name, selected).Clicked() {
					ai := a.square.activeAI || ch.IsAI()
					a.selectSquareCategory(ch.ID, a.square.titleID, ch.Name, ai)
				}
			}
		})
	}
}

func (a *app) squareTabPill(c *ui.Context, label string, selected bool) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Height(30).PaddingX(13).Radius(999).AlignItems(ui.Center).
		FocusRing(false).Label(label)
	bg, fg, weight := ui.Transparent, p.muted, 500
	if selected {
		bg, fg, weight = radixGreen(p.dark, 3), radixGreen(p.dark, 11), 600
	} else if b.Hovered() || b.FocusVisible() {
		bg, fg = p.hover, p.text
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).
		TextColor(b.AnimateColor("fg", fg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(weight).SingleLine()
	})
	return b
}

func (a *app) squareSubChip(c *ui.Context, label string, selected bool) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Height(26).PaddingX(11).Radius(999).AlignItems(ui.Center).
		FocusRing(false).Label(label)
	bg, fg, weight := ui.Transparent, p.muted, 500
	if selected {
		bg, fg, weight = p.hover.Over(p.bg), p.text, 600
	} else if b.Hovered() || b.FocusVisible() {
		bg, fg = p.hover, p.text
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).
		TextColor(b.AnimateColor("fg", fg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(weight).SingleLine()
	})
	return b
}

func (a *app) squareAllTagsButton(c *ui.Context, s shimmer) {
	p := a.pal
	b := ui.ButtonBase(c.Key("all-tags-button")).Bind(&a.square.tagsTrigger).
		Height(30).Padding(0, 10, 0, 11).Shrink(0).Gap(5).Radius(999).
		AlignItems(ui.Center).FocusRing(false).Label("全部分类")
	bg, fg := ui.Transparent, p.muted
	if a.square.tagsOpen {
		bg, fg = p.press.Over(p.bg), p.text
	} else if b.Hovered() || b.FocusVisible() {
		bg, fg = p.hover, p.text
	}
	b.Border(0.5, p.hairline).
		Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).
		TextColor(b.AnimateColor("fg", fg, ui.Animation{Duration: 120 * time.Millisecond}))
	angle := float32(0)
	if a.square.tagsOpen {
		angle = 180
	}
	b.Children(func() {
		ui.Icon(c, iconGrid).Size(13, 13)
		ui.Text(c, "全部分类").FontSize(12.5).FontWeight(500).SingleLine()
		ui.Icon(c, iconDown).Size(11, 11).Rotate(b.AnimateTo("arrow", angle, ui.Animation{Duration: 140 * time.Millisecond}))
	})
	if b.Clicked() {
		a.square.tagsOpen = !a.square.tagsOpen
		if a.square.tagsOpen {
			a.loadSquareAllTags()
		}
	}

	w, h := c.Size()
	inner := w - sidebarWidth - 2*pagePad
	panelW := min(float32(580), max(float32(340), inner))
	panelH := min(float32(430), max(float32(220), h-220))
	pop := popoverColors(p.dark)
	panel := ui.PopoverBase(c.Key("square-tags-popover"), b, &a.square.tagsOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(8, 0, 0, 0).
			Width(panelW).MaxHeight(panelH).Radius(14).Background(pop.fill).
			Border(0.5, pop.edge).Shadow(0, 14, 36, -8, p.cardShade).
			TextColor(p.text).Label("全部分类列表").
			Focusable().FocusRing(false).AutoFocus()

		if a.square.tagsLoading && len(a.square.groups) == 0 {
			ui.Column(c).Padding(18, 20).Gap(12).Children(func() {
				for i := range 4 {
					ui.Row(c.Key(i)).Gap(12).AlignItems(ui.Center).Children(func() {
						s.text(c, 12, 1.3).Width(36)
						s.bone(c, 13).Height(26).Grow(1)
					})
				}
			})
			return
		}
		if a.square.tagsErr != "" && len(a.square.groups) == 0 {
			ui.Column(c).Padding(24).Center().Gap(10).Children(func() {
				ui.Text(c, a.square.tagsErr).FontSize(12.5).TextColor(p.muted)
				if ui.Button(c.Key("retry-tags"), "重试").Clicked() {
					a.loadSquareAllTags()
				}
			})
			return
		}

		ui.Scroll(c.Key("tags-scroll")).Grow(1).Padding(16, 20).Children(func() {
			for gi, g := range a.square.groups {
				if gi > 0 {
					ui.Box(c).Height(0.5).Margin(10, 0).Background(p.hairline)
				}
				ui.Row(c.Key(gi)).AlignItems(ui.Start).Gap(12).Children(func() {
					ui.Text(c, g.Name).Width(44).Padding(4, 0).FontSize(12).FontWeight(600).TextColor(p.muted).SingleLine()
					ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Wrap().Gap(6).Children(func() {
						for _, tag := range g.Tags {
							selected := a.square.activeID > 0 && (a.square.titleID == tag.ID || a.square.activeID == tag.ID)
							if a.squareAllTagChip(c.Key(fmt.Sprintf("%d:%d", gi, tag.ID)), tag.Name, selected).Clicked() {
								a.selectSquareCategory(tag.ID, tag.ID, tag.Name, tag.IsAI())
							}
						}
					})
				})
			}
		})
	})
	if panel.Dismissed() && !panel.PressedOutside() {
		a.square.tagsTrigger.Focus(c)
	}
}

func (a *app) squareAllTagChip(c *ui.Context, label string, selected bool) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Height(26).PaddingX(10).Radius(999).AlignItems(ui.Center).
		FocusRing(false).Label(label)
	bg, fg, weight := ui.Transparent, p.text, 450
	if selected {
		bg, fg, weight = radixGreen(p.dark, 3), radixGreen(p.dark, 11), 600
	} else if b.Hovered() || b.FocusVisible() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond})).
		TextColor(fg)
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(weight).SingleLine()
	})
	return b
}

func (a *app) squareEditorialContent(c *ui.Context, s shimmer) {
	cols := a.columns(c, 168)
	if len(a.square.feedHead) == 0 {
		if a.square.feedPage.loading {
			a.playlistCardsSkeleton(c.Key("head-skeleton"), s, cols*2, 168, false)
			return
		}
		if a.square.feedPage.err != "" {
			a.squareError(c, a.square.feedPage.err, a.loadMoreSquareFeed)
			return
		}
		if a.square.feedPage.loaded {
			a.empty(c, "暂无歌单")
		}
		return
	}

	ui.Grid(c.Key("head-grid")).Columns(cols).GapX(20).GapY(28).Children(func() {
		for i := range a.square.feedHead {
			a.playlistCard(c.Key(a.square.feedHead[i].ID), &a.square.feedHead[i])
		}
	})
}

func (a *app) squareRecommendContent(c *ui.Context, s shimmer) {
	cols := a.columns(c, 168)
	if len(a.square.feedItems) == 0 {
		if a.square.feedPage.loading {
			a.playlistCardsSkeleton(c.Key("feed-skeleton"), s, cols*2, 168, false)
			return
		}
		if a.square.feedPage.err != "" {
			a.squareError(c, a.square.feedPage.err, a.loadMoreSquareFeed)
			return
		}
		if a.square.feedPage.loaded {
			a.empty(c, "暂无歌单")
		}
		return
	}

	ui.Grid(c.Key("feed-grid")).Columns(cols).GapX(20).GapY(28).Children(func() {
		for i := range a.square.feedItems {
			a.playlistCard(c.Key(a.square.feedItems[i].ID), &a.square.feedItems[i])
		}
	})
	a.pageFoot(c, &a.square.feedPage, a.loadMoreSquareFeed)
}

func (a *app) squareCategoryContent(c *ui.Context, s shimmer) {
	key := squareTabKey{id: a.square.activeID, ai: a.square.activeAI}
	cat := a.square.cats[key]
	cols := a.columns(c, 168)
	if cat == nil || (len(cat.items) == 0 && cat.page.loading) {
		a.playlistCardsSkeleton(c.Key("cat-skeleton"), s, cols*2, 168, false)
		return
	}
	if len(cat.items) == 0 {
		if cat.page.err != "" {
			a.squareError(c, cat.page.err, func() { a.loadMoreSquareCategory(key) })
			return
		}
		if cat.page.loaded {
			a.empty(c, "暂无歌单")
		}
		return
	}

	ui.Grid(c.Key("cat-grid")).Columns(cols).GapX(20).GapY(28).Children(func() {
		for i := range cat.items {
			a.playlistCard(c.Key(cat.items[i].ID), &cat.items[i])
		}
	})
	a.pageFoot(c, &cat.page, func() { a.loadMoreSquareCategory(key) })
}

func (a *app) squareError(c *ui.Context, msg string, retry func()) {
	ui.Column(c).Padding(64, 0).Center().Gap(12).Children(func() {
		ui.Text(c, msg).FontSize(13).TextColor(a.pal.muted)
		if retry != nil && ui.Button(c.Key("retry"), "重试").Clicked() {
			retry()
		}
	})
}
