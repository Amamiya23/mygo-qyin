package qqmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// Song shelves retain the server's groups and complete song order after one batched detail request.
func TestRecommendedSongShelves(t *testing.T) {
	for _, loginType := range []int{1, 2} {
		t.Run(map[int]string{1: "WeChat", 2: "QQ"}[loginType], func(t *testing.T) {
			feedCalls, detailCalls := 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]json.RawMessage
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(20) || request.Comm["cv"] != float64(2276) || request.Comm["platform"] != "wk_v17" || request.Comm["uin"] != "10001" || request.Comm["qq"] != "10001" || request.Comm["authst"] != "fixture-key" || request.Comm["tmeLoginType"] != float64(loginType) || request.Comm["QIMEI"] != nil {
					t.Error("song feed lost the PC identity or account")
				}
				if !strings.Contains(r.Header.Get("Cookie"), "qm_keyst=fixture-key") {
					t.Error("song feed lost the account cookie")
				}
				switch request.Req.Module + "/" + request.Req.Method {
				case "music.recommend.RecommendFeed/get_recommend_feed":
					feedCalls++
					if string(request.Req.Param["direction"]) != "0" || string(request.Req.Param["page"]) != "1" || string(request.Req.Param["s_num"]) != "0" || string(request.Req.Param["v_cache"]) != "[]" || string(request.Req.Param["v_uniq"]) != "[]" {
						t.Error("song feed did not request the first page")
					}
					rpcReply(w, json.RawMessage(`{"retcode":0,"v_shelf":[
						{"id":301,"v_niche":[{"v_card":[{"id":"700","type":200}]}]},
						{"id":207,"title_template":"听「{String}」也会喜欢💗","title_content":"种子曲\u2062","miscellany":{"songList":"30,20,99,30,500,400,0,-1,broken"},"v_niche":[{"v_card":[{"id":"20","type":200},{"id":"30","type":200},{"id":"500","type":500},{"id":"400","type":400}]}]},
						{"id":207,"title_template":"「{String}」，这是你的幸运好歌🍀","title_content":"Amamiya\u2062","miscellany":{"songList":"20,40"},"v_niche":[{"v_card":[{"id":"20","type":200},{"id":"40","type":200}]}]},
						{"id":207,"title_template":"","title_content":"为你推荐的宝藏单曲","miscellany":{"songList":"50,60"},"v_niche":[{"v_card":[{"id":"50","type":200}]}]},
						{"id":207,"title_template":"值得红心的宝藏歌曲","title_content":"无需插入的歌手","v_niche":[{"v_card":[{"id":"80","type":200},{"id":"80","type":200},{"id":"800","type":500},{"id":"not-a-song","type":200}]}]}
					]}`))
				case "music.trackInfo.UniformRuleCtrl/CgiGetTrackInfo":
					detailCalls++
					var ids []int64
					var types []int
					_ = json.Unmarshal(request.Req.Param["ids"], &ids)
					_ = json.Unmarshal(request.Req.Param["types"], &types)
					if !reflect.DeepEqual(ids, []int64{30, 20, 99, 40, 50, 60, 80}) || !reflect.DeepEqual(types, make([]int, 7)) {
						t.Errorf("batched wrong song IDs or types: %v %v", ids, types)
					}
					var tracks []any
					for _, id := range []int64{80, 40, 50, 99, 20, 30} {
						tracks = append(tracks, recommendedTrackFixture(id))
					}
					// Unavailable song 60 is omitted; no sparse card may impersonate its details.
					rpcReply(w, map[string]any{"tracks": tracks})
				default:
					t.Errorf("unexpected song feed request: %s/%s", request.Req.Module, request.Req.Method)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: loginType, Device: &wechatDevice{Qimei: "fixture-device"}})
			shelves, err := RecommendedSongShelves()
			if err != nil {
				t.Fatal(err)
			}
			if feedCalls != 1 || detailCalls != 1 || len(shelves) != 4 {
				t.Fatalf("feed=%d detail=%d shelves=%d", feedCalls, detailCalls, len(shelves))
			}
			wantTitles := []string{"听「种子曲」也会喜欢💗", "「Amamiya」，这是你的幸运好歌🍀", "为你推荐的宝藏单曲", "值得红心的宝藏歌曲"}
			wantIDs := [][]int64{{30, 20, 99}, {20, 40}, {50}, {80}}
			for i, shelf := range shelves {
				var ids []int64
				for _, track := range shelf.Tracks {
					ids = append(ids, track.ID)
					if track.MID == "" || track.MediaMID == "" || track.Size128 == 0 || track.Duration != 210000 || len(track.Artists) != 1 || track.Album.ID == 0 || track.Album.PicURL == "" || track.Volume == nil || track.NeedsDetails {
						t.Errorf("song %d lacks complete playback metadata", track.ID)
					}
				}
				if shelf.Title != wantTitles[i] || !reflect.DeepEqual(ids, wantIDs[i]) {
					t.Errorf("shelf %d: title=%q IDs=%v", i, shelf.Title, ids)
				}
			}
		})
	}
}

