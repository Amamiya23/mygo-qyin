package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func squareTestApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.featured = []qqmusic.Playlist{{ID: 900, Name: "精选测试歌单"}}
	a.daily = []qqmusic.Track{{ID: 101, Name: "今日歌曲", Duration: 180000}}
	var mu sync.Mutex
	a.postFn = func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		fn()
	}
	return a
}

// Home's 精选歌单 section opens 歌单广场, and returning from a playlist restores the square's tab and scroll position.
func TestHomeOpensPlaylistSquareAndRestoresScroll(t *testing.T) {
	tabsHook, tagsHook, feedHook := readSquareTabs, readSquareAllTags, readSquareFeed
	defer func() {
		readSquareTabs, readSquareAllTags, readSquareFeed = tabsHook, tagsHook, feedHook
	}()

	var items []qqmusic.Playlist
	for i := range 36 {
		id := int64(2000 + i)
		items = append(items, qqmusic.Playlist{ID: id, Name: fmt.Sprintf("广场歌单 %02d", i+1), PlayCount: 15000})
	}
	readSquareTabs = func() ([]qqmusic.SquareTab, error) {
		return []qqmusic.SquareTab{
			{ID: -1, Name: "推荐", Type: 1},
			{ID: 3903, Name: "互动歌单", Type: 3},
			{ID: 9527, Name: "AI歌单", Type: 5},
			{ID: 3152, Name: "流行", Type: 3},
		}, nil
	}
	readSquareAllTags = func() ([]qqmusic.CategoryGroup, error) {
		return []qqmusic.CategoryGroup{{ID: 1, Name: "流派", Tags: []qqmusic.CategoryTag{{ID: 49, Name: "轻音乐"}}}}, nil
	}
	readSquareFeed = func(from, size int, includeHead bool) (qqmusic.SquareFeedPage, error) {
		head := []qqmusic.Playlist{{ID: 1001, Name: "编辑推荐歌单"}, {ID: 2000, Name: "广场歌单 01"}}
		return qqmusic.SquareFeedPage{Head: head, Items: items, HasMore: false, Limit: 400}, nil
	}

	a := squareTestApp()
	a.playlists[2015] = &qqmusic.Playlist{ID: 2015, Name: "广场歌单 16", Tracks: []qqmusic.Track{{ID: 1, Name: "曲目"}}}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if !tt.HasText("首页") || tt.HasText("为你推荐") {
		t.Fatal("sidebar did not rename 为你推荐 to 首页")
	}

	if err := tt.Click("歌单广场"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageSquare {
		t.Fatalf("opened %+v, want pageSquare", a.route)
	}
	// Head and Items deduplicate independently across 编辑精选 and 推荐 tabs.
	if len(a.square.feedHead) != 2 || len(a.square.feedItems) != 36 {
		t.Fatalf("unexpected counts head=%d items=%d", len(a.square.feedHead), len(a.square.feedItems))
	}
	if tt.HasText("编辑推荐歌单") {
		t.Fatal("推荐 tab should not embed 编辑精选 playlists")
	}

	tt.Scroll(700, 500, 0, 500)
	tt.Advance(time.Second)
	tt.Frame()
	scrolledY := a.squareScrollState().Y
	if scrolledY <= 0 {
		t.Fatal("square page did not scroll")
	}

	cardNames := make([]string, len(a.square.feedItems))
	for i := range a.square.feedItems {
		cardNames[i] = a.square.feedItems[i].Name
	}
	visible, at := visibleNavigationItem(t, tt, cardNames)
	a.playlists[2015] = &qqmusic.Playlist{ID: 2015, Name: "广场歌单 16", Tracks: []qqmusic.Track{{ID: 1, Name: "曲目"}}}
	for _, pl := range a.square.feedItems {
		a.playlists[pl.ID] = &qqmusic.Playlist{ID: pl.ID, Name: pl.Name, Tracks: []qqmusic.Track{{ID: 1, Name: "曲目"}}}
	}
	if err := tt.Click(visible); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pagePlaylist {
		t.Fatalf("card opened %+v, want pagePlaylist", a.route)
	}

	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageSquare {
		t.Fatalf("back opened %+v, want pageSquare", a.route)
	}
	assertNavigationPosition(t, tt, visible, at)
}

// 官方歌单 is placed second, 编辑精选 is its own tab, AI/sub-categories pass the expected params, and no Play All or eyebrow text appears.
func TestPlaylistSquareTabsEditorialOfficialAndAI(t *testing.T) {
	tabsHook, tagsHook, feedHook := readSquareTabs, readSquareAllTags, readSquareFeed
	basicHook, catHook := readCategoryBasic, readCategoryPlaylists
	defer func() {
		readSquareTabs, readSquareAllTags, readSquareFeed = tabsHook, tagsHook, feedHook
		readCategoryBasic, readCategoryPlaylists = basicHook, catHook
	}()

	readSquareTabs = func() ([]qqmusic.SquareTab, error) {
		return []qqmusic.SquareTab{
			{ID: -1, Name: "推荐", Type: 1},
			{ID: 9527, Name: "AI歌单", Type: 5},
			{ID: 3406, Name: "韩语", Type: 3},
			{ID: 3152, Name: "流行", Type: 3},
		}, nil
	}
	readSquareAllTags = func() ([]qqmusic.CategoryGroup, error) {
		return []qqmusic.CategoryGroup{
			{ID: 99, Name: "最近常用", Tags: []qqmusic.CategoryTag{{ID: 9527, Name: "AI歌单", Source: 1}}},
		}, nil
	}
	readSquareFeed = func(int, int, bool) (qqmusic.SquareFeedPage, error) {
		return qqmusic.SquareFeedPage{
			Head:  []qqmusic.Playlist{{ID: 10, Name: "编辑独立歌单"}},
			Items: []qqmusic.Playlist{{ID: 1, Name: "推荐歌单"}},
		}, nil
	}
	readCategoryBasic = func(id int64) (qqmusic.CategoryInfo, error) {
		if id == 3317 {
			return qqmusic.CategoryInfo{
				ID:   3317,
				Name: "官方歌单",
				Children: []qqmusic.CategoryTag{
					{ID: 3317, Name: "全部"},
					{ID: 3403, Name: "华语"},
					{ID: 3406, Name: "韩语"},
					{ID: 3409, Name: "流行"},
				},
			}, nil
		}
		return qqmusic.CategoryInfo{ID: id}, nil
	}
	var gotRequests []struct {
		cid, title int64
		ai         bool
	}
	readCategoryPlaylists = func(cid, title int64, page int, lastID string, size int, ai bool) (qqmusic.CategoryPlaylistsPage, error) {
		gotRequests = append(gotRequests, struct {
			cid, title int64
			ai         bool
		}{cid, title, ai})
		return qqmusic.CategoryPlaylistsPage{
			Items: []qqmusic.Playlist{{ID: cid*10 + 1, Name: fmt.Sprintf("歌单-%d", cid)}},
		}, nil
	}

	a := squareTestApp()
	ready := make(chan struct{}, 8)
	basePost := a.postFn
	a.postFn = func(fn func()) {
		basePost(func() {
			fn()
			if len(a.square.tabs) > 0 && a.square.basics[squareOfficialCatID] != nil {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		})
	}
	a.open(route{page: pageSquare})
	<-ready
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	tabs := a.squareDisplayTabs()
	if len(tabs) != 4 || tabs[0].ID != squareTabRecommendID || tabs[1].ID != squareOfficialCatID || tabs[2].ID != squareTabEditorialID || tabs[3].ID != 9527 {
		t.Fatalf("unexpected filtered display tab priority: %+v", tabs)
	}
	if tt.HasText("韩语") || tt.HasText("流行") {
		t.Fatal("tabs already in 官方歌单 sub-categories should not appear in top tab row")
	}
	if tt.HasText("最近常用") || tt.HasText("精选发现") {
		t.Fatal("unexpected eyebrow text above 歌单广场")
	}

	// Switch to 编辑精选 tab.
	if err := tt.Click("编辑精选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.square.activeID != squareTabEditorialID || !tt.HasText("编辑独立歌单") {
		t.Fatalf("编辑精选 tab did not show head playlists: active=%d", a.square.activeID)
	}

	// Switch to AI tab.
	if err := tt.Click("AI歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.square.activeAI || a.square.activeID != 9527 {
		t.Fatalf("unexpected AI state: %+v", a.square)
	}
	if tt.HasText("播放全部") || tt.HasText("最近常用") {
		t.Fatal("square page should not show 播放全部 button or group eyebrow")
	}

	// Switch to 官方歌单 (second tab) and pick sub-category 韩语 (3406) with titleID=3317.
	if err := tt.Click("官方歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("韩语"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.square.activeID != 3406 || a.square.titleID != 3317 {
		t.Fatalf("unexpected sub-category state: active=%d title=%d", a.square.activeID, a.square.titleID)
	}
	if a.squareTabMatches(qqmusic.SquareTab{ID: 3406, Name: "韩语"}) || !a.squareTabMatches(qqmusic.SquareTab{ID: 3317, Name: "官方歌单"}) {
		t.Fatal("sub-category selection should match only the parent 官方歌单 top tab")
	}

	// Also verify switching to 华语 (3403) passes titleID=3317.
	if err := tt.Click("华语"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.square.activeID != 3403 || a.square.titleID != 3317 {
		t.Fatalf("unexpected sub-category state: active=%d title=%d", a.square.activeID, a.square.titleID)
	}

	wantFirst := struct {
		cid, title int64
		ai         bool
	}{9527, 9527, true}
	wantSub := struct {
		cid, title int64
		ai         bool
	}{3403, 3317, false}
	if len(gotRequests) < 3 || gotRequests[0] != wantFirst || gotRequests[len(gotRequests)-1] != wantSub {
		t.Fatalf("unexpected category requests: %+v", gotRequests)
	}
}

// The 全部分类 popover suppresses Space playback, selects custom tags, and restores trigger focus on Escape.
func TestPlaylistSquareAllTagsPopover(t *testing.T) {
	tabsHook, tagsHook, feedHook := readSquareTabs, readSquareAllTags, readSquareFeed
	basicHook, catHook := readCategoryBasic, readCategoryPlaylists
	defer func() {
		readSquareTabs, readSquareAllTags, readSquareFeed = tabsHook, tagsHook, feedHook
		readCategoryBasic, readCategoryPlaylists = basicHook, catHook
	}()

	readSquareTabs = func() ([]qqmusic.SquareTab, error) {
		return []qqmusic.SquareTab{{ID: -1, Name: "推荐", Type: 1}}, nil
	}
	readSquareAllTags = func() ([]qqmusic.CategoryGroup, error) {
		return []qqmusic.CategoryGroup{
			{ID: 1, Name: "流派", Tags: []qqmusic.CategoryTag{{ID: 49, Name: "轻音乐"}}},
		}, nil
	}
	readSquareFeed = func(int, int, bool) (qqmusic.SquareFeedPage, error) {
		return qqmusic.SquareFeedPage{}, nil
	}
	readCategoryBasic = func(id int64) (qqmusic.CategoryInfo, error) {
		return qqmusic.CategoryInfo{ID: id, Name: "轻音乐"}, nil
	}
	readCategoryPlaylists = func(cid, title int64, page int, lastID string, size int, ai bool) (qqmusic.CategoryPlaylistsPage, error) {
		return qqmusic.CategoryPlaylistsPage{Items: []qqmusic.Playlist{{ID: 4901, Name: "安静钢琴曲"}}}, nil
	}

	a := squareTestApp()
	tagsDone := make(chan struct{}, 8)
	a.postFn = func(fn func()) {
		fn()
		if len(a.square.groups) > 0 {
			select {
			case tagsDone <- struct{}{}:
			default:
			}
		}
	}
	a.pl.Cue([]qqmusic.Track{{ID: 55, Name: "背景音乐", Duration: 180000}})
	a.open(route{page: pageSquare})
	<-tagsDone
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("全部分类"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.square.tagsOpen {
		t.Fatal("all-tags popover did not open")
	}

	// Space inside the popover must not start global playback.
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.pl.Playing() {
		t.Fatal("Space started playback while all-tags popover was open")
	}

	if err := tt.Click("轻音乐"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.square.tagsOpen || a.square.activeID != 49 || !tt.HasText("安静钢琴曲") {
		t.Fatalf("selecting 轻音乐 failed: tagsOpen=%v activeID=%d", a.square.tagsOpen, a.square.activeID)
	}

	// Reopen and dismiss with Escape: trigger regains focus.
	if err := tt.Click("全部分类"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	info, ok := tt.Access("全部分类")
	if a.square.tagsOpen || !ok || !info.Focused {
		t.Fatalf("Escape did not dismiss popover and restore focus: open=%v ok=%v focused=%v", a.square.tagsOpen, ok, info.Focused)
	}
	// Space while trigger is focused must not hijack playback toggle.
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.pl.Playing() {
		t.Fatal("Space started playback while all-tags trigger was focused")
	}
}

// Account changes discard in-flight square responses so old results cannot overwrite the new state.
func TestPlaylistSquareAccountChangeDiscardsStaleResults(t *testing.T) {
	tabsHook, tagsHook, feedHook := readSquareTabs, readSquareAllTags, readSquareFeed
	defer func() {
		readSquareTabs, readSquareAllTags, readSquareFeed = tabsHook, tagsHook, feedHook
	}()

	gate := make(chan struct{})
	delivered := make(chan struct{})
	readSquareTabs = func() ([]qqmusic.SquareTab, error) {
		<-gate
		return []qqmusic.SquareTab{{ID: -1, Name: "旧标签", Type: 1}}, nil
	}
	readSquareAllTags = func() ([]qqmusic.CategoryGroup, error) { return nil, nil }
	readSquareFeed = func(int, int, bool) (qqmusic.SquareFeedPage, error) {
		return qqmusic.SquareFeedPage{}, nil
	}

	a := squareTestApp()
	a.postFn = func(fn func()) {
		fn()
		select {
		case delivered <- struct{}{}:
		default:
		}
	}
	a.loadSquare()
	a.square = squareState{gen: a.square.gen + 1}
	close(gate)
	<-delivered
	if len(a.square.tabs) != 0 {
		t.Fatalf("stale tabs leaked across generation bump: %+v", a.square.tabs)
	}
}

// With NM_FRAMES set, exports Playlist Square light/dark/narrow/popover previews for visual inspection.
func TestSquareFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	tabsHook, tagsHook, feedHook := readSquareTabs, readSquareAllTags, readSquareFeed
	basicHook, catHook := readCategoryBasic, readCategoryPlaylists
	defer func() {
		readSquareTabs, readSquareAllTags, readSquareFeed = tabsHook, tagsHook, feedHook
		readCategoryBasic, readCategoryPlaylists = basicHook, catHook
	}()

	readSquareTabs = func() ([]qqmusic.SquareTab, error) {
		return []qqmusic.SquareTab{
			{ID: -1, Name: "推荐", Type: 1},
			{ID: 3903, Name: "互动歌单", Type: 3},
			{ID: 9527, Name: "AI歌单", Type: 5},
			{ID: 42, Name: "说唱", Type: 3},
			{ID: 3420, Name: "国风", Type: 3},
			{ID: 3419, Name: "私藏", Type: 3},
			{ID: 3152, Name: "流行", Type: 3},
			{ID: 41, Name: "摇滚", Type: 3},
		}, nil
	}
	readSquareAllTags = func() ([]qqmusic.CategoryGroup, error) {
		return []qqmusic.CategoryGroup{
			{ID: 1, Name: "热门", Tags: []qqmusic.CategoryTag{{ID: 3317, Name: "官方歌单"}, {ID: 9527, Name: "AI歌单", Source: 1}, {ID: 3417, Name: "私藏"}}},
			{ID: 2, Name: "场景", Tags: []qqmusic.CategoryTag{{ID: 3248, Name: "学习工作"}, {ID: 3215, Name: "咖啡馆"}, {ID: 3902, Name: "Chill Vibes"}, {ID: 32, Name: "夜店"}}},
			{ID: 3, Name: "心情", Tags: []qqmusic.CategoryTag{{ID: 13, Name: "安静"}, {ID: 74, Name: "伤感"}, {ID: 3142, Name: "快乐"}}},
			{ID: 4, Name: "流派", Tags: []qqmusic.CategoryTag{{ID: 3152, Name: "流行"}, {ID: 41, Name: "摇滚"}, {ID: 49, Name: "轻音乐"}, {ID: 45, Name: "电子"}, {ID: 3420, Name: "国风"}}},
		}, nil
	}
	var head, feed []qqmusic.Playlist
	for i := range 4 {
		head = append(head, qqmusic.Playlist{ID: int64(100 + i), Name: fmt.Sprintf("编辑精选歌单 %d · 晚风与钢琴", i+1), PlayCount: 380000})
	}
	for i := range 12 {
		feed = append(feed, qqmusic.Playlist{ID: int64(200 + i), Name: fmt.Sprintf("安静听歌 · 午后窗台的白噪音 %02d", i+1), PlayCount: float64((i + 1) * 42000)})
	}
	readSquareFeed = func(int, int, bool) (qqmusic.SquareFeedPage, error) {
		return qqmusic.SquareFeedPage{Head: head, Items: feed, HasMore: true, Limit: 400}, nil
	}
	readCategoryBasic = func(id int64) (qqmusic.CategoryInfo, error) {
		return qqmusic.CategoryInfo{
			ID: id, Name: "官方歌单",
			Children: []qqmusic.CategoryTag{{ID: 3317, Name: "全部"}, {ID: 3403, Name: "华语"}, {ID: 3405, Name: "欧美"}, {ID: 3406, Name: "韩语"}, {ID: 3408, Name: "粤语"}},
		}, nil
	}
	readCategoryPlaylists = func(cid, title int64, page int, lastID string, size int, ai bool) (qqmusic.CategoryPlaylistsPage, error) {
		return qqmusic.CategoryPlaylistsPage{Items: feed}, nil
	}

	saveShot := func(name string, dark, openTags, subCat bool, w, h int) {
		a := squareTestApp()
		a.open(route{page: pageSquare})
		tt := ui.NewTester(a.view, w, h)
		tt.SetDark(dark)
		tt.Frame()
		if subCat {
			a.selectSquareCategory(3317, 3317, "官方歌单", false)
			tt.Frame()
			a.selectSquareCategory(3403, 3317, "华语", false)
			tt.Frame()
		}
		if openTags {
			a.square.tagsOpen = true
			tt.Frame()
		}
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}

	saveShot("square-light.png", false, false, false, 1180, 780)
	saveShot("square-dark.png", true, false, false, 1180, 780)
	saveShot("square-popover-light.png", false, true, false, 1180, 780)
	saveShot("square-subcat-dark.png", true, false, true, 1180, 780)
	saveShot("square-narrow-light.png", false, false, true, 880, 720)
}
