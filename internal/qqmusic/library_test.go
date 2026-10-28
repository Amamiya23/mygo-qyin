package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

func fakeLibraryRPC(t *testing.T, fn func(string, map[string]json.RawMessage) any) {
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
		data := fn(body.Req.Method, body.Req.Param)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 0, "data": data}})
	})
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10001", MusicKey: "fake", LoginType: 2, EncryptUin: "fake-encrypted"})
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
