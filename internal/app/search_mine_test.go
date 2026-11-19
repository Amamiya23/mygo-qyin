package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Searching the listener's playlists finds the songs with every word of the
// query in their name, alias, artists or album, in the playlists' order,
// each once, ignoring case.
func TestMineMatches(t *testing.T) {
	mygo := []qqmusic.Artist{{Name: "MyGO!!!!!"}}
	liked := &qqmusic.Playlist{Tracks: []qqmusic.Track{
		{ID: 1, Name: "春日影", Artists: mygo, Album: qqmusic.Album{Name: "迷跡波"}},
		{ID: 2, Name: "Ether", Artists: []qqmusic.Artist{{Name: "Ave Mujica"}}},
		{ID: 3, Name: "影色舞", Alias: []string{"Kageiro"}, Artists: mygo},
	}}
	other := &qqmusic.Playlist{Tracks: []qqmusic.Track{
		{ID: 4, Name: "碧天伴走", Artists: mygo, Album: qqmusic.Album{Name: "迷跡波"}},
		{ID: 1, Name: "春日影", Artists: mygo, Album: qqmusic.Album{Name: "迷跡波"}},
	}}
	lists := []*qqmusic.Playlist{liked, other}
	ids := func(q string) []int64 {
		var out []int64
		for _, tr := range mineMatches(lists, q) {
			out = append(out, tr.ID)
		}
		return out
	}
	for _, tc := range []struct {
		q    string
		want []int64
	}{
		{"影", []int64{1, 3}},
		{"mygo 迷跡波", []int64{1, 4}},
		{"KAGEIRO", []int64{3}},
		{"ave ether", []int64{2}},
		{"mygo ether", nil},
		{"  ", nil},
	} {
		if got := ids(tc.q); !slices.Equal(got, tc.want) {
			t.Errorf("%q found %v, want %v", tc.q, got, tc.want)
		}
	}
}

// The 我的歌单 tab loads the listener's playlists not loaded yet, lists the
// songs in them that match, and a click plays those songs.
func TestSearchMine(t *testing.T) {
	asked := make(chan int64, 4)
	mineDetail = func(id int64) (*qqmusic.Playlist, error) {
		asked <- id
		return &qqmusic.Playlist{ID: id}, nil
	}
	searchResults = func(string, qqmusic.SearchType, int, int) (qqmusic.SearchResult, error) {
		return qqmusic.SearchResult{}, nil
	}
	defer func() { mineDetail, searchResults = qqmusic.PlaylistDetail, qqmusic.SearchPage }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "灯"}
	a.mine = []qqmusic.Playlist{{ID: 10, Name: "我喜欢", SpecialType: 5}, {ID: 11, Name: "练习曲"}}
	a.playlists[10] = &qqmusic.Playlist{ID: 10, Tracks: []qqmusic.Track{
		{ID: 1, Name: "春日影", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Duration: 200000},
		{ID: 2, Name: "Ether", Artists: []qqmusic.Artist{{Name: "Ave Mujica"}}, Duration: 200000},
		{ID: 3, Name: "碧天伴走", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Duration: 200000},
	}}
	a.searchFor(route{page: pageSearch, query: "mygo", kind: qqmusic.SearchSongs})
	// The page alone: the sidebar's section of the same name would take the click.
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() { a.content(c) })
	}, 1200, 800)
	tt.Frame()

	if err := tt.Click("我的歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if id := wait(t, asked, "the playlist not loaded"); id != 11 {
		t.Fatalf("loaded playlist %d", id)
	}
	// As the playlist lands, which post drops without a window.
	a.playlists[11] = &qqmusic.Playlist{ID: 11, Tracks: []qqmusic.Track{
		{ID: 4, Name: "迷星叫", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Duration: 200000},
	}}
	a.loadSearch(a.route, true)
	tt.Frame()
	if len(asked) > 0 {
		t.Error("a loaded playlist was asked for again")
	}

	if err := tt.Click("碧天伴走"); err != nil {
		t.Fatal(err)
	}
	var queue []int64
	for _, tr := range a.pl.Queue() {
		queue = append(queue, tr.ID)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != 3 || !slices.Equal(queue, []int64{1, 3, 4}) {
		t.Fatalf("playing %+v of %v", cur, queue)
	}
}

