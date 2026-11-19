package qqmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
)

func fakeLibraryRPC(t *testing.T, fn func(string, map[string]json.RawMessage) any) {
	t.Helper()
	fakeLibraryModuleRPC(t, func(_ string, method string, param map[string]json.RawMessage) any {
		return fn(method, param)
	})
}

func fakeLibraryModuleRPC(t *testing.T, fn func(string, string, map[string]json.RawMessage) any) {
	t.Helper()
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Req struct {
				Module string                     `json:"module"`
				Method string                     `json:"method"`
				Param  map[string]json.RawMessage `json:"param"`
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		data := fn(body.Req.Module, body.Req.Method, body.Req.Param)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 0, "data": data}})
	})
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10001", MusicKey: "fake", LoginType: 2, EncryptUin: "fake-encrypted"})
}

// Unfavoriting uses the collection endpoint and validates its result instead of deleting the creator's playlist.
func TestUnfavoritePlaylist(t *testing.T) {
	for _, tc := range []struct {
		name string
		data any
		ok   bool
	}{
		{"success", map[string]any{"result": 0, "v_failedPlaylistId": []int64{}}, true},
		{"rejected", map[string]any{"result": 1, "v_failedPlaylistId": []int64{}}, false},
		{"failed ID", map[string]any{"result": 0, "v_failedPlaylistId": []int64{901}}, false},
		{"missing result", map[string]any{"v_failedPlaylistId": []int64{}}, false},
		{"missing failed list", map[string]any{"result": 0}, false},
		{"null failed list", map[string]any{"result": 0, "v_failedPlaylistId": nil}, false},
		{"invalid result", map[string]any{"result": "0", "v_failedPlaylistId": []int64{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				writes++
				if module != "music.musicasset.PlaylistFavWrite" || method != "CancelFavPlaylist" {
					t.Errorf("wrong favorite endpoint: %s/%s", module, method)
				}
				if string(param["uin"]) != `"10001"` || string(param["v_playlistId"]) != "[901]" {
					t.Errorf("wrong account or public playlist ID: %v", param)
				}
				return tc.data
			})
			if err := UnfavoritePlaylist(901); (err == nil) != tc.ok {
				t.Fatalf("UnfavoritePlaylist error = %v, success wanted = %v", err, tc.ok)
			}
			if writes != 1 {
				t.Fatalf("performed %d writes; want exactly one", writes)
			}
		})
	}
}

// Invalid IDs and signed-out requests never send a playlist collection write.
func TestUnfavoritePlaylistRequiresAccountAndID(t *testing.T) {
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		t.Errorf("unexpected write %s", method)
		return nil
	})
	for _, id := range []int64{0, -1} {
		if err := UnfavoritePlaylist(id); err == nil {
			t.Fatalf("accepted invalid playlist ID %d", id)
		}
	}
	setSession(Session{})
	if err := UnfavoritePlaylist(901); err == nil {
		t.Fatal("allowed a signed-out favorite write")
	}
}

// Favorite playlist names use name rather than dirName, which previously left their sidebar rows blank.
func TestUserPlaylistsPreservesNames(t *testing.T) {
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		switch method {
		case "GetPlaylistByUin":
			return map[string]any{"v_playlist": []any{
				map[string]any{"tid": 900, "dirId": 12, "dirName": "纯音乐"},
			}}
		case "CgiGetPlaylistFavInfo":
			return map[string]any{"v_list": []any{
				map[string]any{"tid": 901, "name": "加州日暮 | 潜入打雷的浮华美梦", "logo": "https://example.com/cover.jpg", "songnum": 94},
			}, "hasmore": 0}
		default:
			t.Errorf("unexpected method %s", method)
			return nil
		}
	})
	made, saved, err := UserPlaylists(10001)
	if err != nil {
		t.Fatal(err)
	}
	if len(made) != 1 || made[0].ID != 900 || made[0].Name != "纯音乐" {
		t.Fatalf("owned playlists lost their identity or name: %+v", made)
	}
	if len(saved) != 1 || saved[0].ID != 901 || saved[0].Name != "加州日暮 | 潜入打雷的浮华美梦" {
		t.Fatalf("favorite playlists lost their identity or name: %+v", saved)
	}
	if saved[0].CoverURL != "https://example.com/cover.jpg" || saved[0].TrackCount != 94 {
		t.Fatalf("favorite playlist metadata was lost: %+v", saved[0])
	}
}

