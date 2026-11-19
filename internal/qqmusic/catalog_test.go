package qqmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type redirectTransport struct {
	target string
	base   http.RoundTripper
}

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	destination := *req.URL
	target, _ := http.NewRequest(http.MethodGet, r.target, nil)
	destination.Scheme = target.URL.Scheme
	destination.Host = target.URL.Host
	cloned.URL = &destination
	return r.base.RoundTrip(cloned)
}
func withQQServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	old := httpClient
	oldSession := sessionSnapshot()
	oldSearches := searches
	searches = newSearchEngine()
	searches.interval = 0
	setSession(Session{})
	httpClient = &http.Client{Transport: redirectTransport{server.URL, http.DefaultTransport}}
	t.Cleanup(func() { httpClient = old; searches = oldSearches; setSession(oldSession); server.Close() })
}
func rpcReply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 0, "data": data}})
}

// Anonymous Lite search retains the IDs needed by later playback.
func TestAnonymousSearchKeepsMID(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Comm map[string]any `json:"comm"`
			Req  struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Req.Method != "DoSearchForQQMusicLite" || request.Req.Param["search_type"] != float64(7) {
			t.Errorf("wrong search: %+v", request)
		}
		if request.Comm["tmeAppID"] != "qqmusiclight" || r.Header.Get("Cookie") != "tmeLoginType=-1" {
			t.Errorf("wrong anonymous identity")
		}
		id, _ := request.Req.Param["search_id"].(string)
		if len(id) != 18 || strings.ContainsAny(id, "abcdef") {
			t.Errorf("invalid search id %q", id)
		}
		rpcReply(w, map[string]any{"body": map[string]any{"item_song": []any{map[string]any{"id": 101, "mid": "songMid", "title": "Song", "interval": 180, "singer": []any{map[string]any{"id": 201, "mid": "artistMid", "name": "Artist"}}, "album": map[string]any{"id": 301, "mid": "albumMid", "name": "Album"}, "file": map[string]any{"media_mid": "mediaMid"}}}}, "meta": map[string]any{"sum": 23}})
	})
	result, err := SearchPageSource("Song", SearchSongs, 20, 0, SearchLite)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 23 || len(result.Songs) != 1 {
		t.Fatalf("unexpected result %+v", result)
	}
	song := result.Songs[0]
	if song.MID != "songMid" || song.MediaMID != "mediaMid" || song.Duration != 180000 || song.Artists[0].MID != "artistMid" || song.Album.MID != "albumMid" {
		t.Fatalf("lost song fields: %+v", song)
	}
}