// A failed first page used to stay cached when the listener searched for it again from another page.
func TestSearchRetriesFailedFirstPage(t *testing.T) {
	for _, history := range []bool{false, true} {
		name := "submit"
		if history {
			name = "history"
		}
		t.Run(name, func(t *testing.T) {
			asked := fakeSearch(t)
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.SearchHistory = []string{"春日影"}
			r := route{page: pageSearch, query: "春日影", kind: qqmusic.SearchSongs}
			failed := &searchData{page: paged{err: "搜索失败，请稍后再试"}}
			a.searches[r] = failed
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			if err := tt.Click("搜索"); err != nil {
				t.Fatal(err)
			}
			if history {
				if err := tt.Click("春日影"); err != nil {
					t.Fatal(err)
				}
			} else {
				tt.Type("春日影")
				tt.Key(0, ui.KeyEnter)
			}
			tt.Frame()
			d := a.searches[r]
			if d == failed || !d.page.loading || d.page.err != "" {
				t.Fatal("search reused the failed first page instead of requesting it again")
			}
			if got := wait(t, asked, "the failed search again"); got != (searchRequest{"春日影", qqmusic.SearchSongs, 0}) {
				t.Fatalf("searched %+v", got)
			}
		})
	}
}

// Returning to a failed search category retries it, while the already loaded category stays cached.
func TestSearchKindRetriesFailedFirstPage(t *testing.T) {
	asked := fakeSearch(t)
	a := newTestApp()
	a.settings = defaultSettings()
	songs := route{page: pageSearch, query: "春日影", kind: qqmusic.SearchSongs}
	artists := songs
	artists.kind = qqmusic.SearchArtists
	failed := &searchData{page: paged{err: "搜索失败，请稍后再试"}}
	loaded := &searchData{page: paged{loaded: true}}
	a.searches[songs], a.searches[artists] = failed, loaded
	a.route, a.searched, a.query = songs, songs, songs.query
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != artists || a.searches[artists] != loaded {
		t.Fatal("switching category discarded its loaded results")
	}
	if err := tt.Click("单曲"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if d := a.searches[songs]; d == failed || !d.page.loading || d.page.err != "" {
		t.Fatal("returning to a category reused its failed first page")
	}
	if got := wait(t, asked, "the failed category again"); got != (searchRequest{"春日影", qqmusic.SearchSongs, 0}) {
		t.Fatalf("searched %+v", got)
	}
}

// Navigating to loaded results or an in-flight first page reuses that search rather than starting another request.
func TestSearchReusesLoadedAndLoadingPages(t *testing.T) {
	fakeSearch(t)
	for _, pg := range []paged{{loaded: true}, {loading: true}} {
		a := newTestApp()
		r := route{page: pageSearch, query: "春日影", kind: qqmusic.SearchSongs}
		d := &searchData{page: pg}
		a.searches[r] = d
		a.show(r)
		if a.searches[r] != d {
			t.Fatalf("navigation replaced a cached search: %+v", pg)
		}
	}
}

// Retrying a later page keeps the songs already loaded and requests from their end.
func TestSearchPageRetryKeepsResults(t *testing.T) {
	asked := fakeSearch(t)
	a := newTestApp()
	a.settings = defaultSettings()
	r := route{page: pageSearch, query: "春日影", kind: qqmusic.SearchSongs}
	d := &searchData{
		res:  qqmusic.SearchResult{Songs: []qqmusic.Track{{ID: 1, Name: "春日影"}, {ID: 2, Name: "迷星叫"}}, Total: 40},
		page: paged{loaded: true, more: true, err: "搜索失败，请稍后再试"},
	}
	a.searches[r] = d
	a.show(r)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.searches[r] != d || len(d.res.Songs) != 2 || d.res.Songs[0].ID != 1 || !d.page.loading || d.page.err != "" {
		t.Fatal("retrying a later page discarded the songs already loaded")
	}
	if got := wait(t, asked, "the failed next page again"); got != (searchRequest{"春日影", qqmusic.SearchSongs, 2}) {
		t.Fatalf("searched %+v", got)
	}
}

type sourceSearchRequest struct {
	query         string
	kind          qqmusic.SearchType
	limit, offset int
	source        qqmusic.SearchSource
}

// Filtering unusable rows must not move pagination off its upstream boundary or switch the chosen backend.
func TestSearchPaginationPinsSourceAndOffset(t *testing.T) {
	oldSearch, oldFrom := searchResults, searchResultsFrom
	t.Cleanup(func() { searchResults, searchResultsFrom = oldSearch, oldFrom })
	called := make(chan sourceSearchRequest, 4)
	searchResults = func(q string, kind qqmusic.SearchType, limit, offset int) (qqmusic.SearchResult, error) {
		called <- sourceSearchRequest{q, kind, limit, offset, qqmusic.SearchAuto}
		return qqmusic.SearchResult{}, errors.New("offline")
	}
	searchResultsFrom = func(q string, kind qqmusic.SearchType, limit, offset int, source qqmusic.SearchSource) (qqmusic.SearchResult, error) {
		called <- sourceSearchRequest{q, kind, limit, offset, source}
		return qqmusic.SearchResult{}, errors.New("offline")
	}
	a := newTestApp()
	r := route{page: pageSearch, query: "晴天", kind: qqmusic.SearchSongs}
	d := &searchData{}
	first := qqmusic.SearchResult{Source: qqmusic.SearchLegacy, NextOffset: 20, Total: 40}
	for i := range 18 {
		first.Songs = append(first.Songs, qqmusic.Track{ID: int64(i + 1), Name: "Song"})
	}
	d.finishPage(first, nil)
	if len(d.res.Songs) != 18 || d.res.Source != qqmusic.SearchLegacy || d.res.NextOffset != 20 || !d.page.more {
		t.Fatal("first page lost its source or raw page boundary")
	}
	a.moreResults(r, d)
	want := sourceSearchRequest{"晴天", qqmusic.SearchSongs, searchLimit, 20, qqmusic.SearchLegacy}
	if got := wait(t, called, "pinned next page"); got != want {
		t.Fatalf("requested %+v, want %+v", got, want)
	}
	// An error arriving after the first page must keep its rows, cursor and source.
	d.finishPage(qqmusic.SearchResult{}, &qqmusic.SearchCooldownError{RetryAfter: 6 * time.Second, Cause: errors.New("server rejected")})
	if len(d.res.Songs) != 18 || d.res.Source != qqmusic.SearchLegacy || d.res.NextOffset != 20 || d.page.loading || !d.page.loaded || !d.page.more || d.page.err == "" {
		t.Fatal("failed pagination discarded existing results or changed source")
	}
	a.moreResults(r, d)
	if got := wait(t, called, "pinned retry"); got != want {
		t.Fatalf("retried %+v, want %+v", got, want)
	}
	last := qqmusic.SearchResult{Source: qqmusic.SearchLegacy, NextOffset: 40, Total: 40, Songs: []qqmusic.Track{{ID: 19}, {ID: 20}}}
	d.finishPage(last, nil)
	if len(d.res.Songs) != 20 || d.res.NextOffset != 40 || d.page.more {
		t.Fatal("search kept paging by filtered row count after the upstream end")
	}
}

type suggestionRequest struct {
	query string
	ctx   context.Context
}

func cancelableSuggestions(t *testing.T) (chan suggestionRequest, chan string) {
	t.Helper()
	started, stopped := make(chan suggestionRequest, 8), make(chan string, 8)
	original := searchSuggest
	searchSuggest = func(ctx context.Context, q string) (qqmusic.Suggestions, error) {
		started <- suggestionRequest{q, ctx}
		<-ctx.Done()
		stopped <- q
		return qqmusic.Suggestions{}, ctx.Err()
	}
	t.Cleanup(func() { searchSuggest = original })
	return started, stopped
}

// Editing, dismissing, leaving or submitting the search cancels an already running suggestion request.
func TestSuggestionRequestCancellation(t *testing.T) {
	for _, action := range []string{"edit", "escape", "blur", "submit", "clear"} {
		t.Run(action, func(t *testing.T) {
			queries := fakeSearch(t)
			started, stopped := cancelableSuggestions(t)
			a := newTestApp()
			a.settings = defaultSettings()
			t.Cleanup(a.dismissSuggestions)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			if err := tt.Click("搜索"); err != nil {
				t.Fatal(err)
			}
			tt.Type("晴")
			tt.Frame()
			req := wait(t, started, "suggestions")
			if req.query != "晴" {
				t.Fatalf("suggestions for %q", req.query)
			}
			switch action {
			case "edit":
				tt.Type("天")
			case "escape":
				tt.Key(0, ui.KeyEscape)
			case "blur":
				tt.ClickAt(1100, 650)
			case "submit":
				tt.Key(0, ui.KeyEnter)
			case "clear":
				tt.Key(ui.Cmd, ui.KeyA)
				tt.Key(0, ui.KeyBackspace)
			}
			tt.Frame()
			if action == "submit" {
				if got := wait(t, queries, "submitted search"); got.query != "晴" {
					t.Fatalf("searched %q", got.query)
				}
			}
			if q := wait(t, stopped, "canceled suggestions"); q != "晴" {
				t.Fatalf("canceled %q", q)
			}
			select {
			case <-req.ctx.Done():
			default:
				t.Fatal("obsolete suggestion context remains active")
			}
		})
	}
}

// Returning to the same word must not let an older success or failure overwrite its new request.
func TestSuggestionReplyAfterQueryReentered(t *testing.T) {
	original := searchSuggest
	searchSuggest = func(context.Context, string) (qqmusic.Suggestions, error) { return qqmusic.Suggestions{}, nil }
	t.Cleanup(func() { searchSuggest = original })
	a := newTestApp()
	t.Cleanup(a.dismissSuggestions)
	a.suggest("晴天")
	old := a.suggestions.gen
	a.suggest("春日影")
	a.suggest("晴天")
	current := a.suggestions.gen
	if a.suggestions.timer != nil {
		a.suggestions.timer.Stop()
	}
	stale := qqmusic.Suggestions{Songs: []qqmusic.Track{{MID: "old", Name: "old"}}}
	a.finishSuggestions(old, "晴天", stale, nil)
	a.finishSuggestions(old, "晴天", qqmusic.Suggestions{}, errors.New("old failure"))
	if !a.suggestions.loading || a.suggestions.failed || len(a.suggestions.items) != 0 {
		t.Fatal("obsolete reply changed the reentered query")
	}
	fresh := qqmusic.Suggestions{Songs: []qqmusic.Track{{MID: "new", Name: "new"}}}
	a.finishSuggestions(current, "晴天", fresh, nil)
	if a.suggestions.loading || a.suggestions.failed || len(a.suggestions.items) != 1 || a.suggestions.items[0].name != "new" {
		t.Fatal("the current reply was not applied")
	}
}

// Sparse smartbox songs without numeric IDs still select the right candidate instead of sharing song:0.
func TestSuggestionMIDIdentity(t *testing.T) {
	queries := fakeSearch(t)
	original := searchSuggest
	searchSuggest = func(context.Context, string) (qqmusic.Suggestions, error) { return qqmusic.Suggestions{}, nil }
	t.Cleanup(func() { searchSuggest = original })
	a := newTestApp()
	a.settings = defaultSettings()
	t.Cleanup(a.dismissSuggestions)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("晴")
	tt.Frame()
	a.suggestions.loading = false
	a.suggestions.items = suggestionList(qqmusic.Suggestions{Songs: []qqmusic.Track{{MID: "first", Name: "晴天"}, {MID: "second", Name: "春日影"}}})
	if len(a.suggestions.items) != 2 || a.suggestions.items[0].key == a.suggestions.items[1].key {
		t.Fatal("suggestions share an identity")
	}
	tt.Frame()
	if err := tt.Click("单曲：春日影"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, queries, "chosen song search"); got.query != "春日影" {
		t.Fatalf("searched %q", got.query)
	}
}
