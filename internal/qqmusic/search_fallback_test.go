package qqmusic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A rejected main endpoint falls back once, skips its cooldown and pins later pages to the successful source.
func TestSearchFallbackAndPinnedPages(t *testing.T) {
	mainCalls, legacyCalls := 0, 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/musicu.fcg" {
			mainCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 2001}})
			return
		}
		legacyCalls++
		if r.Method != http.MethodGet || r.URL.Path != "/soso/fcgi-bin/client_search_cp" || r.URL.Query().Get("new_json") != "1" || r.URL.Query().Get("n") != "20" {
			t.Error("wrong fallback request")
		}
		page := r.URL.Query().Get("p")
		if page != "1" && page != "2" {
			t.Errorf("wrong fallback page %q", page)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"song": map[string]any{"totalnum": 60, "list": []any{map[string]any{"id": 8101, "mid": "fallback-song", "title": "Song", "interval": 180, "file": map[string]any{"media_mid": "fallback-media", "size_128mp3": 123456}}}}}})
	})
	first, err := SearchPage("Song", SearchSongs, 20, 0)
	if err != nil || first.Source != SearchLegacy || first.NextOffset != 20 || first.Len() != 1 {
		t.Fatalf("fallback: %+v, %v", first, err)
	}
	if mainCalls != 1 || legacyCalls != 1 {
		t.Fatalf("requests main=%d legacy=%d", mainCalls, legacyCalls)
	}
	if _, err := SearchPage("Another", SearchSongs, 20, 0); err != nil {
		t.Fatal(err)
	}
	if mainCalls != 1 || legacyCalls != 2 {
		t.Fatalf("cooldown did not skip main: %d/%d", mainCalls, legacyCalls)
	}
	if next, err := SearchPageSource("Song", SearchSongs, 20, first.NextOffset, first.Source); err != nil || next.NextOffset != 40 || next.Source != SearchLegacy {
		t.Fatalf("pinned page: %+v, %v", next, err)
	}
	if mainCalls != 1 || legacyCalls != 3 {
		t.Fatalf("pinned page changed source: %d/%d", mainCalls, legacyCalls)
	}
	_, err = SearchPageSource("Song", SearchSongs, 20, 20, SearchLite)
	var cooling *SearchCooldownError
	if !errors.As(err, &cooling) || legacyCalls != 3 {
		t.Fatalf("pinned cooling source silently switched: %v", err)
	}
}

// Legacy album and artist searches use their own type codes and resource IDs, then keep their source and latest total when paging.
func TestLegacyAlbumAndArtistFallback(t *testing.T) {
	for _, tc := range []struct {
		name, code, key string
		kind            SearchType
		entry           map[string]any
	}{
		{"albums", "8", "album", SearchAlbums, map[string]any{"docid": "987654321", "albumID": 87495226, "albumMID": "legacy-album", "albumName": "Album", "albumPic": "https://example.com/album.jpg", "publicTime": "2022-07-15", "song_count": 12, "singer_list": []any{map[string]any{"id": 4558, "mid": "legacy-artist", "name": "Artist"}}}},
		{"artists", "9", "singer", SearchArtists, map[string]any{"docid": "987654322", "singerID": 4558, "singerMID": "legacy-artist", "singerName": "Artist", "singerPic": "https://example.com/artist.jpg", "songNum": 120, "albumNum": 15}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mainCalls, legacyCalls := 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/cgi-bin/musicu.fcg" {
					mainCalls++
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 2001}})
					return
				}
				legacyCalls++
				q := r.URL.Query()
				if q.Get("t") != tc.code || q.Get("p") != map[int]string{1: "1", 2: "2"}[legacyCalls] || q.Get("n") != "5" {
					t.Errorf("incorrect legacy type or page: %v", q)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{tc.key: map[string]any{"totalnum": 5 + legacyCalls*2, "list": []any{tc.entry}}}})
			})
			first, err := SearchPage("Artist", tc.kind, 5, 0)
			if err != nil || first.Source != SearchLegacy || first.Len() != 1 || first.NextOffset != 5 || first.Total != 7 {
				t.Fatalf("fallback category: %+v %v", first, err)
			}
			if tc.kind == SearchAlbums {
				a := first.Albums[0]
				if a.ID != 87495226 || a.MID != "legacy-album" || a.Name != "Album" || a.PicURL != "https://example.com/album.jpg" || a.Size != 12 || a.PublishTime != time.Date(2022, 7, 15, 0, 0, 0, 0, time.UTC).UnixMilli() || len(a.Artists) != 1 || a.Artists[0].ID != 4558 || albumMID(a.ID) != a.MID {
					t.Fatalf("legacy album lost details or registered a document ID: %+v", a)
				}
			} else {
				a := first.Artists[0]
				if a.ID != 4558 || a.MID != "legacy-artist" || a.Name != "Artist" || a.Avatar != "https://example.com/artist.jpg" || a.MusicSize != 120 || a.AlbumSize != 15 || artistMID(a.ID) != a.MID {
					t.Fatalf("legacy artist lost details or registered a document ID: %+v", a)
				}
			}
			next, err := SearchPageSource("Artist", tc.kind, 5, first.NextOffset, first.Source)
			if err != nil || next.Source != SearchLegacy || next.NextOffset != 10 {
				t.Fatalf("pinned category page: %+v %v", next, err)
			}
			first.Append(next)
			if first.Total != 9 || first.NextOffset != 10 || mainCalls != 1 || legacyCalls != 2 {
				t.Fatalf("category paging lost latest total or changed source: %+v main=%d legacy=%d", first, mainCalls, legacyCalls)
			}
		})
	}
}