// A like action must write directory 201 and verify the resulting state before reporting success.
func TestLikeWritesAndVerifies(t *testing.T) {
	liked := false
	writes := 0
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		switch method {
		case "get_song_detail_yqq":
			return map[string]any{"track_info": map[string]any{"id": 42, "mid": "songmid42", "title": "Song"}}
		case "IsSongFanByMid":
			return map[string]any{"m_fan": map[string]bool{"songmid42": liked}}
		case "AddSonglist":
			if string(param["dirId"]) != "201" {
				t.Error("like did not target the built-in liked directory")
			}
			var songs []struct {
				ID   int64 `json:"songId"`
				Type int   `json:"songType"`
			}
			_ = json.Unmarshal(param["v_songInfo"], &songs)
			if len(songs) != 1 || songs[0].ID != 42 || songs[0].Type != 0 {
				t.Error("incorrect QQ song identity")
			}
			writes++
			liked = true
			return map[string]any{"retCode": 0, "result": map[string]any{"dirId": 201, "tid": 901}}
		default:
			t.Errorf("unexpected method %s", method)
			return nil
		}
	})
	if err := Like(42, true); err != nil {
		t.Fatal(err)
	}
	if err := Like(42, true); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatalf("like action wrote %d times; want one", writes)
	}
}

// Playlist writes resolve the account's directory ID and reject system directories without writing.
func TestPlaylistEditResolvesOwnedDirectory(t *testing.T) {
	writes := 0
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		switch method {
		case "GetPlaylistByUin":
			return map[string]any{"v_playlist": []any{map[string]any{"tid": 900, "dirId": 12, "dirType": 0, "uin": "10001"}, map[string]any{"tid": 901, "dirId": 201, "uin": "10001"}}}
		case "AddSonglist":
			if string(param["dirId"]) != "12" {
				t.Error("used public playlist ID as account directory")
			}
			writes++
			return map[string]any{"retCode": 0, "result": map[string]any{"tid": 900, "dirId": 12}}
		default:
			t.Errorf("unexpected method %s", method)
			return nil
		}
	})
	if err := AddToPlaylist(900, 42); err != nil {
		t.Fatal(err)
	}
	if err := AddToPlaylist(901, 42); err == nil {
		t.Fatal("allowed editing the system liked directory as an ordinary playlist")
	}
	if writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
}

// A repeated liked-song page must fail instead of silently treating a truncated list as complete.
func TestLikedIDsRejectsIncompletePaging(t *testing.T) {
	reads := 0
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		if method != "CgiGetDiss" {
			t.Errorf("unexpected method %s", method)
		}
		reads++
		return map[string]any{"songlist": []any{map[string]any{"id": 42, "mid": "songmid42", "title": "Song"}}, "hasmore": 1, "total_song_num": 2}
	})
	if _, err := LikedIDs(10001); err == nil {
		t.Fatal("accepted repeated incomplete page")
	}
	if reads != 2 {
		t.Fatalf("reads=%d; expected first page and one continuation", reads)
	}
}