func recommendedTrackFixture(id int64) map[string]any {
	return map[string]any{
		"id": id, "mid": fmt.Sprint("song-mid-", id), "title": fmt.Sprint("Song ", id), "interval": 210,
		"singer": []any{map[string]any{"id": 6, "mid": "artist-mid", "name": "Artist"}},
		"album":  map[string]any{"id": 7, "mid": "album-mid", "name": "Album"},
		"file":   map[string]any{"media_mid": fmt.Sprint("media-mid-", id), "size_128mp3": 4000000},
		"volume": map[string]any{"gain": -6.2, "peak": 0.98, "lra": 7.5},
	}
}

// Signed-out song recommendations must not contact QQ or relabel generic songs as personal.
func TestRecommendedSongShelvesRequiresAccount(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("signed-out song feed contacted QQ") })
	if _, err := RecommendedSongShelves(); err == nil {
		t.Fatal("accepted a signed-out song recommendation request")
	}
}

// Missing feed status and detail metadata must fail instead of creating empty playable songs.
func TestRecommendedSongShelvesRejectsInvalidData(t *testing.T) {
	for _, tc := range []struct {
		name, feed, details string
		ok                  bool
	}{
		{"missing status", `{"v_shelf":[]}`, "", false},
		{"missing shelves", `{"retcode":0}`, "", false},
		{"rejected", `{"retcode":12913,"v_shelf":[]}`, "", false},
		{"empty feed", `{"retcode":0,"v_shelf":[]}`, "", true},
		{"only other types", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"400,500"},"v_niche":[{"v_card":[{"id":"400","type":400},{"id":"500","type":500}]}]}]}`, "", true},
		{"missing details", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30"}}]}`, `{}`, false},
		{"empty details", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30"}}]}`, `{"tracks":[]}`, false},
		{"incomplete details", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30"}}]}`, `{"tracks":[{"id":30,"title":"Sparse"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					rpcReply(w, json.RawMessage(tc.feed))
				} else {
					if tc.details == "" {
						t.Error("feed without song IDs fetched details")
						http.Error(w, "unexpected details", http.StatusBadRequest)
						return
					}
					rpcReply(w, json.RawMessage(tc.details))
				}
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: 2})
			shelves, err := RecommendedSongShelves()
			if (err == nil) != tc.ok || len(shelves) != 0 {
				t.Fatalf("shelves=%d error=%v, want success=%v", len(shelves), err, tc.ok)
			}
			if tc.name == "rejected" {
				var protocol *ProtocolError
				if !errors.As(err, &protocol) || protocol.Code != 12913 {
					t.Fatalf("lost feed rejection: %v", err)
				}
			}
		})
	}
}

type songPagingReply struct {
	data   string
	status int
	wire   bool
}

type songPagingRequest struct {
	Direction int `json:"direction"`
	Page      int `json:"page"`
	SNum      int `json:"s_num"`
}