// A zero-result first page is valid, but a continuation losing its entire total must preserve the existing search.
func TestLegacyEmptyFirstPageAndLostContinuation(t *testing.T) {
	calls := 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"song": map[string]any{"totalnum": 0, "list": []any{}}}})
	})
	for range 2 {
		result, err := SearchPageSource("No matches", SearchSongs, 20, 0, SearchLegacy)
		if err != nil || result.Len() != 0 || result.Total != 0 {
			t.Fatalf("genuine empty first page rejected: %+v %v", result, err)
		}
	}
	if calls != 2 {
		t.Fatal("empty first page was incorrectly cooled down")
	}
	if _, err := SearchPageSource("Prior matches", SearchSongs, 20, 20, SearchLegacy); err == nil {
		t.Fatal("empty continuation silently erased the prior total")
	}
	_, err := SearchPageSource("Prior matches", SearchSongs, 20, 20, SearchLegacy)
	var cooling *SearchCooldownError
	if !errors.As(err, &cooling) || calls != 3 {
		t.Fatalf("inconsistent continuation did not cool down: %v calls=%d", err, calls)
	}
}

// Cancelling a formal search reaches the HTTP request and does not trigger backup traffic or poison endpoint recovery.
func TestSearchContextCancelsHTTPWithoutFallback(t *testing.T) {
	started, aborted := make(chan struct{}), make(chan struct{})
	var mainCalls, legacyCalls atomic.Int32
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/musicu.fcg" {
			legacyCalls.Add(1)
			t.Error("cancelled search used fallback")
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		if mainCalls.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			close(aborted)
			return
		}
		rpcReply(w, map[string]any{"body": map[string]any{"song": map[string]any{"list": []any{map[string]any{"id": 8105, "mid": "after-cancel", "name": "Song"}}}}, "meta": map[string]any{"sum": 1}})
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := SearchPageContext(ctx, "Song", SearchSongs, 20, 0, SearchAuto); done <- err }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("search did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("search cancellation blocked")
	}
	select {
	case <-aborted:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP request was not cancelled")
	}
	result, err := SearchPage("Song", SearchSongs, 20, 0)
	if err != nil || result.Len() != 1 || mainCalls.Load() != 2 || legacyCalls.Load() != 0 {
		t.Fatalf("cancel poisoned next search: %+v %v main=%d backup=%d", result, err, mainCalls.Load(), legacyCalls.Load())
	}
}

// Genuine empty results must remain empty instead of consuming another endpoint or opening a cooldown.
func TestSearchEmptyDoesNotFailOver(t *testing.T) {
	calls := 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/cgi-bin/musicu.fcg" {
			t.Error("empty result incorrectly triggered fallback")
		}
		rpcReply(w, map[string]any{"code": 0, "body": map[string]any{"song": map[string]any{"list": []any{}}}, "meta": map[string]any{"sum": 0, "is_filter": 0}})
	})
	for range 2 {
		result, err := SearchPage("No matches", SearchSongs, 20, 0)
		if err != nil || result.Len() != 0 || result.Source != preferredSearchSource {
			t.Fatalf("empty result: %+v %v", result, err)
		}
	}
	if calls != 2 {
		t.Fatalf("sent %d requests for two empty searches", calls)
	}
}