// Favorite albums use the credential-bearing profile CGI and inclusive page ranges, preserving album identity.
func TestLikedAlbumsPages(t *testing.T) {
	requests := 0
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != likedAlbumsPath || q.Get("reqtype") != "2" || q.Get("userid") != "10001" || q.Get("sin") != fmt.Sprint(requests) || q.Get("ein") != fmt.Sprint(requests) {
			t.Errorf("unexpected album page: %s", r.URL)
		}
		if cookie, err := r.Cookie("qm_keyst"); err != nil || cookie.Value != "fake-music-key" {
			t.Error("missing account credential")
		}
		requests++
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"totalalbum":2,"albumlist":[{"albumid":%d,"albummid":"AlbumMID%d","albumname":"Album","singername":"Singer","pubtime":"2025-01-02"}]}}`, requests, requests)
	})
	for offset := 0; offset < 2; offset++ {
		albums, more, err := LikedAlbums(1, offset)
		if err != nil || len(albums) != 1 || more != (offset == 0) {
			t.Fatalf("page %d: albums=%+v more=%v err=%v", offset, albums, more, err)
		}
		album := albums[0]
		if album.ID != int64(offset+1) || album.MID != fmt.Sprintf("AlbumMID%d", offset+1) || albumMID(album.ID) != album.MID || album.ArtistNames() != "Singer" || album.PublishTime == 0 {
			t.Fatalf("lost album metadata: %+v", album)
		}
	}
}

// Rejected or truncated favorite responses must not be presented as an empty collection.
func TestLikedAlbumsRejectsInvalidData(t *testing.T) {
	body := ""
	requests := 0
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) { requests++; _, _ = io.WriteString(w, body) })
	setSession(Session{})
	if _, _, err := LikedAlbums(20, 0); err == nil || requests != 0 {
		t.Fatal("signed-out album request reached the network")
	}
	setSession(Session{MusicID: "10001", MusicKey: "fake", LoginType: 2})
	for _, response := range []string{
		`{"code":4000}`, `{"code":0}`, `{"code":0,"data":{}}`,
		`{"code":0,"data":{"totalalbum":1,"albumlist":[]}}`,
		`{"code":0,"data":{"totalalbum":1,"albumlist":[null]}}`,
	} {
		body = response
		if _, _, err := LikedAlbums(20, 0); err == nil {
			t.Errorf("accepted malformed collection: %s", response)
		}
	}
	body = `{"code":0,"data":{"totalalbum":0,"albumlist":[]}}`
	if albums, more, err := LikedAlbums(20, 0); err != nil || len(albums) != 0 || more {
		t.Fatalf("rejected empty collection: %v", err)
	}
}

// Album writes use MIDs, avoid redundant mutations and confirm the result after exactly one write.
func TestLikeAlbumWritesAndVerifies(t *testing.T) {
	for _, want := range []bool{true, false} {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			liked, writes := !want, 0
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				if string(param["v_albumMid"]) != `["AlbumMID"]` {
					t.Error("did not use album MID")
				}
				switch method {
				case "IsAlbumFan":
					if module != "music.musicasset.AlbumFavRead" || string(param["uin"]) != "10001" {
						t.Errorf("wrong status owner/module: %s %v", module, param)
					}
					return map[string]any{"m_fan": map[string]bool{"AlbumMID": liked}}
				case "FavAlbum", "CancelFavAlbum":
					if module != "music.musicasset.AlbumFavWrite" || string(param["uin"]) != `"10001"` || (method == "FavAlbum") != want {
						t.Errorf("wrong write: %s %s %v", module, method, param)
					}
					writes++
					liked = want
					return map[string]any{"result": 0, "v_failedAlbumId": []int{}, "v_failedAlbumMid": []string{}}
				default:
					t.Errorf("unexpected method %s", method)
					return nil
				}
			})
			for range 2 {
				if err := LikeAlbum("AlbumMID", want); err != nil {
					t.Fatal(err)
				}
			}
			if writes != 1 {
				t.Fatalf("writes = %d", writes)
			}
		})
	}
}

// Unknown status, incomplete write results and failed readback cannot report a successful album mutation.
func TestLikeAlbumRejectsUnconfirmedWrites(t *testing.T) {
	for _, mode := range []string{"unknown", "missing result", "failed album", "unchanged"} {
		t.Run(mode, func(t *testing.T) {
			writes := 0
			fakeLibraryRPC(t, func(method string, _ map[string]json.RawMessage) any {
				if method == "IsAlbumFan" {
					if mode == "unknown" {
						return map[string]any{"m_fan": map[string]bool{}}
					}
					return map[string]any{"m_fan": map[string]bool{"AlbumMID": false}}
				}
				writes++
				if mode == "missing result" {
					return map[string]any{"result": 0}
				}
				failed := []string{}
				if mode == "failed album" {
					failed = append(failed, "AlbumMID")
				}
				return map[string]any{"result": 0, "v_failedAlbumId": []int{}, "v_failedAlbumMid": failed}
			})
			if err := LikeAlbum("AlbumMID", true); err == nil {
				t.Fatal("accepted unconfirmed mutation")
			}
			if mode == "unknown" && writes != 0 || writes > 1 {
				t.Fatalf("writes = %d", writes)
			}
		})
	}
}

func fakePurchasedCGI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	fakeAuthServer(t, handler)
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10001", MusicKey: "fake-music-key", LoginType: 2})
}

// Purchased songs use the music session and continue by item offset while preserving playable identity.
func TestPurchasedSongsPages(t *testing.T) {
	requests := 0
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.Host != "c.y.qq.com" || r.URL.Path != purchasedPath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		for key, want := range map[string]string{"from": "1", "cmd": "sales_album", "type": "2", "uin": "10001", "num": "1", "platform": "yqq", "format": "json", "g_tk": "665448288", "g_tk_new_20200303": "665448288"} {
			if q.Get(key) != want {
				t.Errorf("%s=%q; want %q", key, q.Get(key), want)
			}
		}
		for key, want := range map[string]string{"uin": "10001", "qqmusic_uin": "10001", "qm_keyst": "fake-music-key", "qqmusic_key": "fake-music-key"} {
			cookie, err := r.Cookie(key)
			if err != nil || cookie.Value != want {
				t.Errorf("missing account cookie %s", key)
			}
		}
		offset, _ := strconv.Atoi(q.Get("start"))
		if offset != requests-1 {
			t.Errorf("offset=%d; want %d", offset, requests-1)
		}
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"total":2,"songlist":[{"songid":%d,"songmid":"purchased-song-%d","songname":"Song","albummid":"album-mid","albumid":71,"albumname":"Album","interval":123,"singer":[{"id":3,"mid":"artist-mid","name":"Singer"}],"file":{"media_mid":"media-mid","size_320mp3":12345}}]}}`, 501+offset, offset)
	})
	for offset := 0; offset < 2; offset++ {
		songs, more, err := PurchasedSongs(1, offset)
		if err != nil || len(songs) != 1 || more != (offset == 0) {
			t.Fatalf("page %d: songs=%v more=%v err=%v", offset, songs, more, err)
		}
		song := songs[0]
		if song.ID != int64(501+offset) || song.MID != fmt.Sprintf("purchased-song-%d", offset) || song.MediaMID != "media-mid" || song.Size320 != 12345 || song.Duration != 123000 || song.Album.MID != "album-mid" || len(song.Artists) != 1 {
			t.Fatalf("lost song metadata: %+v", song)
		}
		remembered, ok := knownTrack(song.ID)
		if !ok || remembered.MediaMID != song.MediaMID {
			t.Fatal("purchased song identity not registered for playback")
		}
	}
}

