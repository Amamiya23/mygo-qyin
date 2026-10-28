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
	setSession(Session{})
	httpClient = &http.Client{Transport: redirectTransport{server.URL, http.DefaultTransport}}
	t.Cleanup(func() { httpClient = old; setSession(oldSession); server.Close() })
}
func rpcReply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 0, "data": data}})
}

// Anonymous search uses QQ's accepted type and retains the IDs needed by later playback.
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
	result, err := SearchPage("Song", SearchSongs, 20, 0)
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