// Logged-in search needs the PC identity; Android comm returned filtered empty results and forced anonymity was rejected.
func TestSearchUsesPCIdentityWithSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind SearchType
		code int
		key  string
		item string
	}{
		{"songs", SearchSongs, 0, "item_song", `{"id":101,"mid":"songMid","title":"Song"}`},
		{"artists", SearchArtists, 1, "singer", `{"id":201,"mid":"artistMid","name":"Artist"}`},
		{"albums", SearchAlbums, 2, "item_album", `{"id":301,"mid":"albumMid","name":"Album"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
				}
				if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["uin"] != "123" || request.Comm["qq"] != "123" || request.Comm["authst"] != "fixture-credential" || request.Comm["tmeLoginType"] != float64(2) || request.Comm["format"] != "json" || request.Comm["tmeAppID"] != nil || r.UserAgent() != UserAgent || !strings.Contains(r.Header.Get("Cookie"), "qm_keyst=fixture-credential") {
					t.Error("logged-in search did not use the verified PC identity")
					// The old Android identity returned success with this filtered empty body.
					rpcReply(w, map[string]any{"code": 0, "body": map[string]any{tc.key: []any{}}, "meta": map[string]any{"sum": 0, "is_filter": -9}})
					return
				}
				if request.Req.Method != "DoSearchForQQMusicLite" || request.Req.Param["search_type"] != float64(tc.code) || request.Req.Param["page_num"] != float64(2) || request.Req.Param["num_per_page"] != float64(20) || request.Req.Param["query"] != "周杰伦" {
					t.Error("wrong search kind, query or pagination")
				}
				rpcReply(w, map[string]any{"code": 0, "body": map[string]any{tc.key: []json.RawMessage{json.RawMessage(tc.item)}}, "meta": map[string]any{"sum": 45}})
			})
			setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
			result, err := SearchPageSource(" 周杰伦 ", tc.kind, 20, 20, SearchLite)
			if err != nil || result.Len() != 1 || result.Total != 45 {
				t.Fatalf("search became empty: length=%d total=%d error=%v", result.Len(), result.Total, err)
			}
			if sessionSnapshot().MusicKey != "fixture-credential" {
				t.Fatal("search changed the account session")
			}
		})
	}
}

// Inner search rejections and malformed lists used to appear as successful empty searches.
func TestSearchResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		ok   bool
	}{
		{"rejected", `{"code":2001,"body":{"singer":[]},"meta":{"sum":0}}`, false},
		{"invalid status", `{"code":"invalid","body":{"singer":[]},"meta":{"sum":0}}`, false},
		{"object list", `{"code":0,"body":{"singer":{}},"meta":{"sum":0}}`, false},
		{"null list", `{"code":0,"body":{"singer":null},"meta":{"sum":0}}`, false},
		{"null entry", `{"code":0,"body":{"singer":[null]},"meta":{"sum":1}}`, false},
		{"missing list", `{"code":0,"body":{},"meta":{"sum":0}}`, false},
		{"invalid meta", `{"code":0,"body":{"singer":[]},"meta":[]}`, false},
		{"filtered empty", `{"code":0,"body":{"singer":[]},"meta":{"sum":0,"is_filter":-9}}`, false},
		{"anonymous filtered empty", `{"code":0,"body":{"singer":[]},"meta":{"sum":0,"is_filter":-12}}`, false},
		{"empty search", `{"code":0,"body":{"singer":[]},"meta":{"sum":0}}`, true},
		{"unfiltered empty search", `{"code":0,"body":{"singer":[]},"meta":{"sum":0,"is_filter":0}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				rpcReply(w, json.RawMessage(tc.data))
			})
			result, err := SearchPageSource("周杰伦", SearchArtists, 20, 0, SearchLite)
			if (err == nil) != tc.ok {
				t.Fatalf("error=%v, want success=%v", err, tc.ok)
			}
			if tc.name == "rejected" {
				var protocol *ProtocolError
				if !errors.As(err, &protocol) || protocol.Code != 2001 {
					t.Fatalf("lost search rejection: %v", err)
				}
			}
			if tc.name == "filtered empty" || tc.name == "anonymous filtered empty" {
				var filtered *SearchFilterError
				if !errors.As(err, &filtered) || filtered.Filter >= 0 {
					t.Fatalf("lost server filtering: %v", err)
				}
			}
			if tc.ok && (result.Len() != 0 || result.Total != 0) {
				t.Fatalf("not an empty search: %+v", result)
			}
		})
	}
}

// A rejected RPC must be surfaced instead of turning into an empty successful search.
func TestRPCRejectsBusinessAndGatewayCodes(t *testing.T) {
	for _, body := range []string{`{"code":0,"req_0":{"code":2001,"data":{}}}`, `{"code":1000,"req_0":{"code":0,"data":{}}}`, `{"code":0,"req_0":{"code":0,"subcode":12,"data":{}}}`} {
		t.Run(body, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			var out object
			err := rpc("test", "read", map[string]any{}, &out)
			var protocol *ProtocolError
			if !errors.As(err, &protocol) {
				t.Fatalf("got %v, want protocol error", err)
			}
		})
	}
}

// Legacy song album IDs must not accidentally become the song's own ID.
func TestLegacyTrackAlbum(t *testing.T) {
	track := convertTrack(obj(json.RawMessage(`{"songid":777,"songmid":"legacySong","songname":"Song","albumid":888,"albummid":"legacyAlbum","albumname":"Album","interval":1}`)))
	if track.Album.ID != 888 || track.Album.Name != "Album" || track.Album.MID != "legacyAlbum" {
		t.Fatalf("wrong legacy album: %+v", track.Album)
	}
}