// Purchased albums retain the MID needed to open their existing detail page, including legacy order fields.
func TestPurchasedAlbumsIdentity(t *testing.T) {
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("type") != "1" || q.Get("start") != "3" || q.Get("num") != "2" {
			t.Errorf("incorrect album page: %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"total":5,"albumlist":[{"albumID":701,"albumMid":"album-order-mid","albumName":"Album","singerid":3,"singermid":"artist-mid","singerName":"Singer","publishDate":"2025-01-02"},{"id":702,"pmid":"photo-fallback-mid","name":"Second","singers":[{"id":4,"mid":"artist-two","name":"Two"}]}]}}`)
	})
	albums, more, err := PurchasedAlbums(2, 3)
	if err != nil || len(albums) != 2 || more {
		t.Fatalf("albums=%v more=%v err=%v", albums, more, err)
	}
	if albums[0].ID != 701 || albums[0].MID != "album-order-mid" || albumMID(701) != "album-order-mid" || albums[0].Artists[0].Name != "Singer" || albums[0].PublishTime == 0 || albums[1].MID != "photo-fallback-mid" || albums[1].Artists[0].MID != "artist-two" {
		t.Fatalf("lost album metadata: %+v", albums)
	}
}

// An unauthenticated or rejected purchase request must never look like an empty library.
func TestPurchasedRequiresLoginAndRejectsInvalidData(t *testing.T) {
	responses := []string{
		`{"code":1000,"subcode":1000}`,
		`{"code":0,"data":{"code":104401,"songlist":[],"total":0}}`,
		`{"code":0}`,
		`{"code":0,"data":{"total":1}}`,
		`{"code":0,"data":{"total":1,"songlist":null}}`,
		`{"code":0,"data":{"total":0,"songlist":{}}}`,
		`{"code":0,"data":{"songlist":[]}}`,
		`{"code":0,"data":{"total":-1,"songlist":[]}}`,
		`{"code":0,"data":{"total":1,"songlist":[]}}`,
		`{"code":0,"data":{"total":1,"songlist":[null]}}`,
		`{"code":0,"data":{"total":1,"songlist":[{"songid":1}]}}`,
	}
	requests := 0
	body := responses[0]
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) { requests++; _, _ = io.WriteString(w, body) })
	setSession(Session{})
	if _, _, err := PurchasedSongs(30, 0); err == nil || requests != 0 {
		t.Fatal("unauthenticated purchase request reached the network")
	}
	if _, _, err := PurchasedAlbums(30, 0); err == nil || requests != 0 {
		t.Fatal("unauthenticated album request reached the network")
	}
	setSession(Session{MusicID: "10001", MusicKey: "fake-music-key", LoginType: 2})
	for i, response := range responses {
		body = response
		if _, _, err := PurchasedSongs(30, 0); err == nil {
			t.Errorf("accepted invalid response %d", i)
		} else if i == 0 {
			var protocol *ProtocolError
			if !errors.As(err, &protocol) || protocol.Code != 1000 {
				t.Errorf("lost authentication rejection: %v", err)
			}
		}
	}
	for _, response := range []string{`{"code":0,"data":{"total":1,"albumlist":null}}`, `{"code":0,"data":{"total":1}}`, `{"code":0,"data":{"total":0,"albumlist":{}}}`, `{"code":0,"data":{"total":1,"albumlist":[null]}}`, `{"code":0,"data":{"total":1,"albumlist":[{"id":1}]}}`} {
		body = response
		if _, _, err := PurchasedAlbums(30, 0); err == nil {
			t.Error("accepted invalid album response")
		}
	}
	for _, response := range []string{`{"code":0,"data":{"total":0,"songlist":[],"albumlist":[]}}`, `{"code":0,"data":{"total":0}}`, `{"code":0,"data":{"total":0,"songlist":null,"albumlist":null}}`} {
		body = response
		if songs, more, err := PurchasedSongs(30, 0); err != nil || len(songs) != 0 || more {
			t.Fatalf("valid empty purchases rejected: %v", err)
		}
		if albums, more, err := PurchasedAlbums(30, 0); err != nil || len(albums) != 0 || more {
			t.Fatalf("valid empty albums rejected: %v", err)
		}
	}
}