type songPagingResult struct {
	shelves     []SongShelf
	err         error
	requests    []songPagingRequest
	detailCalls int
	detailIDs   []int64
}

const songPagingFirst = `{"retcode":0,"load_mark":0,"v_shelf":[{"id":301},{"id":207,"title_content":"First","miscellany":{"songList":"30,20"}},{"id":272},{"id":271}]}`
const songPagingNext = `{"retcode":0,"load_mark":-1,"v_shelf":[{"id":205},{"id":207,"title_content":"Second","miscellany":{"songList":"20,40"}}]}`

func runSongPaging(t *testing.T, replies []songPagingReply) songPagingResult {
	t.Helper()
	var result songPagingResult
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Req struct {
				Module string
				Method string
				Param  json.RawMessage
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch request.Req.Module + "/" + request.Req.Method {
		case "music.recommend.RecommendFeed/get_recommend_feed":
			var param struct {
				songPagingRequest
				Cache json.RawMessage `json:"v_cache"`
				Uniq  json.RawMessage `json:"v_uniq"`
			}
			if err := json.Unmarshal(request.Req.Param, &param); err != nil {
				t.Error(err)
			}
			if string(param.Cache) != "[]" || string(param.Uniq) != "[]" {
				t.Errorf("feed changed the empty cache arguments: %s %s", param.Cache, param.Uniq)
			}
			index := len(result.requests)
			result.requests = append(result.requests, param.songPagingRequest)
			if index >= len(replies) {
				t.Errorf("unexpected feed request: %+v", param.songPagingRequest)
				http.Error(w, "unexpected page", http.StatusBadRequest)
				return
			}
			reply := replies[index]
			if reply.status != 0 {
				w.WriteHeader(reply.status)
			}
			if reply.wire {
				_, _ = w.Write([]byte(reply.data))
			} else {
				rpcReply(w, json.RawMessage(reply.data))
			}
		case "music.trackInfo.UniformRuleCtrl/CgiGetTrackInfo":
			result.detailCalls++
			var param struct {
				IDs []int64 `json:"ids"`
			}
			if err := json.Unmarshal(request.Req.Param, &param); err != nil {
				t.Error(err)
			}
			result.detailIDs = append([]int64(nil), param.IDs...)
			var tracks []any
			for i := len(param.IDs) - 1; i >= 0; i-- {
				tracks = append(tracks, recommendedTrackFixture(param.IDs[i]))
			}
			rpcReply(w, map[string]any{"tracks": tracks})
		default:
			t.Errorf("unexpected method: %s/%s", request.Req.Module, request.Req.Method)
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	})
	setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: 2})
	result.shelves, result.err = RecommendedSongShelves()
	return result
}

func songPagingIDs(shelves []SongShelf) [][]int64 {
	var ids [][]int64
	for _, shelf := range shelves {
		var group []int64
		for _, track := range shelf.Tracks {
			group = append(group, track.ID)
		}
		ids = append(ids, group)
	}
	return ids
}

// Home used to omit the continuation batch, losing its second song shelf entirely.
func TestRecommendedSongShelvesLoadsContinuation(t *testing.T) {
	got := runSongPaging(t, []songPagingReply{{data: songPagingFirst}, {data: songPagingNext}})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if want := []songPagingRequest{{0, 1, 0}, {1, 2, 4}}; !reflect.DeepEqual(got.requests, want) {
		t.Fatalf("feed requests=%+v, want %+v", got.requests, want)
	}
	if want := [][]int64{{30, 20}, {20, 40}}; !reflect.DeepEqual(songPagingIDs(got.shelves), want) {
		t.Fatalf("shelf song order=%v, want %v", songPagingIDs(got.shelves), want)
	}
	if got.detailCalls != 1 || !reflect.DeepEqual(got.detailIDs, []int64{30, 20, 40}) {
		t.Fatalf("detail calls=%d IDs=%v; shared songs must be requested once but retained in both shelves", got.detailCalls, got.detailIDs)
	}
}