// Singer album responses use albumMid, which must survive to artwork and album navigation.
func TestSingerAlbumMID(t *testing.T) {
	album := convertAlbum(obj(json.RawMessage(`{"albumID":87495226,"albumMid":"fixtureAlbum","albumName":"Fixture"}`)))
	if album.ID != 87495226 || album.MID != "fixtureAlbum" || album.PicURL == "" {
		t.Fatalf("lost album MID: %+v", album)
	}
}

// Login used to send the mobile client identity to the web discography RPC, which rejects it with 104403.
func TestArtistAlbumsKeepsWebIdentity(t *testing.T) {
	const artistID = 62001
	convertArtist(obj(json.RawMessage(`{"id":62001,"mid":"fixtureSinger","name":"Singer"}`)))
	for _, account := range []struct {
		name    string
		session Session
	}{
		{"anonymous", Session{}},
		{"QQ", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2}},
		{"WeChat", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 1}},
	} {
		t.Run(account.name, func(t *testing.T) {
			expectedOffset := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]any
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["qq"] != nil || request.Comm["authst"] != nil {
					_, _ = io.WriteString(w, `{"code":0,"req_0":{"code":104403,"data":{"albumList":[],"total":0}}}`)
					return
				}
				if request.Req.Module != "music.musichallAlbum.AlbumListServer" || request.Req.Method != "GetAlbumList" || request.Req.Param["singermid"] != "fixtureSinger" || request.Req.Param["num"] != float64(30) || request.Req.Param["sort"] != float64(5) {
					t.Errorf("wrong discography request: %+v", request)
				}
				if offset := request.Req.Param["begin"]; offset != float64(expectedOffset) {
					t.Errorf("offset = %v, want %d", offset, expectedOffset)
				}
				rpcReply(w, json.RawMessage(`{"albumList":[{"albumID":62002,"albumMid":"fixtureAlbum","albumName":"Album","publishDate":"2026-10-30","totalNum":8}],"total":2}`))
			})
			setSession(account.session)
			for offset := 0; offset < 2; offset++ {
				expectedOffset = offset
				albums, more, err := ArtistAlbums(artistID, 30, offset)
				if err != nil {
					t.Fatal(err)
				}
				if len(albums) != 1 || albums[0].ID != 62002 || albums[0].MID != "fixtureAlbum" || albums[0].Name != "Album" || albums[0].PicURL == "" || albums[0].Size != 8 || albums[0].PublishTime == 0 || more != (offset == 0) {
					t.Fatalf("lost album metadata or pagination: %+v, more=%v", albums, more)
				}
			}
			if sessionSnapshot().MusicKey != account.session.MusicKey {
				t.Fatal("discography changed the account session")
			}
		})
	}
}