// Membership follows HugeVip and iVipFlag without mistaking green or unknown flags for another tier.
func TestUserMembership(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]any
		want  Membership
	}{
		{"free", map[string]any{"HugeVip": 0, "iVipFlag": 0, "iSuperVip": 1}, MembershipFree},
		{"green", map[string]any{"HugeVip": 0, "iVipFlag": 1, "iSuperVip": 1}, MembershipGreen},
		{"super", map[string]any{"HugeVip": 1, "iVipFlag": 1}, MembershipSuper},
		{"missing huge", map[string]any{"iVipFlag": 1}, MembershipUnknown},
		{"missing vip", map[string]any{"HugeVip": 0}, MembershipUnknown},
		{"unknown huge", map[string]any{"HugeVip": 2, "iVipFlag": 0}, MembershipUnknown},
		{"unknown vip", map[string]any{"HugeVip": 0, "iVipFlag": 2}, MembershipUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				if module != "userInfo.VipQueryServer" || method != "SRFVipQuery_V2" || string(param["uin_list"]) != `["10001"]` {
					t.Errorf("unexpected membership request %s/%s %v", module, method, param)
				}
				return map[string]any{"infoMap": map[string]any{"10001": tc.flags}}
			})
			got, err := UserMembership()
			if err != nil || got != tc.want {
				t.Fatalf("membership = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// An absent account entry or malformed flag must not become a free membership.
func TestUserMembershipInvalidResponse(t *testing.T) {
	for _, data := range []any{
		map[string]any{"infoMap": map[string]any{}},
		map[string]any{"infoMap": map[string]any{"10001": map[string]any{"HugeVip": "0", "iVipFlag": 0}}},
	} {
		fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any { return data })
		if got, err := UserMembership(); err == nil || got != MembershipUnknown {
			t.Fatalf("invalid response returned %q, %v", got, err)
		}
	}
}

// Signed-out membership queries never contact the service.
func TestUserMembershipRequiresSession(t *testing.T) {
	fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any {
		t.Error("sent a signed-out membership request")
		return nil
	})
	setSession(Session{})
	if got, err := UserMembership(); err == nil || got != MembershipUnknown {
		t.Fatalf("signed-out membership = %q, %v", got, err)
	}
}

// The date query returns green and super membership dates separately.
func TestUserMembershipDates(t *testing.T) {
	fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
		return map[string]any{"identity": map[string]any{"overdate": "2030-02-03", "HugeVipEnd": "2032-04-05"}}
	})
	green, super, err := UserMembershipDates()
	if err != nil || green != "2030-02-03" || super != "2032-04-05" {
		t.Fatalf("dates = %q, %q, %v", green, super, err)
	}
}