// Feed termination comes from the server marker or an empty batch, including older replies without a marker.
func TestRecommendedSongShelvesPagingStops(t *testing.T) {
	for _, tt := range []struct {
		name    string
		replies []songPagingReply
		calls   int
		ids     [][]int64
	}{
		{"negative_marker", []songPagingReply{{data: `{"retcode":0,"load_mark":-1,"v_shelf":[{"id":207,"miscellany":{"songList":"30,20"}}]}`}}, 1, [][]int64{{30, 20}}},
		{"positive_marker", []songPagingReply{{data: `{"retcode":0,"load_mark":1,"v_shelf":[{"id":207,"miscellany":{"songList":"30,20"}}]}`}}, 1, [][]int64{{30, 20}}},
		{"missing_marker", []songPagingReply{{data: `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30,20"}}]}`}}, 1, [][]int64{{30, 20}}},
		{"empty_first_batch", []songPagingReply{{data: `{"retcode":0,"load_mark":0,"v_shelf":[]}`}}, 1, nil},
		{"empty_continuation", []songPagingReply{{data: songPagingFirst}, {data: `{"retcode":0,"load_mark":0,"v_shelf":[]}`}}, 2, [][]int64{{30, 20}}},
		{"continuation_without_marker", []songPagingReply{{data: songPagingFirst}, {data: `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"40"}}]}`}}, 2, [][]int64{{30, 20}, {40}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := runSongPaging(t, tt.replies)
			if got.err != nil {
				t.Fatal(got.err)
			}
			if len(got.requests) != tt.calls || !reflect.DeepEqual(songPagingIDs(got.shelves), tt.ids) {
				t.Fatalf("feed calls=%d shelves=%v, want calls=%d shelves=%v", len(got.requests), songPagingIDs(got.shelves), tt.calls, tt.ids)
			}
			wantDetails := 0
			if len(tt.ids) > 0 {
				wantDetails = 1
			}
			if got.detailCalls != wantDetails {
				t.Fatalf("detail calls=%d, want %d", got.detailCalls, wantDetails)
			}
		})
	}
}