// A private owned playlist's empty public response must fall back to the signed owner RPC.
func TestPrivatePlaylistUsesOwnerRead(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"code":0,"cdlist":[]}`)
			return
		}
		var request struct {
			Req struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Req.Method != "CgiGetDiss" || request.Req.Param["disstid"] != float64(123) || request.Req.Param["enc_host_uin"] != "encrypted-owner" {
			t.Errorf("wrong owner read: %+v", request)
		}
		rpcReply(w, map[string]any{"dirinfo": map[string]any{"title": "Private"}, "total_song_num": 1, "songlist": []any{map[string]any{"id": 9, "mid": "privateSong", "title": "Track"}}, "hasmore": 0})
	})
	setSession(Session{MusicID: "123", MusicKey: "credential", LoginType: 2, EncryptUin: "encrypted-owner"})
	p, err := PlaylistDetail(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tracks) != 1 || p.Tracks[0].MID != "privateSong" {
		t.Fatalf("private playlist lost songs: %+v", p)
	}
}

// Inner playlist rejections and malformed lists must not appear as a successfully empty playlist.
func TestSessionPlaylistRejectsInnerStatus(t *testing.T) {
	for _, kind := range []string{"owner", "algorithmic"} {
		for _, body := range []string{`{"code":1000,"songlist":[]}`, `{"subcode":80120,"songlist":[]}`, `{"code":0}`, `{"code":0,"songlist":{}}`} {
			t.Run(kind+body, func(t *testing.T) {
				withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
					var data any
					_ = json.Unmarshal([]byte(body), &data)
					rpcReply(w, data)
				})
				setSession(Session{MusicID: "123", MusicKey: "credential", LoginType: 2, EncryptUin: "owner"})
				var err error
				if kind == "owner" {
					_, err = ownedPlaylist(123)
				} else {
					_, err = authenticatedPlaylist(123)
				}
				if err == nil {
					t.Fatal("rejected/malformed playlist accepted")
				}
			})
		}
	}
}

// A known owned list goes directly to its signed API because private public shells can omit totals.
func TestOwnedPlaylistBypassesPublicShell(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("owned playlist queried through public CGI")
			_, _ = io.WriteString(w, `{"code":0,"cdlist":[{"total_song_num":0,"songlist":[]}]}`)
			return
		}
		rpcReply(w, map[string]any{"dirinfo": map[string]any{"title": "Private"}, "total_song_num": 1, "songlist": []any{map[string]any{"id": 10, "mid": "ownedSong", "title": "Track"}}, "hasmore": 0})
	})
	setSession(Session{MusicID: "456", MusicKey: "credential", LoginType: 2, EncryptUin: "encrypted-owner"})
	RememberOwnedPlaylist(124)
	p, err := PlaylistDetail(124)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tracks) != 1 {
		t.Fatal("private playlist became empty")
	}
	setSession(Session{MusicID: "789", MusicKey: "other", LoginType: 2, EncryptUin: "other-owner"})
	if isOwnedPlaylist(124) {
		t.Fatal("playlist ownership leaked between accounts")
	}
}

// Arbitrary UI sizes used to produce QQ CDN 404s: 96 and 600 failed where the player's 120 worked.
func TestSizedArtwork(t *testing.T) {
	for _, kind := range []string{"T001", "T002"} {
		for _, size := range []struct{ request, want int }{
			{38, 90}, {90, 90}, {96, 120}, {120, 120}, {150, 150},
			{160, 180}, {180, 180}, {200, 300}, {240, 300}, {300, 300},
			{320, 500}, {500, 500}, {600, 800}, {800, 800}, {1600, 800},
		} {
			t.Run(fmt.Sprintf("%s/%d", kind, size.request), func(t *testing.T) {
				pic := "http://y.gtimg.cn/music/photo_new/" + kind + "R500x500M000album.jpg?max_age=2592000"
				want := fmt.Sprintf("https://y.gtimg.cn/music/photo_new/%sR%dx%dM000album.jpg?max_age=2592000", kind, size.want, size.want)
				if got := Sized(pic, size.request); got != want {
					t.Errorf("Sized(%d) = %q, want %q", size.request, got, want)
				}
			})
		}
	}
}

// Login used to expose album, MV and native-only charts as song playlists that opened empty.
func TestChartsKeepsOnlySongChartsWithWebIdentity(t *testing.T) {
	for _, account := range []struct {
		name    string
		session Session
	}{
		{"anonymous", Session{}},
		{"QQ", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2}},
		{"WeChat", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 1}},
	} {
		t.Run(account.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct{ Module, Method string } `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["qq"] != nil || request.Comm["authst"] != nil {
					t.Error("chart catalog inherited the account's mobile identity")
				}
				if request.Req.Module != "musicToplist.ToplistInfoServer" || request.Req.Method != "GetAll" {
					t.Errorf("wrong chart request: %+v", request)
				}
				rpcReply(w, json.RawMessage(`{"group":[{"toplist":[
					{"topId":62,"recType":10005,"title":"Song chart","totalNum":100,"song":[{"title":"Song","singerName":"Artist"}]},
					{"topId":23,"recType":10010,"title":"Album chart"},
					{"topId":506,"recType":10010,"title":"Global-K Chart"},
					{"topId":201,"recType":10010,"title":"MV chart"},
					{"topId":75,"recType":10099,"title":"Audio chart"},
					{"topId":135,"recType":10005,"title":"Weekly song chart","period":""}
				]}]}`))
			})
			setSession(account.session)
			charts, err := Charts()
			if err != nil {
				t.Fatal(err)
			}
			if len(charts) != 2 || charts[0].ID != chartIDBase+62 || charts[1].ID != chartIDBase+135 {
				t.Fatalf("non-song charts entered the playlist catalog: %+v", charts)
			}
			if charts[0].TrackCount != 100 || len(charts[0].Preview) != 1 || charts[0].Preview[0].First != "Song" {
				t.Fatalf("song chart lost metadata: %+v", charts[0])
			}
		})
	}
}

