package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

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