// Paging advances by upstream page size even when a malformed identity is filtered out.
func TestSearchNextOffsetUsesUpstreamPage(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		rpcReply(w, map[string]any{"body": map[string]any{"item_song": []any{map[string]any{"id": 8102, "mid": "page-song", "name": "Song"}, map[string]any{"name": "unusable"}}}, "meta": map[string]any{"sum": 100}})
	})
	result, err := SearchPageSource("Song", SearchSongs, 20, 20, SearchLite)
	if err != nil || result.Len() != 1 || result.NextOffset != 40 {
		t.Fatalf("page cursor followed decoded row count: %+v %v", result, err)
	}
}

// Automatic and pinned reads of the same backend page share one HTTP operation.
func TestSearchCoalescesAutomaticAndPinnedPage(t *testing.T) {
	engine := newSearchEngine()
	engine.interval = 0
	var calls atomic.Int32
	release := make(chan struct{})
	engine.fetch = func(ctx context.Context, source SearchSource, _ string, _ SearchType, _, _ int, _ Session) (SearchResult, error) {
		calls.Add(1)
		select {
		case <-ctx.Done():
			return SearchResult{}, ctx.Err()
		case <-release:
			return SearchResult{Total: 1}, nil
		}
	}
	done := make(chan error, 2)
	for _, source := range []SearchSource{SearchAuto, preferredSearchSource} {
		go func() {
			result, err := engine.search(context.Background(), source, "Song", SearchSongs, 20, 0, Session{}, "anonymous")
			if err == nil && result.Source != preferredSearchSource {
				err = errors.New("wrong source")
			}
			done <- err
		}()
	}
	waitGroupWaiters(t, &engine.requests, searchRequestKey("anonymous", preferredSearchSource, "Song", SearchSongs, 20, 0), 2)
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("identical backend page fetched %d times", calls.Load())
	}
}

// Sparse backup records preserve known media fields and carry their need for details through queue JSON.
func TestSparseSearchPlaybackMetadata(t *testing.T) {
	// Each run starts with an unresolved record, even after a previous -count iteration resolved it.
	mediaRegistry.Lock()
	delete(mediaRegistry.tracks, 8104)
	mediaRegistry.Unlock()
	RememberTracks([]Track{{ID: 8103, MID: "known-search-song", MediaMID: "known-search-media", Duration: 180000, Size128: 1234, Size320: 5678, Volume: &Loudness{Gain: -7.603, Peak: .946}}})
	known := searchTracks([]object{obj(json.RawMessage(`{"id":8103,"mid":"known-search-song","name":"Song"}`))})
	if len(known) != 1 || known[0].NeedsDetails || known[0].MediaMID != "known-search-media" || known[0].Duration != 180000 || known[0].Size320 != 5678 || known[0].Volume == nil {
		t.Fatalf("sparse record destroyed known playback metadata: %+v", known)
	}
	sparse := searchTracks([]object{obj(json.RawMessage(`{"id":8104,"mid":"sparse-search-song","name":"Song","interval":180}`))})
	b, _ := json.Marshal(sparse)
	var restored []Track
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || !restored[0].NeedsDetails {
		t.Fatal("restored sparse queue lost its detail requirement")
	}
	RememberTracks(restored)
	details, vkeys := 0, 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Req struct {
				Method string
				Param  map[string]any
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Req.Method == "get_song_detail_yqq" {
			details++
			rpcReply(w, map[string]any{"track_info": map[string]any{"id": 8104, "mid": "sparse-search-song", "title": "Song", "interval": 180, "file": map[string]any{"media_mid": "resolved-search-media", "size_128mp3": 1234}}})
			return
		}
		vkeys++
		filenames, _ := request.Req.Param["filename"].([]any)
		if len(filenames) != 1 || filenames[0] != "M500sparse-search-songresolved-search-media.mp3" {
			t.Errorf("incomplete playback filename: %v", filenames)
		}
		rpcReply(w, map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": "sparse-search-song", "purl": "file.mp3"}}})
	})
	stream, err := SongURL(8104, "standard")
	if err != nil || stream == nil || details != 1 || vkeys != 1 {
		t.Fatalf("sparse playback: details=%d vkeys=%d stream=%v error=%v", details, vkeys, stream, err)
	}
}