// Song charts must load the latest tracks with the web identity even when an account is present.
func TestChartDetailKeepsTracksWithSession(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Comm map[string]any
			Req  struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["authst"] != nil {
			t.Error("chart detail inherited the account's mobile identity")
		}
		if request.Req.Module != "musicToplist.ToplistInfoServer" || request.Req.Method != "GetDetail" || request.Req.Param["topid"] != float64(62) || request.Req.Param["period"] != "" || request.Req.Param["num"] != float64(100) {
			t.Errorf("wrong chart detail request: %+v", request)
		}
		rpcReply(w, json.RawMessage(`{"data":{"topId":62,"recType":10005,"title":"Song chart","totalNum":1,"updateTime":"Weekly"},"songInfoList":[{"id":901,"mid":"chartSong","title":"Song","singer":[{"id":902,"mid":"chartArtist","name":"Artist"}],"album":{"id":903,"mid":"chartAlbum","name":"Album"}}]}`))
	})
	setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
	chart, err := PlaylistDetail(chartIDBase + 62)
	if err != nil {
		t.Fatal(err)
	}
	if len(chart.Tracks) != 1 || chart.Tracks[0].ID != 901 || chart.Tracks[0].MID != "chartSong" || chart.TrackCount != 1 || chart.UpdateFreq != "Weekly" {
		t.Fatalf("lost chart contents: %+v", chart)
	}
}

// Custom chart metadata and missing song lists used to be cached as successful zero-song playlists.
func TestChartDetailRejectsEmptyShells(t *testing.T) {
	for _, data := range []string{
		`{"data":{"topId":23,"recType":10010,"totalNum":20},"songInfoList":[]}`,
		`{"data":{"topId":506,"recType":10010,"totalNum":3},"songInfoList":[]}`,
		`{"data":{"topId":62,"recType":10005,"totalNum":100},"songInfoList":[]}`,
		`{"data":{"topId":62,"recType":10005,"totalNum":100}}`,
	} {
		t.Run(data, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				rpcReply(w, json.RawMessage(data))
			})
			chart, err := PlaylistDetail(chartIDBase + 62)
			if err == nil || chart != nil {
				t.Fatalf("accepted an empty chart shell: %+v, %v", chart, err)
			}
		})
	}
}