// Non-song continuation batches still advance the feed instead of imposing a fixed batch-count limit.
func TestRecommendedSongShelvesContinuesPastNonSongBatches(t *testing.T) {
	got := runSongPaging(t, []songPagingReply{
		{data: songPagingFirst},
		{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":205}]}`},
		{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":272}]}`},
		{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":271}]}`},
		{data: songPagingNext},
	})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if want := []songPagingRequest{{0, 1, 0}, {1, 2, 4}, {1, 3, 5}, {1, 4, 6}, {1, 5, 7}}; !reflect.DeepEqual(got.requests, want) {
		t.Fatalf("feed requests=%+v, want %+v", got.requests, want)
	}
	if want := [][]int64{{30, 20}, {20, 40}}; !reflect.DeepEqual(songPagingIDs(got.shelves), want) || got.detailCalls != 1 {
		t.Fatalf("shelves=%v detail calls=%d, want shelves=%v and one detail request", songPagingIDs(got.shelves), got.detailCalls, want)
	}
}

// The home limit counts usable song shelves and retains every song shelf already returned in the final batch.
func TestRecommendedSongShelvesStopsAtHomeLimit(t *testing.T) {
	for _, tt := range []struct {
		name string
		last string
		ids  [][]int64
	}{
		{"exact_limit", `{"retcode":0,"load_mark":0,"v_shelf":[{"id":207,"miscellany":{"songList":"50"}}]}`, [][]int64{{30, 20}, {40}, {50}}},
		{"complete_final_batch", `{"retcode":0,"load_mark":0,"v_shelf":[{"id":207,"miscellany":{"songList":"50"}},{"id":207,"miscellany":{"songList":"60"}}]}`, [][]int64{{30, 20}, {40}, {50}, {60}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{
				{data: songPagingFirst},
				{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":207,"miscellany":{"songList":"0,invalid"}},{"id":205},{"id":207,"miscellany":{"songList":"40"}}]}`},
				{data: tt.last},
			})
			if got.err != nil {
				t.Fatal(got.err)
			}
			if want := []songPagingRequest{{0, 1, 0}, {1, 2, 4}, {1, 3, 7}}; !reflect.DeepEqual(got.requests, want) {
				t.Fatalf("feed requests=%+v, want %+v", got.requests, want)
			}
			if !reflect.DeepEqual(songPagingIDs(got.shelves), tt.ids) || got.detailCalls != 1 {
				t.Fatalf("shelves=%v detail calls=%d, want shelves=%v and one detail request", songPagingIDs(got.shelves), got.detailCalls, tt.ids)
			}
		})
	}
}

func songPagingFailures() map[string]songPagingReply {
	return map[string]songPagingReply{
		"http":            {status: http.StatusServiceUnavailable, data: "unavailable", wire: true},
		"decode":          {data: "{", wire: true},
		"envelope_status": {data: `{"code":1000}`, wire: true},
		"rpc_status":      {data: `{"code":0,"req_0":{"code":12913,"data":{}}}`, wire: true},
		"feed_status":     {data: `{"retcode":12913,"load_mark":-1,"v_shelf":[]}`},
		"missing_retcode": {data: `{"load_mark":-1,"v_shelf":[]}`},
		"missing_shelves": {data: `{"retcode":0,"load_mark":-1}`},
		"null_shelves":    {data: `{"retcode":0,"load_mark":-1,"v_shelf":null}`},
		"invalid_shelves": {data: `{"retcode":0,"load_mark":-1,"v_shelf":"invalid"}`},
	}
}

// A failed continuation must keep the playable first batch and still resolve its song details.
func TestRecommendedSongShelvesContinuationFailureKeepsEarlierSongs(t *testing.T) {
	for name, failure := range songPagingFailures() {
		t.Run(name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{{data: songPagingFirst}, failure})
			if got.err != nil {
				t.Fatal(got.err)
			}
			if len(got.requests) != 2 || !reflect.DeepEqual(songPagingIDs(got.shelves), [][]int64{{30, 20}}) {
				t.Fatalf("feed calls=%d shelves=%v; first-batch songs were not retained", len(got.requests), songPagingIDs(got.shelves))
			}
			if got.detailCalls != 1 || !reflect.DeepEqual(got.detailIDs, []int64{30, 20}) {
				t.Fatalf("detail calls=%d IDs=%v, want one request for [30 20]", got.detailCalls, got.detailIDs)
			}
		})
	}
}

// A continuation failure without earlier usable songs must remain an error instead of an empty successful feed.
func TestRecommendedSongShelvesContinuationFailureWithoutSongs(t *testing.T) {
	for name, failure := range songPagingFailures() {
		t.Run(name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{
				{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":301},{"id":207,"miscellany":{"songList":"0,invalid"}}]}`},
				failure,
			})
			if got.err == nil || len(got.requests) != 2 || got.detailCalls != 0 || len(got.shelves) != 0 {
				t.Fatalf("error=%v feed calls=%d detail calls=%d shelves=%d; expected a continuation error without songs", got.err, len(got.requests), got.detailCalls, len(got.shelves))
			}
		})
	}
}

// First-batch failures must surface directly because there are no earlier shelves to fall back to.
func TestRecommendedSongShelvesFirstBatchFailure(t *testing.T) {
	for name, failure := range songPagingFailures() {
		t.Run(name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{failure})
			if got.err == nil || len(got.requests) != 1 || got.detailCalls != 0 || len(got.shelves) != 0 {
				t.Fatalf("error=%v feed calls=%d detail calls=%d shelves=%d; expected the first-batch error", got.err, len(got.requests), got.detailCalls, len(got.shelves))
			}
		})
	}
}