// Endpoint recovery retries after 6, 12 and 24 seconds and a successful response resets the backoff.
func TestSearchGatePacingAndRecovery(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	gate := &searchGate{interval: time.Second}
	for range 3 {
		if delay, err := gate.reserve(now); delay != 0 || err != nil {
			t.Fatalf("initial burst: %v %v", delay, err)
		}
	}
	if delay, err := gate.reserve(now.Add(500 * time.Millisecond)); err != nil || delay != 500*time.Millisecond {
		t.Fatalf("pacing: %v %v", delay, err)
	}
	if delay, err := gate.reserve(now.Add(time.Second)); err != nil || delay != 0 {
		t.Fatalf("refill: %v %v", delay, err)
	}
	cause := &ProtocolError{Code: 2001}
	for _, delay := range []time.Duration{6 * time.Second, 12 * time.Second, 24 * time.Second, 24 * time.Second} {
		gate.failed(now, cause)
		_, err := gate.reserve(now.Add(delay - time.Second))
		var cooling *SearchCooldownError
		if !errors.As(err, &cooling) || cooling.RetryAfter != time.Second {
			t.Fatalf("cooldown %v: %v", delay, err)
		}
		if _, err := gate.reserve(now.Add(delay)); err != nil {
			t.Fatalf("did not recover after %v: %v", delay, err)
		}
		now = now.Add(delay)
	}
	gate.succeeded(now)
	gate.failed(now, cause)
	if !gate.until.Equal(now.Add(6 * time.Second)) {
		t.Fatal("successful recovery did not reset backoff")
	}
}

// A late success from the same burst must not erase a newer endpoint rejection.
func TestSearchGateLateSuccessPreservesCooldown(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	gate := &searchGate{}
	gate.failed(now, &ProtocolError{Code: 2001})
	gate.succeeded(now.Add(time.Second))
	if _, err := gate.reserve(now.Add(2 * time.Second)); err == nil {
		t.Fatal("old in-flight success cleared the active cooldown")
	}
	gate.succeeded(now.Add(6 * time.Second))
	if gate.strikes != 0 || !gate.until.IsZero() {
		t.Fatal("successful request after cooldown did not recover")
	}
}