// The old CGI fails; public comment RPCs need a 25-item cap and the preceding page's opaque sequence.
func TestCommentsWebRPCAndCursor(t *testing.T) {
	for _, account := range []Session{{}, {MusicID: "123", MusicKey: "fixture-key", LoginType: 2}, {MusicID: "456", MusicKey: "fixture-key", LoginType: 1}} {
		t.Run(map[int]string{0: "anonymous", 2: "QQ", 1: "WeChat"}[account.LoginType], func(t *testing.T) {
			newCalls, hotCalls := 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]any
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				p := request.Req.Param
				if request.Req.Module != "music.globalComment.CommentRead" || p["BizId"] != "42" || p["BizType"] != float64(1) || p["PageSize"] != float64(25) {
					t.Errorf("wrong comments request: %+v", request)
				}
				if len(request.Comm) != 2 || request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || r.Header.Get("Cookie") != "" {
					t.Error("public comments inherited account identity")
				}
				entries := []map[string]any{}
				more := any(false)
				switch request.Req.Method {
				case "GetHotCommentList":
					hotCalls++
					if p["HotType"] != float64(1) || p["WithAirborne"] != float64(0) {
						t.Error("wrong hot comment parameters")
					}
					entries = append(entries, map[string]any{"CmId": "hot!opaque", "Content": "hot"})
				case "GetNewCommentList":
					newCalls++
					if p["WithHot"] != float64(0) || p["FromCommentId"] != "" {
						t.Error("hot comments mixed into latest")
					}
					if newCalls == 1 {
						if p["PageNum"] != float64(0) || p["LastCommentSeqNo"] != "" {
							t.Error("first page has a stale cursor")
						}
						entries = append(entries, map[string]any{"CmId": "1!opaque", "SeqNo": "18446744073709551615", "Content": "one &amp; 二", "Nick": "listener", "Avatar": "https://fixture.invalid/avatar", "PubTime": 1700000000, "PraiseNum": 7})
						more = 1
					} else {
						if p["PageNum"] != float64(1) || p["LastCommentSeqNo"] != "18446744073709551615" || p["LastTotal"] != float64(2) || p["LastTotalVer"] != "1793343305312403406" {
							t.Errorf("lost next-page state: %+v", p)
						}
						entries = append(entries, map[string]any{"CmId": "2!opaque", "SeqNo": "last-sequence", "Content": "two"})
					}
				default:
					t.Errorf("unexpected method %s", request.Req.Method)
				}
				rpcReply(w, map[string]any{"TotalVer": "1793343305312403406", "CommentList": map[string]any{"Comments": entries, "Total": 2, "HasMore": more}})
			})
			setSession(account)
			first, err := ThreadComments("42", 0, 30)
			if err != nil {
				t.Fatal(err)
			}
			if !first.More || first.Total != 2 || len(first.Latest) != 1 || len(first.Hot) != 1 {
				t.Fatalf("first page: %+v", first)
			}
			cm := first.Latest[0]
			if cm.ID == 0 || cm.RemoteID != "1!opaque" || cm.Content != "one & 二" || cm.Time != 1700000000000 || cm.LikedCount != 7 || cm.User.Nickname != "listener" || cm.User.AvatarURL == "" {
				t.Fatalf("lost comment metadata: %+v", cm)
			}
			next, err := ThreadComments("42", 1, 30)
			if err != nil {
				t.Fatal(err)
			}
			if next.More || len(next.Latest) != 1 || next.Latest[0].Content != "two" || next.Latest[0].ID == cm.ID || len(next.Hot) != 0 || newCalls != 2 || hotCalls != 1 {
				t.Fatalf("next page: %+v; calls new=%d hot=%d", next, newCalls, hotCalls)
			}
			if sessionSnapshot() != account {
				t.Error("public read changed the session")
			}
		})
	}
}

// An empty comment list is valid, but rejected or malformed replies must remain errors.
func TestCommentsEmptyAndMalformed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    any
		code    int
		wantErr bool
	}{
		{"empty", map[string]any{"CommentList": map[string]any{"Comments": []any{}, "Total": 0, "HasMore": 0}}, 0, false},
		{"missing list", map[string]any{}, 0, true},
		{"malformed list", map[string]any{"CommentList": map[string]any{"Comments": "bad"}}, 0, true},
		{"missing IDs", map[string]any{"CommentList": map[string]any{"Comments": []any{map[string]any{"Content": "bad"}}}}, 0, true},
		{"rejected", map[string]any{}, 10000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": tc.code, "data": tc.data}})
			})
			page, err := ThreadComments("43", 0, 25)
			if (err != nil) != tc.wantErr {
				t.Fatalf("page=%+v error=%v", page, err)
			}
			if !tc.wantErr && (page.More || page.Total != 0 || len(page.Latest) != 0 || len(page.Hot) != 0) {
				t.Fatalf("empty page: %+v", page)
			}
		})
	}
}

// Personal radio must keep the QQ track MID so the existing player can resolve it.
func TestPersonalRadioTrackIdentity(t *testing.T) {
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		if method != "get_radio_track" || string(param["id"]) != "99" {
			t.Errorf("unexpected radio %s: %v", method, param)
		}
		return map[string]any{"tracks": []map[string]any{{"id": 42, "mid": "qqRadioMid", "title": "Radio", "interval": 120}}}
	})
	tracks, err := PersonalFM(FMDefault)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].MID != "qqRadioMid" || tracks[0].Duration != 120000 {
		t.Fatalf("radio tracks: %+v", tracks)
	}
}
