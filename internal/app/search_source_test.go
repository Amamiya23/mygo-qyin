package app

import (
	"errors"
	"testing"
	"time"

	"lilt/internal/qqmusic"
)

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