// A failed anonymous endpoint must not prevent a newly signed-in account from trying its own search.
func TestSearchCooldownScopeAndCancellation(t *testing.T) {
	engine := newSearchEngine()
	engine.interval = 0
	var calls []SearchSource
	engine.fetch = func(ctx context.Context, source SearchSource, _ string, _ SearchType, _, _ int, _ Session) (SearchResult, error) {
		calls = append(calls, source)
		return SearchResult{}, &ProtocolError{Code: 2001}
	}
	_, _ = engine.search(context.Background(), SearchLite, "Song", SearchSongs, 20, 0, Session{}, "anonymous")
	if len(calls) != 1 {
		t.Fatal("first endpoint was not called")
	}
	_, _ = engine.search(context.Background(), SearchLite, "Song", SearchSongs, 20, 0, Session{}, "account:fixture")
	if len(calls) != 2 {
		t.Fatal("anonymous cooldown blocked the signed-in scope")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := engine.search(ctx, SearchAuto, "Song", SearchSongs, 20, 0, Session{}, "cancelled")
	if !errors.Is(err, context.Canceled) || len(calls) != 2 {
		t.Fatalf("cancelled search sent a request: %v calls=%d", err, len(calls))
	}
}

// Desktop uses ct=11 with wrapped lists for all three categories, independently of the Lite identity.
func TestDesktopSearchCategories(t *testing.T) {
	for _, signedIn := range []bool{false, true} {
		for _, tc := range []struct {
			name, key string
			kind      SearchType
			code      int
			item      map[string]any
		}{
			{"songs", "song", SearchSongs, 0, map[string]any{"id": 8201, "mid": "desktop-song", "name": "Song", "file": map[string]any{"media_mid": "desktop-media"}}},
			{"artists", "singer", SearchArtists, 1, map[string]any{"id": 8202, "mid": "desktop-artist", "name": "Artist"}},
			{"albums", "album", SearchAlbums, 2, map[string]any{"id": 8203, "mid": "desktop-album", "name": "Album"}},
		} {
			t.Run(tc.name+map[bool]string{false: " anonymous", true: " account"}[signedIn], func(t *testing.T) {
				withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
					var request struct {
						Comm map[string]any
						Req  struct {
							Method string
							Param  map[string]any
						} `json:"req_0"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if request.Req.Method != "DoSearchForQQMusicDesktop" || request.Comm["ct"] != float64(11) || request.Comm["cv"] != float64(0) || request.Req.Param["search_type"] != float64(tc.code) || request.Req.Param["page_num"] != float64(2) || request.Req.Param["num_per_page"] != float64(20) {
						t.Error("incorrect Desktop identity or paging")
					}
					if signedIn {
						if request.Comm["uin"] != "123" || request.Comm["authst"] != "desktop-key" || !strings.Contains(r.Header.Get("Cookie"), "qm_keyst=desktop-key") {
							t.Error("Desktop lost account identity")
						}
					} else if request.Comm["uin"] != "0" || request.Comm["authst"] != nil || r.Header.Get("Cookie") != "tmeLoginType=-1" {
						t.Error("Desktop anonymous request inherited account state")
					}
					rpcReply(w, map[string]any{"code": 0, "body": map[string]any{tc.key: map[string]any{"list": []any{tc.item}}}, "meta": map[string]any{"sum": 61, "is_filter": 0}})
				})
				if signedIn {
					setSession(Session{MusicID: "123", MusicKey: "desktop-key", LoginType: 2})
				}
				result, err := SearchPage("Jay", tc.kind, 20, 20)
				if err != nil || result.Source != SearchDesktop || result.Len() != 1 || result.Total != 61 || result.NextOffset != 40 {
					t.Fatalf("Desktop result: %+v %v", result, err)
				}
			})
		}
	}
}

// Lite search preserves account identifiers as strings and uses anonymity only without a valid local session.
func TestSearchSessionSelection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		session       Session
		authenticated bool
	}{
		{"QQ", Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 2}, true},
		{"WeChat large ID", Session{MusicID: "9007199254740993", MusicKey: "fixture-key", LoginType: 1, Device: &wechatDevice{Qimei: "fixture-device"}}, true},
		{"expired", Session{MusicID: "123", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1}, false},
		{"missing key", Session{MusicID: "123", LoginType: 2}, false},
		{"invalid login type", Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 9}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct{ Param map[string]any } `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if tc.authenticated {
					if request.Comm["ct"] != float64(24) || request.Comm["uin"] != tc.session.MusicID || request.Comm["qq"] != tc.session.MusicID || request.Comm["authst"] != tc.session.MusicKey || request.Comm["tmeLoginType"] != float64(tc.session.LoginType) || request.Req.Param["search_type"] != float64(0) {
						t.Error("account search did not preserve the PC identity")
					}
				} else if request.Comm["tmeAppID"] != "qqmusiclight" || request.Comm["authst"] != nil || r.Header.Get("Cookie") != "tmeLoginType=-1" || request.Req.Param["search_type"] != float64(7) {
					t.Error("anonymous search sent an invalid or expired account")
				}
				rpcReply(w, map[string]any{"code": 0, "body": map[string]any{"item_song": []any{map[string]any{"id": 101, "mid": "songMid", "title": "Song"}}}, "meta": map[string]any{"sum": 1}})
			})
			setSession(tc.session)
			result, err := SearchPageSource("Song", SearchSongs, 20, 0, SearchLite)
			if err != nil || result.Len() != 1 {
				t.Fatalf("search failed: rows=%d error=%v", result.Len(), err)
			}
			if sessionSnapshot() != tc.session {
				t.Fatal("search changed the account session")
			}
		})
	}
}

type groupTestResult struct {
	value int
	err   error
}

func waitGroupWaiters[T any](t *testing.T, group *requestGroup[T], key string, want int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		group.mu.Lock()
		call := group.calls[key]
		joined := call != nil && call.waiters == want
		group.mu.Unlock()
		if joined {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("request did not acquire %d waiting callers", want)
		default:
			runtime.Gosched()
		}
	}
}

func receiveGroupResult(t *testing.T, results <-chan groupTestResult) groupTestResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete")
		return groupTestResult{}
	}
}