// Personal recommendations used to call the playlist square and repeat the public featured lists.
func TestRecommendedPlaylistsUsesAccountHomeFeed(t *testing.T) {
	for _, loginType := range []int{1, 2} {
		t.Run(map[int]string{1: "WeChat", 2: "QQ"}[loginType], func(t *testing.T) {
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]json.RawMessage
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Req.Module != "music.recommend.RecommendFeed" || request.Req.Method != "get_recommend_feed" {
					t.Errorf("wrong recommendation source: %s/%s", request.Req.Module, request.Req.Method)
				}
				if request.Comm["ct"] != float64(20) || request.Comm["cv"] != float64(13020008) || request.Comm["qq"] != "10001" || request.Comm["authst"] != "fixture-key" || request.Comm["tmeLoginType"] != float64(loginType) || request.Comm["QIMEI"] != nil {
					t.Error("home feed lost its client identity or account")
				}
				if string(request.Req.Param["direction"]) != "0" || string(request.Req.Param["page"]) != "1" || string(request.Req.Param["s_num"]) != "0" || string(request.Req.Param["v_cache"]) != "[]" {
					t.Errorf("wrong first feed page: %v", request.Req.Param)
				}
				rpcReply(w, json.RawMessage(`{"retcode":0,"v_shelf":[
					{"id":301,"v_niche":[{"v_card":[
						{"id":"0","type":500,"subtype":510,"jumptype":10014,"title":"Daily"},
						{"id":"11","type":500,"subtype":513,"jumptype":10014,"title":"Navigation"}
					]}]},
					{"id":271,"v_niche":[{"v_card":[
						{"id":"9001","type":500,"subtype":0,"jumptype":10014,"title":"Personal A","cover":"https://example.com/a.jpg","cnt":2085000},
						{"id":"42","type":200,"subtype":0,"jumptype":10014,"title":"Song"}
					]},{"v_card":[
						{"id":"9002","type":500,"subtype":0,"jumptype":10014,"title":"Personal B","cover":"https://example.com/b.jpg","cnt":1493000},
						{"id":"9001","type":500,"subtype":0,"jumptype":10014,"title":"Repeated"},
						{"id":"video","type":300,"subtype":311,"jumptype":10012},
						{"id":"not-a-playlist","type":500,"subtype":0,"jumptype":10014},
						{"id":"9003","type":500,"subtype":0,"jumptype":10012}
					]}]}
				]}`))
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: loginType, Device: &wechatDevice{Qimei: "fixture-device"}})
			lists, err := RecommendedPlaylists()
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(lists) != 2 {
				t.Fatalf("got %d playlists from %d requests", len(lists), calls)
			}
			if lists[0].ID != 9001 || lists[0].Name != "Personal A" || lists[0].Cover() != "https://example.com/a.jpg" || lists[0].PlayCount != 2085000 || lists[1].ID != 9002 {
				t.Fatalf("lost feed order, playlist identity or metadata: %+v", lists)
			}
		})
	}
}

// A signed-out request must not relabel generic recommendations as personal ones.
func TestRecommendedPlaylistsRequiresAccount(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("signed-out recommendations contacted QQ")
	})
	if _, err := RecommendedPlaylists(); err == nil {
		t.Fatal("accepted a signed-out personal recommendation request")
	}
}

// Rejected or missing feed data must not silently fall back to the public playlist square.
func TestRecommendedPlaylistsRejectsInvalidFeed(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		ok   bool
	}{
		{"rejected", `{"retcode":12913,"v_shelf":[]}`, false},
		{"missing status", `{"v_shelf":[]}`, false},
		{"missing shelves", `{"retcode":0}`, false},
		{"empty feed", `{"retcode":0,"v_shelf":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				rpcReply(w, json.RawMessage(tc.data))
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: 2})
			lists, err := RecommendedPlaylists()
			if (err == nil) != tc.ok || len(lists) != 0 || calls != 1 {
				t.Fatalf("playlists=%d requests=%d error=%v, want success=%v", len(lists), calls, err, tc.ok)
			}
			if tc.name == "rejected" {
				var protocol *ProtocolError
				if !errors.As(err, &protocol) || protocol.Code != 12913 {
					t.Fatalf("lost feed rejection: %v", err)
				}
			}
		})
	}
}