// Membership expiry selects dates for the requested tier without confusing green and super dates.
func TestUserMembershipExpiry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		level    Membership
		identity map[string]any
		userinfo map[string]any
		want     string
	}{
		{"green prefers overdate", MembershipGreen, map[string]any{"overdate": "2030-02-03 12:34:56", "LMEnd": "2031-03-04", "HugeVipEnd": "2032-04-05"}, map[string]any{"expire": 1893456000}, "2030-02-03"},
		{"green does not use milestone date", MembershipGreen, map[string]any{"overdate": "2030-02-30", "LMEnd": "2031-03-04"}, nil, ""},
		{"green does not use unverified userinfo date", MembershipGreen, nil, map[string]any{"expire": 1893456000}, ""},
		{"super uses HugeVipEnd", MembershipSuper, map[string]any{"overdate": "2030-02-03", "HugeVipEnd": "2032-04-05 12:00:00"}, nil, "2032-04-05"},
		{"super does not use green date", MembershipSuper, map[string]any{"overdate": "2030-02-03", "HugeVipEnd": "0000-00-00"}, map[string]any{"expire": 1893456000}, ""},
		{"invalid and missing dates", MembershipGreen, map[string]any{"overdate": "2030-02-30", "LMEnd": "not a date"}, map[string]any{"expire": -1}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				if module != "VipLogin.VipLoginInter" || method != "vip_login_base" || param == nil || len(param) != 0 {
					t.Errorf("unexpected request %s/%s %v", module, method, param)
				}
				return map[string]any{"identity": tc.identity, "userinfo": tc.userinfo}
			})
			got, err := UserMembershipExpiry(tc.level)
			if err != nil || got != tc.want {
				t.Fatalf("expiry = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// Free or unknown membership does not trigger a request, and paid membership requires a session.
func TestUserMembershipExpiryRequiresPaidSession(t *testing.T) {
	fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any {
		t.Error("unexpected expiry request")
		return nil
	})
	setSession(Session{})
	for _, level := range []Membership{MembershipFree, MembershipUnknown, "other"} {
		if got, err := UserMembershipExpiry(level); got != "" || err != nil {
			t.Fatalf("expiry for %q = %q, %v", level, got, err)
		}
	}
	if got, err := UserMembershipExpiry(MembershipGreen); got != "" || err == nil {
		t.Fatalf("signed-out expiry = %q, %v", got, err)
	}
}

// Malformed upstream data is reported rather than interpreted as a date.
func TestUserMembershipExpiryMalformedResponse(t *testing.T) {
	fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any {
		return map[string]any{"identity": "invalid"}
	})
	if got, err := UserMembershipExpiry(MembershipGreen); got != "" || err == nil {
		t.Fatalf("malformed expiry = %q, %v", got, err)
	}
}

// Both login channels send their music credentials with the expiry request.
func TestUserMembershipExpiryAuthentication(t *testing.T) {
	for _, loginType := range []int{1, 2} {
		t.Run(string(rune('0'+loginType)), func(t *testing.T) {
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Comm map[string]any `json:"comm"`
					Req  struct {
						Module string         `json:"module"`
						Method string         `json:"method"`
						Param  map[string]any `json:"param"`
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Comm["qq"] != "10001" || body.Comm["authst"] != "fake" || body.Comm["tmeLoginType"] != float64(loginType) || body.Req.Module != "VipLogin.VipLoginInter" || body.Req.Method != "vip_login_base" || body.Req.Param == nil || len(body.Req.Param) != 0 {
					t.Error("incorrect authenticated expiry request")
				}
				for _, name := range []string{"uin", "qqmusic_uin", "qm_keyst", "qqmusic_key"} {
					if cookie, err := r.Cookie(name); err != nil || cookie.Value == "" {
						t.Errorf("missing cookie %s", name)
					}
				}
				_, _ = w.Write([]byte(`{"code":0,"req_0":{"code":0,"data":{"identity":{"overdate":"2030-01-02"}}}}`))
			})
			old := httpClient
			httpClient = authHTTP
			t.Cleanup(func() { httpClient = old })
			setSession(Session{MusicID: "10001", MusicKey: "fake", LoginType: loginType})
			if got, err := UserMembershipExpiry(MembershipGreen); got != "2030-01-02" || err != nil {
				t.Fatalf("expiry = %q, %v", got, err)
			}
		})
	}
}