// Concurrent identical reads share one load, and a completed result is not retained by the group.
func TestRequestGroupSharesInFlight(t *testing.T) {
	var group requestGroup[int]
	var loads atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	load := func(context.Context) (int, error) {
		loads.Add(1)
		close(started)
		<-finish
		return 42, nil
	}
	results := make(chan groupTestResult, 2)
	go func() { v, err := group.do(context.Background(), "query", load); results <- groupTestResult{v, err} }()
	<-started
	go func() { v, err := group.do(context.Background(), "query", load); results <- groupTestResult{v, err} }()
	waitGroupWaiters(t, &group, "query", 2)
	close(finish)
	for range 2 {
		if got := receiveGroupResult(t, results); got.value != 42 || got.err != nil {
			t.Fatalf("shared result: %+v", got)
		}
	}
	value, err := group.do(context.Background(), "query", func(context.Context) (int, error) { loads.Add(1); return 43, nil })
	if value != 43 || err != nil || loads.Load() != 2 {
		t.Fatalf("next read: value=%d err=%v loads=%d", value, err, loads.Load())
	}
}

// Canceling one caller must not abort a shared read that another caller still needs.
func TestRequestGroupCancellationKeepsOtherWaiter(t *testing.T) {
	var group requestGroup[int]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan context.Context, 1)
	finish := make(chan struct{})
	load := func(shared context.Context) (int, error) { started <- shared; <-finish; return 42, nil }
	first, second := make(chan groupTestResult, 1), make(chan groupTestResult, 1)
	go func() { v, err := group.do(ctx, "query", load); first <- groupTestResult{v, err} }()
	shared := <-started
	go func() { v, err := group.do(context.Background(), "query", load); second <- groupTestResult{v, err} }()
	waitGroupWaiters(t, &group, "query", 2)
	cancel()
	if got := receiveGroupResult(t, first); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled caller: %+v", got)
	}
	select {
	case <-shared.Done():
		t.Fatal("one caller canceled the other caller's read")
	default:
	}
	close(finish)
	if got := receiveGroupResult(t, second); got.value != 42 || got.err != nil {
		t.Fatalf("remaining caller: %+v", got)
	}
}

// The last caller cancels its load, and that old load cannot remove a newer read for the same query.
func TestRequestGroupLastCancelAllowsFreshRead(t *testing.T) {
	var group requestGroup[int]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldStarted, oldFinish := make(chan context.Context, 1), make(chan struct{})
	oldResult := make(chan groupTestResult, 1)
	go func() {
		v, err := group.do(ctx, "query", func(shared context.Context) (int, error) {
			oldStarted <- shared
			<-shared.Done()
			<-oldFinish
			return 0, shared.Err()
		})
		oldResult <- groupTestResult{v, err}
	}()
	oldContext := <-oldStarted
	group.mu.Lock()
	oldCall := group.calls["query"]
	group.mu.Unlock()
	cancel()
	if got := receiveGroupResult(t, oldResult); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled last caller: %+v", got)
	}
	select {
	case <-oldContext.Done():
	default:
		t.Fatal("last caller left its load running")
	}
	started, finish := make(chan struct{}), make(chan struct{})
	var freshLoads atomic.Int32
	load := func(context.Context) (int, error) { freshLoads.Add(1); close(started); <-finish; return 99, nil }
	fresh := make(chan groupTestResult, 2)
	go func() { v, err := group.do(context.Background(), "query", load); fresh <- groupTestResult{v, err} }()
	<-started
	close(oldFinish)
	<-oldCall.done
	go func() { v, err := group.do(context.Background(), "query", load); fresh <- groupTestResult{v, err} }()
	waitGroupWaiters(t, &group, "query", 2)
	close(finish)
	for range 2 {
		if got := receiveGroupResult(t, fresh); got.value != 99 || got.err != nil {
			t.Fatalf("fresh result: %+v", got)
		}
	}
	if freshLoads.Load() != 1 {
		t.Fatalf("fresh read loaded %d times", freshLoads.Load())
	}
}

// Already canceled callers do not start work, and failed reads remain retryable.
func TestRequestGroupDoesNotKeepErrors(t *testing.T) {
	var group requestGroup[int]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if _, err := group.do(ctx, "query", func(context.Context) (int, error) { called = true; return 0, nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("already canceled read: called=%t err=%v", called, err)
	}
	want := errors.New("temporary")
	if _, err := group.do(context.Background(), "query", func(context.Context) (int, error) { return 0, want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if value, err := group.do(context.Background(), "query", func(context.Context) (int, error) { return 7, nil }); value != 7 || err != nil {
		t.Fatalf("retry: value=%d err=%v", value, err)
	}
}
